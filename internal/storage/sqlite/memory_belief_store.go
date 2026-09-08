package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/memory"
)

const memoryBeliefColumns = `
    belief_id, proposition, fingerprint, scope_kind, scope_ref, status,
    origin_provenance, confidence_json, valid_from, valid_until, as_of_ref,
    created_at, updated_at, last_reviewed_at, version, speculative, origin_fork_id`

func (s *Store) CreateBelief(ctx context.Context, belief memory.Belief, links []memory.EvidenceLink, edges []memory.BeliefEdge) error {
	confidence, err := json.Marshal(belief.Confidence)
	if err != nil {
		return errs.Wrap(errs.CodeInvalidArgument, "sqlite.memory.belief.create", "encode confidence", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.create", "begin belief transaction", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO memory_beliefs (
    belief_id, proposition, fingerprint, scope_kind, scope_ref, status,
    origin_provenance, confidence_json, valid_from, valid_until, as_of_ref,
    created_at, updated_at, last_reviewed_at, version, speculative, origin_fork_id, search_text
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		belief.ID.String(), belief.Proposition, belief.Fingerprint, string(belief.Scope.Kind), belief.Scope.Ref,
		string(belief.Status), string(belief.OriginProvenance), string(confidence),
		formatOptionalTime(belief.Validity.ValidFrom), formatOptionalTime(belief.Validity.ValidUntil), belief.Validity.AsOfRef,
		formatTime(belief.CreatedAt), formatTime(belief.UpdatedAt), formatOptionalTime(belief.LastReviewedAt),
		belief.Version, boolInt(belief.Speculative), belief.OriginForkID.String(), belief.Proposition,
	)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.create", "insert belief", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.memory.belief.create", "read belief insert result", err)
	}
	if changed != 1 {
		return memory.ErrConflict
	}
	for _, link := range links {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO memory_belief_evidence (belief_id, evidence_id, relation)
VALUES (?, ?, ?)`, link.BeliefID.String(), link.EvidenceID.String(), string(link.Relation)); err != nil {
			return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.create", "insert evidence link", err)
		}
	}
	for _, edge := range edges {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO memory_belief_edges (from_belief_id, to_belief_id, relation, created_at)
VALUES (?, ?, ?, ?)`, edge.From.String(), edge.To.String(), string(edge.Relation), formatTime(edge.CreatedAt)); err != nil {
			return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.create", "insert belief edge", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO memory_belief_status_history (belief_id, from_status, to_status, reason, job_id, changed_at)
VALUES (?, '', ?, 'created', '', ?)`, belief.ID.String(), string(belief.Status), formatTime(belief.CreatedAt)); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.create", "insert initial status history", err)
	}
	if err := tx.Commit(); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.create", "commit belief transaction", err)
	}
	return nil
}

func (s *Store) Belief(ctx context.Context, beliefID id.BeliefID) (memory.Belief, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+memoryBeliefColumns+` FROM memory_beliefs WHERE belief_id = ?`, beliefID.String())
	value, err := scanMemoryBelief(row)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Belief{}, memory.ErrNotFound
	}
	if err != nil {
		return memory.Belief{}, errs.Wrap(errs.CodeCorruption, "sqlite.memory.belief.get", "scan belief", err)
	}
	return value, nil
}

func (s *Store) BeliefEvidence(ctx context.Context, beliefID id.BeliefID) ([]memory.EvidenceLink, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT belief_id, evidence_id, relation
FROM memory_belief_evidence
WHERE belief_id = ?
ORDER BY evidence_id, relation`, beliefID.String())
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.evidence", "query belief evidence", err)
	}
	defer rows.Close()
	out := make([]memory.EvidenceLink, 0)
	for rows.Next() {
		var belief, evidence, relation string
		if err := rows.Scan(&belief, &evidence, &relation); err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.belief.evidence", "scan evidence link", err)
		}
		out = append(out, memory.EvidenceLink{BeliefID: id.BeliefID(belief), EvidenceID: id.EvidenceID(evidence), Relation: memory.EvidenceRelation(relation)})
	}
	return out, rows.Err()
}

func (s *Store) EvidenceDependents(ctx context.Context, evidenceID id.EvidenceID) ([]memory.EvidenceLink, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT belief_id, evidence_id, relation
FROM memory_belief_evidence
WHERE evidence_id = ?
ORDER BY belief_id, relation`, evidenceID.String())
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.evidence.dependents", "query evidence dependents", err)
	}
	defer rows.Close()
	out := make([]memory.EvidenceLink, 0)
	for rows.Next() {
		var belief, evidence, relation string
		if err := rows.Scan(&belief, &evidence, &relation); err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.evidence.dependents", "scan evidence dependent", err)
		}
		out = append(out, memory.EvidenceLink{BeliefID: id.BeliefID(belief), EvidenceID: id.EvidenceID(evidence), Relation: memory.EvidenceRelation(relation)})
	}
	return out, rows.Err()
}

func (s *Store) BeliefIncoming(ctx context.Context, beliefID id.BeliefID) ([]memory.BeliefEdge, error) {
	return s.memoryBeliefEdges(ctx, "to_belief_id", beliefID)
}

func (s *Store) BeliefOutgoing(ctx context.Context, beliefID id.BeliefID) ([]memory.BeliefEdge, error) {
	return s.memoryBeliefEdges(ctx, "from_belief_id", beliefID)
}

func (s *Store) memoryBeliefEdges(ctx context.Context, column string, beliefID id.BeliefID) ([]memory.BeliefEdge, error) {
	if column != "to_belief_id" && column != "from_belief_id" {
		return nil, errs.New(errs.CodeInternal, "sqlite.memory.belief.edges", "invalid edge column")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT from_belief_id, to_belief_id, relation, created_at
FROM memory_belief_edges WHERE `+column+` = ?
ORDER BY relation, from_belief_id, to_belief_id`, beliefID.String())
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.edges", "query belief edges", err)
	}
	defer rows.Close()
	out := make([]memory.BeliefEdge, 0)
	for rows.Next() {
		var from, to, relation, created string
		if err := rows.Scan(&from, &to, &relation, &created); err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.belief.edges", "scan belief edge", err)
		}
		at, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.belief.edges", "parse edge timestamp", err)
		}
		out = append(out, memory.BeliefEdge{From: id.BeliefID(from), To: id.BeliefID(to), Relation: memory.BeliefRelation(relation), CreatedAt: at})
	}
	return out, rows.Err()
}

func (s *Store) PutBeliefEdge(ctx context.Context, edge memory.BeliefEdge) error {
	result, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO memory_belief_edges (from_belief_id, to_belief_id, relation, created_at)
VALUES (?, ?, ?, ?)`, edge.From.String(), edge.To.String(), string(edge.Relation), formatTime(edge.CreatedAt))
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.edge_put", "insert belief edge", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.memory.belief.edge_put", "read edge insert result", err)
	}
	if changed != 1 {
		return memory.ErrConflict
	}
	return nil
}

func (s *Store) TransitionBelief(ctx context.Context, beliefID id.BeliefID, from, to memory.BeliefStatus, profile memory.ConfidenceProfile, reviewedAt *time.Time, reason string, jobID id.PropagationID, at time.Time) (memory.Belief, error) {
	confidence, err := json.Marshal(profile)
	if err != nil {
		return memory.Belief{}, errs.Wrap(errs.CodeInvalidArgument, "sqlite.memory.belief.transition", "encode confidence", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return memory.Belief{}, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.transition", "begin transition", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
UPDATE memory_beliefs
SET status = ?, confidence_json = ?, updated_at = ?, last_reviewed_at = COALESCE(?, last_reviewed_at), version = version + 1
WHERE belief_id = ? AND status = ?`,
		string(to), string(confidence), formatTime(at), formatOptionalTime(reviewedAt), beliefID.String(), string(from))
	if err != nil {
		return memory.Belief{}, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.transition", "update belief", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return memory.Belief{}, errs.Wrap(errs.CodeInternal, "sqlite.memory.belief.transition", "read transition result", err)
	}
	if changed != 1 {
		return memory.Belief{}, memory.ErrConflict
	}
	if from != to {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO memory_belief_status_history (belief_id, from_status, to_status, reason, job_id, changed_at)
VALUES (?, ?, ?, ?, ?, ?)`, beliefID.String(), string(from), string(to), reason, jobID.String(), formatTime(at)); err != nil {
			return memory.Belief{}, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.transition", "insert status history", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return memory.Belief{}, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.transition", "commit transition", err)
	}
	return s.Belief(ctx, beliefID)
}

func (s *Store) SearchBeliefs(ctx context.Context, query memory.SearchQuery) ([]memory.SearchCandidate, error) {
	where, args := memoryBeliefFilters(query)
	terms := ftsTerms(query.Text)
	withRank := len(terms) > 0
	var rows *sql.Rows
	var err error
	if withRank {
		args = append([]any{strings.Join(terms, " OR ")}, args...)
		args = append(args, query.Limit)
		rows, err = s.db.QueryContext(ctx, `
SELECT `+prefixedMemoryBeliefColumns("b")+`, bm25(memory_beliefs_fts)
FROM memory_beliefs_fts
JOIN memory_beliefs b ON b.rowid = memory_beliefs_fts.rowid
WHERE memory_beliefs_fts MATCH ? AND `+where+`
ORDER BY bm25(memory_beliefs_fts), b.updated_at DESC, b.belief_id
LIMIT ?`, args...)
	} else {
		args = append(args, query.Limit)
		rows, err = s.db.QueryContext(ctx, `
SELECT `+prefixedMemoryBeliefColumns("b")+`
FROM memory_beliefs b
WHERE `+where+`
ORDER BY b.updated_at DESC, b.belief_id
LIMIT ?`, args...)
	}
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.search", "search beliefs", err)
	}
	defer rows.Close()
	out := make([]memory.SearchCandidate, 0)
	for rows.Next() {
		if withRank {
			belief, rank, scanErr := scanMemoryBeliefRank(rows, true)
			if scanErr != nil { return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.belief.search", "scan ranked belief", scanErr) }
			out = append(out, memory.SearchCandidate{Belief: belief, LexicalScore: normalizeBM25(rank)})
		} else {
			belief, _, scanErr := scanMemoryBeliefRank(rows, false)
			if scanErr != nil { return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.belief.search", "scan belief", scanErr) }
			out = append(out, memory.SearchCandidate{Belief: belief})
		}
	}
	return out, rows.Err()
}

func (s *Store) StatusHistory(ctx context.Context, beliefID id.BeliefID) ([]memory.StatusTransition, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT belief_id, from_status, to_status, reason, job_id, changed_at
FROM memory_belief_status_history
WHERE belief_id = ? ORDER BY seq`, beliefID.String())
	if err != nil { return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.history", "query status history", err) }
	defer rows.Close()
	out := make([]memory.StatusTransition, 0)
	for rows.Next() {
		var belief, from, to, job, changed string
		var value memory.StatusTransition
		if err := rows.Scan(&belief, &from, &to, &value.Reason, &job, &changed); err != nil { return nil, err }
		value.BeliefID = id.BeliefID(belief); value.From = memory.BeliefStatus(from); value.To = memory.BeliefStatus(to); value.JobID = id.PropagationID(job)
		value.ChangedAt, err = time.Parse(time.RFC3339Nano, changed); if err != nil { return nil, err }
		out = append(out, value)
	}
	return out, rows.Err()
}

func (s *Store) RecordBeliefUse(ctx context.Context, value memory.UsageRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO memory_belief_uses (belief_id, agent_id, invocation_id, context_page_id, purpose, used_at)
VALUES (?, ?, ?, ?, ?, ?)`, value.BeliefID.String(), value.AgentID.String(), value.InvocationID.String(), value.ContextPageID.String(), value.Purpose, formatTime(value.UsedAt))
	if err != nil { return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.use", "insert belief use", err) }
	return nil
}

func (s *Store) BeliefUses(ctx context.Context, beliefID id.BeliefID, limit int) ([]memory.UsageRecord, error) {
	if limit <= 0 { limit = 256 }
	rows, err := s.db.QueryContext(ctx, `
SELECT belief_id, agent_id, invocation_id, context_page_id, purpose, used_at
FROM memory_belief_uses WHERE belief_id = ? ORDER BY used_at DESC, seq DESC LIMIT ?`, beliefID.String(), limit)
	if err != nil { return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.belief.uses", "query belief uses", err) }
	defer rows.Close()
	out := make([]memory.UsageRecord, 0)
	for rows.Next() {
		var belief, agent, invocation, page, used string
		var value memory.UsageRecord
		if err := rows.Scan(&belief, &agent, &invocation, &page, &value.Purpose, &used); err != nil { return nil, err }
		value.BeliefID = id.BeliefID(belief); value.AgentID = id.AgentID(agent); value.InvocationID = id.InvocationID(invocation); value.ContextPageID = id.ContextPageID(page)
		value.UsedAt, err = time.Parse(time.RFC3339Nano, used); if err != nil { return nil, err }
		out = append(out, value)
	}
	return out, rows.Err()
}

func memoryBeliefFilters(query memory.SearchQuery) (string, []any) {
	parts := []string{"1 = 1"}
	args := make([]any, 0)
	if len(query.Scopes) > 0 {
		scopeParts := make([]string, 0, len(query.Scopes))
		for _, scope := range query.Scopes {
			if scope.Ref == "" { scopeParts = append(scopeParts, "b.scope_kind = ?"); args = append(args, string(scope.Kind)) } else { scopeParts = append(scopeParts, "(b.scope_kind = ? AND b.scope_ref = ?)"); args = append(args, string(scope.Kind), scope.Ref) }
		}
		parts = append(parts, "("+strings.Join(scopeParts, " OR ")+")")
	}
	if len(query.Statuses) > 0 {
		parts = append(parts, "b.status IN ("+questionMarks(len(query.Statuses))+")")
		for _, status := range query.Statuses { args = append(args, string(status)) }
	} else if !query.IncludeHistorical {
		parts = append(parts, "b.status NOT IN ('Invalidated','Superseded','Rejected')")
	}
	if !query.IncludeSpeculative { parts = append(parts, "b.speculative = 0") }
	if query.AsOf != nil { parts = append(parts, "b.created_at <= ?"); args = append(args, formatTime(query.AsOf.UTC())) }
	return strings.Join(parts, " AND "), args
}

func prefixedMemoryBeliefColumns(prefix string) string {
	columns := []string{"belief_id", "proposition", "fingerprint", "scope_kind", "scope_ref", "status", "origin_provenance", "confidence_json", "valid_from", "valid_until", "as_of_ref", "created_at", "updated_at", "last_reviewed_at", "version", "speculative", "origin_fork_id"}
	for i := range columns { columns[i] = prefix + "." + columns[i] }
	return strings.Join(columns, ", ")
}

func scanMemoryBelief(scanner rowScanner) (memory.Belief, error) {
	value, _, err := scanMemoryBeliefRank(scanner, false)
	return value, err
}

func scanMemoryBeliefRank(scanner rowScanner, withRank bool) (memory.Belief, float64, error) {
	var value memory.Belief
	var beliefID, scopeKind, status, origin, confidence, created, updated, originFork string
	var validFrom, validUntil, reviewed sql.NullString
	var speculative int
	var rank float64
	dest := []any{&beliefID, &value.Proposition, &value.Fingerprint, &scopeKind, &value.Scope.Ref, &status, &origin, &confidence, &validFrom, &validUntil, &value.Validity.AsOfRef, &created, &updated, &reviewed, &value.Version, &speculative, &originFork}
	if withRank { dest = append(dest, &rank) }
	if err := scanner.Scan(dest...); err != nil { return memory.Belief{}, 0, err }
	value.ID = id.BeliefID(beliefID); value.Scope.Kind = memory.ScopeKind(scopeKind); value.Status = memory.BeliefStatus(status); value.OriginProvenance = memory.ProvenanceClass(origin); value.Speculative = speculative != 0; value.OriginForkID = id.ForkID(originFork)
	if err := json.Unmarshal([]byte(confidence), &value.Confidence); err != nil { return memory.Belief{}, 0, err }
	var err error
	value.CreatedAt, err = time.Parse(time.RFC3339Nano, created); if err != nil { return memory.Belief{}, 0, err }
	value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); if err != nil { return memory.Belief{}, 0, err }
	if validFrom.Valid { t, parseErr := time.Parse(time.RFC3339Nano, validFrom.String); if parseErr != nil { return memory.Belief{}, 0, parseErr }; value.Validity.ValidFrom = &t }
	if validUntil.Valid { t, parseErr := time.Parse(time.RFC3339Nano, validUntil.String); if parseErr != nil { return memory.Belief{}, 0, parseErr }; value.Validity.ValidUntil = &t }
	if reviewed.Valid { t, parseErr := time.Parse(time.RFC3339Nano, reviewed.String); if parseErr != nil { return memory.Belief{}, 0, parseErr }; value.LastReviewedAt = &t }
	return value, rank, nil
}
