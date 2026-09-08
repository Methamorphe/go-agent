package fork

import (
	"context"
	"sort"
	"sync"

	"github.com/Methamorphe/go-agent/internal/id"
)

type MemoryStore struct {
	mu          sync.Mutex
	checkpoints map[id.CheckpointID]Checkpoint
	groups      map[id.ForkGroupID]Group
	branches    map[id.ForkID]Branch
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		checkpoints: make(map[id.CheckpointID]Checkpoint),
		groups:      make(map[id.ForkGroupID]Group),
		branches:    make(map[id.ForkID]Branch),
	}
}

func (s *MemoryStore) PutCheckpoint(_ context.Context, cp Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.checkpoints[cp.ID]; ok { return ErrConflict }
	s.checkpoints[cp.ID] = cloneCheckpoint(cp)
	return nil
}
func (s *MemoryStore) Checkpoint(_ context.Context, checkpointID id.CheckpointID) (Checkpoint, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	cp, ok := s.checkpoints[checkpointID]; if !ok { return Checkpoint{}, ErrNotFound }
	return cloneCheckpoint(cp), nil
}
func (s *MemoryStore) CreateGroup(_ context.Context, g Group) error {
	s.mu.Lock(); defer s.mu.Unlock()
	if _, ok := s.groups[g.ID]; ok { return ErrConflict }
	if _, ok := s.checkpoints[g.CheckpointID]; !ok { return ErrNotFound }
	s.groups[g.ID] = g; return nil
}
func (s *MemoryStore) Group(_ context.Context, groupID id.ForkGroupID) (Group, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	g, ok := s.groups[groupID]; if !ok { return Group{}, ErrNotFound }; return g, nil
}
func (s *MemoryStore) UpdateGroup(_ context.Context, g Group) error {
	s.mu.Lock(); defer s.mu.Unlock()
	if _, ok := s.groups[g.ID]; !ok { return ErrNotFound }
	s.groups[g.ID] = g; return nil
}
func (s *MemoryStore) CreateBranch(_ context.Context, b Branch) error {
	s.mu.Lock(); defer s.mu.Unlock()
	if _, ok := s.branches[b.ID]; ok { return ErrConflict }
	if _, ok := s.groups[b.GroupID]; !ok { return ErrNotFound }
	s.branches[b.ID] = cloneBranch(b); return nil
}
func (s *MemoryStore) Branch(_ context.Context, forkID id.ForkID) (Branch, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	b, ok := s.branches[forkID]; if !ok { return Branch{}, ErrNotFound }; return cloneBranch(b), nil
}
func (s *MemoryStore) UpdateBranch(_ context.Context, b Branch) error {
	s.mu.Lock(); defer s.mu.Unlock()
	if _, ok := s.branches[b.ID]; !ok { return ErrNotFound }
	s.branches[b.ID] = cloneBranch(b); return nil
}
func (s *MemoryStore) Branches(_ context.Context, groupID id.ForkGroupID) ([]Branch, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	if _, ok := s.groups[groupID]; !ok { return nil, ErrNotFound }
	out := make([]Branch, 0)
	for _, b := range s.branches { if b.GroupID == groupID { out = append(out, cloneBranch(b)) } }
	sort.Slice(out, func(i,j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, nil
}
func (s *MemoryStore) DeleteBranch(_ context.Context, forkID id.ForkID) error { s.mu.Lock(); defer s.mu.Unlock(); delete(s.branches, forkID); return nil }
func (s *MemoryStore) DeleteGroup(_ context.Context, groupID id.ForkGroupID) error { s.mu.Lock(); defer s.mu.Unlock(); delete(s.groups, groupID); return nil }
func (s *MemoryStore) DeleteCheckpoint(_ context.Context, checkpointID id.CheckpointID) error { s.mu.Lock(); defer s.mu.Unlock(); delete(s.checkpoints, checkpointID); return nil }

func cloneCheckpoint(cp Checkpoint) Checkpoint {
	copy := cp
	copy.ContextFrontier = append([]id.ContextPageID(nil), cp.ContextFrontier...)
	copy.WorldRef.Metadata = append([]byte(nil), cp.WorldRef.Metadata...)
	if cp.RetainUntil != nil { t := *cp.RetainUntil; copy.RetainUntil = &t }
	return copy
}
func cloneBranch(b Branch) Branch {
	copy := b
	copy.WorldRef.Metadata = append([]byte(nil), b.WorldRef.Metadata...)
	copy.CognitiveOverlay = append([]OverlayEntry(nil), b.CognitiveOverlay...)
	for i := range copy.CognitiveOverlay { copy.CognitiveOverlay[i].Value = append([]byte(nil), b.CognitiveOverlay[i].Value...) }
	if b.Evaluation != nil { e := *b.Evaluation; copy.Evaluation = &e }
	return copy
}
