package admin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/monsterxx03/linko/pkg/mitm"
)

func TestSerializeTrafficEvent(t *testing.T) {
	event := &mitm.TrafficEvent{Hostname: "api.example.com", Direction: "client->server"}

	name, data, err := serializeTrafficEvent(event)
	if err != nil {
		t.Fatalf("serializeTrafficEvent failed: %v", err)
	}
	if name != "traffic" {
		t.Errorf("name = %q, want %q", name, "traffic")
	}
	var decoded mitm.TrafficEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("payload is not the event JSON: %v", err)
	}
	if decoded.Hostname != "api.example.com" {
		t.Errorf("hostname = %q, want %q", decoded.Hostname, "api.example.com")
	}
}

func TestSerializeLLMEvent(t *testing.T) {
	extra := map[string]any{"conversation_id": "conv-1", "delta": "hello"}

	t.Run("extra with llm_token direction", func(t *testing.T) {
		event := &mitm.TrafficEvent{Direction: "llm_token", Extra: extra}
		name, data, err := serializeLLMEvent(event)
		if err != nil {
			t.Fatalf("serializeLLMEvent failed: %v", err)
		}
		if name != "llm_token" {
			t.Errorf("name = %q, want %q", name, "llm_token")
		}
		// Payload must be the Extra field itself, not the wrapping TrafficEvent.
		if !strings.Contains(string(data), "conv-1") {
			t.Errorf("payload does not contain Extra content: %s", data)
		}
		if strings.Contains(string(data), "hostname") {
			t.Errorf("payload should not contain TrafficEvent wrapper fields: %s", data)
		}
	})

	t.Run("extra with conversation direction", func(t *testing.T) {
		event := &mitm.TrafficEvent{Direction: "conversation", Extra: extra}
		name, _, err := serializeLLMEvent(event)
		if err != nil {
			t.Fatalf("serializeLLMEvent failed: %v", err)
		}
		if name != "conversation" {
			t.Errorf("name = %q, want %q", name, "conversation")
		}
	})

	t.Run("extra with unknown direction falls back to traffic", func(t *testing.T) {
		event := &mitm.TrafficEvent{Direction: "llm_error", Extra: extra}
		name, data, err := serializeLLMEvent(event)
		if err != nil {
			t.Fatalf("serializeLLMEvent failed: %v", err)
		}
		if name != "traffic" {
			t.Errorf("name = %q, want %q", name, "traffic")
		}
		if !strings.Contains(string(data), "conv-1") {
			t.Errorf("payload does not contain Extra content: %s", data)
		}
	})

	t.Run("no extra falls back to whole event as traffic", func(t *testing.T) {
		event := &mitm.TrafficEvent{Hostname: "api.example.com"}
		name, data, err := serializeLLMEvent(event)
		if err != nil {
			t.Fatalf("serializeLLMEvent failed: %v", err)
		}
		if name != "traffic" {
			t.Errorf("name = %q, want %q", name, "traffic")
		}
		var decoded mitm.TrafficEvent
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("payload is not the event JSON: %v", err)
		}
		if decoded.Hostname != "api.example.com" {
			t.Errorf("hostname = %q, want %q", decoded.Hostname, "api.example.com")
		}
	})
}
