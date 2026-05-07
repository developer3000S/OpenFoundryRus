package catalog

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerStatusJSONShape(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   HandlerStatus
		want string
	}{
		{"native", HandlerStatus{Kind: StatusNative}, `{"kind":"native"}`},
		{"external", HandlerStatus{Kind: StatusExternal, Binary: "ffmpeg"}, `{"kind":"external","binary":"ffmpeg"}`},
		{"not_implemented", HandlerStatus{Kind: StatusNotImplemented, Reason: "x"}, `{"kind":"not_implemented","reason":"x"}`},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(c.in)
			require.NoError(t, err)
			assert.JSONEq(t, c.want, string(b))

			var got HandlerStatus
			require.NoError(t, json.Unmarshal([]byte(c.want), &got))
			assert.Equal(t, c.in, got)
		})
	}
}

func TestCatalogContainsAllRustKeys(t *testing.T) {
	t.Parallel()
	expectedKeys := []string{
		// Image
		"thumbnail", "resize", "resize_within_bounding_box", "rotate", "crop", "grayscale",
		"geo_tile", "render_dicom_image_layer", "ocr", "embedding",
		// Audio
		"chunk", "channel_select", "hls_stream", "transcode", "waveform", "transcription",
		// Video
		"scene_frames_timestamps", "extract_audio", "extract_frames", "video_chunk",
		"scene_frames_all", "video_hls_stream", "video_transcode",
		// Document
		"pdf_page_dimensions", "render_page", "extract_text_raw", "extract_form_fields",
		"extract_toc", "slice_pdf_range", "doc_ocr", "layout_aware_v2", "vlm_extract",
		// Spreadsheet
		"render_sheet",
	}
	for _, k := range expectedKeys {
		_, ok := Lookup(k)
		assert.True(t, ok, "catalog must contain key %q", k)
	}
	assert.Equal(t, len(expectedKeys), len(Catalog), "catalog length must match Rust source")
}

func TestImageHandlersAreNative(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"thumbnail", "resize", "resize_within_bounding_box", "rotate", "crop", "grayscale"} {
		s, ok := Lookup(k)
		require.True(t, ok)
		assert.Equal(t, StatusNative, s.Kind, "%s should be native in Go (matches Rust)", k)
	}
}

func TestExternalBinariesPreserved(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"ocr":                      "tesseract",
		"render_dicom_image_layer": "dcmtk",
		"chunk":                    "ffmpeg",
		"transcode":                "ffmpeg",
		"pdf_page_dimensions":      "pdfium",
		"extract_text_raw":         "pdftotext",
		"slice_pdf_range":          "qpdf",
	}
	for k, want := range cases {
		s, ok := Lookup(k)
		require.True(t, ok, "missing key %q", k)
		assert.Equal(t, StatusExternal, s.Kind, "%s should be external", k)
		assert.Equal(t, want, s.Binary, "%s binary mismatch", k)
	}
}

func TestRustNotImplementedEntriesPreserved(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"geo_tile":        "Geo tile pyramids land in the geospatial-intelligence-service follow-up.",
		"embedding":       "Image embeddings depend on libs/ai-kernel which is not yet wired.",
		"transcription":   "Transcription depends on libs/ai-kernel (Whisper / VLM) which is not yet wired.",
		"layout_aware_v2": "Layout-aware extraction depends on libs/ai-kernel which is not yet wired.",
		"vlm_extract":     "VLM extraction depends on libs/ai-kernel which is not yet wired.",
		"render_sheet":    "Spreadsheet rendering depends on the spreadsheet-computation domain absorbed into notebook-runtime-service (S8 / ADR-0030); runtime not yet wired.",
	}
	for key, wantReason := range cases {
		key, wantReason := key, wantReason
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			status, ok := Lookup(key)
			require.True(t, ok, "missing key %q", key)
			assert.Equal(t, StatusNotImplemented, status.Kind, "%s must stay NotImplemented to match Rust", key)
			assert.Equal(t, wantReason, status.Reason, "%s reason must match Rust catalog.rs verbatim", key)
			assert.Empty(t, status.Binary, "%s must not be marked external/native unless Rust wires a real handler", key)
		})
	}
}

func TestUnknownKeyLookup(t *testing.T) {
	t.Parallel()
	_, ok := Lookup("not-a-real-kind")
	assert.False(t, ok)
}
