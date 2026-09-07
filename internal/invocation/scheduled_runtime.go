package invocation

import (
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/scheduler"
)

func (s *ScheduledService) SetRootBudget(root id.AgentID, limit scheduler.Resources) error {
	if s == nil || s.runtime == nil || s.runtime.Budgets == nil {
		return errs.New(errs.CodeInternal, "invocation.scheduled.set_root_budget", "scheduler runtime is not configured")
	}
	return s.runtime.Budgets.SetLimit(root, limit)
}

func (s *ScheduledService) Budget(root id.AgentID) scheduler.BudgetSnapshot {
	if s == nil || s.runtime == nil || s.runtime.Budgets == nil {
		return scheduler.BudgetSnapshot{}
	}
	return s.runtime.Budgets.Snapshot(root)
}

func (s *ScheduledService) Metrics() scheduler.MetricsSnapshot {
	if s == nil || s.runtime == nil || s.runtime.Metrics == nil {
		return scheduler.MetricsSnapshot{}
	}
	return s.runtime.Metrics.Snapshot()
}
