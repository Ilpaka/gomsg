package ws

import (
	"encoding/json"
)

// Inbound / outbound event names.
const (
	EventPing           = "ping"
	EventPong           = "pong"
	EventMessageSend    = "message.send"
	EventMessageRead    = "message.read"
	EventTypingStart    = "typing.start"
	EventTypingStop     = "typing.stop"
	EventMessageCreated = "message.created"
	EventMessageUpdated = "message.updated"
	EventMessageDeleted = "message.deleted"
	EventTypingStarted  = "typing.started"
	EventTypingStopped  = "typing.stopped"
	EventError          = "error"
)

// Envelope is the wire format for both directions.
type Envelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
	Meta  json.RawMessage `json:"meta,omitempty"`
}

// MarshalEnvelope builds JSON: { "event", "data", "meta" } with meta defaulting to {}.
func MarshalEnvelope(event string, data any, meta any) ([]byte, error) {
	var dataRaw json.RawMessage
	if data == nil {
		dataRaw = json.RawMessage(`{}`)
	} else {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		dataRaw = json.RawMessage(b)
	}
	var metaRaw json.RawMessage
	if meta == nil {
		metaRaw = json.RawMessage(`{}`)
	} else {
		b, err := json.Marshal(meta)
		if err != nil {
			return nil, err
		}
		metaRaw = json.RawMessage(b)
	}
	return json.Marshal(Envelope{
		Event: event,
		Data:  dataRaw,
		Meta:  metaRaw,
	})
}
