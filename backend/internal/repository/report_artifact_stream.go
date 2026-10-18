package repository

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
)

// maxReportArtifactBytes mirrors the report_artifact ceiling enforced by FinishReportJob.
const maxReportArtifactBytes = 16 << 20

// StreamedArtifact is what a streaming writer reports after writing the artifact body.
type StreamedArtifact struct {
	Bytes  int64
	SHA256 string
	// LimitNote records any truncation or limit applied while rendering.
	LimitNote string
}

// ArtifactWriter writes the artifact body to w and returns its size and hash.
type ArtifactWriter func(w io.Writer) (StreamedArtifact, error)

// ArtifactWriteError marks a failure raised by the ArtifactWriter itself (for
// example a report limit), so the caller can fail the job with that reason
// instead of treating it as a storage error.
type ArtifactWriteError struct{ Err error }

func (e *ArtifactWriteError) Error() string { return e.Err.Error() }
func (e *ArtifactWriteError) Unwrap() error { return e.Err }

// FinishReportJobStream stores the artifact by streaming it into
// report_artifact with COPY (PF-07): the bytes go from the renderer to the
// database through a pipe, hex-encoded on the fly, so the worker never holds
// the whole file (nor its bytea parameter encoding) in memory. The job only
// flips to success at the end, after the database confirmed size, hash and
// envelope of the stored bytes; until then the job row is not locked, so a
// cancel during rendering wins and the artifact is rolled back.
func (s *Store) FinishReportJobStream(ctx context.Context, id, filename, contentType string, write ArtifactWriter) error {
	if contentType != "application/pdf" && contentType != "text/markdown; charset=utf-8" {
		return fmt.Errorf("invalid report artifact")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var format, status string
	err = tx.QueryRow(ctx, `SELECT report_format,status FROM report_job WHERE id=$1::uuid`, id).Scan(&format, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrReportConflict
	}
	if err != nil {
		return err
	}
	if status != "running" {
		return ErrReportConflict
	}
	if format == "pdf" && contentType != "application/pdf" || format == "markdown" && contentType != "text/markdown; charset=utf-8" {
		return fmt.Errorf("artifact MIME does not match report format")
	}
	if _, err = tx.Exec(ctx, `DELETE FROM report_artifact WHERE report_job_id=$1::uuid`, id); err != nil {
		return err
	}
	reader, pipe := io.Pipe()
	var writeErr error
	var written StreamedArtifact
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffered := bufio.NewWriterSize(pipe, 64<<10)
		err := func() error {
			if _, err := fmt.Fprintf(buffered, "%s\t%s\t%s\t\\\\x", copyText(id), copyText(filename), copyText(contentType)); err != nil {
				return err
			}
			result, err := write(hex.NewEncoder(buffered))
			if err != nil {
				writeErr = err
				return err
			}
			if result.Bytes <= 0 || result.Bytes > maxReportArtifactBytes || len(result.SHA256) != 64 {
				writeErr = fmt.Errorf("invalid report artifact")
				return writeErr
			}
			written = result
			if _, err = fmt.Fprintf(buffered, "\t%s\t%d\n", copyText(result.SHA256), result.Bytes); err != nil {
				return err
			}
			return buffered.Flush()
		}()
		_ = pipe.CloseWithError(err)
	}()
	_, copyErr := tx.Conn().PgConn().CopyFrom(ctx, reader, `COPY report_artifact(report_job_id,filename,content_type,content,sha256,size_bytes) FROM STDIN`)
	_ = reader.CloseWithError(io.ErrClosedPipe)
	<-done
	if writeErr != nil {
		return &ArtifactWriteError{Err: writeErr}
	}
	if copyErr != nil {
		return copyErr
	}
	var matches bool
	if err = tx.QueryRow(ctx, `SELECT octet_length(content)=$2 AND encode(sha256(content),'hex')=$3
 AND (content_type<>'application/pdf' OR (substring(content FROM 1 FOR 8)='%PDF-1.4'::bytea AND substring(content FROM octet_length(content)-5)=E'%%EOF\n'::bytea))
FROM report_artifact WHERE report_job_id=$1::uuid`, id, written.Bytes, written.SHA256).Scan(&matches); err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("report hash mismatch")
	}
	cmd, err := tx.Exec(ctx, `UPDATE report_job SET status='success',finished_at=now(),limit_note=left($2,1000) WHERE id=$1::uuid AND status='running'`, id, written.LimitNote)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() != 1 {
		return ErrReportConflict
	}
	return tx.Commit(ctx)
}

// copyText escapes a value for COPY text format.
func copyText(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`).Replace(value)
}
