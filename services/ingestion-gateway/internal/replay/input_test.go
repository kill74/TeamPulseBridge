package replay

import "testing"

func TestParseInputWrappedEnvelope(t *testing.T) {
	payload := []byte(`{"source":"github","headers":{"Content-Type":"application/json"},"body":{"action":"opened"}}`)
	event, err := ParseInput(payload, "", nil)
	if err != nil {
		t.Fatalf("parse wrapped envelope: %v", err)
	}
	if event.Source != "github" {
		t.Fatalf("expected source github, got %q", event.Source)
	}
	if got := event.Headers["Content-Type"]; got != "application/json" {
		t.Fatalf("expected content-type header, got %q", got)
	}
	if string(event.Body) != `{"action":"opened"}` {
		t.Fatalf("unexpected body: %s", string(event.Body))
	}
}

func TestParseInputRawPayload(t *testing.T) {
	payload := []byte(`{"action":"opened","number":42}`)
	event, err := ParseInput(payload, "github", map[string]string{"X-Replay": "true"})
	if err != nil {
		t.Fatalf("parse raw payload: %v", err)
	}
	if event.Source != "github" {
		t.Fatalf("expected source github, got %q", event.Source)
	}
	if got := event.Headers["X-Replay"]; got != "true" {
		t.Fatalf("expected replay header, got %q", got)
	}
}

func TestParseInputRejectsRawPayloadWithoutSource(t *testing.T) {
	payload := []byte(`{"action":"opened"}`)
	if _, err := ParseInput(payload, "", nil); err == nil {
		t.Fatal("expected missing source error")
	}
}

func TestParseInputRejectsInvalidJSON(t *testing.T) {
	payload := []byte(`{`)
	if _, err := ParseInput(payload, "github", nil); err == nil {
		t.Fatal("expected invalid json error")
	}
}
