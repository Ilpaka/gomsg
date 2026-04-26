-- Outbox for reliable domain event publishing (Kafka relay).

CREATE TABLE IF NOT EXISTS outbox_events (
    id bigserial PRIMARY KEY,
    event_id uuid NOT NULL UNIQUE,
    event_type text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    chat_id uuid,
    occurred_at timestamptz NOT NULL,
    version int NOT NULL DEFAULT 1,
    payload jsonb NOT NULL,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox_events (id ASC)
WHERE published_at IS NULL;
