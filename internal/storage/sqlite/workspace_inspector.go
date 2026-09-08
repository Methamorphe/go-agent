package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/team"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

func (s *Store) WorkspaceInspector(ctx context.Context, rootID id.AgentID) (workspace.Inspector, error) {
	transactions, err := s.workspaceTransactions(ctx, rootID)
	if err != nil {
		return workspace.Inspector{}, err
	}
	forks, err := s.workspaceForks(ctx, rootID)
	if err != nil {
		return workspace.Inspector{}, err
	}
	faults, err := s.workspaceContextFaults(ctx, rootID)
	if err != nil {
		return workspace.Inspector{}, err
	}
	contextRuntime, err := s.workspaceContextRuntime(ctx, rootID, faults)
	if err != nil {
		return workspace.Inspector{}, err
	}
	scheduler, err := s.workspaceScheduler(ctx, rootID)
	if err != nil {
		return workspace.Inspector{}, err
	}
	authority, err := s.workspaceAuthority(ctx, rootID)
	if err != nil {
		return workspace.Inspector{}, err
	}
	teams, err := s.workspaceTeams(ctx, rootID)
	if err != nil {
		return workspace.Inspector{}, err
	}
	improvements, err := s.workspaceImprovements(ctx)
	if err != nil {
		return workspace.Inspector{}, err
	}
	return workspace.Inspector{
		Transactions: transactions,
		Forks: forks,
		Context: contextRuntime,
		ContextFaults: faults,
		Scheduler: scheduler,
		Authority: authority,
		Teams: teams,
		Improvements: improvements,
	}, nil
}

func (s *Store) workspaceTransactions(ctx context.Context, rootID id.AgentID) ([]workspace.TransactionSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT t.transaction_id, t.agent_id, t.world_id, t.state, t.version,
       COALESCE((SELECT v.status FROM agent_transaction_verifications v
                 WHERE v.transaction_id=t.transaction_id ORDER BY v.started_at DESC LIMIT 1), ''),
       (SELECT COUNT(*) FROM agent_transaction_effects e WHERE e.transaction_id=t.transaction_id),
       (SELECT COUNT(*) FROM agent_transaction_effects e WHERE e.transaction_id=t.transaction_id
            AND e.outcome_certainty='unknown'),
       t.reconcile_reason, t.updated_at
FROM agent_transactions t
JOIN agent_processes p ON p.agent_id=t.agent_id
WHERE p.root_agent_id=?
ORDER BY t.updated_at DESC
LIMIT 32`, rootID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.transactions", "query transaction inspector", err)
	}
	defer rows.Close()
	result := make([]workspace.TransactionSummary, 0, 16)
	for rows.Next() {
		var item workspace.TransactionSummary
		var updated string
		if err := rows.Scan(&item.ID, &item.AgentID, &item.WorldID, &item.State, &item.Version,
			&item.VerificationStatus, &item.EffectCount, &item.UncertainEffects, &item.ReconcileReason, &updated); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.transactions", "scan transaction inspector", err)
		}
		item.UpdatedAt, err = parseWorkspaceTime(updated)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.transactions", "iterate transaction inspector", err)
	}
	return result, nil
}

func (s *Store) workspaceForks(ctx context.Context, rootID id.AgentID) ([]workspace.ForkSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT g.group_id, g.source_agent_id, g.state, COALESCE(g.winner_fork_id,''),
       g.selection_reason, g.updated_at
FROM cognitive_fork_groups g
JOIN agent_processes p ON p.agent_id=g.source_agent_id
WHERE p.root_agent_id=?
ORDER BY g.updated_at DESC
LIMIT 16`, rootID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.forks", "query fork inspector", err)
	}
	defer rows.Close()
	result := make([]workspace.ForkSummary, 0, 8)
	for rows.Next() {
		var item workspace.ForkSummary
		var updated string
		if err := rows.Scan(&item.GroupID, &item.SourceAgentID, &item.State, &item.WinnerForkID, &item.SelectionReason, &updated); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.forks", "scan fork inspector", err)
		}
		item.UpdatedAt, err = parseWorkspaceTime(updated)
		if err != nil {
			return nil, err
		}
		branches, err := s.workspaceForkBranches(ctx, item.GroupID)
		if err != nil {
			return nil, err
		}
		item.Branches = branches
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.forks", "iterate fork inspector", err)
	}
	return result, nil
}

func (s *Store) workspaceForkBranches(ctx context.Context, groupID id.ForkGroupID) ([]workspace.ForkBranchSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT fork_id, agent_id, world_id, state, budget_spent_money_micros,
       budget_spent_tokens, COALESCE(CAST(evaluation_json AS TEXT),'')
FROM cognitive_fork_branches
WHERE group_id=?
ORDER BY created_at ASC
LIMIT 32`, groupID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.fork_branches", "query fork branches", err)
	}
	defer rows.Close()
	result := make([]workspace.ForkBranchSummary, 0, 4)
	for rows.Next() {
		var item workspace.ForkBranchSummary
		if err := rows.Scan(&item.ForkID, &item.AgentID, &item.WorldID, &item.State,
			&item.SpentMoneyMicros, &item.SpentTokens, &item.Evaluation); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.fork_branches", "scan fork branch", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.fork_branches", "iterate fork branches", err)
	}
	return result, nil
}

func (s *Store) workspaceContextFaults(ctx context.Context, rootID id.AgentID) ([]workspace.ContextFaultSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT f.fault_id, f.agent_id, f.state, f.record_json, f.created_at, f.updated_at
FROM context_faults f
JOIN agent_processes p ON p.agent_id=f.agent_id
WHERE p.root_agent_id=?
ORDER BY f.updated_at DESC
LIMIT 32`, rootID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.context_faults", "query context fault inspector", err)
	}
	defer rows.Close()
	result := make([]workspace.ContextFaultSummary, 0, 16)
	for rows.Next() {
		var item workspace.ContextFaultSummary
		var record string
		var created, updated string
		if err := rows.Scan(&item.FaultID, &item.AgentID, &item.State, &record, &created, &updated); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.context_faults", "scan context fault", err)
		}
		item.CreatedAt, err = parseWorkspaceTime(created)
		if err != nil {
			return nil, err
		}
		item.UpdatedAt, err = parseWorkspaceTime(updated)
		if err != nil {
			return nil, err
		}
		var body map[string]any
		if json.Unmarshal([]byte(record), &body) == nil {
			item.Kind = mapText(body, "kind", "type", "fault_kind")
			item.Reference = mapText(body, "reference", "semantic_reference", "key")
			item.Resolution = mapText(body, "resolution", "resolution_reason", "message")
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.context_faults", "iterate context faults", err)
	}
	return result, nil
}

func (s *Store) workspaceContextRuntime(ctx context.Context, rootID id.AgentID, faults []workspace.ContextFaultSummary) (workspace.ContextRuntimeSummary, error) {
	item := workspace.ContextRuntimeSummary{AgentID: rootID}
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*), COALESCE(SUM(c.token_estimate), 0)
FROM context_pages c
JOIN agent_processes p ON p.agent_id=c.agent_id
WHERE p.root_agent_id=? AND c.superseded_by IS NULL AND c.compacted_by IS NULL`, rootID).Scan(&item.PageCount, &item.EstimatedTokens); err != nil {
		return item, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.context", "query active context pages", err)
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM context_leases l
JOIN context_pages c ON c.page_id=l.page_id
JOIN agent_processes p ON p.agent_id=c.agent_id
WHERE p.root_agent_id=? AND l.remaining_builds > 0
  AND (l.expires_at IS NULL OR l.expires_at > ?)`, rootID, time.Now().UTC().Format(time.RFC3339Nano)).Scan(&item.ActiveLeaseCount); err != nil {
		return item, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.context", "query active context leases", err)
	}
	for _, fault := range faults {
		if fault.State != "RESOLVED" {
			item.UnresolvedFaults++
		}
	}
	var created string
	err := s.db.QueryRowContext(ctx, `
SELECT m.agent_id, m.object_ref, m.created_at
FROM context_manifests m
JOIN agent_processes p ON p.agent_id=m.agent_id
WHERE p.root_agent_id=?
ORDER BY m.created_at DESC
LIMIT 1`, rootID).Scan(&item.AgentID, &item.LatestManifestRef, &created)
	if err == nil {
		item.LatestManifestAt, err = parseWorkspaceTime(created)
	}
	if err != nil && err != sql.ErrNoRows {
		return item, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.context", "query latest context manifest", err)
	}
	return item, nil
}

func (s *Store) workspaceScheduler(ctx context.Context, rootID id.AgentID) (workspace.SchedulerSummary, error) {
	item := workspace.SchedulerSummary{RootAgentID: rootID}
	err := s.db.QueryRowContext(ctx, `
SELECT limit_money_micros, limit_tokens, spent_money_micros, spent_tokens,
       reserved_money_micros, reserved_tokens
FROM scheduler_budget_accounts
WHERE root_agent_id=?`, rootID).Scan(
		&item.LimitMoneyMicros, &item.LimitTokens, &item.SpentMoneyMicros, &item.SpentTokens,
		&item.ReservedMoneyMicros, &item.ReservedTokens,
	)
	if err != nil && err != sql.ErrNoRows {
		return item, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.scheduler", "query scheduler budget", err)
	}

	var body []byte
	var occurred string
	err = s.db.QueryRowContext(ctx, `
SELECT payload_json, occurred_at
FROM ledger_events
WHERE root_agent_id=? AND event_type='CognitiveRoutingDecided'
ORDER BY ledger_sequence DESC
LIMIT 1`, rootID).Scan(&body, &occurred)
	if err == sql.ErrNoRows {
		return item, nil
	}
	if err != nil {
		return item, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.scheduler", "query latest routing decision", err)
	}
	var payload agentprocess.CognitiveRoutingDecidedPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return item, errs.Wrap(errs.CodeCorruption, "sqlite.workspace.scheduler", "decode routing decision", err)
	}
	item.LastDecisionID = payload.DecisionID
	item.LastProvider = payload.Provider
	item.LastModel = payload.Model
	item.LastProfileVersion = payload.ProfileVersion
	item.LastEstimatedMoney = payload.EstimatedMoneyMicros
	item.LastEstimatedTokens = payload.EstimatedTokens
	item.LastDecisionAt, err = parseWorkspaceTime(occurred)
	if err != nil {
		return item, err
	}
	return item, nil
}

func (s *Store) workspaceAuthority(ctx context.Context, rootID id.AgentID) (workspace.AuthoritySummary, error) {
	item := workspace.AuthoritySummary{
		AgentID: rootID,
		CapabilityProjection: "not projected as grants; runtime authority remains canonical below the TUI",
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT state_json FROM agent_processes WHERE agent_id=?`, rootID).Scan(&body); err != nil {
		return item, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.authority", "query root process intent", err)
	}
	var state agentprocess.State
	if err := json.Unmarshal(body, &state); err != nil {
		return item, errs.Wrap(errs.CodeCorruption, "sqlite.workspace.authority", "decode root process state", err)
	}
	if state.RootIntent != nil {
		item.IntentID = state.RootIntent.ID
		item.IntentVersion = state.RootIntent.SchemaVersion
		item.Goal = state.RootIntent.Goal
	}
	var occurred string
	err := s.db.QueryRowContext(ctx, `
SELECT event_type, occurred_at
FROM ledger_events
WHERE root_agent_id=? AND (
      event_type LIKE '%Authorization%' OR event_type LIKE '%Authority%' OR
      event_type LIKE '%Denied%' OR event_type LIKE '%Approval%')
ORDER BY ledger_sequence DESC
LIMIT 1`, rootID).Scan(&item.LastSecurityEvent, &occurred)
	if err == nil {
		item.LastSecurityAt, err = parseWorkspaceTime(occurred)
	}
	if err != nil && err != sql.ErrNoRows {
		return item, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.authority", "query latest security event", err)
	}
	return item, nil
}

func (s *Store) workspaceTeams(ctx context.Context, rootID id.AgentID) ([]workspace.TeamSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT team_id, state, team_json, updated_at
FROM adaptive_teams
ORDER BY updated_at DESC
LIMIT 64`)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.teams", "query team inspector", err)
	}
	defer rows.Close()
	result := make([]workspace.TeamSummary, 0, 8)
	for rows.Next() {
		var teamID id.TeamID
		var state, body, updated string
		if err := rows.Scan(&teamID, &state, &body, &updated); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.teams", "scan team inspector", err)
		}
		var value team.Team
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			return nil, errs.Wrap(errs.CodeCorruption, "sqlite.workspace.teams", "decode team projection", err)
		}
		if value.Proposal.RootAgentID != rootID {
			continue
		}
		item := workspace.TeamSummary{
			TeamID: teamID, RootAgentID: value.Proposal.RootAgentID, LeadAgentID: value.Proposal.LeadAgentID,
			State: state, Objective: value.Proposal.Objective, MemberCount: len(value.Members),
			MaxRounds: value.Proposal.Limits.MaxRounds, Escalated: state == string(team.TeamEscalated),
		}
		item.UpdatedAt, err = parseWorkspaceTime(updated)
		if err != nil {
			return nil, err
		}
		_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(round),0) FROM agent_negotiations WHERE team_id=?`, teamID).Scan(&item.Round)
		result = append(result, item)
		if len(result) == 16 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.teams", "iterate team inspector", err)
	}
	return result, nil
}

func (s *Store) workspaceImprovements(ctx context.Context) ([]workspace.ImprovementSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT v.artifact_id, v.kind, v.version, v.status, v.scope_level, v.scope_key, v.created_at,
       CASE WHEN a.artifact_id IS NULL THEN 0 ELSE 1 END
FROM improvement_artifact_versions v
LEFT JOIN improvement_active_artifacts a ON a.artifact_id=v.artifact_id AND a.version=v.version
WHERE v.version=(SELECT MAX(v2.version) FROM improvement_artifact_versions v2 WHERE v2.artifact_id=v.artifact_id)
ORDER BY v.created_at DESC
LIMIT 64`)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.improvements", "query improvement inspector", err)
	}
	defer rows.Close()
	result := make([]workspace.ImprovementSummary, 0, 16)
	for rows.Next() {
		var item workspace.ImprovementSummary
		var created string
		var active int
		if err := rows.Scan(&item.ArtifactID, &item.Kind, &item.Version, &item.Status,
			&item.ScopeLevel, &item.ScopeKey, &created, &active); err != nil {
			return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.improvements", "scan improvement inspector", err)
		}
		item.Active = active == 1
		item.CreatedAt, err = parseWorkspaceTime(created)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "sqlite.workspace.improvements", "iterate improvement inspector", err)
	}
	return result, nil
}

func parseWorkspaceTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, errs.Wrap(errs.CodeCorruption, "sqlite.workspace.time", "invalid workspace projection timestamp", err)
	}
	return parsed, nil
}

func mapText(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := body[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
