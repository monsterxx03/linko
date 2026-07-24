package mitm

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// receivePublished reads one PublishedEvent with a timeout.
func receivePublished(t *testing.T, sub *Subscriber) *PublishedEvent {
	t.Helper()
	select {
	case pe := <-sub.Channel:
		return pe
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for published event")
		return nil
	}
}

// TestEventBusSerializeOnce verifies that the serializer runs exactly once
// per published event no matter how many subscribers there are, and that
// every subscriber receives the same pre-serialized payload.
func TestEventBusSerializeOnce(t *testing.T) {
	logger := slog.Default()
	bus := NewEventBus(logger, 10)

	calls := 0
	bus.SetSerializer(func(event *TrafficEvent) (string, []byte, error) {
		calls++
		return "custom_type", []byte(`{"id":"` + event.ID + `"}`), nil
	})

	subs := []*Subscriber{bus.Subscribe(), bus.Subscribe(), bus.Subscribe()}

	// Drain history replays (none yet, but keep the channels empty).
	bus.Publish(&TrafficEvent{Hostname: "api.example.com"})

	if calls != 1 {
		t.Fatalf("serializer called %d times for 1 event with 3 subscribers, want 1", calls)
	}

	var reference []byte
	for i, sub := range subs {
		pe := receivePublished(t, sub)
		if pe.Name != "custom_type" {
			t.Errorf("subscriber %d: name = %q, want %q", i, pe.Name, "custom_type")
		}
		if pe.Err != nil {
			t.Errorf("subscriber %d: unexpected serialization error: %v", i, pe.Err)
		}
		if reference == nil {
			reference = pe.Data
		} else if string(pe.Data) != string(reference) {
			t.Errorf("subscriber %d: payload differs from other subscribers", i)
		}
	}
}

// TestEventBusDefaultSerializer verifies that without a registered
// serializer, events are delivered as whole-event JSON under "traffic".
func TestEventBusDefaultSerializer(t *testing.T) {
	logger := slog.Default()
	bus := NewEventBus(logger, 10)
	sub := bus.Subscribe()

	bus.Publish(&TrafficEvent{Hostname: "api.example.com", Direction: "client->server"})

	pe := receivePublished(t, sub)
	if pe.Name != "traffic" {
		t.Errorf("name = %q, want %q", pe.Name, "traffic")
	}
	var decoded TrafficEvent
	if err := json.Unmarshal(pe.Data, &decoded); err != nil {
		t.Fatalf("default payload is not the event JSON: %v", err)
	}
	if decoded.Hostname != "api.example.com" {
		t.Errorf("decoded hostname = %q, want %q", decoded.Hostname, "api.example.com")
	}
}

// TestEventBusHistoryReplayKeepsPayload verifies that history replay to new
// subscribers delivers the pre-serialized payload without re-serializing.
func TestEventBusHistoryReplayKeepsPayload(t *testing.T) {
	logger := slog.Default()
	bus := NewEventBus(logger, 10)

	calls := 0
	bus.SetSerializer(func(event *TrafficEvent) (string, []byte, error) {
		calls++
		return "custom_type", []byte(`{"n":1}`), nil
	})

	// Publish with no subscribers; events land in history.
	bus.Publish(&TrafficEvent{Hostname: "a.example.com"})
	bus.Publish(&TrafficEvent{Hostname: "b.example.com"})
	if calls != 2 {
		t.Fatalf("serializer called %d times, want 2", calls)
	}

	// A late subscriber receives the replayed history with payloads intact.
	sub := bus.Subscribe()
	for i := 0; i < 2; i++ {
		pe := receivePublished(t, sub)
		if pe.Name != "custom_type" || len(pe.Data) == 0 {
			t.Errorf("replayed event %d missing pre-serialized payload: %+v", i, pe)
		}
	}
	if calls != 2 {
		t.Fatalf("serializer called during replay: got %d calls, want 2", calls)
	}
}

// TestEventBusSerializationError verifies that a serialization error is
// carried on the PublishedEvent instead of being silently dropped.
func TestEventBusSerializationError(t *testing.T) {
	logger := slog.Default()
	bus := NewEventBus(logger, 10)
	bus.SetSerializer(func(event *TrafficEvent) (string, []byte, error) {
		return "", nil, errTestSerialize
	})
	sub := bus.Subscribe()

	bus.Publish(&TrafficEvent{Hostname: "api.example.com"})

	pe := receivePublished(t, sub)
	if pe.Err == nil || !strings.Contains(pe.Err.Error(), "serialize") {
		t.Errorf("expected serialization error on PublishedEvent, got %v", pe.Err)
	}
}

var errTestSerialize = &testSerializeError{msg: "serialize failed"}

type testSerializeError struct{ msg string }

func (e *testSerializeError) Error() string { return e.msg }
