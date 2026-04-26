package kafka

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
)

// Producer publishes domain events to Kafka.
type Producer struct {
	w *kafka.Writer
}

func NewProducer(brokers []string, topic string) *Producer {
	if len(brokers) == 0 || topic == "" {
		return nil
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
		BatchTimeout: 10 * time.Millisecond,
	}
	return &Producer{w: w}
}

func (p *Producer) Close() error {
	if p == nil || p.w == nil {
		return nil
	}
	return p.w.Close()
}

func (p *Producer) Publish(ctx context.Context, key string, value []byte) error {
	if p == nil || p.w == nil {
		return nil
	}
	msg := kafka.Message{
		Key:   []byte(key),
		Value: value,
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return p.w.WriteMessages(ctx, msg)
}
