package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/media-transform-runtime-service/internal/runtime"
)

// makeSolidPNG produces a w×h PNG filled with c — used by the
// HTTP-level round-trip tests that exercise the native dispatch.
func makeSolidPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(BuildRouter(observability.NewMetrics()))
}

func TestHealthzPlainText(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	b, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", string(b))
}

func TestCatalogListEndpoint(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/catalog")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var entries []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&entries))
	assert.GreaterOrEqual(t, len(entries), 32, "catalog should expose all Rust entries")
	first := entries[0]
	assert.Equal(t, "thumbnail", first["key"])
	status := first["status"].(map[string]any)
	assert.Equal(t, "native", status["kind"])
}

func TestCatalogEntryByKey(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/catalog/ocr")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var entry map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&entry))
	assert.Equal(t, "ocr", entry["key"])
	status := entry["status"].(map[string]any)
	assert.Equal(t, "external", status["kind"])
	assert.Equal(t, "tesseract", status["binary"])
}

func TestCatalogEntryUnknownKindIs400(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/catalog/not-a-kind")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, runtime.CodeUnknownKind, body["code"])
}

func TestTransformExternalReturnsNotImplementedEnvelope(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	body := `{"kind":"ocr","mime_type":"image/png","schema":"IMAGE","bytes_base64":""}`
	resp, err := http.Post(srv.URL+"/transform", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Equal(t, "NOT_IMPLEMENTED", out["status"])
	assert.Equal(t, "ocr", out["kind"])
	assert.Equal(t, "image/png", out["output_mime_type"])
	assert.Contains(t, out["reason"], "tesseract")
}

func TestTransformRustNotImplementedEntriesReturnParityEnvelope(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	cases := map[string]string{
		"geo_tile":        "Geo tile pyramids land in the geospatial-intelligence-service follow-up.",
		"embedding":       "Image embeddings depend on libs/ai-kernel which is not yet wired.",
		"transcription":   "Transcription depends on libs/ai-kernel (Whisper / VLM) which is not yet wired.",
		"layout_aware_v2": "Layout-aware extraction depends on libs/ai-kernel which is not yet wired.",
		"vlm_extract":     "VLM extraction depends on libs/ai-kernel which is not yet wired.",
		"render_sheet":    "Spreadsheet rendering depends on the spreadsheet-computation domain absorbed into notebook-runtime-service (S8 / ADR-0030); runtime not yet wired.",
	}
	for kind, wantReason := range cases {
		kind, wantReason := kind, wantReason
		t.Run(kind, func(t *testing.T) {
			body := `{"kind":"` + kind + `","mime_type":"application/octet-stream","schema":"IMAGE","bytes_base64":""}`
			resp, err := http.Post(srv.URL+"/transform", "application/json", strings.NewReader(body))
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var out map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
			assert.Equal(t, "NOT_IMPLEMENTED", out["status"])
			assert.Equal(t, kind, out["kind"])
			assert.Equal(t, "application/octet-stream", out["output_mime_type"])
			assert.Equal(t, float64(0), out["compute_seconds"])
			assert.Equal(t, wantReason, out["reason"])
			assert.NotContains(t, out, "output_bytes_base64")
			assert.NotContains(t, out, "output_json")
		})
	}
}

func TestTransformThumbnailRoundTripsPNG(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	// 8×4 black PNG — the runtime decodes, fits inside a 4×4 box,
	// re-encodes, and surfaces the bytes back to the caller.
	src := makeSolidPNG(t, 8, 4, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	reqBody := `{
		"kind": "thumbnail",
		"mime_type": "image/png",
		"schema": "IMAGE",
		"params": {"max_dim": 4},
		"bytes_base64": "` + base64.StdEncoding.EncodeToString(src) + `"
	}`
	resp, err := http.Post(srv.URL+"/transform", "application/json", strings.NewReader(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Equal(t, "OK", out["status"])
	assert.Equal(t, "thumbnail", out["kind"])
	assert.Equal(t, "image/png", out["output_mime_type"])
	require.NotEmpty(t, out["output_bytes_base64"])

	decoded, err := base64.StdEncoding.DecodeString(out["output_bytes_base64"].(string))
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(decoded))
	require.NoError(t, err)
	assert.LessOrEqual(t, img.Bounds().Dx(), 4)
	assert.LessOrEqual(t, img.Bounds().Dy(), 4)
}

func TestTransformUnknownMimeIs400(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	src := makeSolidPNG(t, 2, 2, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	body := `{
		"kind": "grayscale",
		"mime_type": "image/avif",
		"schema": "IMAGE",
		"bytes_base64": "` + base64.StdEncoding.EncodeToString(src) + `"
	}`
	resp, err := http.Post(srv.URL+"/transform", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var b map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&b))
	assert.Equal(t, runtime.CodeBadInput, b["code"])
	assert.Contains(t, b["error"], "image/avif")
}

func TestTransformUnknownKindIs400(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	body := `{"kind":"not-a-kind","mime_type":"image/png","schema":"IMAGE","bytes_base64":""}`
	resp, err := http.Post(srv.URL+"/transform", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body2 map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body2))
	assert.Equal(t, runtime.CodeUnknownKind, body2["code"])
}

func TestTransformBadJSONIs400(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/transform", "application/json", strings.NewReader("not-json"))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, runtime.CodeBadInput, body["code"])
}
