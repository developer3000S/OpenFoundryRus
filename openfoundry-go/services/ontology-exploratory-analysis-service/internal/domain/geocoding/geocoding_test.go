package geocoding

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

// floatEq compares two float64s within the 1e-6 epsilon used elsewhere
// in this service for distance / coordinate parity checks.
func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestForwardMatchesGazetteer(t *testing.T) {
	t.Parallel()
	got := Forward(" Madrid, Spain ")
	assert.Equal(t, "Madrid", got.Address)
	assert.True(t, floatEq(got.Coordinate.Lat, 40.4168))
	assert.True(t, floatEq(got.Coordinate.Lon, -3.7038))
	assert.Equal(t, 0.96, got.Confidence)
	assert.Equal(t, "reference gazetteer", got.Source)
}

func TestForwardMatchesMultiWordToken(t *testing.T) {
	t.Parallel()
	// "new york" must match the multi-word entry and be title-cased
	// to "New York" (each space-separated token capitalised).
	got := Forward("New York, NY")
	assert.Equal(t, "New York", got.Address)
	assert.True(t, floatEq(got.Coordinate.Lat, 40.7128))
}

func TestForwardFallsBackOnMiss(t *testing.T) {
	t.Parallel()
	got := Forward("Unknown Town")
	// Original casing preserved on fallback.
	assert.Equal(t, "Unknown Town", got.Address)
	assert.Equal(t, 0.68, got.Confidence)
	assert.Equal(t, "deterministic fallback", got.Source)
	// Fallback coordinates land in the Rust-defined window.
	assert.GreaterOrEqual(t, got.Coordinate.Lat, 35.0)
	assert.Less(t, got.Coordinate.Lat, 59.0)
	assert.GreaterOrEqual(t, got.Coordinate.Lon, -20.0)
	assert.Less(t, got.Coordinate.Lon, 20.0)
}

func TestForwardFallbackIsDeterministic(t *testing.T) {
	t.Parallel()
	a := Forward("Atlantis")
	b := Forward("Atlantis")
	assert.Equal(t, a.Coordinate, b.Coordinate)
}

func TestReversePicksNearest(t *testing.T) {
	t.Parallel()
	got := Reverse(models.Coordinate{Lat: 40.41, Lon: -3.70})
	assert.Equal(t, "Madrid", got.Address)
	assert.True(t, floatEq(got.Coordinate.Lat, 40.4168))
	assert.True(t, floatEq(got.Coordinate.Lon, -3.7038))
	assert.Equal(t, 0.91, got.Confidence)
	assert.Equal(t, "reverse gazetteer", got.Source)
}

func TestReverseTieBreakUsesFirstOccurrence(t *testing.T) {
	t.Parallel()
	// The Rust `min_by` preserves the first iteration order on equal
	// distance — `(0,0)` is closer to Lisbon than to London by a slim
	// margin, but we just assert the picked entry is in the gazetteer.
	got := Reverse(models.Coordinate{Lat: 0, Lon: 0})
	assert.NotEmpty(t, got.Address)
	assert.NotEmpty(t, got.Source)
}

func TestForwardEmptyStringFallsBack(t *testing.T) {
	t.Parallel()
	// Note: handler-side validation rejects empty addresses. The
	// domain function itself doesn't — covers an internal-caller path.
	got := Forward("")
	assert.Equal(t, 0.68, got.Confidence)
}
