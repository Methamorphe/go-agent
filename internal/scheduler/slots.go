package scheduler

import (
	"github.com/Methamorphe/go-agent/internal/id"
	"sync"
)

type SlotConfig struct {
	Global             int
	PerRoot            int
	DefaultPerProvider int
}
type SlotLease struct {
	slots    *Slots
	root     id.AgentID
	provider ProviderID
	once     sync.Once
}

func (l *SlotLease) Release() {
	if l == nil || l.slots == nil {
		return
	}
	l.once.Do(func() { l.slots.release(l.root, l.provider) })
}

type Slots struct {
	mu        sync.Mutex
	cfg       SlotConfig
	global    int
	roots     map[id.AgentID]int
	providers map[ProviderID]int
}

func NewSlots(cfg SlotConfig) *Slots {
	if cfg.Global <= 0 {
		cfg.Global = 8
	}
	if cfg.PerRoot <= 0 {
		cfg.PerRoot = maxInt(1, cfg.Global/2)
	}
	if cfg.DefaultPerProvider <= 0 {
		cfg.DefaultPerProvider = cfg.Global
	}
	return &Slots{cfg: cfg, roots: make(map[id.AgentID]int), providers: make(map[ProviderID]int)}
}
func (s *Slots) Acquire(root id.AgentID, provider ProviderID, providerLimit int) (*SlotLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if providerLimit <= 0 {
		providerLimit = s.cfg.DefaultPerProvider
	}
	if s.global >= s.cfg.Global || s.roots[root] >= s.cfg.PerRoot || s.providers[provider] >= providerLimit {
		return nil, ErrSlotsExhausted
	}
	s.global++
	s.roots[root]++
	s.providers[provider]++
	return &SlotLease{slots: s, root: root, provider: provider}, nil
}
func (s *Slots) release(root id.AgentID, provider ProviderID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.global > 0 {
		s.global--
	}
	if s.roots[root] > 0 {
		s.roots[root]--
	}
	if s.providers[provider] > 0 {
		s.providers[provider]--
	}
}
func (s *Slots) Load(provider ProviderID) (global, rootless, providerCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.global, 0, s.providers[provider]
}
