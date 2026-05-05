//go:build integration

package testingx

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// PostgresHarness is the ephemeral postgres:16-alpine container plus a
// connected pool. Call Stop() (or rely on t.Cleanup) to tear down.
type PostgresHarness struct {
	Container *tcpostgres.PostgresContainer
	Pool      *pgxpool.Pool
	URL       string
}

// Stop terminates the container. Safe to call multiple times.
func (h *PostgresHarness) Stop(ctx context.Context) {
	if h.Pool != nil {
		h.Pool.Close()
	}
	if h.Container != nil {
		_ = h.Container.Terminate(ctx)
	}
}

// BootPostgres starts a postgres:16-alpine container and returns a
// connected harness. Wires t.Cleanup so the container disappears with
// the test even when the test panics.
//
// Hardened against transient connection refusals during container
// startup with up to 30 retries / 500 ms each — same cadence as the
// Rust version.
func BootPostgres(ctx context.Context, t *testing.T) *PostgresHarness {
	t.Helper()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("openfoundry"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("postgres container start: %v", err)
	}

	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("postgres connection string: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("parse pgx config: %v", err)
	}
	cfg.MaxConns = 8

	var pool *pgxpool.Pool
	for attempt := 1; attempt <= 30; attempt++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				break
			}
			pool.Close()
		}
		if attempt == 30 {
			_ = container.Terminate(ctx)
			t.Fatalf("postgres never became reachable: %v", err)
		}
		t.Logf("waiting for postgres (%d): %v", attempt, err)
		time.Sleep(500 * time.Millisecond)
	}

	h := &PostgresHarness{Container: container, Pool: pool, URL: url}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		h.Stop(stopCtx)
	})

	t.Logf("postgres harness ready at %s", h.URL)
	return h
}

// MustExec is a tiny helper for setting up schema in tests. Panics
// on failure — matches the Rust crate's permissive style.
func (h *PostgresHarness) MustExec(ctx context.Context, sql string) {
	if _, err := h.Pool.Exec(ctx, sql); err != nil {
		panic(fmt.Errorf("MustExec: %w", err))
	}
}
