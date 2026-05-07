// Package mediafunctions ports services/ontology-actions-service/src/media_functions.rs
// 1:1: the Foundry "Functions on objects → Media" surface.
//
// The five public entry points (read_raw, ocr, extract_text,
// transcribe_audio, read_metadata) live here; each delegates to a
// pluggable Runtime so production wiring (HTTP client into
// media-transform-runtime-service) and tests (MockRuntime, below)
// share a single trait surface. This is the only piece of
// ontology-actions-service that is fully self-contained — the rest
// of the kernel-handler logic depends on libs/ontology-kernel-go,
// which only ships models today.
package mediafunctions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// ItemHandle mirrors the Foundry MediaItemHandle. JSON tags use
// camelCase to match the Rust `#[serde(rename_all = "camelCase")]`.
type ItemHandle struct {
	MediaSetRID  string  `json:"mediaSetRid"`
	MediaItemRID string  `json:"mediaItemRid"`
	Branch       *string `json:"branch,omitempty"`
	Schema       *string `json:"schema,omitempty"`
}

// NewItemHandle is a convenience constructor mirroring the Rust
// `MediaItemHandle::new`.
func NewItemHandle(setRID, itemRID string) ItemHandle {
	return ItemHandle{MediaSetRID: setRID, MediaItemRID: itemRID}
}

// TranscriptSegment is one timestamped span in a transcription.
type TranscriptSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// Transcription is the result of `transcribe_audio`.
type Transcription struct {
	Text     string              `json:"text"`
	Segments []TranscriptSegment `json:"segments"`
}

// ErrorKind matches Rust's `MediaFunctionError` discriminants.
type ErrorKind int

const (
	KindNotFound ErrorKind = iota
	KindNotImplemented
	KindRuntime
)

// Error is the typed failure shape of every public entry point.
type Error struct {
	Kind   ErrorKind
	Target string
	Reason string
}

func (e *Error) Error() string {
	switch e.Kind {
	case KindNotFound:
		return fmt.Sprintf("media item not found: %s", e.Target)
	case KindNotImplemented:
		return fmt.Sprintf("media transformation `%s` is not implemented yet: %s", e.Target, e.Reason)
	case KindRuntime:
		return fmt.Sprintf("runtime error: %s", e.Reason)
	default:
		return e.Reason
	}
}

// IsNotFound reports whether err is a not-found Error (Rust
// `matches!(err, MediaFunctionError::NotFound(_))`).
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Kind == KindNotFound
}

// Runtime is what the binary wires against
// media-transform-runtime-service and what tests emulate. Implementations
// must be safe for concurrent use across goroutines.
type Runtime interface {
	ReadRaw(ctx context.Context, item ItemHandle) ([]byte, error)
	OCR(ctx context.Context, item ItemHandle) (string, error)
	ExtractText(ctx context.Context, item ItemHandle) (string, error)
	TranscribeAudio(ctx context.Context, item ItemHandle) (Transcription, error)
	ReadMetadata(ctx context.Context, item ItemHandle) (json.RawMessage, error)
}

// Public function entry points. Each forwards to the runtime so
// callers (TS / Python function bodies, tests, gRPC handlers) can
// pin which implementation runs without touching a global.
func ReadRaw(ctx context.Context, rt Runtime, item ItemHandle) ([]byte, error) {
	return rt.ReadRaw(ctx, item)
}
func OCR(ctx context.Context, rt Runtime, item ItemHandle) (string, error) {
	return rt.OCR(ctx, item)
}
func ExtractText(ctx context.Context, rt Runtime, item ItemHandle) (string, error) {
	return rt.ExtractText(ctx, item)
}
func TranscribeAudio(ctx context.Context, rt Runtime, item ItemHandle) (Transcription, error) {
	return rt.TranscribeAudio(ctx, item)
}
func ReadMetadata(ctx context.Context, rt Runtime, item ItemHandle) (json.RawMessage, error) {
	return rt.ReadMetadata(ctx, item)
}

// MockCall is one entry in MockRuntime's call log.
type MockCall struct {
	Method string
	Item   ItemHandle
}

// MockRuntime records every call and returns scripted responses
// keyed by MediaItemRID. Lives in the public package surface (not
// behind a build tag) so cross-package tests can use it without a
// dev-dependency cycle.
type MockRuntime struct {
	mu          sync.Mutex
	rawData     map[string][]byte
	ocrData     map[string]string
	textData    map[string]string
	transcripts map[string]Transcription
	metadata    map[string]json.RawMessage
	callLog     []MockCall
}

func NewMockRuntime() *MockRuntime {
	return &MockRuntime{
		rawData:     map[string][]byte{},
		ocrData:     map[string]string{},
		textData:    map[string]string{},
		transcripts: map[string]Transcription{},
		metadata:    map[string]json.RawMessage{},
	}
}

func (m *MockRuntime) PutRaw(itemRID string, b []byte) *MockRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rawData[itemRID] = b
	return m
}
func (m *MockRuntime) PutOCR(itemRID, text string) *MockRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ocrData[itemRID] = text
	return m
}
func (m *MockRuntime) PutText(itemRID, text string) *MockRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.textData[itemRID] = text
	return m
}
func (m *MockRuntime) PutTranscript(itemRID string, t Transcription) *MockRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transcripts[itemRID] = t
	return m
}
func (m *MockRuntime) PutMetadata(itemRID string, v json.RawMessage) *MockRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metadata[itemRID] = v
	return m
}

func (m *MockRuntime) Calls() []MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MockCall, len(m.callLog))
	copy(out, m.callLog)
	return out
}

func (m *MockRuntime) record(method string, item ItemHandle) {
	m.callLog = append(m.callLog, MockCall{Method: method, Item: item})
}

func (m *MockRuntime) ReadRaw(_ context.Context, item ItemHandle) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("read_raw", item)
	if b, ok := m.rawData[item.MediaItemRID]; ok {
		return b, nil
	}
	return nil, &Error{Kind: KindNotFound, Target: item.MediaItemRID}
}

func (m *MockRuntime) OCR(_ context.Context, item ItemHandle) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("ocr", item)
	if v, ok := m.ocrData[item.MediaItemRID]; ok {
		return v, nil
	}
	return "", &Error{
		Kind:   KindNotImplemented,
		Target: "ocr",
		Reason: "no OCR scripted for `" + item.MediaItemRID + "` in MockRuntime",
	}
}

func (m *MockRuntime) ExtractText(_ context.Context, item ItemHandle) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("extract_text", item)
	if v, ok := m.textData[item.MediaItemRID]; ok {
		return v, nil
	}
	return "", &Error{
		Kind:   KindNotImplemented,
		Target: "extract_text",
		Reason: "no extract_text scripted for `" + item.MediaItemRID + "` in MockRuntime",
	}
}

func (m *MockRuntime) TranscribeAudio(_ context.Context, item ItemHandle) (Transcription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("transcribe_audio", item)
	if v, ok := m.transcripts[item.MediaItemRID]; ok {
		return v, nil
	}
	return Transcription{}, &Error{
		Kind:   KindNotImplemented,
		Target: "transcribe_audio",
		Reason: "no transcript scripted for `" + item.MediaItemRID + "` in MockRuntime",
	}
}

func (m *MockRuntime) ReadMetadata(_ context.Context, item ItemHandle) (json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("read_metadata", item)
	if v, ok := m.metadata[item.MediaItemRID]; ok {
		return v, nil
	}
	return nil, &Error{Kind: KindNotFound, Target: item.MediaItemRID}
}
