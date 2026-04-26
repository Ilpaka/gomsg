package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/kafka"
	"goflow/backend/internal/observability/metrics"
	wstransport "goflow/backend/internal/transport/ws"

	kafkago "github.com/segmentio/kafka-go"
)

// KafkaWSConsumer reads domain events from Kafka and fans them out to the local WebSocket hub.
type KafkaWSConsumer struct {
	Brokers []string
	Topic   string
	GroupID string
	BC      *wstransport.Broadcaster
	Log     *slog.Logger
	Metrics *metrics.M
}

func (c *KafkaWSConsumer) Run(ctx context.Context) error {
	if c == nil || len(c.Brokers) == 0 || c.Topic == "" || c.BC == nil {
		return nil
	}
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     c.Brokers,
		GroupID:     c.GroupID,
		Topic:       c.Topic,
		// New consumer groups start at the oldest available offset so dev/relay-delivered
		// events are not skipped before the reader attaches (tune for prod if replay is undesired).
		StartOffset: kafkago.FirstOffset,
		MaxWait:     2 * time.Second,
	})
	defer func() { _ = r.Close() }()

	for {
		msg, err := r.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if c.Log != nil {
				c.Log.Warn("kafka read", "err", err)
			}
			time.Sleep(300 * time.Millisecond)
			continue
		}
		fanoutErr := FanoutKafkaDomainEvent(ctx, c.BC, msg.Value, c.Log)
		if c.Metrics != nil {
			if fanoutErr != nil {
				c.Metrics.KafkaFail.Inc()
			} else {
				c.Metrics.KafkaHandled.Inc()
			}
		}
		if fanoutErr != nil {
			continue
		}
	}
}

// FanoutKafkaDomainEvent unmarshals a kafka-go message value and delivers the WS envelope locally.
func FanoutKafkaDomainEvent(ctx context.Context, bc ChatEnvelopeDeliverer, raw []byte, log *slog.Logger) error {
	if bc == nil {
		return nil
	}
	var ev kafka.DomainEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		if log != nil {
			log.Warn("kafka decode", "err", err)
		}
		return err
	}
	if ev.ChatID == "" {
		return nil
	}
	var data any
	if len(ev.Payload) > 0 {
		if err := json.Unmarshal(ev.Payload, &data); err != nil {
			if log != nil {
				log.Warn("kafka payload", "err", err)
			}
			return err
		}
	} else {
		data = map[string]any{}
	}
	env, err := wstransport.MarshalEnvelope(ev.EventType, data, map[string]any{})
	if err != nil {
		if log != nil {
			log.Warn("kafka envelope", "err", err)
		}
		return err
	}
	if err := bc.DeliverEnvelopeBytes(ctx, domain.ID(ev.ChatID), env); err != nil && log != nil {
		log.Warn("kafka fanout", "err", err)
	}
	return err
}
