package storageabstraction

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// PhysicalLocation is the Go mirror of the Rust backing filesystem mapping:
// a stable filesystem identifier plus a backend-relative object key.
type PhysicalLocation struct {
	FSID          string
	BaseDirectory string
	RelativePath  string
	VersionToken  *string
}

// URI returns the canonical persisted URI for a physical file mapping.
func (p PhysicalLocation) URI() string {
	key := joinObjectKey(p.BaseDirectory, p.RelativePath)
	if strings.HasPrefix(p.FSID, "s3:") {
		return "s3://" + strings.TrimPrefix(p.FSID, "s3:") + "/" + key
	}
	if strings.HasPrefix(p.FSID, "hdfs:") {
		return "hdfs://" + strings.TrimPrefix(p.FSID, "hdfs:") + "/" + key
	}
	return "local:///" + key
}

// ParsePhysicalURI accepts the persisted URI stored in dataset_files and
// returns a PhysicalLocation suitable for a BackingFS presigner.
func ParsePhysicalURI(raw string) PhysicalLocation {
	switch {
	case strings.HasPrefix(raw, "s3://"):
		rest := strings.TrimPrefix(raw, "s3://")
		bucket, key, ok := strings.Cut(rest, "/")
		if !ok {
			key = ""
		}
		return PhysicalLocation{FSID: "s3:" + bucket, RelativePath: key}
	case strings.HasPrefix(raw, "hdfs://"):
		rest := strings.TrimPrefix(raw, "hdfs://")
		host, key, ok := strings.Cut(rest, "/")
		if !ok {
			key = ""
		}
		return PhysicalLocation{FSID: "hdfs:" + host, RelativePath: key}
	case strings.HasPrefix(raw, "local://"):
		return PhysicalLocation{FSID: "local", RelativePath: strings.TrimLeft(strings.TrimPrefix(raw, "local://"), "/")}
	default:
		return PhysicalLocation{FSID: "local", RelativePath: strings.TrimLeft(raw, "/")}
	}
}

// PresignedURL is a temporary URL granting direct access to a physical object.
type PresignedURL struct {
	URL       string
	ExpiresAt time.Time
	Method    string
}

// BackingFS presigns physical object accesses without exposing backend-specific
// details to dataset-versioning handlers.
type BackingFS interface {
	FSID() string
	BaseDirectory() string
	PresignedURL(location PhysicalLocation, ttl time.Duration) (PresignedURL, error)
}

// LocalBackingFS produces HMAC-protected URLs for the service's local file
// proxy. The URL is deterministic and testable; the proxy can verify it with
// VerifyLocalSignature before streaming bytes from storage.
type LocalBackingFS struct {
	BaseURL string
	// BaseDir is the stable object-key prefix included in physical_uri values.
	BaseDir string
	// RootDir is the local filesystem root used by the development proxy.
	// When empty, the current working directory is used.
	RootDir string
	Secret  []byte
	Now     func() time.Time
}

func NewLocalBackingFS(baseURL, baseDir string, secret []byte) *LocalBackingFS {
	if len(secret) == 0 {
		secret = []byte("openfoundry-local-backing-fs")
	}
	return &LocalBackingFS{BaseURL: strings.TrimRight(baseURL, "/"), BaseDir: strings.Trim(baseDir, "/"), Secret: secret}
}

func (l *LocalBackingFS) FSID() string          { return "local" }
func (l *LocalBackingFS) BaseDirectory() string { return l.BaseDir }

func (l *LocalBackingFS) PresignedURL(location PhysicalLocation, ttl time.Duration) (PresignedURL, error) {
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	expiresAt := now().UTC().Add(ttl)
	key := joinObjectKey(location.BaseDirectory, location.RelativePath)
	if key == "" {
		key = joinObjectKey(l.BaseDir, location.RelativePath)
	}
	expires := expiresAt.Unix()
	sig := l.SignLocalKey(key, expires)
	values := url.Values{}
	values.Set("expires", strconv.FormatInt(expires, 10))
	values.Set("sig", sig)
	return PresignedURL{
		URL:       l.BaseURL + "/v1/_internal/local-fs/" + path.Clean("/" + key)[1:] + "?" + values.Encode(),
		ExpiresAt: expiresAt,
		Method:    "GET",
	}, nil
}

func (l *LocalBackingFS) SignLocalKey(key string, expires int64) string {
	mac := hmac.New(sha256.New, l.Secret)
	mac.Write([]byte(key))
	mac.Write([]byte("\n"))
	mac.Write([]byte(time.Unix(expires, 0).UTC().Format(time.RFC3339)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (l *LocalBackingFS) VerifyLocalSignature(key string, expires time.Time, sig string) bool {
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	if now().UTC().After(expires.UTC()) {
		return false
	}
	expected := l.SignLocalKey(key, expires.Unix())
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	want, _ := hex.DecodeString(expected)
	return hmac.Equal(got, want)
}

// ReadLocalObject returns bytes for a previously signed local object key after
// enforcing path traversal safety relative to RootDir.
func (l *LocalBackingFS) ReadLocalObject(key string) ([]byte, error) {
	path, err := l.localPath(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// WriteLocalObject stores bytes at a stable local object key after enforcing
// path traversal safety relative to RootDir.
func (l *LocalBackingFS) WriteLocalObject(key string, data []byte) error {
	path, err := l.localPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (l *LocalBackingFS) localPath(key string) (string, error) {
	cleanKey := path.Clean("/" + strings.TrimSpace(key))[1:]
	if cleanKey == "." || cleanKey == "" || cleanKey != strings.Trim(key, "/") || hasDotDot(cleanKey) {
		return "", errors.New("invalid local object key")
	}
	root := l.RootDir
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absPath := filepath.Join(absRoot, filepath.FromSlash(cleanKey))
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", errors.New("local object key escapes root")
	}
	return absPath, nil
}

func hasDotDot(key string) bool {
	for _, part := range strings.Split(key, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func joinObjectKey(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, "/")
}
