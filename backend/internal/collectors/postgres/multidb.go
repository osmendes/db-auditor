package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// DatabaseVisitor is called once per connectable user database.
// Returning an error records a PartialError for that database and continues.
type DatabaseVisitor func(ctx context.Context, conn *pgx.Conn, databaseName string) error

type databaseHooksKey struct{}
type DatabaseHooks struct {
	Semaphore  chan struct{}
	Progress   func(databaseName, status, message string)
	ShouldSkip func(databaseName string) bool
	OnStart    func(databaseName string, cancel context.CancelFunc) func()
}

func WithDatabaseHooks(ctx context.Context, hooks DatabaseHooks) context.Context {
	return context.WithValue(ctx, databaseHooksKey{}, hooks)
}

// ForEachUserDatabase lists databases from baseURL, then connects to each
// non-template, connectable database in scope. Failures on individual databases
// are accumulated as PartialError and do not abort the loop.
// A fatal error is only returned when the initial connection or database list fails.
func ForEachUserDatabase(
	ctx context.Context,
	baseURL string,
	scope config.Scope,
	visit DatabaseVisitor,
) (partial []PartialError, err error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base DSN is empty")
	}
	if visit == nil {
		return nil, fmt.Errorf("database visitor is nil")
	}

	root, err := pgx.Connect(ctx, baseURL)
	if err != nil {
		return nil, fmt.Errorf("connect target: %s", config.SanitizeError(err))
	}
	defer func() { _ = root.Close(ctx) }()

	databases, err := CollectDatabases(ctx, root, scope)
	if err != nil {
		return nil, err
	}

	for _, db := range databases {
		if ctx.Err() != nil {
			partial = append(partial, PartialError{
				Database: db.Name,
				Op:       "cancelled",
				Message:  ctx.Err().Error(),
			})
			break
		}
		if db.IsTemplate || !db.AllowConnections {
			continue
		}
		hooks, _ := ctx.Value(databaseHooksKey{}).(DatabaseHooks)
		if hooks.ShouldSkip != nil && hooks.ShouldSkip(db.Name) {
			message := "database cancelled by operator"
			if hooks.Progress != nil {
				hooks.Progress(db.Name, "failed", message)
			}
			partial = append(partial, PartialError{Database: db.Name, Op: "cancelled", Message: message})
			continue
		}
		if hooks.Semaphore != nil {
			select {
			case hooks.Semaphore <- struct{}{}:
			case <-ctx.Done():
				return partial, ctx.Err()
			}
		}
		func() {
			dbCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			if hooks.OnStart != nil {
				unregister := hooks.OnStart(db.Name, cancel)
				if unregister != nil {
					defer unregister()
				}
			}
			if hooks.Semaphore != nil {
				defer func() { <-hooks.Semaphore }()
			}
			if hooks.Progress != nil {
				hooks.Progress(db.Name, "attempted", "")
			}
			dbURL, err := rewriteDatabase(baseURL, db.Name)
			if err != nil {
				if hooks.Progress != nil {
					hooks.Progress(db.Name, "failed", config.SanitizeError(err))
				}
				partial = append(partial, PartialError{
					Database: db.Name, Op: "rewrite_url", Message: config.SanitizeError(err),
				})
				return
			}
			dbConn, err := pgx.Connect(dbCtx, dbURL)
			if err != nil {
				if hooks.Progress != nil {
					hooks.Progress(db.Name, "failed", config.SanitizeError(err))
				}
				partial = append(partial, PartialError{
					Database: db.Name, Op: "connect", Message: config.SanitizeError(err),
				})
				return
			}
			visitErr := visit(dbCtx, dbConn, db.Name)
			_ = dbConn.Close(context.WithoutCancel(ctx))
			if visitErr != nil {
				if hooks.Progress != nil {
					hooks.Progress(db.Name, "failed", config.SanitizeError(visitErr))
				}
				partial = append(partial, PartialError{
					Database: db.Name, Op: "collect", Message: config.SanitizeError(visitErr),
				})
				return
			}
			if hooks.Progress != nil {
				hooks.Progress(db.Name, "success", "")
			}
		}()
	}
	return partial, nil
}

// FormatPartialErrors returns a short human-readable summary of partial errors.
// Messages are passed through SanitizeDSN for safety in logs and UI.
func FormatPartialErrors(partial []PartialError, max int) string {
	if len(partial) == 0 {
		return ""
	}
	if max <= 0 {
		max = 5
	}
	n := len(partial)
	if n > max {
		n = max
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		p := partial[i]
		msg := config.SanitizeDSN(p.Message)
		parts = append(parts, fmt.Sprintf("%s[%s]: %s", p.Database, p.Op, msg))
	}
	s := strings.Join(parts, "; ")
	if len(partial) > max {
		s += fmt.Sprintf(" (+%d)", len(partial)-max)
	}
	return s
}
