package kafka

import (
	"encoding/json"
	"time"
)

// DomainEvent is the canonical Kafka payload for goflow domain notifications.
type DomainEvent struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	ChatID        string          `json:"chat_id,omitempty"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Version       int             `json:"version"`
	Payload       json.RawMessage `json:"payload"`
}

func (e DomainEvent) Marshal() ([]byte, error) {
	return json.Marshal(e)
}
