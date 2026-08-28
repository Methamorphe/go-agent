package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/mmu"
	"github.com/Methamorphe/go-agent/internal/objectstore"
)

const contextPageColumns = `
    page_id,
    agent_id,
    page_type,
    scope,
    source_ref,
    object_ref,
    token_estimate,
    importance,
    confidence,
    created_at,
    last_accessed_at,
    pinned_until,
    superseded_by,
    compacted_by,
    summary_of_json`

func (s *Store) PutContextPage(ctx context.Context, meta mmu.PageMeta, searchText string) error {
	summary, err := json.Marshal(meta.SummaryOf)
	if err != nil {
		return errs.Wrap(errs.CodeInvalidArgument, "sqlite.context_page.put", "encode summary references", err)
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO context_pages (
    page_id, agent_id, page_type, scope, source_ref, object_ref,
    token_estimate, importance, confidence, created_at, last_accessed_at,
    pinned_until, superseded_by, compacted_by, summary_of_json, search_text
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		meta.ID.String(), meta.AgentID.String(), string(meta.Type), string(meta.Scope), meta.SourceRef,
		string(meta.ObjectRef), meta.TokenEstimate, meta.Importance, meta.Confidence,
		formatTime(meta.CreatedAt), formatTime(meta.LastAccessedAt), formatOptionalTime(meta.PinnedUntil),
		optionalPageID(meta.SupersededBy), optionalPageID(meta.CompactedBy), string(summary), searchText,
	)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_page.put", "insert context page", err)
	}
	return nil
}

func (s *Store) ContextPage(ctx context.Context, agentID id.AgentID, pageID id.ContextPageID) (mmu.PageMeta, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+contextPageColumns+` FROM context_pages WHERE agent_id = ? AND page_id = ?`, agentID.String(), pageID.String())
	meta, err := scanContextPage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return mmu.PageMeta{}, errs.Wrap(errs.CodeNotFound, "sqlite.context_page.get", "context page not found", err)
	}
	if err != nil {
		return mmu.PageMeta{}, errs.Wrap(errs.CodeUnavailable, "sqlite.context_page.get", "query context page", err)
	}
	return meta, nil
}

func (s *Store) SearchContextPages(ctx context.Context, query mmu.CandidateQuery) ([]mmu.Candidate, error) {
	if query.Limit <= 0 {
		return nil, nil
	}
	where, args := contextFilters(query.AgentID, query.Scopes, query.Types)
	terms := ftsTerms(query.Text)

	var rows *sql.Rows
	var err error
	withRank := len(terms) > 0
	if withRank {
		args = append([]any{strings.Join(terms, " OR ")}, args...)
		args = append(args, query.Limit)
		rows, err = s.db.QueryContext(ctx, `
SELECT `+prefixedContextColumns("p")+`, bm25(context_pages_fts)
FROM context_pages_fts
JOIN context_pages p ON p.rowid = context_pages_fts.rowid
WHERE context_pages_fts MATCH ? AND `+where+`
  AND p.superseded_by IS NULL AND p.compacted_by IS NULL
ORDER BY bm25(context_pages_fts), p.last_accessed_at DESC, p.page_id
LIMIT ?`, args...)
	} else {
		args = append(args, query.Limit)
		rows, err = s.db.QueryContext(ctx, `
SELECT `+prefixedContextColumns("p")+`
FROM context_pages p
WHERE `+where+`
  AND p.superseded_by IS NULL AND p.compacted_by IS NULL
ORDER BY p.last_accessed_at DESC, p.page_id
LIMIT ?`, args...)
	}
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.context_page.search", "search context pages", err)
	}
	defer rows.Close()

	results := make([]mmu.Candidate, 0, query.Limit)
	for rows.Next() {
		if withRank {
			meta, rank, scanErr := scanContextPageWithRank(rows)
			if scanErr != nil {
				return nil, errs.Wrap(errs.CodeCorruption, "sqlite.context_page.search", "scan ranked context page", scanErr)
			}
			results = append(results, mmu.Candidate{Meta: meta, LexicalScore: 1 / (1 + math.Abs(rank))})
			continue
		}
		meta, scanErr := scanContextPage(rows)
		if scanErr != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.context_page.search", "scan context page", scanErr)
		}
		results = append(results, mmu.Candidate{Meta: meta})
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.context_page.search", "iterate context pages", err)
	}
	return results, nil
}

func (s *Store) TouchContextPages(ctx context.Context, agentID id.AgentID, pageIDs []id.ContextPageID, at time.Time) error {
	if len(pageIDs) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(pageIDs)), ",")
	args := make([]any, 0, len(pageIDs)+2)
	args = append(args, formatTime(at), agentID.String())
	for _, pageID := range pageIDs {
		args = append(args, pageID.String())
	}
	_, err := s.db.ExecContext(ctx, `UPDATE context_pages SET last_accessed_at = ? WHERE agent_id = ? AND page_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_page.touch", "touch context pages", err)
	}
	return nil
}

func (s *Store) MarkContextPagesCompacted(ctx context.Context, agentID id.AgentID, pageIDs []id.ContextPageID, summaryID id.ContextPageID) error {
	if len(pageIDs) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(pageIDs)), ",")
	args := make([]any, 0, len(pageIDs)+2)
	args = append(args, summaryID.String(), agentID.String())
	for _, pageID := range pageIDs {
		args = append(args, pageID.String())
	}
	_, err := s.db.ExecContext(ctx, `UPDATE context_pages SET compacted_by = ? WHERE agent_id = ? AND page_id IN (`+placeholders+`) AND superseded_by IS NULL`, args...)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_page.compact", "mark compacted context pages", err)
	}
	return nil
}

func (s *Store) PutContextLease(ctx context.Context, lease mmu.ContextLease) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO context_leases (lease_id, agent_id, page_id, reason, remaining_builds, expires_at, hard_pin, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(agent_id, page_id) DO UPDATE SET
    lease_id = excluded.lease_id,
    reason = excluded.reason,
    remaining_builds = MAX(context_leases.remaining_builds, excluded.remaining_builds),
    expires_at = excluded.expires_at,
    hard_pin = excluded.hard_pin,
    created_at = excluded.created_at`,
		lease.ID.String(), lease.AgentID.String(), lease.PageID.String(), lease.Reason, lease.RemainingBuilds,
		formatOptionalTime(lease.ExpiresAt), boolInt(lease.HardPin), formatTime(lease.CreatedAt),
	)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_lease.put", "upsert context lease", err)
	}
	return nil
}

func (s *Store) ActiveContextLeases(ctx context.Context, agentID id.AgentID, now time.Time) ([]mmu.ContextLease, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT lease_id, page_id, agent_id, reason, remaining_builds, expires_at, hard_pin, created_at
FROM context_leases
WHERE agent_id = ? AND remaining_builds > 0 AND (expires_at IS NULL OR expires_at > ?)
ORDER BY hard_pin DESC, created_at, page_id`, agentID.String(), formatTime(now))
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.context_lease.active", "query context leases", err)
	}
	defer rows.Close()

	leases := make([]mmu.ContextLease, 0)
	for rows.Next() {
		var lease mmu.ContextLease
		var leaseID, pageID, rawAgent, created string
		var expires sql.NullString
		var hard int
		if err := rows.Scan(&leaseID, &pageID, &rawAgent, &lease.Reason, &lease.RemainingBuilds, &expires, &hard, &created); err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.context_lease.active", "scan context lease", err)
		}
		lease.ID = id.LeaseID(leaseID)
		lease.PageID = id.ContextPageID(pageID)
		lease.AgentID = id.AgentID(rawAgent)
		lease.HardPin = hard != 0
		lease.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.context_lease.active", "parse lease timestamp", err)
		}
		if expires.Valid {
			value, parseErr := time.Parse(time.RFC3339Nano, expires.String)
			if parseErr != nil {
				return nil, errs.Wrap(errs.CodeCorruption, "sqlite.context_lease.active", "parse lease expiry", parseErr)
			}
			lease.ExpiresAt = &value
		}
		leases = append(leases, lease)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.context_lease.active", "iterate context leases", err)
	}
	return leases, nil
}

func (s *Store) ConsumeContextLeases(ctx context.Context, agentID id.AgentID, pageIDs []id.ContextPageID) error {
	if len(pageIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_lease.consume", "begin lease transaction", err)
	}
	defer tx.Rollback()
	for _, pageID := range pageIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE context_leases SET remaining_builds = remaining_builds - 1 WHERE agent_id = ? AND page_id = ? AND remaining_builds > 0`, agentID.String(), pageID.String()); err != nil {
			return errs.Wrap(errs.CodeUnavailable, "sqlite.context_lease.consume", "consume context lease", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM context_leases WHERE agent_id = ? AND remaining_builds <= 0`, agentID.String()); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_lease.consume", "remove exhausted context leases", err)
	}
	if err := tx.Commit(); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_lease.consume", "commit lease transaction", err)
	}
	return nil
}

func (s *Store) PutContextManifest(ctx context.Context, invocationID id.InvocationID, agentID id.AgentID, ref objectstore.Ref, createdAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO context_manifests (invocation_id, agent_id, object_ref, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(invocation_id) DO UPDATE SET object_ref = excluded.object_ref, created_at = excluded.created_at`,
		invocationID.String(), agentID.String(), string(ref), formatTime(createdAt),
	)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.context_manifest.put", "persist context manifest", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanContextPage(scanner rowScanner) (mmu.PageMeta, error) {
	meta, _, err := scanContextPageRank(scanner, false)
	return meta, err
}

func scanContextPageWithRank(scanner rowScanner) (mmu.PageMeta, float64, error) {
	return scanContextPageRank(scanner, true)
}

func scanContextPageRank(scanner rowScanner, withRank bool) (mmu.PageMeta, float64, error) {
	var meta mmu.PageMeta
	var pageID, agentID, pageType, scope, objectRef, created, accessed, summary string
	var pinned, superseded, compacted sql.NullString
	var rank float64
	dest := []any{&pageID, &agentID, &pageType, &scope, &meta.SourceRef, &objectRef, &meta.TokenEstimate, &meta.Importance, &meta.Confidence, &created, &accessed, &pinned, &superseded, &compacted, &summary}
	if withRank {
		dest = append(dest, &rank)
	}
	if err := scanner.Scan(dest...); err != nil {
		return mmu.PageMeta{}, 0, err
	}
	meta.ID = id.ContextPageID(pageID)
	meta.AgentID = id.AgentID(agentID)
	meta.Type = mmu.PageType(pageType)
	meta.Scope = mmu.Scope(scope)
	meta.ObjectRef = objectstore.Ref(objectRef)
	var err error
	meta.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return mmu.PageMeta{}, 0, err
	}
	meta.LastAccessedAt, err = time.Parse(time.RFC3339Nano, accessed)
	if err != nil {
		return mmu.PageMeta{}, 0, err
	}
	if pinned.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, pinned.String)
		if parseErr != nil {
			return mmu.PageMeta{}, 0, parseErr
		}
		meta.PinnedUntil = &value
	}
	if superseded.Valid {
		value := id.ContextPageID(superseded.String)
		meta.SupersededBy = &value
	}
	if compacted.Valid {
		value := id.ContextPageID(compacted.String)
		meta.CompactedBy = &value
	}
	if err := json.Unmarshal([]byte(summary), &meta.SummaryOf); err != nil {
		return mmu.PageMeta{}, 0, err
	}
	return meta, rank, nil
}

func contextFilters(agentID id.AgentID, scopes []mmu.Scope, types []mmu.PageType) (string, []any) {
	parts := []string{"p.agent_id = ?"}
	args := []any{agentID.String()}
	if len(scopes) > 0 {
		parts = append(parts, "p.scope IN ("+questionMarks(len(scopes))+")")
		for _, scope := range scopes {
			args = append(args, string(scope))
		}
	}
	if len(types) > 0 {
		parts = append(parts, "p.page_type IN ("+questionMarks(len(types))+")")
		for _, pageType := range types {
			args = append(args, string(pageType))
		}
	}
	return strings.Join(parts, " AND "), args
}

func ftsTerms(text string) []string {
	seen := make(map[string]struct{})
	terms := make([]string, 0, 16)
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' }) {
		if len(field) < 2 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		terms = append(terms, `"`+strings.ReplaceAll(field, `"`, `""`)+`"`)
		if len(terms) == 16 {
			break
		}
	}
	return terms
}

func prefixedContextColumns(prefix string) string {
	columns := []string{"page_id", "agent_id", "page_type", "scope", "source_ref", "object_ref", "token_estimate", "importance", "confidence", "created_at", "last_accessed_at", "pinned_until", "superseded_by", "compacted_by", "summary_of_json"}
	for i := range columns {
		columns[i] = prefix + "." + columns[i]
	}
	return strings.Join(columns, ", ")
}

func questionMarks(n int) string        { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }
func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}
func optionalPageID(value *id.ContextPageID) any {
	if value == nil {
		return nil
	}
	return value.String()
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

var _ mmu.Repository = (*Store)(nil)
