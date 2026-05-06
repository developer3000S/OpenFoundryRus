package queryengine

import (
	"context"
	"errors"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Endpoint normalisation ────────────────────────────────────────────

func TestNormaliseFlightEndpoint(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"http://flight.internal:50051":          "flight.internal:50051",
		"https://flight.internal:50051":         "flight.internal:50051",
		"grpc://flight.internal:50051":          "flight.internal:50051",
		"grpc+tcp://flight.internal:50051":      "flight.internal:50051",
		"grpc+tls://flight.internal:50051":      "flight.internal:50051",
		"flight.internal:50051":                 "flight.internal:50051",
		"   http://flight.internal:50051/   ":   "flight.internal:50051",
		"http://flight.internal:50051/path/":    "flight.internal:50051/path",
	}
	for in, want := range cases {
		assert.Equal(t, want, normaliseFlightEndpoint(in), "input=%q", in)
	}
}

// ─── Schema projection ─────────────────────────────────────────────────

func newDemoSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
		{Name: "name", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "score", Type: arrow.PrimitiveTypes.Float64, Nullable: false},
	}, nil)
}

func TestProjectSchemaNilReturnsOriginal(t *testing.T) {
	t.Parallel()
	s := newDemoSchema()
	got, err := projectSchema(s, nil)
	require.NoError(t, err)
	assert.Same(t, s, got)
}

func TestProjectSchemaSubset(t *testing.T) {
	t.Parallel()
	s := newDemoSchema()
	got, err := projectSchema(s, []int{2, 0})
	require.NoError(t, err)
	require.Equal(t, 2, len(got.Fields()))
	assert.Equal(t, "score", got.Field(0).Name)
	assert.Equal(t, "id", got.Field(1).Name)
}

func TestProjectSchemaOutOfRange(t *testing.T) {
	t.Parallel()
	_, err := projectSchema(newDemoSchema(), []int{99})
	require.Error(t, err)
	assert.True(t, IsServerError(err), "out-of-range projection should map to FlightServer kind")
}

// ─── Batch projection ──────────────────────────────────────────────────

func newDemoBatch(t *testing.T, alloc memory.Allocator) arrow.RecordBatch {
	t.Helper()
	schema := newDemoSchema()

	idB := array.NewInt64Builder(alloc)
	defer idB.Release()
	idB.AppendValues([]int64{1, 2, 3}, nil)

	nameB := array.NewStringBuilder(alloc)
	defer nameB.Release()
	nameB.AppendValues([]string{"a", "b", "c"}, nil)

	scoreB := array.NewFloat64Builder(alloc)
	defer scoreB.Release()
	scoreB.AppendValues([]float64{0.1, 0.2, 0.3}, nil)

	cols := []arrow.Array{idB.NewArray(), nameB.NewArray(), scoreB.NewArray()}
	rec := array.NewRecordBatch(schema, cols, 3)
	for _, c := range cols {
		c.Release()
	}
	return rec
}

func TestProjectBatchNilReturnsRetainedClone(t *testing.T) {
	t.Parallel()
	alloc := memory.NewCheckedAllocator(memory.NewGoAllocator())
	defer alloc.AssertSize(t, 0)

	batch := newDemoBatch(t, alloc)
	defer batch.Release()

	got, err := projectBatch(alloc, batch, nil)
	require.NoError(t, err)
	defer got.Release()

	assert.Equal(t, int64(3), got.NumCols())
	assert.Equal(t, int64(3), got.NumRows())
}

func TestProjectBatchKeepsRequestedColumns(t *testing.T) {
	t.Parallel()
	alloc := memory.NewCheckedAllocator(memory.NewGoAllocator())
	defer alloc.AssertSize(t, 0)

	batch := newDemoBatch(t, alloc)
	defer batch.Release()

	got, err := projectBatch(alloc, batch, []int{2, 0})
	require.NoError(t, err)
	defer got.Release()

	require.Equal(t, int64(2), got.NumCols())
	assert.Equal(t, "score", got.Schema().Field(0).Name)
	assert.Equal(t, "id", got.Schema().Field(1).Name)
	assert.Equal(t, int64(3), got.NumRows())
}

func TestProjectBatchOutOfRange(t *testing.T) {
	t.Parallel()
	alloc := memory.NewCheckedAllocator(memory.NewGoAllocator())
	defer alloc.AssertSize(t, 0)

	batch := newDemoBatch(t, alloc)
	defer batch.Release()

	_, err := projectBatch(alloc, batch, []int{42})
	require.Error(t, err)
	assert.True(t, IsServerError(err))
}

// ─── Error taxonomy ────────────────────────────────────────────────────

func TestFlightProviderErrorClassifies(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		err     error
		matcher func(error) bool
	}{
		{"invalid", &FlightProviderError{Kind: FlightInvalidEndpoint, Endpoint: "x"}, IsInvalidEndpoint},
		{"connect", &FlightProviderError{Kind: FlightConnect, Endpoint: "x", Cause: errors.New("rst")}, IsConnectError},
		{"server", &FlightProviderError{Kind: FlightServer, Cause: errors.New("oom")}, IsServerError},
		{"missing-ticket", &FlightProviderError{Kind: FlightMissingTicket}, IsMissingTicket},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.True(t, c.matcher(c.err))
			// Wrapping survives errors.Join.
			wrapped := errors.Join(errors.New("transport"), c.err)
			assert.True(t, c.matcher(wrapped))
		})
	}
}

func TestFlightProviderErrorMessageRendering(t *testing.T) {
	t.Parallel()
	e := &FlightProviderError{Kind: FlightInvalidEndpoint, Endpoint: "http://x", Cause: errors.New("boom")}
	assert.Contains(t, e.Error(), "invalid Flight SQL endpoint")
	assert.Contains(t, e.Error(), "http://x")
	assert.Contains(t, e.Error(), "boom")

	mt := &FlightProviderError{Kind: FlightMissingTicket}
	assert.Equal(t, "Flight SQL endpoint returned no ticket for query", mt.Error())
}

// ─── Provider getters ──────────────────────────────────────────────────

func TestNewFlightSQLProviderGetters(t *testing.T) {
	t.Parallel()
	schema := newDemoSchema()
	p := NewFlightSQLProvider("http://flight.internal:50051", "SELECT 1", schema)
	assert.Equal(t, "http://flight.internal:50051", p.Endpoint())
	assert.Equal(t, "SELECT 1", p.Query())
	assert.Same(t, schema, p.Schema())
}

func TestDialFlightRejectsEmpty(t *testing.T) {
	t.Parallel()
	_, err := dialFlight(context.Background(), "   ", nil)
	require.Error(t, err)
	assert.True(t, IsInvalidEndpoint(err))
}

// ─── QueryContext stubs ────────────────────────────────────────────────

func TestQueryContextRegisterStubsReturnUnsupported(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	qc := New()

	assert.ErrorIs(t, qc.RegisterParquet(ctx, "t", "/tmp/x.parquet"), ErrUnsupportedLocalExecution)
	assert.ErrorIs(t, qc.RegisterCSV(ctx, "t", "/tmp/x.csv"), ErrUnsupportedLocalExecution)
	assert.ErrorIs(t, qc.RegisterBatch("t", nil), ErrUnsupportedLocalExecution)
}

func TestQueryContextSQLDelegatesToExecuteSQL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	qc := New()
	rec, schema, err := qc.SQL(ctx, "SELECT 1")
	require.NoError(t, err)
	require.NotNil(t, rec)
	defer rec.Release()
	require.NotNil(t, schema)
	assert.Equal(t, int64(1), rec.NumRows())
	assert.Equal(t, int64(1), rec.NumCols())
}

func TestQueryContextExplainSQLReturnsTrivialPlans(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	qc := New()
	logical, physical, err := qc.ExplainSQL(ctx, "SELECT 1, 2, 3")
	require.NoError(t, err)
	assert.Contains(t, logical, "LiteralProjection")
	assert.Contains(t, logical, "3 expr(s)")
	assert.Contains(t, physical, "LiteralProjectionExec")
}
