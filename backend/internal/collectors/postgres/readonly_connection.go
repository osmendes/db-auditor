package postgres

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/config"
)

// ConnectReadOnly establishes the target boundary before catalog discovery.
// The startup setting covers accidental implicit transactions as well; the
// explicit transaction covers all collector reads until the connection closes.
func ConnectReadOnly(ctx context.Context, dsn string) (*pgx.Conn, error) {
	return connectReadOnly(ctx, dsn, pgx.ParseConfig, pgx.ConnectConfig)
}

// targetError is the only way ConnectReadOnly reports a failure: the stage
// plus the driver text redacted against the DSN, so a driver that echoes the
// password (in or outside a URI) never reaches the run record or the API.
func targetError(stage string, err error, dsn string) error {
	return fmt.Errorf("%s: %s", stage, config.RedactSecrets(err.Error(), dsn))
}

// connectReadOnly takes the driver entry points so tests can inject errors.
func connectReadOnly(ctx context.Context, dsn string,
	parse func(string) (*pgx.ConnConfig, error),
	connect func(context.Context, *pgx.ConnConfig) (*pgx.Conn, error),
) (*pgx.Conn, error) {
	cfg, err := parse(dsn)
	if err != nil {
		return nil, targetError("target config", err, dsn)
	}
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = map[string]string{}
	}
	statementTimeout := 30 * time.Second
	if configured := cfg.RuntimeParams["statement_timeout"]; configured != "" {
		if ms, err := strconv.ParseInt(configured, 10, 64); err == nil && ms > 0 && ms < statementTimeout.Milliseconds() {
			statementTimeout = time.Duration(ms) * time.Millisecond
		} else if parsed, err := time.ParseDuration(configured); err == nil && parsed >= time.Millisecond && parsed < statementTimeout {
			statementTimeout = parsed
		}
	}
	cfg.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.RuntimeParams["statement_timeout"] = strconv.FormatInt(statementTimeout.Milliseconds(), 10)
	cfg.RuntimeParams["lock_timeout"] = "3000"
	conn, err := connect(ctx, cfg)
	if err != nil {
		return nil, targetError("target connect", err, dsn)
	}
	fail := func(err error) (*pgx.Conn, error) { _ = conn.Close(context.WithoutCancel(ctx)); return nil, err }
	if _, err = conn.Exec(ctx, "BEGIN READ ONLY"); err != nil {
		return fail(targetError("target read-only transaction", err, dsn))
	}
	if _, err = conn.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", statementTimeout.Milliseconds())); err != nil {
		return fail(targetError("target statement timeout", err, dsn))
	}
	if _, err = conn.Exec(ctx, "SET LOCAL lock_timeout = '3s'"); err != nil {
		return fail(targetError("target lock timeout", err, dsn))
	}
	var readonly bool
	if err = conn.QueryRow(ctx, "SELECT current_setting('transaction_read_only') = 'on'").Scan(&readonly); err != nil {
		return fail(targetError("target read-only check", err, dsn))
	}
	if !readonly {
		return fail(fmt.Errorf("target transaction is not read-only"))
	}
	var privileged bool
	const privilegeSQL = `SELECT r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolbypassrls
 OR has_database_privilege(current_user,current_database(),'CREATE')
 OR EXISTS(SELECT 1 FROM pg_namespace n WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname <> 'information_schema' AND has_schema_privilege(current_user,n.oid,'CREATE'))
 OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname <> 'information_schema' AND c.relkind IN ('r','p','f') AND has_table_privilege(current_user,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE'))
 FROM pg_roles r WHERE r.rolname=current_user`
	if err = conn.QueryRow(ctx, privilegeSQL).Scan(&privileged); err != nil {
		return fail(targetError("target privilege preflight", err, dsn))
	}
	if privileged {
		return fail(fmt.Errorf("target role has effective write or administration privilege"))
	}
	return conn, nil
}
