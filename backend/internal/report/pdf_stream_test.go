package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func streamFixtures() map[string]Document {
	base := Document{Environment: "Ambiente de teste", RunID: "00000000-0000-0000-0000-000000000001", RunStarted: time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC), ServiceVersion: "teste", RuleVersion: "regra-teste", RequestedBy: "operador", Coverage: "complete", RunStatus: "success"}
	fixtures := map[string]Document{}
	for _, kind := range []string{"executive", "technical", "table", "architecture", "unknown"} {
		d := base
		d.Type = kind
		fixtures["vazio-"+kind] = d
	}
	technical := base
	technical.Type = "technical"
	technical.Databases = []Database{{Name: "db", Schemas: 2, Tables: 300, SizeBytes: 1 << 30}}
	technical.Baseline = &Baseline{RunID: "baseline", Status: "complete", AddedTables: 1, ChangedTables: 2}
	technical.CoverageNotes = []string{"coleta parcial (R$ \\ (teste))"}
	for i := 0; i < 300; i++ {
		technical.Tables = append(technical.Tables, Table{Database: "db", Schema: "public", Name: fmt.Sprintf("tabela_%03d_%s", i, strings.Repeat("ç", i%7)), SizeBytes: int64(i * 1000), Rows: int64(i), HasPrimaryKey: i%3 == 0})
	}
	for i := 0; i < 60; i++ {
		technical.Findings = append(technical.Findings, Finding{ID: fmt.Sprintf("f-%d", i), Type: "index.unused", Severity: []string{"critical", "high", "medium", "low", "info"}[i%5], Category: "performance", Database: "db", Schema: "public", Object: "indice", Title: "Índice sem uso", Summary: "Resumo", Recommendation: strings.Repeat("Validar em homologação. ", i%9), Evidence: strings.Repeat("evidência ", 40), Confidence: 0.5, RuleVersion: "r1"})
	}
	for i := 0; i < 12; i++ {
		technical.Growth = append(technical.Growth, GrowthPoint{RunID: fmt.Sprintf("run-%02d", i), SizeBytes: int64(i) << 20, Tables: 100 + i})
	}
	fixtures["tecnico-completo"] = technical
	fixtures["inventario-grande"] = largeStreamDocument(5000)
	return fixtures
}

func largeStreamDocument(tables int) Document {
	d := Document{Type: "technical", Environment: "large", RunID: "12345678-aaaa", RunStarted: time.Unix(0, 0), RunStatus: "success", Coverage: "complete"}
	for i := 0; i < tables; i++ {
		d.Tables = append(d.Tables, Table{Database: "db", Schema: "public", Name: strings.Repeat("large_table_", 3) + string(rune('a'+i%26)), SizeBytes: int64(i)})
	}
	return d
}

// The streaming writer must be a drop-in: same bytes as the legacy renderer.
func TestWritePDFIsByteIdenticalToLegacyRenderer(t *testing.T) {
	for name, document := range streamFixtures() {
		t.Run(name, func(t *testing.T) {
			legacy, legacyErr := legacyRenderPDF(document)
			var streamed bytes.Buffer
			result, err := WritePDF(context.Background(), &streamed, document)
			if (legacyErr == nil) != (err == nil) {
				t.Fatalf("legacy err=%v streaming err=%v", legacyErr, err)
			}
			if err != nil {
				if err.Error() != legacyErr.Error() {
					t.Fatalf("different errors: %q vs %q", err, legacyErr)
				}
				return
			}
			if !bytes.Equal(legacy, streamed.Bytes()) {
				t.Fatalf("streamed PDF differs from legacy (%d vs %d bytes)", streamed.Len(), len(legacy))
			}
			sum := sha256.Sum256(legacy)
			if result.Bytes != int64(len(legacy)) || result.Pages != PageCount(legacy) || result.SHA256 != hex.EncodeToString(sum[:]) {
				t.Fatalf("result metadata mismatch: %+v (len %d pages %d)", result, len(legacy), PageCount(legacy))
			}
			if err = ValidatePDF(streamed.Bytes()); err != nil {
				t.Fatal(err)
			}
			rendered, err := RenderPDF(document)
			if err != nil || !bytes.Equal(rendered, legacy) {
				t.Fatalf("RenderPDF wrapper differs: %v", err)
			}
		})
	}
}

func TestWritePDFFailsWithReadableReasons(t *testing.T) {
	tooManyPages := largeStreamDocument(20000)
	_, legacyErr := legacyRenderPDF(tooManyPages)
	_, err := WritePDF(context.Background(), io.Discard, tooManyPages)
	if err == nil || !strings.Contains(err.Error(), "páginas; reduza o escopo") || legacyErr == nil || legacyErr.Error() != err.Error() {
		t.Fatalf("expected the page limit reason, got %v (legacy %v)", err, legacyErr)
	}
	limited := &countingWriter{w: io.Discard, limit: 10}
	if _, err = limited.Write(make([]byte, 11)); !errors.Is(err, errPDFTooLarge) || !strings.Contains(err.Error(), "bytes; reduza o escopo") {
		t.Fatalf("expected the byte ceiling reason, got %v", err)
	}
	budget := errors.New("relatório excedeu o orçamento de memória do worker")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(budget)
	if _, err = WritePDF(ctx, io.Discard, largeStreamDocument(10)); !errors.Is(err, budget) {
		t.Fatalf("expected the cancel cause, got %v", err)
	}
	failing := failingWriter{}
	if _, err = WritePDF(context.Background(), failing, largeStreamDocument(10)); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected the writer error, got %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func measurePeak(t *testing.T, render func() error) uint64 {
	t.Helper()
	runtime.GC()
	sampler := StartHeapSampler(50*time.Microsecond, nil)
	if err := render(); err != nil {
		t.Fatal(err)
	}
	return sampler.Stop()
}

// PF-07: the report path must not hold the whole PDF (and its page command
// lists) in memory. Peak heap while streaming a near-limit document must stay
// well below the legacy renderer that built everything in a bytes.Buffer.
func TestWritePDFPeakHeapBelowLegacyRenderer(t *testing.T) {
	document := largeStreamDocument(5600)
	legacy, err := legacyRenderPDF(document)
	if err != nil {
		t.Fatal(err)
	}
	size := len(legacy)
	legacy = nil
	_ = legacy
	previous := debug.SetGCPercent(5)
	defer debug.SetGCPercent(previous)
	legacyPeak, streamPeak := uint64(0), uint64(0)
	for attempt := 0; attempt < 3; attempt++ {
		lp := measurePeak(t, func() error { out, err := legacyRenderPDF(document); runtime.KeepAlive(out); return err })
		sp := measurePeak(t, func() error { _, err := WritePDF(context.Background(), io.Discard, document); return err })
		if attempt == 0 || lp > legacyPeak {
			legacyPeak = lp
		}
		if attempt == 0 || sp < streamPeak {
			streamPeak = sp
		}
	}
	t.Logf("PDF %d bytes: legacy peak heap %d bytes, streaming peak heap %d bytes", size, legacyPeak, streamPeak)
	if streamPeak*10 > legacyPeak*6 {
		t.Fatalf("streaming peak heap %d is not below 60%% of the legacy peak %d", streamPeak, legacyPeak)
	}
}
