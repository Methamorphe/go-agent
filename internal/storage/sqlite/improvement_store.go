package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/improvement"
)

func (s *Store) CreateVersion(ctx context.Context, v improvement.ArtifactVersion) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.improvement.create_version", "marshal artifact", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO improvement_artifact_versions (artifact_id, version, kind, status, scope_level, scope_key, payload_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, v.ArtifactID, v.Version, v.Kind, v.Status, v.Scope.Level, v.Scope.Key, string(payload), v.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return errs.Wrap(errs.CodeConflict, "sqlite.improvement.create_version", "insert immutable artifact version", err)
	}
	return nil
}

func (s *Store) Version(ctx context.Context, artifactID improvement.ArtifactID, version uint32) (improvement.ArtifactVersion, error) {
	var payload, status string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json, status FROM improvement_artifact_versions WHERE artifact_id = ? AND version = ?`, artifactID, version).Scan(&payload, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return improvement.ArtifactVersion{}, errs.New(errs.CodeNotFound, "sqlite.improvement.version", "artifact version not found")
	}
	if err != nil {
		return improvement.ArtifactVersion{}, errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.version", "query artifact version", err)
	}
	var out improvement.ArtifactVersion
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return improvement.ArtifactVersion{}, errs.Wrap(errs.CodeCorruption, "sqlite.improvement.version", "decode artifact payload", err)
	}
	out.Status = improvement.ArtifactStatus(status)
	return out, nil
}

func (s *Store) Versions(ctx context.Context, artifactID improvement.ArtifactID) ([]improvement.ArtifactVersion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload_json, status FROM improvement_artifact_versions WHERE artifact_id = ? ORDER BY version`, artifactID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.versions", "query artifact versions", err)
	}
	defer rows.Close()
	out := []improvement.ArtifactVersion{}
	for rows.Next() {
		var payload, status string
		if err := rows.Scan(&payload, &status); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.versions", "scan artifact version", err)
		}
		var v improvement.ArtifactVersion
		if err := json.Unmarshal([]byte(payload), &v); err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.improvement.versions", "decode artifact payload", err)
		}
		v.Status = improvement.ArtifactStatus(status)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.versions", "iterate artifact versions", err)
	}
	if len(out) == 0 {
		return nil, errs.New(errs.CodeNotFound, "sqlite.improvement.versions", "artifact not found")
	}
	return out, nil
}

func (s *Store) SetStatus(ctx context.Context, artifactID improvement.ArtifactID, version uint32, status improvement.ArtifactStatus) error {
	r, err := s.db.ExecContext(ctx, `UPDATE improvement_artifact_versions SET status = ? WHERE artifact_id = ? AND version = ?`, status, artifactID, version)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.set_status", "update artifact status", err)
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return errs.New(errs.CodeNotFound, "sqlite.improvement.set_status", "artifact version not found")
	}
	return nil
}

func (s *Store) PutEvaluation(ctx context.Context, e improvement.Evaluation) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.improvement.put_evaluation", "marshal evaluation", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO improvement_evaluations (id, artifact_id, baseline_version, candidate_version, payload_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`, e.ID, e.ArtifactID, e.BaselineVersion, e.CandidateVersion, string(payload), e.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return errs.Wrap(errs.CodeConflict, "sqlite.improvement.put_evaluation", "insert evaluation", err)
	}
	return nil
}

func (s *Store) Evaluation(ctx context.Context, id improvement.EvaluationID) (improvement.Evaluation, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM improvement_evaluations WHERE id = ?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return improvement.Evaluation{}, errs.New(errs.CodeNotFound, "sqlite.improvement.evaluation", "evaluation not found")
	}
	if err != nil {
		return improvement.Evaluation{}, errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.evaluation", "query evaluation", err)
	}
	var e improvement.Evaluation
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		return improvement.Evaluation{}, errs.Wrap(errs.CodeCorruption, "sqlite.improvement.evaluation", "decode evaluation", err)
	}
	return e, nil
}

func (s *Store) Promote(ctx context.Context, r improvement.PromotionRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.promote", "begin promotion", err)
	}
	defer func() { _ = tx.Rollback() }()
	var current uint32
	err = tx.QueryRowContext(ctx, `SELECT version FROM improvement_active_artifacts WHERE artifact_id = ?`, r.ArtifactID).Scan(&current)
	if r.BaselineVersion == 0 {
		if err == nil {
			return errs.New(errs.CodeConflict, "sqlite.improvement.promote", "artifact already active")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.promote", "query active baseline", err)
		}
	} else {
		if errors.Is(err, sql.ErrNoRows) || current != r.BaselineVersion {
			return errs.New(errs.CodeConflict, "sqlite.improvement.promote", "active baseline changed")
		}
		if err != nil {
			return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.promote", "query active baseline", err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE improvement_artifact_versions SET status = ? WHERE artifact_id = ? AND version = ?`, improvement.StatusDeprecated, r.ArtifactID, r.BaselineVersion); err != nil {
			return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.promote", "deprecate baseline", err)
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE improvement_artifact_versions SET status = ? WHERE artifact_id = ? AND version = ?`, improvement.StatusPromoted, r.ArtifactID, r.CandidateVersion)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.promote", "promote candidate", err)
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errs.New(errs.CodeNotFound, "sqlite.improvement.promote", "candidate not found")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO improvement_active_artifacts (artifact_id, version) VALUES (?, ?) ON CONFLICT(artifact_id) DO UPDATE SET version = excluded.version`, r.ArtifactID, r.CandidateVersion)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.promote", "set active artifact", err)
	}
	payload, err := json.Marshal(r)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.improvement.promote", "marshal promotion", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO improvement_promotions (artifact_id, candidate_version, baseline_version, evaluation_id, payload_json, promoted_at) VALUES (?, ?, ?, ?, ?, ?)`, r.ArtifactID, r.CandidateVersion, r.BaselineVersion, r.EvaluationID, string(payload), r.PromotedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return errs.Wrap(errs.CodeConflict, "sqlite.improvement.promote", "record promotion", err)
	}
	if err := tx.Commit(); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.promote", "commit promotion", err)
	}
	return nil
}

func (s *Store) Promotion(ctx context.Context, artifactID improvement.ArtifactID, version uint32) (improvement.PromotionRecord, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM improvement_promotions WHERE artifact_id = ? AND candidate_version = ?`, artifactID, version).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return improvement.PromotionRecord{}, errs.New(errs.CodeNotFound, "sqlite.improvement.promotion", "promotion not found")
	}
	if err != nil {
		return improvement.PromotionRecord{}, errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.promotion", "query promotion", err)
	}
	var r improvement.PromotionRecord
	if err := json.Unmarshal([]byte(payload), &r); err != nil {
		return improvement.PromotionRecord{}, errs.Wrap(errs.CodeCorruption, "sqlite.improvement.promotion", "decode promotion", err)
	}
	return r, nil
}

func (s *Store) ActiveVersion(ctx context.Context, artifactID improvement.ArtifactID) (uint32, error) {
	var v uint32
	err := s.db.QueryRowContext(ctx, `SELECT version FROM improvement_active_artifacts WHERE artifact_id = ?`, artifactID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errs.New(errs.CodeNotFound, "sqlite.improvement.active", "active artifact not found")
	}
	if err != nil {
		return 0, errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.active", "query active artifact", err)
	}
	return v, nil
}

func (s *Store) Rollback(ctx context.Context, artifactID improvement.ArtifactID, from, to uint32, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.rollback", "begin rollback", err)
	}
	defer func() { _ = tx.Rollback() }()
	var active uint32
	if err := tx.QueryRowContext(ctx, `SELECT version FROM improvement_active_artifacts WHERE artifact_id = ?`, artifactID).Scan(&active); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.rollback", "query active version", err)
	}
	if active != from {
		return errs.New(errs.CodeConflict, "sqlite.improvement.rollback", "active version changed")
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM improvement_artifact_versions WHERE artifact_id = ? AND version = ?`, artifactID, to).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.New(errs.CodeNotFound, "sqlite.improvement.rollback", "target version not found")
		}
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.rollback", "query target", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE improvement_artifact_versions SET status = ? WHERE artifact_id = ? AND version = ?`, improvement.StatusRolledBack, artifactID, from); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.rollback", "mark rolled back", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE improvement_artifact_versions SET status = ? WHERE artifact_id = ? AND version = ?`, improvement.StatusPromoted, artifactID, to); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.rollback", "restore target", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE improvement_active_artifacts SET version = ? WHERE artifact_id = ?`, to, artifactID); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.rollback", "activate rollback target", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO improvement_rollbacks (artifact_id, from_version, to_version, rolled_back_at) VALUES (?, ?, ?, ?)`, artifactID, from, to, at.UTC().Format(time.RFC3339Nano)); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.rollback", "record rollback", err)
	}
	if err := tx.Commit(); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.rollback", "commit rollback", err)
	}
	return nil
}

func (s *Store) BindInvocation(ctx context.Context, m improvement.InvocationManifest) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.improvement.bind_invocation", "marshal manifest", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO improvement_invocation_manifests (invocation_id, payload_json, captured_at) VALUES (?, ?, ?)`, m.InvocationID, string(payload), m.CapturedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return errs.Wrap(errs.CodeConflict, "sqlite.improvement.bind_invocation", "invocation manifest is immutable", err)
	}
	return nil
}

func (s *Store) InvocationManifest(ctx context.Context, id string) (improvement.InvocationManifest, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM improvement_invocation_manifests WHERE invocation_id = ?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return improvement.InvocationManifest{}, errs.New(errs.CodeNotFound, "sqlite.improvement.invocation", "manifest not found")
	}
	if err != nil {
		return improvement.InvocationManifest{}, errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.invocation", "query manifest", err)
	}
	var m improvement.InvocationManifest
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return improvement.InvocationManifest{}, errs.Wrap(errs.CodeCorruption, "sqlite.improvement.invocation", "decode manifest", err)
	}
	return m, nil
}

func (s *Store) KillSwitch(ctx context.Context) (improvement.KillSwitch, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT kill_switch_json FROM improvement_runtime_control WHERE singleton = 1`).Scan(&payload)
	if err != nil {
		return improvement.KillSwitch{}, errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.kill_switch", "query kill switch", err)
	}
	var k improvement.KillSwitch
	if err := json.Unmarshal([]byte(payload), &k); err != nil {
		return improvement.KillSwitch{}, errs.Wrap(errs.CodeCorruption, "sqlite.improvement.kill_switch", "decode kill switch", err)
	}
	if k.Families == nil {
		k.Families = map[improvement.ArtifactKind]bool{}
	}
	if k.Versions == nil {
		k.Versions = map[string]bool{}
	}
	return k, nil
}

func (s *Store) SetKillSwitch(ctx context.Context, k improvement.KillSwitch) error {
	payload, err := json.Marshal(k)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "sqlite.improvement.set_kill_switch", "marshal kill switch", err)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE improvement_runtime_control SET kill_switch_json = ? WHERE singleton = 1`, string(payload))
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.set_kill_switch", "update kill switch", err)
	}
	return nil
}

func (s *Store) ReserveCanaryUse(ctx context.Context, artifactID improvement.ArtifactID, version uint32, cap uint64) error {
	if cap == 0 {
		return errs.New(errs.CodeResourceExhausted, "sqlite.improvement.canary", "canary sample cap exhausted")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.canary", "begin canary reservation", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO improvement_canary_usage (artifact_id, version, used) VALUES (?, ?, 0)`, artifactID, version); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.canary", "initialize canary counter", err)
	}
	r, err := tx.ExecContext(ctx, `UPDATE improvement_canary_usage SET used = used + 1 WHERE artifact_id = ? AND version = ? AND used < ?`, artifactID, version, cap)
	if err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.canary", "reserve canary use", err)
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return errs.New(errs.CodeResourceExhausted, "sqlite.improvement.canary", "canary sample cap exhausted")
	}
	if err := tx.Commit(); err != nil {
		return errs.Wrap(errs.CodeUnavailable, "sqlite.improvement.canary", "commit canary reservation", err)
	}
	return nil
}

var _ improvement.Store = (*Store)(nil)
