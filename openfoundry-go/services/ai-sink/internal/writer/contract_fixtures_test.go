package writer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/envelope"
)

func TestAIJSONLWriterPayloadContractFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/prompt_jsonl_record.json")
	if err != nil {
		t.Fatal(err)
	}
	env, err := envelope.Decode(raw)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	dir := t.TempDir()
	w, err := NewJSONLWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(context.Background(), map[string][]envelope.AiEventEnvelope{envelope.TablePrompts: {env}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, envelope.TablePrompts+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(out))
	assertSameJSON(t, raw, []byte(line))
}

func assertSameJSON(t *testing.T, wantRaw, gotRaw []byte) {
	t.Helper()
	var want, got any
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(gotRaw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("JSON mismatch\nwant: %s\n got: %s", wantRaw, gotRaw)
	}
}
