package dbpool_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	dbpool "github.com/openfoundry/openfoundry-go/libs/db-pool"
)

func TestDefaultPoolSizingFitsPgBouncerBudget(t *testing.T) {
	t.Parallel()
	s := dbpool.DefaultPoolSizing()
	assert.LessOrEqual(t, s.MaxConns, int32(50), "stay under pooler default_pool_size=50")
	assert.LessOrEqual(t, s.MinConns, s.MaxConns)
	assert.Less(t, s.AcquireTimeout, s.IdleTimeout)
}

func TestFromEnvRequiresWriterURL(t *testing.T) {
	// Cannot t.Parallel because we mutate process env.
	prev, hadPrev := os.LookupEnv(dbpool.EnvWriterURL)
	prevReader, hadPrevReader := os.LookupEnv(dbpool.EnvReaderURL)
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv(dbpool.EnvWriterURL, prev)
		} else {
			_ = os.Unsetenv(dbpool.EnvWriterURL)
		}
		if hadPrevReader {
			_ = os.Setenv(dbpool.EnvReaderURL, prevReader)
		} else {
			_ = os.Unsetenv(dbpool.EnvReaderURL)
		}
	})
	_ = os.Unsetenv(dbpool.EnvWriterURL)
	_ = os.Unsetenv(dbpool.EnvReaderURL)

	_, err := dbpool.FromEnv(context.Background())
	assert.True(t, dbpool.IsMissingEnv(err), "expected ErrMissingEnv, got %v", err)
}
