package app

import (
	"strings"
	"testing"
)

func TestKafkaWSFanoutConsumerGroup_uniquePerCall(t *testing.T) {
	t.Parallel()
	a := kafkaWSFanoutConsumerGroup("my-base")
	b := kafkaWSFanoutConsumerGroup("my-base")
	if a == b {
		t.Fatal("expected distinct group ids")
	}
	if !strings.HasPrefix(a, "my-base-") {
		t.Fatalf("unexpected prefix: %q", a)
	}
}

func TestKafkaWSFanoutConsumerGroup_defaultBase(t *testing.T) {
	t.Parallel()
	g := kafkaWSFanoutConsumerGroup("")
	if !strings.HasPrefix(g, "goflow-ws-fanout-") {
		t.Fatalf("unexpected default: %q", g)
	}
}
