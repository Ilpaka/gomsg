package app

import (
	"strings"

	"github.com/google/uuid"
)

// kafkaWSFanoutConsumerGroup builds a unique Kafka consumer group for this process.
//
// For WebSocket multi-instance fan-out, every app replica must read all messages from the
// domain-events topic and push them to its local Hub. A single shared consumer group would
// partition the topic across members, so each instance would only see a subset of events.
// Independent consumer groups (base + UUID per process) give each instance a full copy of
// the stream (at the cost of extra consumer groups and offsets in the broker).
func kafkaWSFanoutConsumerGroup(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "goflow-ws-fanout"
	}
	return base + "-" + uuid.NewString()
}
