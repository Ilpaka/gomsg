package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"goflow/backend/internal/repository"
)

// OutboxRepository reads pending outbox rows for the Kafka / local relay.
type OutboxRepository struct {
	pool *pgxpool.Pool
}

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{pool: pool}
}

var _ repository.OutboxRepository = (*OutboxRepository)(nil)

func (r *OutboxRepository) FetchPending(ctx context.Context, limit int) ([]repository.OutboxRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	const q = `
SELECT id, event_id::text, event_type, aggregate_type, aggregate_id, chat_id::text, occurred_at, version, payload
FROM outbox_events
WHERE published_at IS NULL
ORDER BY id ASC
LIMIT $1`

	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []repository.OutboxRow
	for rows.Next() {
		var row repository.OutboxRow
		var chatID *string
		if err := rows.Scan(
			&row.ID,
			&row.EventID,
			&row.EventType,
			&row.AggregateType,
			&row.AggregateID,
			&chatID,
			&row.OccurredAt,
			&row.Version,
			&row.Payload,
		); err != nil {
			return nil, err
		}
		row.ChatID = chatID
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, id int64) error {
	const q = `UPDATE outbox_events SET published_at = now() WHERE id = $1 AND published_at IS NULL`
	tag, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("outbox: mark published: no row for id %d", id)
	}
	return nil
}
