package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/memory"
	"github.com/Methamorphe/go-agent/internal/objectstore"
)

func (s *Store) PutEvidence(ctx context.Context, value memory.Evidence) error {
	result, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO memory_evidence (
    evidence_id, kind, scope_kind, scope_ref, source_ref, object_ref,
    content_hash, observed_at, source_version, provenance_class, trust_class, sensitivity
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID.String(), string(value.Kind), string(value.Scope.Kind), value.Scope.Ref, value.SourceRef,
		string(value.ObjectRef), value.ContentHash, formatTime(value.ObservedAt), value.SourceVersion,
		string(value.Provenance), string(value.TrustClass), string(value.Sensitivity),
	)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.evidence.put", "insert evidence", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.memory.evidence.put", "read affected rows", err)
	}
	if rows != 1 {
		return memory.ErrConflict
	}
	return nil
}

func (s *Store) Evidence(ctx context.Context, evidenceID id.EvidenceID) (memory.Evidence, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT evidence_id, kind, scope_kind, scope_ref, source_ref, object_ref,
       content_hash, observed_at, source_version, provenance_class, trust_class, sensitivity
FROM memory_evidence WHERE evidence_id = ?`, evidenceID.String())
	value, err := scanMemoryEvidence(row)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Evidence{}, memory.ErrNotFound
	}
	if err != nil {
		return memory.Evidence{}, errs.Wrap(errs.CodeCorruption, "sqlite.memory.evidence.get", "scan evidence", err)
	}
	return value, nil
}

func (s *Store) EvidenceBySource(ctx context.Context, sourceRef string) ([]memory.Evidence, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT evidence_id, kind, scope_kind, scope_ref, source_ref, object_ref,
       content_hash, observed_at, source_version, provenance_class, trust_class, sensitivity
FROM memory_evidence
WHERE source_ref = ?
ORDER BY observed_at, evidence_id`, sourceRef)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.evidence.by_source", "query evidence by source", err)
	}
	defer rows.Close()
	out := make([]memory.Evidence, 0)
	for rows.Next() {
		value, scanErr := scanMemoryEvidence(rows)
		if scanErr != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.evidence.by_source", "scan evidence", scanErr)
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.evidence.by_source", "iterate evidence", err)
	}
	return out, nil
}

func (s *Store) InvalidateEvidence(ctx context.Context, value memory.EvidenceInvalidation) error {
	result, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO memory_evidence_invalidations (
    evidence_id, reason, replacement_evidence_id, invalidated_at
) VALUES (?, ?, ?, ?)`, value.EvidenceID.String(), value.Reason, value.ReplacementEvidenceID.String(), formatTime(value.InvalidatedAt))
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.evidence.invalidate", "insert evidence invalidation", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.memory.evidence.invalidate", "read affected rows", err)
	}
	if rows != 1 {
		return memory.ErrConflict
	}
	return nil
}

func (s *Store) EvidenceInvalidation(ctx context.Context, evidenceID id.EvidenceID) (*memory.EvidenceInvalidation, error) {
	var value memory.EvidenceInvalidation
	var rawEvidence, replacement, invalidated string
	err := s.db.QueryRowContext(ctx, `
SELECT evidence_id, reason, replacement_evidence_id, invalidated_at
FROM memory_evidence_invalidations WHERE evidence_id = ?`, evidenceID.String()).Scan(
		&rawEvidence, &value.Reason, &replacement, &invalidated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		var exists int
		if checkErr := s.db.QueryRowContext(ctx, `SELECT 1 FROM memory_evidence WHERE evidence_id = ?`, evidenceID.String()).Scan(&exists); errors.Is(checkErr, sql.ErrNoRows) {
			return nil, memory.ErrNotFound
		} else if checkErr != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.evidence.invalidation", "check evidence existence", checkErr)
		}
		return nil, nil
	}
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.evidence.invalidation", "query evidence invalidation", err)
	}
	value.EvidenceID = id.EvidenceID(rawEvidence)
	value.ReplacementEvidenceID = id.EvidenceID(replacement)
	value.InvalidatedAt, err = time.Parse(time.RFC3339Nano, invalidated)
	if err != nil {
		return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.evidence.invalidation", "parse invalidation timestamp", err)
	}
	return &value, nil
}

func (s *Store) PutSourceVersion(ctx context.Context, value memory.SourceVersion) error {
	result, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO memory_source_versions (source_ref, version, content_hash, observed_at)
VALUES (?, ?, ?, ?)`, value.SourceRef, value.Version, value.ContentHash, formatTime(value.ObservedAt))
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.memory.source_version.put", "insert source version", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.memory.source_version.put", "read affected rows", err)
	}
	if rows != 1 {
		return memory.ErrConflict
	}
	return nil
}

func (s *Store) SourceVersions(ctx context.Context, sourceRef string, limit int) ([]memory.SourceVersion, error) {
	if limit <= 0 {
		limit = 64
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT source_ref, version, content_hash, observed_at
FROM memory_source_versions
WHERE source_ref = ?
ORDER BY observed_at DESC, version DESC
LIMIT ?`, sourceRef, limit)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.memory.source_version.list", "query source versions", err)
	}
	defer rows.Close()
	out := make([]memory.SourceVersion, 0)
	for rows.Next() {
		var value memory.SourceVersion
		var observed string
		if err := rows.Scan(&value.SourceRef, &value.Version, &value.ContentHash, &observed); err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.source_version.list", "scan source version", err)
		}
		value.ObservedAt, err = time.Parse(time.RFC3339Nano, observed)
		if err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.memory.source_version.list", "parse source timestamp", err)
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func scanMemoryEvidence(scanner rowScanner) (memory.Evidence, error) {
	var value memory.Evidence
	var evidenceID, kind, scopeKind, objectRef, observed, provenance, trust, sensitivity string
	if err := scanner.Scan(
		&evidenceID, &kind, &scopeKind, &value.Scope.Ref, &value.SourceRef, &objectRef,
		&value.ContentHash, &observed, &value.SourceVersion, &provenance, &trust, &sensitivity,
	); err != nil {
		return memory.Evidence{}, err
	}
	value.ID = id.EvidenceID(evidenceID)
	value.Kind = memory.EvidenceKind(kind)
	value.Scope.Kind = memory.ScopeKind(scopeKind)
	value.ObjectRef = objectstore.Ref(objectRef)
	value.Provenance = memory.ProvenanceClass(provenance)
	value.TrustClass = memory.TrustClass(trust)
	value.Sensitivity = memory.SensitivityClass(sensitivity)
	var err error
	value.ObservedAt, err = time.Parse(time.RFC3339Nano, observed)
	return value, err
}
