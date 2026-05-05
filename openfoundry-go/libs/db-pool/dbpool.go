// Package dbpool exposes a writer + reader Postgres pool pair, the Go
// equivalent of the Rust `db-pool` crate's DualPool.
//
// Why two pools
//
//   - Writer pool — targets the CNPG cluster's Pooler service
//     (`<cluster>-pooler-rw`) in transaction mode. All INSERT/UPDATE/
//     DELETE and read-your-writes paths go through here.
//   - Reader pool — targets the CNPG read-replica service
//     (`<cluster>-ro`) directly. Used for analytics, dashboards and
//     list endpoints that tolerate replication lag.
//
// When no reader URL is configured (DATABASE_READ_URL unset) Reader
// transparently returns the writer pool so a service runs unchanged in
// dev environments without a replica.
//
// Connection-string contract
//
// Both URLs MUST embed the per-service search_path so transaction-mode
// pooling cannot leak across schemas:
//
//	postgresql://svc_<bc>:<pwd>@pg-<role>-pooler-rw.openfoundry.svc:5432/app
//	    ?sslmode=require&options=-c%20search_path%3D<bc>
//
// dbpool does not rewrite the URL — the operator-supplied value
// (Helm envSecrets.DATABASE_URL) is the single source of truth.
package dbpool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Environment variables consumed by FromEnv.
const (
	EnvWriterURL = "DATABASE_URL"
	EnvReaderURL = "DATABASE_READ_URL"
)

// PoolSizing tunes the underlying pgxpool. Defaults match the Rust
// crate (20 max client connections per service, fitting under the
// PgBouncer 50-connection server-side budget).
type PoolSizing struct {
	MaxConns        int32
	MinConns        int32
	AcquireTimeout  time.Duration
	IdleTimeout     time.Duration
	MaxConnLifetime time.Duration
}

// DefaultPoolSizing returns the sizing the Rust crate ships with.
func DefaultPoolSizing() PoolSizing {
	return PoolSizing{
		MaxConns:        20,
		MinConns:        1,
		AcquireTimeout:  5 * time.Second,
		IdleTimeout:     120 * time.Second,
		MaxConnLifetime: 30 * time.Minute,
	}
}

// ErrMissingEnv is returned when DATABASE_URL is not set.
type ErrMissingEnv struct{ Var string }

func (e *ErrMissingEnv) Error() string {
	return fmt.Sprintf("environment variable %q is required", e.Var)
}

// ErrConnect wraps a pool-construction failure with the role that
// failed (writer / reader) so logs make the topology obvious.
type ErrConnect struct {
	Role  string
	Cause error
}

func (e *ErrConnect) Error() string { return fmt.Sprintf("connect to %s pool: %s", e.Role, e.Cause) }
func (e *ErrConnect) Unwrap() error { return e.Cause }

// DualPool is the writer + (optional) reader pair.
//
// Use `Writer()` for writes and read-your-writes; use `Reader()` for
// replica-tolerant reads. When no reader is configured, Reader()
// returns the writer.
type DualPool struct {
	writer *pgxpool.Pool
	reader *pgxpool.Pool // nil when no dedicated replica configured
}

// FromEnv builds the pair from DATABASE_URL (required) and
// DATABASE_READ_URL (optional).
func FromEnv(ctx context.Context) (*DualPool, error) {
	return FromEnvWith(ctx, DefaultPoolSizing())
}

// FromEnvWith is FromEnv with explicit sizing.
func FromEnvWith(ctx context.Context, sizing PoolSizing) (*DualPool, error) {
	writerURL, ok := os.LookupEnv(EnvWriterURL)
	if !ok {
		return nil, &ErrMissingEnv{Var: EnvWriterURL}
	}
	readerURL := os.Getenv(EnvReaderURL)
	return Connect(ctx, writerURL, readerURL, sizing)
}

// Connect builds the pair from explicit URLs (test + non-env config callers).
//
// readerURL may be empty; whitespace-only values are treated as empty,
// matching the Rust impl's lenient handling of operator-supplied configs.
func Connect(ctx context.Context, writerURL, readerURL string, sizing PoolSizing) (*DualPool, error) {
	writer, err := buildPool(ctx, writerURL, sizing)
	if err != nil {
		return nil, &ErrConnect{Role: "writer", Cause: err}
	}

	dp := &DualPool{writer: writer}
	if strings.TrimSpace(readerURL) == "" {
		slog.Info("DATABASE_READ_URL unset — reader requests fall back to writer pool")
		return dp, nil
	}

	reader, err := buildPool(ctx, readerURL, sizing)
	if err != nil {
		writer.Close()
		return nil, &ErrConnect{Role: "reader", Cause: err}
	}
	dp.reader = reader
	slog.Info("dual pool initialised with dedicated reader replica")
	return dp, nil
}

// FromPools wraps pre-built pools (tests + services owning lifecycle).
//
// reader may be nil; when it is, Reader() falls back to the writer.
func FromPools(writer, reader *pgxpool.Pool) *DualPool {
	if writer == nil {
		panic("dbpool: writer pool must not be nil")
	}
	return &DualPool{writer: writer, reader: reader}
}

// Writer returns the writer pool.
func (d *DualPool) Writer() *pgxpool.Pool { return d.writer }

// Reader returns the reader pool, or the writer when no replica is configured.
func (d *DualPool) Reader() *pgxpool.Pool {
	if d.reader != nil {
		return d.reader
	}
	return d.writer
}

// HasDedicatedReader reports whether a separate reader pool is in use.
func (d *DualPool) HasDedicatedReader() bool { return d.reader != nil }

// Close terminates both pools (idempotent).
func (d *DualPool) Close() {
	if d.reader != nil {
		d.reader.Close()
	}
	d.writer.Close()
}

func buildPool(ctx context.Context, url string, sizing PoolSizing) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	cfg.MaxConns = sizing.MaxConns
	cfg.MinConns = sizing.MinConns
	cfg.MaxConnLifetime = sizing.MaxConnLifetime
	cfg.MaxConnIdleTime = sizing.IdleTimeout

	connectCtx, cancel := context.WithTimeout(ctx, sizing.AcquireTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(connectCtx, cfg)
	if err != nil {
		return nil, err
	}
	// pgxpool.New does not eagerly verify connectivity; ping once so
	// the failure mode matches sqlx's connect-on-create behaviour.
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// IsMissingEnv reports whether err is an ErrMissingEnv.
func IsMissingEnv(err error) bool {
	var me *ErrMissingEnv
	return errors.As(err, &me)
}
