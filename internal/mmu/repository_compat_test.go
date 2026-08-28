package mmu

import (
	"context"
	"sort"
	"time"

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

func (f *fakeRepo) PinnedContextPages(_ context.Context, agentID id.AgentID, scopes []Scope, now time.Time) ([]PageMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []PageMeta
	for _, meta := range f.pages {
		if meta.AgentID != agentID || meta.PinnedUntil == nil || !meta.PinnedUntil.After(now) || meta.SupersededBy != nil {
			continue
		}
		if len(scopes) > 0 && !scopeAllowed(meta.Scope, scopes) {
			continue
		}
		result = append(result, meta)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
