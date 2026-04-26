package domain

// Outbound / Kafka domain event types for the messaging aggregate.
const (
	EventMessageCreated    = "message.created"
	EventMessageUpdated    = "message.updated"
	EventMessageDeleted    = "message.deleted"
	EventMessageReadReceipt  = "message.read_receipt"
	AggregateTypeMessage    = "message"
	AggregateTypeReadState  = "read_state"
)
