package mmu

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/id"
)

func (m *Manager) AttachPendingFaults(ctx context.Context, manifest *ContextManifest) error {
	if manifest == nil { return nil }
	journal, ok := m.repo.(FaultJournal)
	if !ok { return nil }
	records, err := journal.PendingResolvedContextFaults(ctx, manifest.AgentID)
	if err != nil { return err }
	manifest.Faults = manifest.Faults[:0]
	for _, record := range records {
		requested := ""
		if record.Request.Reference != nil { requested = record.Request.Reference.String() }
		if requested == "" && record.Request.Query != nil { requested = record.Request.Query.Text }
		manifest.Faults = append(manifest.Faults, ManifestFault{
			ID: record.Request.ID,
			Kind: record.Request.Kind,
			Requested: requested,
			Purpose: record.Request.Purpose,
			ResolvedPages: append([]id.ContextPageID(nil), record.PageIDs...),
			ResolvedRefs: append([]CognitiveRef(nil), record.ResolvedRefs...),
			Tokens: record.Tokens,
			Lease: "next_invocation",
		})
	}
	return nil
}

func (m *Manager) MarkManifestFaultsPersisted(ctx context.Context, invocationID id.InvocationID, manifest ContextManifest) error {
	journal, ok := m.repo.(FaultJournal)
	if !ok || len(manifest.Faults) == 0 { return nil }
	ids := make([]id.ContextFaultID, 0, len(manifest.Faults))
	for _, fault := range manifest.Faults { ids = append(ids, fault.ID) }
	return journal.MarkContextFaultsManifested(ctx, ids, invocationID)
}
