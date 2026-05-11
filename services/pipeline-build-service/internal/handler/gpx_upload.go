package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	geospatialcore "github.com/openfoundry/openfoundry-go/libs/geospatial-core"
)

const maxGPXUploadBytes = 16 << 20

type parseGPXUploadRequest struct {
	GPX        string `json:"gpx,omitempty"`
	GPXXML     string `json:"gpx_xml,omitempty"`
	GPXContent string `json:"gpx_content,omitempty"`
	TrailID    string `json:"trail_id,omitempty"`
	TrailName  string `json:"trail_name,omitempty"`
	SourceName string `json:"source_name,omitempty"`
}

type parseGPXUploadResponse struct {
	Row    map[string]json.RawMessage      `json:"row"`
	Schema []pipelineStrictValidationField `json:"schema"`
	Trail  geospatialcore.GPXTrail         `json:"trail"`
	Meta   map[string]any                  `json:"meta"`
}

func ParseGPXUpload(w http.ResponseWriter, r *http.Request) {
	raw, opts, err := gpxUploadPayload(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_gpx_upload", "detail": err.Error()})
		return
	}
	trail, err := geospatialcore.ParseGPXTrail(raw, opts)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "gpx_parse_failed", "detail": err.Error()})
		return
	}
	row, err := gpxTrailToRuntimeRow(trail)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "gpx_row_mapping_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, parseGPXUploadResponse{
		Row:    map[string]json.RawMessage(row),
		Schema: gpxTrailStrictSchema().Fields,
		Trail:  trail,
		Meta: map[string]any{
			"runtime":        "lightweight_table",
			"transform_type": "gpx_parse",
			"rows_affected":  1,
		},
	})
}

func gpxUploadPayload(r *http.Request) ([]byte, geospatialcore.GPXParseOptions, error) {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "multipart/form-data") {
		return multipartGPXUploadPayload(r)
	}
	return jsonGPXUploadPayload(r)
}

func multipartGPXUploadPayload(r *http.Request) ([]byte, geospatialcore.GPXParseOptions, error) {
	if err := r.ParseMultipartForm(maxGPXUploadBytes); err != nil {
		return nil, geospatialcore.GPXParseOptions{}, err
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		file, header, err = r.FormFile("gpx_file")
	}
	if err != nil {
		return nil, geospatialcore.GPXParseOptions{}, fmt.Errorf("multipart upload requires file or gpx_file")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxGPXUploadBytes+1))
	if err != nil {
		return nil, geospatialcore.GPXParseOptions{}, err
	}
	if len(raw) > maxGPXUploadBytes {
		return nil, geospatialcore.GPXParseOptions{}, fmt.Errorf("GPX upload exceeds %d bytes", maxGPXUploadBytes)
	}
	return raw, geospatialcore.GPXParseOptions{
		TrailID:    r.FormValue("trail_id"),
		TrailName:  r.FormValue("trail_name"),
		SourceName: firstNonEmpty(r.FormValue("source_name"), header.Filename),
	}, nil
}

func jsonGPXUploadPayload(r *http.Request) ([]byte, geospatialcore.GPXParseOptions, error) {
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, maxGPXUploadBytes+1))
	if err != nil {
		return nil, geospatialcore.GPXParseOptions{}, err
	}
	if len(rawBody) > maxGPXUploadBytes {
		return nil, geospatialcore.GPXParseOptions{}, fmt.Errorf("GPX JSON body exceeds %d bytes", maxGPXUploadBytes)
	}
	var req parseGPXUploadRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		return nil, geospatialcore.GPXParseOptions{}, err
	}
	raw := strings.TrimSpace(firstNonEmpty(req.GPXContent, req.GPXXML, req.GPX))
	if raw == "" {
		return nil, geospatialcore.GPXParseOptions{}, fmt.Errorf("JSON body requires gpx, gpx_xml, or gpx_content")
	}
	return []byte(raw), geospatialcore.GPXParseOptions{
		TrailID:    req.TrailID,
		TrailName:  req.TrailName,
		SourceName: req.SourceName,
	}, nil
}
