package sqlite

import (
	"context"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/orchestration"
)

func (s *Store) PutMessage(ctx context.Context, message orchestration.AgentMessage, maxMessages int, maxBytes int64) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return false, err }
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM orchestration_messages WHERE message_id = ?`, message.ID.String()).Scan(&exists); err != nil { return false, err }
	if exists > 0 { return false, nil }

	var count int
	var bytes int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(payload_bytes), 0) FROM orchestration_messages WHERE to_agent_id = ? AND consumed_at IS NULL`, message.To.String()).Scan(&count, &bytes); err != nil { return false, err }
	size := int64(len(message.Inline) + len(message.PayloadRef))
	if count >= maxMessages || bytes+size > maxBytes { return false, errs.New(errs.CodeResourceExhausted, "sqlite.orchestration.message", "mailbox backpressure") }

	_, err = tx.ExecContext(ctx, `
INSERT INTO orchestration_messages
(message_id, from_agent_id, to_agent_id, message_type, payload_ref, inline_payload, correlation_id, payload_bytes, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		message.ID.String(), message.From.String(), message.To.String(), string(message.Type), string(message.PayloadRef), message.Inline,
		message.CorrelationID.String(), size, formatTime(message.CreatedAt),
	)
	if err != nil { return false, err }
	if err := tx.Commit(); err != nil { return false, err }
	return true, nil
}

func (s *Store) Mailbox(ctx context.Context, agentID id.AgentID, limit int) ([]orchestration.AgentMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT message_id, from_agent_id, to_agent_id, message_type, COALESCE(payload_ref, ''), COALESCE(inline_payload, ''), COALESCE(correlation_id, ''), created_at
FROM orchestration_messages
WHERE to_agent_id = ? AND consumed_at IS NULL
ORDER BY CASE message_type WHEN 'result' THEN 0 WHEN 'cancel_request' THEN 1 ELSE 2 END, created_at, message_id
LIMIT ?`, agentID.String(), limit)
	if err != nil { return nil, err }
	defer rows.Close()
	var result []orchestration.AgentMessage
	for rows.Next() {
		var message orchestration.AgentMessage
		var messageID, fromID, toID, typ, payloadRef, correlationID, createdAt string
		if err := rows.Scan(&messageID, &fromID, &toID, &typ, &payloadRef, &message.Inline, &correlationID, &createdAt); err != nil { return nil, err }
		message.ID = id.MessageID(messageID)
		message.From = id.AgentID(fromID)
		message.To = id.AgentID(toID)
		message.Type = orchestration.MessageType(typ)
		message.PayloadRef = objectstore.Ref(payloadRef)
		message.CorrelationID = id.CorrelationID(correlationID)
		message.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil { return nil, err }
		result = append(result, message)
	}
	return result, rows.Err()
}

func (s *Store) ConsumeMessage(ctx context.Context, agentID id.AgentID, messageID id.MessageID, at time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE orchestration_messages SET consumed_at = ? WHERE message_id = ? AND to_agent_id = ? AND consumed_at IS NULL`, formatTime(at), messageID.String(), agentID.String())
	if err != nil { return false, err }
	rows, err := result.RowsAffected()
	if err != nil { return false, err }
	return rows == 1, nil
}
