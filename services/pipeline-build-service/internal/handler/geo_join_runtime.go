package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	geospatialcore "github.com/openfoundry/openfoundry-go/libs/geospatial-core"
	pipelineexpression "github.com/openfoundry/openfoundry-go/libs/pipeline-expression"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

const (
	defaultGeoJoinMaxLeftRows       = 1_000
	defaultGeoJoinMaxRightRows      = 1_000
	defaultGeoJoinMaxCandidatePairs = 250_000
	defaultGeoJoinMaxK              = 50
)

type geoJoinMatch struct {
	rightIndex int
	distance   float64
	rank       int
}

func (rt *lightweightTableRuntime) runGeoJoin(node executor.NodeContext, draft runtimeGeoJoinDraft) ([]pipelineexpression.Row, error) {
	if len(node.Node.DependsOn) < 2 {
		return nil, errors.New("lightweight_geo_join_requires_two_inputs")
	}
	left, err := rt.dependencyRows(node.Node.DependsOn[0])
	if err != nil {
		return nil, err
	}
	right, err := rt.dependencyRows(node.Node.DependsOn[1])
	if err != nil {
		return nil, err
	}
	if err := validateGeoJoinScale(len(left), len(right), draft); err != nil {
		return nil, err
	}
	leftGeometries, err := runtimeGeoJoinGeometries(left, draft.LeftGeometryColumn, draft.LeftLatColumn, draft.LeftLonColumn, "left")
	if err != nil {
		return nil, err
	}
	rightGeometries, err := runtimeGeoJoinGeometries(right, draft.RightGeometryColumn, draft.RightLatColumn, draft.RightLonColumn, "right")
	if err != nil {
		return nil, err
	}

	mode := normalizedGeoJoinMode(draft.Mode)
	unit := strings.TrimSpace(firstNonEmpty(draft.Unit, "miles"))
	if _, err := geospatialcore.ParseDistanceUnit(unit); err != nil {
		return nil, fmt.Errorf("lightweight_geo_join_invalid_unit:%w", err)
	}
	if mode == "distance" && draft.MaxDistance <= 0 {
		return nil, errors.New("lightweight_geo_join_distance_requires_max_distance")
	}

	leftColumns := selectRuntimeColumns(left, draft.LeftColumns, draft.AutoSelectLeft || len(draft.LeftColumns) == 0)
	rightColumns := selectRuntimeColumns(right, draft.RightColumns, draft.AutoSelectRight || len(draft.RightColumns) == 0)
	rightPrefix := strings.TrimSpace(firstNonEmpty(draft.RightPrefix, "right_"))
	joinType := strings.ToLower(strings.TrimSpace(draft.JoinType))
	if joinType == "" {
		joinType = "inner"
	}
	k := draft.K
	if k <= 0 {
		k = 1
	}
	if k > defaultGeoJoinMaxK {
		return nil, fmt.Errorf("lightweight_geo_join_k_exceeds_limit:%d>%d", k, defaultGeoJoinMaxK)
	}
	distanceColumn := strings.TrimSpace(firstNonEmpty(draft.DistanceColumn, "geo_distance_"+geoJoinUnitSuffix(unit)))
	if mode == "intersection" && strings.TrimSpace(draft.DistanceColumn) == "" {
		distanceColumn = ""
	}
	rankColumn := strings.TrimSpace(firstNonEmpty(draft.RankColumn, "geo_rank"))

	out := make([]pipelineexpression.Row, 0)
	for leftIndex, lrow := range left {
		matches, err := runtimeGeoJoinMatches(leftGeometries[leftIndex], rightGeometries, mode, unit, draft.MaxDistance, k)
		if err != nil {
			return nil, fmt.Errorf("lightweight_geo_join row %d: %w", leftIndex, err)
		}
		if len(matches) == 0 {
			if joinType == "left" {
				out = append(out, composeRuntimeGeoJoinRow(lrow, nil, leftColumns, rightColumns, rightPrefix, distanceColumn, rankColumn, nil))
			}
			continue
		}
		for _, match := range matches {
			rightRow := right[match.rightIndex]
			out = append(out, composeRuntimeGeoJoinRow(lrow, rightRow, leftColumns, rightColumns, rightPrefix, distanceColumn, rankColumn, &match))
		}
	}
	return out, nil
}

func validateGeoJoinScale(leftRows, rightRows int, draft runtimeGeoJoinDraft) error {
	maxLeft := intWithDefault(draft.MaxLeftRows, defaultGeoJoinMaxLeftRows)
	maxRight := intWithDefault(draft.MaxRightRows, defaultGeoJoinMaxRightRows)
	maxPairs := intWithDefault(draft.MaxCandidatePairs, defaultGeoJoinMaxCandidatePairs)
	if leftRows > maxLeft {
		return fmt.Errorf("lightweight_geo_join_left_scale_exceeded:%d>%d", leftRows, maxLeft)
	}
	if rightRows > maxRight {
		return fmt.Errorf("lightweight_geo_join_right_scale_exceeded:%d>%d", rightRows, maxRight)
	}
	if leftRows*rightRows > maxPairs {
		return fmt.Errorf("lightweight_geo_join_candidate_scale_exceeded:%d>%d", leftRows*rightRows, maxPairs)
	}
	return nil
}

func runtimeGeoJoinGeometries(rows []pipelineexpression.Row, geometryColumn, latColumn, lonColumn, side string) ([]geospatialcore.GeoJSONGeometry, error) {
	out := make([]geospatialcore.GeoJSONGeometry, 0, len(rows))
	for index, row := range rows {
		geometry, err := runtimeGeoJoinGeometry(row, geometryColumn, latColumn, lonColumn)
		if err != nil {
			return nil, fmt.Errorf("%s row %d: %w", side, index, err)
		}
		out = append(out, geometry)
	}
	return out, nil
}

func runtimeGeoJoinGeometry(row pipelineexpression.Row, geometryColumn, latColumn, lonColumn string) (geospatialcore.GeoJSONGeometry, error) {
	geometryColumn = strings.TrimSpace(geometryColumn)
	if geometryColumn != "" {
		raw, ok := row[geometryColumn]
		if !ok || isRuntimeNullish(raw, true) {
			return geospatialcore.GeoJSONGeometry{}, fmt.Errorf("geometry column %q is null or missing", geometryColumn)
		}
		return parseRuntimeGeoJSONGeometry(raw)
	}
	latColumn = strings.TrimSpace(latColumn)
	lonColumn = strings.TrimSpace(lonColumn)
	if latColumn == "" || lonColumn == "" {
		return geospatialcore.GeoJSONGeometry{}, errors.New("geo join requires geometry columns or lat/lon columns")
	}
	lat, null, err := runtimeNullableFloat(row, latColumn)
	if err != nil {
		return geospatialcore.GeoJSONGeometry{}, fmt.Errorf("latitude column %q: %w", latColumn, err)
	}
	if null {
		return geospatialcore.GeoJSONGeometry{}, fmt.Errorf("latitude column %q is null", latColumn)
	}
	lon, null, err := runtimeNullableFloat(row, lonColumn)
	if err != nil {
		return geospatialcore.GeoJSONGeometry{}, fmt.Errorf("longitude column %q: %w", lonColumn, err)
	}
	if null {
		return geospatialcore.GeoJSONGeometry{}, fmt.Errorf("longitude column %q is null", lonColumn)
	}
	point, err := geospatialcore.NewGeoPoint(lat, lon)
	if err != nil {
		return geospatialcore.GeoJSONGeometry{}, err
	}
	return geospatialcore.NewGeoJSONPoint(point)
}

func parseRuntimeGeoJSONGeometry(raw json.RawMessage) (geospatialcore.GeoJSONGeometry, error) {
	if stringValue := runtimeScalarString(raw); strings.HasPrefix(strings.TrimSpace(stringValue), "{") {
		return geospatialcore.ParseGeoJSONGeometry([]byte(stringValue))
	}
	return geospatialcore.ParseGeoJSONGeometry(raw)
}

func runtimeGeoJoinMatches(
	left geospatialcore.GeoJSONGeometry,
	right []geospatialcore.GeoJSONGeometry,
	mode string,
	unit string,
	maxDistance float64,
	k int,
) ([]geoJoinMatch, error) {
	matches := make([]geoJoinMatch, 0)
	for index, candidate := range right {
		switch mode {
		case "intersection":
			ok, err := geospatialcore.GeoJSONIntersects(left, candidate)
			if err != nil {
				return nil, err
			}
			if ok {
				matches = append(matches, geoJoinMatch{rightIndex: index})
			}
		case "distance", "nearest":
			distance, err := geospatialcore.GeoJSONDistance(left, candidate, unit)
			if err != nil {
				return nil, err
			}
			if maxDistance > 0 && distance > maxDistance {
				continue
			}
			matches = append(matches, geoJoinMatch{rightIndex: index, distance: distance})
		default:
			return nil, fmt.Errorf("unsupported_geo_join_mode:%s", mode)
		}
	}
	if mode == "nearest" {
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].distance != matches[j].distance {
				return matches[i].distance < matches[j].distance
			}
			return matches[i].rightIndex < matches[j].rightIndex
		})
		if len(matches) > k {
			matches = matches[:k]
		}
		for i := range matches {
			matches[i].rank = i + 1
		}
	}
	return matches, nil
}

func composeRuntimeGeoJoinRow(
	left pipelineexpression.Row,
	right pipelineexpression.Row,
	leftColumns []string,
	rightColumns []string,
	rightPrefix string,
	distanceColumn string,
	rankColumn string,
	match *geoJoinMatch,
) pipelineexpression.Row {
	out := composeRuntimeJoinRow(left, right, leftColumns, rightColumns, rightPrefix)
	if distanceColumn != "" {
		if match == nil {
			out[distanceColumn] = json.RawMessage("null")
		} else {
			out[distanceColumn] = mustRuntimeJSON(match.distance)
		}
	}
	if rankColumn != "" && match != nil && match.rank > 0 {
		out[rankColumn] = mustRuntimeJSON(match.rank)
	}
	return out
}

func normalizedGeoJoinMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "intersects", "intersection", "geometry_intersection", "intersect":
		return "intersection"
	case "within_distance", "distance", "geometry_distance":
		return "distance"
	case "nearest", "nearest_neighbor", "knn", "geometry_nearest_neighbor":
		return "nearest"
	default:
		if strings.TrimSpace(mode) == "" {
			return "intersection"
		}
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func geoJoinUnitSuffix(unit string) string {
	parsed, err := geospatialcore.ParseDistanceUnit(unit)
	if err != nil {
		return "miles"
	}
	switch parsed {
	case geospatialcore.DistanceUnitMeters:
		return "meters"
	case geospatialcore.DistanceUnitKilometers:
		return "km"
	default:
		return "miles"
	}
}

func intWithDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
