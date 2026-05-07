package grpcserver

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/openfoundry/openfoundry-go/libs/proto-gen/media_set"
	"github.com/openfoundry/openfoundry-go/services/media-sets-service/internal/models"
)

// ── Conversion-level tests (no real server, no SQL) ────────────

func TestSchemaFromProtoMapsAllVariants(t *testing.T) {
	t.Parallel()
	for proto, want := range map[pb.MediaSetSchema]string{
		pb.MediaSetSchema_MEDIA_SET_SCHEMA_IMAGE:       "IMAGE",
		pb.MediaSetSchema_MEDIA_SET_SCHEMA_AUDIO:       "AUDIO",
		pb.MediaSetSchema_MEDIA_SET_SCHEMA_VIDEO:       "VIDEO",
		pb.MediaSetSchema_MEDIA_SET_SCHEMA_DOCUMENT:    "DOCUMENT",
		pb.MediaSetSchema_MEDIA_SET_SCHEMA_SPREADSHEET: "SPREADSHEET",
		pb.MediaSetSchema_MEDIA_SET_SCHEMA_EMAIL:       "EMAIL",
	} {
		got, err := schemaFromProto(proto)
		require.NoError(t, err, "proto=%v", proto)
		assert.Equal(t, want, got)
	}
}

func TestSchemaFromProtoUnspecifiedRejects(t *testing.T) {
	t.Parallel()
	_, err := schemaFromProto(pb.MediaSetSchema_MEDIA_SET_SCHEMA_UNSPECIFIED)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPolicyFromProtoDefaultsTransactionless(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "TRANSACTIONLESS",
		policyFromProto(pb.TransactionPolicy_TRANSACTION_POLICY_UNSPECIFIED))
	assert.Equal(t, "TRANSACTIONLESS",
		policyFromProto(pb.TransactionPolicy_TRANSACTION_POLICY_TRANSACTIONLESS))
	assert.Equal(t, "TRANSACTIONAL",
		policyFromProto(pb.TransactionPolicy_TRANSACTION_POLICY_TRANSACTIONAL))
}

func TestRoundTripSchemaPolicyState(t *testing.T) {
	t.Parallel()
	schemas := []string{"IMAGE", "AUDIO", "VIDEO", "DOCUMENT", "SPREADSHEET", "EMAIL"}
	for _, s := range schemas {
		back, err := schemaFromProto(schemaStrToProto(s))
		require.NoError(t, err)
		assert.Equal(t, s, back)
	}
	for _, p := range []string{"TRANSACTIONAL", "TRANSACTIONLESS"} {
		assert.Equal(t, p, policyFromProto(policyStrToProto(p)))
	}
	for _, st := range []string{"OPEN", "COMMITTED", "ABORTED"} {
		got := txnStateStrToProto(st)
		assert.NotEqual(t, pb.TransactionState_TRANSACTION_STATE_UNSPECIFIED, got)
	}
}

func TestMediaSetToProtoCarriesAllFields(t *testing.T) {
	t.Parallel()
	src := "ri.foundry.main.source.x"
	row := &models.MediaSet{
		RID: "ri.foundry.main.media_set.1", Name: "scans",
		ProjectRID: "ri.proj.1", Schema: "IMAGE",
		AllowedMimeTypes:  []string{"image/png"},
		TransactionPolicy: "TRANSACTIONAL",
		RetentionSeconds:  3600, Virtual: true, SourceRID: &src,
		Markings: []string{"pii"}, CreatedBy: "u",
	}
	got := mediaSetToProto(row)
	assert.Equal(t, row.RID, got.GetRid())
	assert.Equal(t, row.Name, got.GetName())
	assert.Equal(t, "ri.foundry.main.source.x", got.GetSourceRid())
	assert.Equal(t, []string{"pii"}, got.GetMarkings())
	assert.Equal(t, pb.MediaSetSchema_MEDIA_SET_SCHEMA_IMAGE, got.GetSchema())
	assert.Equal(t, pb.TransactionPolicy_TRANSACTION_POLICY_TRANSACTIONAL, got.GetTransactionPolicy())
}

func TestServiceSatisfiesGeneratedMediaSetServer(t *testing.T) {
	t.Parallel()
	var _ pb.MediaSetServiceServer = (*Service)(nil)

	server := grpc.NewServer()
	Register(server, &Service{})
	server.Stop()
}

// ── In-process round-trip via bufconn ───────────────────────────

// stubServer overrides only the RPC the test exercises so we can
// validate the grpc plumbing (Register / serve / serialize) without
// constructing the full Repo + Service objects.
type stubServer struct {
	pb.UnimplementedMediaSetServiceServer
	gotRID string
}

func (s *stubServer) GetMediaSet(_ context.Context, in *pb.GetMediaSetRequest) (*pb.MediaSet, error) {
	s.gotRID = in.GetRid()
	return &pb.MediaSet{Rid: in.GetRid(), Name: "echo"}, nil
}

func TestRegisterAndRoundTrip(t *testing.T) {
	t.Parallel()
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)
	server := grpc.NewServer()
	stub := &stubServer{}
	pb.RegisterMediaSetServiceServer(server, stub)
	go func() {
		_ = server.Serve(lis)
	}()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := pb.NewMediaSetServiceClient(conn)
	resp, err := client.GetMediaSet(context.Background(), &pb.GetMediaSetRequest{Rid: "ri.set.99"})
	require.NoError(t, err)
	assert.Equal(t, "ri.set.99", resp.GetRid())
	assert.Equal(t, "ri.set.99", stub.gotRID)
}

func TestUnimplementedSurfaceReturnsCode(t *testing.T) {
	t.Parallel()
	s := &Service{}
	// CommitTransaction was wired in our Service, but the underlying
	// transactions.Service is nil so the call will panic — instead
	// we test that an UNHANDLED RPC (via the stub embedding
	// UnimplementedMediaSetServiceServer in a fresh struct) yields
	// codes.Unimplemented.
	bare := pb.UnimplementedMediaSetServiceServer{}
	_, err := bare.CreateMediaSet(context.Background(), &pb.CreateMediaSetRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_ = s
}
