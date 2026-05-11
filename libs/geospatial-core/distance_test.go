package geospatialcore

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHaversineDistanceGoldenUnits(t *testing.T) {
	tests := []struct {
		name string
		unit string
		want float64
	}{
		{name: "meters", unit: "meters", want: 111195.0802335329},
		{name: "kilometers", unit: "km", want: 111.1950802335329},
		{name: "miles", unit: "miles", want: 69.09341957563636},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := HaversineDistance(0, 0, 0, 1, tc.unit)
			require.NoError(t, err)
			require.InDelta(t, tc.want, got, 1e-9)
		})
	}
}

func TestHaversineDistanceRejectsInvalidCoordinatesAndUnits(t *testing.T) {
	_, err := HaversineDistance(91, 0, 0, 1, "meters")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid start coordinate")

	_, err = HaversineDistance(0, 0, 0, 1, "yards")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported distance unit")
}

func TestHaversineDistanceBetweenPoints(t *testing.T) {
	start, err := NewGeoPoint(40.0, -105.3)
	require.NoError(t, err)
	end, err := NewGeoPoint(40.003, -105.295)
	require.NoError(t, err)

	got, err := HaversineDistanceBetweenPoints(start, end, "mi")
	require.NoError(t, err)
	require.InDelta(t, 0.3362, got, 0.001)
}
