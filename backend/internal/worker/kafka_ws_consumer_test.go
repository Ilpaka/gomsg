package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/kafka"
)

func TestFanoutKafkaDomainEvent_deliversEnvelope(t *testing.T) {
	t.Parallel()
	chat := "30000000-0000-4000-8000-000000000003"
	pl, _ := json.Marshal(map[string]string{"text": "hi"})
	ev := kafka.DomainEvent{
		EventID:       "e1",
		EventType:     domain.EventMessageCreated,
		AggregateType: "message",
		AggregateID:   "m1",
		ChatID:        chat,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		Payload:       pl,
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	del := &fakeDeliver{}
	if err := FanoutKafkaDomainEvent(context.Background(), del, raw, nil); err != nil {
		t.Fatal(err)
	}
	if del.calls != 1 || string(del.chats[0]) != chat {
		t.Fatalf("unexpected deliver: calls=%d chats=%v", del.calls, del.chats)
	}
	var env map[string]any
	if err := json.Unmarshal(del.envs[0], &env); err != nil {
		t.Fatal(err)
	}
	if env["event"] != domain.EventMessageCreated {
		t.Fatalf("event field: %v", env["event"])
	}
}
