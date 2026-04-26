package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/kafka"
	"goflow/backend/internal/observability/metrics"
	"goflow/backend/internal/repository"
	wstransport "goflow/backend/internal/transport/ws"
)

// ChatEnvelopeDeliverer is implemented by *ws.Broadcaster for local WS fan-out.
type ChatEnvelopeDeliverer interface {
	DeliverEnvelopeBytes(ctx context.Context, chatID domain.ID, payload []byte) error
}

// KafkaPublisher sends serialized domain events to Kafka.
type KafkaPublisher interface {
	Publish(ctx context.Context, key string, value []byte) error
}

// OutboxRelay polls outbox rows and publishes them (Kafka or local fanout).
type OutboxRelay struct {
	Outbox   repository.OutboxRepository
	Kafka    KafkaPublisher
	Fallback ChatEnvelopeDeliverer
	Log      *slog.Logger
	UseKafka bool
	Metrics  *metrics.M
}

func (r *OutboxRelay) Run(ctx context.Context) {
	if r == nil || r.Outbox == nil {
		return
	}
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.tick(ctx)
		}
	}
}

func (r *OutboxRelay) tick(ctx context.Context) {
	if r.Metrics != nil {
		r.Metrics.RelayIter.Inc()
	}
	rows, err := r.Outbox.FetchPending(ctx, 100)
	if err != nil {
		if r.Log != nil {
			r.Log.Warn("outbox fetch", "err", err)
		}
		return
	}
	for i := range rows {
		row := rows[i]
		ev := kafka.DomainEvent{
			EventID:       row.EventID,
			EventType:     row.EventType,
			AggregateType: row.AggregateType,
			AggregateID:   row.AggregateID,
			OccurredAt:    row.OccurredAt.UTC(),
			Version:       row.Version,
			Payload:       row.Payload,
		}
		if row.ChatID != nil {
			ev.ChatID = *row.ChatID
		}
		b, err := ev.Marshal()
		if err != nil {
			if r.Log != nil {
				r.Log.Error("outbox marshal", "err", err, "id", row.ID)
			}
			continue
		}
		key := ev.ChatID
		if key == "" {
			key = ev.AggregateID
		}

		var pubErr error
		if r.UseKafka && r.Kafka != nil {
			pubErr = r.Kafka.Publish(ctx, key, b)
		} else if r.Fallback != nil && row.ChatID != nil {
			pubErr = r.publishLocal(ctx, row.EventType, row.Payload, domain.ID(*row.ChatID))
		}

		if pubErr != nil {
			if r.Metrics != nil {
				r.Metrics.RelayPubFail.Inc()
			}
			if r.Log != nil {
				r.Log.Warn("outbox publish", "err", pubErr, "outbox_id", row.ID)
			}
			continue
		}
		if r.Metrics != nil {
			r.Metrics.RelayPubOK.Inc()
		}

		if err := r.Outbox.MarkPublished(ctx, row.ID); err != nil && r.Log != nil {
			r.Log.Error("outbox mark published", "err", err, "id", row.ID)
		}
	}
}

func (r *OutboxRelay) publishLocal(ctx context.Context, eventType string, payload []byte, chatID domain.ID) error {
	var data any
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &data); err != nil {
			return err
		}
	} else {
		data = map[string]any{}
	}
	env, err := wstransport.MarshalEnvelope(eventType, data, map[string]any{})
	if err != nil {
		return err
	}
	return r.Fallback.DeliverEnvelopeBytes(ctx, chatID, env)
}
