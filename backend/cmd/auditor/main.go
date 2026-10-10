package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
	"github.com/mayconmendes-qc/db-auditor/internal/api"
	"github.com/mayconmendes-qc/db-auditor/internal/audit"
	"github.com/mayconmendes-qc/db-auditor/internal/buildinfo"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
	"github.com/mayconmendes-qc/db-auditor/internal/database"
	"github.com/mayconmendes-qc/db-auditor/internal/migrate"
	"github.com/mayconmendes-qc/db-auditor/internal/observability"
	"github.com/mayconmendes-qc/db-auditor/internal/reportworker"
	"github.com/mayconmendes-qc/db-auditor/internal/repository"
	"github.com/mayconmendes-qc/db-auditor/internal/scheduler"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "recover-operator" {
		if err := recoverOperator(os.Args[2:]); err != nil {
			// Connection errors may contain the snapshot-store DSN.
			_, _ = os.Stderr.WriteString("Recuperação não concluída. Verifique a conta, a conexão e o backup; consulte os logs do banco.\n")
			os.Exit(1)
		}
		return
	}
	observability.SetupLogging()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	targets := config.LoadTargetDSNs()
	mongoTargets := config.LoadMongoTargetURIs()
	if err := config.ApplyTargetStatementTimeout(targets, cfg.TargetStatementTimeout); err != nil {
		slog.Error("invalid target query budget", "error", err)
		os.Exit(1)
	}
	if err := config.ValidateTargetDSNs(targets); err != nil {
		slog.Error("unsafe target configuration", "error", err)
		os.Exit(1)
	}
	if hosts := config.InsecureTargetHosts(); len(hosts) > 0 {
		slog.Warn("weak TLS allowlist is explicit and empty by default; these hosts may skip verify-full", "hosts", hosts)
	} else {
		slog.Info("target TLS default is verify-full; AUDITOR_TARGET_INSECURE_HOSTS is empty")
	}
	if err := config.ValidateMongoTargetURIs(mongoTargets); err != nil {
		slog.Error("unsafe MongoDB target configuration", "error", err)
		os.Exit(1)
	}
	if len(targets) == 0 {
		slog.Warn("nenhum AUDITOR_TARGET_DSN_* configurado; execuções de auditoria falharão até definir DSNs somente leitura")
	} else {
		slog.Info("target DSNs carregados", "count", len(targets))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		slog.Error("could not connect to snapshot store", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := migrate.Apply(ctx, pool, os.Getenv("AUDITOR_MIGRATIONS_DIR")); err != nil {
		slog.Error("could not apply snapshot store migrations", "error", err)
		os.Exit(1)
	}

	store := repository.NewStore(pool)
	if err := store.EnsureIdentitySchema(ctx); err != nil {
		slog.Error("could not ensure identity schema", "error", err)
		os.Exit(1)
	}
	hasUsers, err := store.HasAuditorUsers(ctx)
	if err != nil {
		slog.Error("identity schema unavailable", "error", err)
		os.Exit(1)
	}
	if !hasUsers {
		username := strings.TrimSpace(os.Getenv("AUDITOR_BOOTSTRAP_USER"))
		password := strings.TrimRight(os.Getenv("AUDITOR_BOOTSTRAP_PASSWORD"), "\r\n")
		if password == "" {
			secretPath := os.Getenv("AUDITOR_BOOTSTRAP_PASSWORD_FILE")
			if secretPath != "" {
				secret, err := os.ReadFile(secretPath)
				if err != nil {
					slog.Error("could not read bootstrap secret", "error", err)
					os.Exit(1)
				}
				password = strings.TrimRight(string(secret), "\r\n")
			}
		}
		if username == "" || password == "" {
			slog.Error("first start requires AUDITOR_BOOTSTRAP_USER and AUDITOR_BOOTSTRAP_PASSWORD in the environment")
			os.Exit(1)
		}
		if err := store.BootstrapOperator(ctx, username, password); err != nil {
			slog.Error("could not bootstrap operator", "error", err)
			os.Exit(1)
		}
	}
	if err := store.EnsureRuleCatalog(ctx); err != nil {
		slog.Error("could not initialize rule catalog", "error", err)
		os.Exit(1)
	}
	go (reportworker.Worker{Store: store}).Run(ctx)
	go observeBackupAge(ctx, store)
	runStore := &repository.AuditRunStore{Store: store}
	liveOpts := audit.LiveRegistryOptions{
		Targets: targets,
		Scope:   cfg.Scope,
		Policy:  cfg.Collection,
		Writer:  store,
	}
	registry := audit.NewLiveRegistry(liveOpts)
	audit.AttachStructuralCollectors(registry, liveOpts)
	binaryVersion := buildinfo.String()
	analysisService := analyzer.NewService(store, store, binaryVersion)
	runner := audit.NewRunner(registry, runStore, audit.RunnerOptions{
		ServiceVersion:           binaryVersion,
		CollectorVersion:         binaryVersion,
		MaxWorkers:               cfg.MaxCollectorWorkers,
		MaxDatabaseConnections:   cfg.MaxDatabaseConnections,
		OptionalCollectorTimeout: cfg.OptionalCollectorTimeout,
		AnalysisProcessor:        analysisService,
		EngineRegistries: map[string]*audit.Registry{
			"mongodb": audit.NewMongoRegistry(mongoTargets, store),
		},
	})
	sch := scheduler.NewWithLimits(runner, cfg.MaxConcurrentRuns)
	sch.UseStore(store)
	if err := sch.Load(ctx); err != nil {
		slog.Error("could not load schedules", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr: cfg.HTTPAddress,
		Handler: api.NewHandlerWithOptions(store, api.HandlerOptions{
			Runner:   sch,
			Analysis: analysisService,
			Targets:  targets,
			Auth:     store,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	metricsAddr := os.Getenv("AUDITOR_METRICS_ADDRESS")
	if metricsAddr == "" {
		metricsAddr = ":9090"
	}
	metricsServer := &http.Server{
		Addr:              metricsAddr,
		Handler:           observability.DefaultMetrics.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n := sch.TickDue(ctx)
				if n > 0 {
					slog.Info("scheduler triggered runs", "count", n)
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := store.DeleteExpiredAuditorSessions(ctx); err != nil {
					slog.Warn("could not remove expired sessions", "error", err)
				}
			}
		}
	}()

	go func() {
		slog.Info("HTTP server listening", "address", cfg.HTTPAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
			stop()
		}
	}()
	go func() {
		slog.Info("metrics server listening", "address", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP shutdown failed", "error", err)
	}
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("metrics shutdown failed", "error", err)
	}
}

func observeBackupAge(ctx context.Context, store *repository.Store) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	sample := func() {
		age, err := store.SnapshotBackupAgeSeconds(ctx, time.Now().UTC())
		if err != nil {
			slog.Warn("snapshot backup age", "error", err)
			return
		}
		if age < 0 {
			age = 0
		}
		observability.DefaultMetrics.SetSnapshotBackupAge(uint64(age))
	}
	sample()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			sample()
		}
	}
}
