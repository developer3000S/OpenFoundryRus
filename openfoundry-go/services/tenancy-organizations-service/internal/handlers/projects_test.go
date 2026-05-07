package handlers

// Tests for the unexported normalisation helpers in projects.go.
// They live in `package handlers` so the assertions can call the
// helpers directly without re-exporting them. The Rust handler keeps
// the same parity tests in `mod tests` at the bottom of projects.rs.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeFolderNameCollapsesWhitespace(t *testing.T) {
	t.Parallel()
	got, err := normalizeFolderName("  Weekly   Reviews  ")
	require.NoError(t, err)
	assert.Equal(t, "Weekly Reviews", got)
}

func TestNormalizeFolderNameRejectsEmpty(t *testing.T) {
	t.Parallel()
	_, err := normalizeFolderName("   ")
	assert.Error(t, err)
}

func TestFolderSlugFromHumanReadableName(t *testing.T) {
	t.Parallel()
	got, err := folderSlugFromName("Weekly Reviews / Q2")
	require.NoError(t, err)
	assert.Equal(t, "weekly-reviews-q2", got)
}

func TestFolderSlugRejectsNonAlphanumericInput(t *testing.T) {
	t.Parallel()
	_, err := folderSlugFromName("✨ / 🚀")
	assert.Error(t, err)
}

func TestNormalizeSlugAccepts(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"FraudModels":   "fraudmodels",
		"  fraud-models ": "fraud-models",
		"PROJECT-2026":  "project-2026",
	}
	for in, want := range cases {
		got, err := normalizeSlug(in, "slug")
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
}

func TestNormalizeSlugRejectsEmpty(t *testing.T) {
	t.Parallel()
	_, err := normalizeSlug("   ", "slug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug is required")
}

func TestNormalizeSlugRejectsInvalidChars(t *testing.T) {
	t.Parallel()
	_, err := normalizeSlug("fraud_models", "slug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lowercase letters, digits, and hyphens")
}

func TestNormalizeSlugRejectsLeadingTrailingHyphen(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"-fraud", "fraud-", "-fraud-"} {
		_, err := normalizeSlug(in, "slug")
		require.Error(t, err, in)
		assert.Contains(t, err.Error(), "cannot start or end with a hyphen", in)
	}
}

func TestNormalizeOptionalSlugCollapsesEmpty(t *testing.T) {
	t.Parallel()
	got, err := normalizeOptionalSlug(nil, "workspace_slug")
	require.NoError(t, err)
	assert.Nil(t, got)

	blank := "   "
	got, err = normalizeOptionalSlug(&blank, "workspace_slug")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestNormalizeOptionalSlugReturnsCanonicalForm(t *testing.T) {
	t.Parallel()
	in := "  Engineering "
	got, err := normalizeOptionalSlug(&in, "workspace_slug")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "engineering", *got)
}

func TestAsciiLowerLeavesNonASCIIBytesIntact(t *testing.T) {
	t.Parallel()
	// Mirrors Rust `str::to_ascii_lowercase`: non-ASCII bytes pass
	// through, ASCII A–Z lowers in place. The downstream slug check
	// then rejects the non-ASCII bytes — same observable result.
	assert.Equal(t, "abc", asciiLower("ABC"))
	assert.Equal(t, "café", asciiLower("CAFé"))
}
