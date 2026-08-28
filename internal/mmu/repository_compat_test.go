package mmu

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/id"
)

func (f *fakeRepo) MarkContextPageSuperseded(_ context.Context, agentID id.AgentID, pageID id.ContextPageID, replacement id.ContextPageID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	meta, ok := f.pages[pageID]
	if !ok || meta.AgentID != agentID {
		return nil
	}
	value := replacement
	meta.SupersededBy = &value
	f.pages[pageID] = meta
	return nil
}
