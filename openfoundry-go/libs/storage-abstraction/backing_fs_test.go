package storageabstraction

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLocalBackingFSPresignedURLRoundTrip(t *testing.T) {
	fixed := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	fs := NewLocalBackingFS("http://files.local/", "datasets", []byte("secret"))
	fs.Now = func() time.Time { return fixed }

	signed, err := fs.PresignedURL(PhysicalLocation{FSID: "local", BaseDirectory: "datasets", RelativePath: "sales/part.parquet"}, time.Minute)
	require.NoError(t, err)
	require.Equal(t, "GET", signed.Method)
	require.Equal(t, fixed.Add(time.Minute), signed.ExpiresAt)
	require.True(t, strings.HasPrefix(signed.URL, "http://files.local/v1/_internal/local-fs/datasets/sales/part.parquet?"))
}

func TestParsePhysicalURI(t *testing.T) {
	require.Equal(t, PhysicalLocation{FSID: "s3:bucket", RelativePath: "base/file.parquet"}, ParsePhysicalURI("s3://bucket/base/file.parquet"))
	require.Equal(t, PhysicalLocation{FSID: "local", RelativePath: "base/file.parquet"}, ParsePhysicalURI("local:///base/file.parquet"))
	require.Equal(t, "local:///base/file.parquet", PhysicalLocation{FSID: "local", RelativePath: "base/file.parquet"}.URI())
}
