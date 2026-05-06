package vectorstore_test

import (
	"context"
	"errors"
	"testing"

	vectorstore "github.com/openfoundry/openfoundry-go/libs/vector-store"
)

func TestBackendErrorIsClassifiesByKind(t *testing.T) {
	cases := []struct {
		err  *vectorstore.BackendError
		want error
	}{
		{vectorstore.NewTransportError("boom"), vectorstore.ErrTransport},
		{vectorstore.NewBackendError("boom"), vectorstore.ErrBackend},
		{vectorstore.NewSerializationError("boom"), vectorstore.ErrSerialization},
		{vectorstore.NewUnsupportedError("op"), vectorstore.ErrUnsupported},
		{vectorstore.NewUnimplementedError("op"), vectorstore.ErrUnimplemented},
	}
	for _, tc := range cases {
		if !errors.Is(tc.err, tc.want) {
			t.Fatalf("errors.Is(%v, %v) = false", tc.err, tc.want)
		}
	}
}

func TestPgVectorBackendReturnsUnimplemented(t *testing.T) {
	b := vectorstore.NewPgVectorBackend()
	ctx := context.Background()
	if err := b.Upsert(ctx, "doc", nil, nil); !errors.Is(err, vectorstore.ErrUnimplemented) {
		t.Fatalf("upsert err: %v", err)
	}
	if err := b.Delete(ctx, "doc"); !errors.Is(err, vectorstore.ErrUnimplemented) {
		t.Fatalf("delete err: %v", err)
	}
	if _, err := b.HybridQuery(ctx, "", nil, vectorstore.Filter{}, 1); !errors.Is(err, vectorstore.ErrUnimplemented) {
		t.Fatalf("hybrid err: %v", err)
	}
}

func TestFilterEqEncodesScalarValue(t *testing.T) {
	f := vectorstore.FilterEq("tenant_id", "acme")
	got, ok := f.Equals["tenant_id"]
	if !ok {
		t.Fatalf("missing tenant_id: %+v", f)
	}
	if string(got) != `"acme"` {
		t.Fatalf("encoded: %s", got)
	}
}
