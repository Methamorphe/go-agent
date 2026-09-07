package scheduler

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	max      int
	profiles map[string]ModelProfile
}

func NewRegistry(max int) *Registry {
	if max <= 0 {
		max = DefaultMaxProfiles
	}
	return &Registry{max: max, profiles: make(map[string]ModelProfile)}
}
func (r *Registry) Register(profile ModelProfile) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := profile.Key()
	if _, exists := r.profiles[key]; !exists && len(r.profiles) >= r.max {
		return fmt.Errorf("model registry full: %d", r.max)
	}
	if current, exists := r.profiles[key]; exists && profile.ProfileVersion <= current.ProfileVersion {
		return errors.New("model profile version must increase")
	}
	profile.Capabilities = append([]Capability(nil), profile.Capabilities...)
	if profile.Quality != nil {
		q := make(map[TaskKind]float64, len(profile.Quality))
		for k, v := range profile.Quality {
			q[k] = v
		}
		profile.Quality = q
	}
	r.profiles[key] = profile
	return nil
}
func (r *Registry) Remove(providerID ProviderID, modelID ModelID) {
	r.mu.Lock()
	delete(r.profiles, string(providerID)+"/"+string(modelID))
	r.mu.Unlock()
}
func (r *Registry) Snapshot() []ModelProfile {
	r.mu.RLock()
	out := make([]ModelProfile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, p)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}
func (r *Registry) Get(ref ModelRef) (ModelProfile, bool) {
	r.mu.RLock()
	p, ok := r.profiles[ref.Key()]
	r.mu.RUnlock()
	if !ok || p.ProfileVersion != ref.Version {
		return ModelProfile{}, false
	}
	return p, true
}
