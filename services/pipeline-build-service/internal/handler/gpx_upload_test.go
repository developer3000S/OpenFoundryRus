package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGPXUploadJSONReturnsTrailRow(t *testing.T) {
	raw, err := os.ReadFile("testdata/mesa_loop.gpx")
	require.NoError(t, err)
	body, err := json.Marshal(parseGPXUploadRequest{GPX: string(raw), SourceName: "mesa_loop.gpx"})
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/geospatial/gpx/parse", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ParseGPXUpload(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var payload parseGPXUploadResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.JSONEq(t, `"mesa-loop"`, string(payload.Row["trail_id"]))
	require.JSONEq(t, `"Mesa Loop"`, string(payload.Row["trail_name"]))
	require.JSONEq(t, `"40,-105.3"`, string(payload.Row["trailhead_geo_point"]))
	require.Len(t, payload.Schema, len(gpxTrailStrictSchema().Fields))
}

func TestParseGPXUploadMultipartReturnsTrailRow(t *testing.T) {
	raw, err := os.ReadFile("testdata/mesa_loop.gpx")
	require.NoError(t, err)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("trail_name", "Uploaded Mesa"))
	file, err := writer.CreateFormFile("file", "uploaded-mesa.gpx")
	require.NoError(t, err)
	_, err = file.Write(raw)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/geospatial/gpx/parse", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ParseGPXUpload(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var payload parseGPXUploadResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.JSONEq(t, `"uploaded-mesa"`, string(payload.Row["trail_id"]))
	require.JSONEq(t, `"Uploaded Mesa"`, string(payload.Row["trail_name"]))
	require.JSONEq(t, `4`, string(payload.Row["point_count"]))
}
