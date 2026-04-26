package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/kafka"
	"goflow/backend/internal/repository"
)

type fakeOutboxRepo struct {
	rows     []repository.OutboxRow
	markedID []int64
}

func (f *fakeOutboxRepo) FetchPending(ctx context.Context, limit int) ([]repository.OutboxRow, error) {
	return f.rows, nil
}

func (f *fakeOutboxRepo) MarkPublished(ctx context.Context, id int64) error {
	f.markedID = append(f.markedID, id)
	return nil
}

type fakeKafkaPub struct {
	keys   []string
	values [][]byte
}

func (f *fakeKafkaPub) Publish(ctx context.Context, key string, value []byte) error {
	f.keys = append(f.keys, key)
	f.values = append(f.values, value)
	return nil
}

type fakeDeliver struct {
	chats  []domain.ID
	envs   [][]byte
	err    error
	calls  int
}

func (f *fakeDeliver) DeliverEnvelopeBytes(ctx context.Context, chatID domain.ID, payload []byte) error {
	f.calls++
	f.chats = append(f.chats, chatID)
	f.envs = append(f.envs, append([]byte(nil), payload...))
	return f.err
}

func TestOutboxRelay_tick_kafkaMapsRowAndMarksPublished(t *testing.T) {
	t.Parallel()
	chat := "30000000-0000-4000-8000-000000000003"
	payload, _ := json.Marshal(map[string]string{"message_id": "50000000-0000-4000-8000-000000000005"})
	row := repository.OutboxRow{
		ID:            42,
		EventID:       "e1",
		EventType:     domain.EventMessageCreated,
		AggregateType: "message",
		AggregateID:   "50000000-0000-4000-8000-000000000005",
		ChatID:        &chat,
		OccurredAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Version:       1,
		Payload:       payload,
	}
	ob := &fakeOutboxRepo{rows: []repository.OutboxRow{row}}
	kp := &fakeKafkaPub{}
	r := &OutboxRelay{
		Outbox:   ob,
		Kafka:    kp,
		UseKafka: true,
		Log:      nil,
	}
	r.tick(context.Background())
	if len(kp.keys) != 1 || kp.keys[0] != chat {
		t.Fatalf("expected kafka key chat_id, got %#v", kp.keys)
	}
	var ev kafka.DomainEvent
	if err := json.Unmarshal(kp.values[0], &ev); err != nil {
		t.Fatal(err)
	}
	if ev.EventType != domain.EventMessageCreated || ev.ChatID != chat || string(ev.Payload) != string(payload) {
		t.Fatalf("unexpected kafka event: %+v", ev)
	}
	if len(ob.markedID) != 1 || ob.markedID[0] != 42 {
		t.Fatalf("expected mark published id 42, got %#v", ob.markedID)
	}
}

func TestOutboxRelay_tick_localFallbackCallsBroadcaster(t *testing.T) {
	t.Parallel()
	chat := "30000000-0000-4000-8000-000000000003"
	payload, _ := json.Marshal(map[string]string{"ok": "1"})
	row := repository.OutboxRow{
		ID:            7,
		EventType:     domain.EventMessageUpdated,
		AggregateType: "message",
		AggregateID:   "m1",
		ChatID:        &chat,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		Payload:       payload,
	}
	del := &fakeDeliver{}
	ob := &fakeOutboxRepo{rows: []repository.OutboxRow{row}}
	r := &OutboxRelay{
		Outbox:   ob,
		UseKafka: false,
		Fallback: del,
	}
	r.tick(context.Background())
	if del.calls != 1 {
		t.Fatalf("expected deliver call, got %d", del.calls)
	}
	if len(del.chats) != 1 || string(del.chats[0]) != chat {
		t.Fatalf("unexpected chat id: %#v", del.chats)
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(del.envs[0], &outer); err != nil {
		t.Fatal(err)
	}
	if string(outer["event"]) != `"`+domain.EventMessageUpdated+`"` {
		t.Fatalf("envelope event: %s", outer["event"])
	}
	if len(ob.markedID) != 1 || ob.markedID[0] != 7 {
		t.Fatalf("expected published stamp, got %#v", ob.markedID)
	}
}
