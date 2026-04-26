package metrics

import (
	"encoding/json"
	"strings"
)

var wsEventAllowlist = map[string]struct{}{
	"ping": {}, "pong": {},
	"message.send": {}, "message.read": {},
	"typing.start": {}, "typing.stop": {},
	"typing.started": {}, "typing.stopped": {},
	"error": {},
	"message.created": {}, "message.updated": {}, "message.deleted": {}, "message.read_receipt": {},
}

// SanitizeWSEvent maps unknown event names to "other" to avoid label cardinality explosion.
func SanitizeWSEvent(event string) string {
	e := strings.TrimSpace(strings.ToLower(event))
	if e == "" {
		return "other"
	}
	if _, ok := wsEventAllowlist[e]; ok {
		return e
	}
	return "other"
}

// EnvelopeEventName extracts the envelope "event" field for metrics; returns "other" if missing.
func EnvelopeEventName(payload []byte) string {
	var partial struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(payload, &partial); err != nil || partial.Event == "" {
		return "other"
	}
	return SanitizeWSEvent(partial.Event)
}
