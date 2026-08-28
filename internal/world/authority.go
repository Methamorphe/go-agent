package world

import (
	"path/filepath"
	"strings"
	"time"
)

type DecisionSink func(Action, AuthorizationDecision)

type Authorizer struct {
	intent Intent
	grants []Capability
	sink   DecisionSink
}

func NewAuthorizer(intent Intent, grants []Capability, sink DecisionSink) *Authorizer {
	return &Authorizer{intent: intent, grants: append([]Capability(nil), grants...), sink: sink}
}

func (a *Authorizer) Authorize(action Action, now time.Time) AuthorizationDecision {
	canonical := CanonicalEffect(action.Kind)
	if action.Effect != canonical {
		decision := AuthorizationDecision{Allowed: false, Code: "effect_mismatch"}
		a.emit(action, decision)
		return decision
	}
	if strings.TrimSpace(action.Purpose) == "" {
		decision := AuthorizationDecision{Allowed: false, Code: "missing_purpose"}
		a.emit(action, decision)
		return decision
	}
	for _, forbidden := range a.intent.ForbiddenDomains {
		if domainFor(action.Kind) == forbidden {
			decision := AuthorizationDecision{Allowed: false, Code: "intent_forbidden_domain"}
			a.emit(action, decision)
			return decision
		}
	}
	if len(a.intent.AllowedDomains) > 0 && !contains(a.intent.AllowedDomains, domainFor(action.Kind)) {
		decision := AuthorizationDecision{Allowed: false, Code: "intent_domain_not_allowed"}
		a.emit(action, decision)
		return decision
	}

	domain := domainFor(action.Kind)
	for _, grant := range a.grants {
		if grant.Domain != domain {
			continue
		}
		if grant.ExpiresAt != nil && !now.Before(*grant.ExpiresAt) {
			continue
		}
		if !scopeAllows(grant.Scope, action.Resource) {
			continue
		}
		decision := AuthorizationDecision{Allowed: true, Code: "allowed", Proof: &ActionProof{
			IntentVersion: a.intent.Version,
			Purpose: action.Purpose,
			CapabilityDomain: grant.Domain,
			CapabilityScope: grant.Scope,
		}}
		a.emit(action, decision)
		return decision
	}
	decision := AuthorizationDecision{Allowed: false, Code: "capability_missing_or_expired"}
	a.emit(action, decision)
	return decision
}

func (a *Authorizer) emit(action Action, decision AuthorizationDecision) {
	if a.sink != nil {
		a.sink(action, decision)
	}
}

func ValidateDelegation(parent, child []Capability, now time.Time) bool {
	for _, requested := range child {
		covered := false
		for _, grant := range parent {
			if grant.Domain != requested.Domain {
				continue
			}
			if grant.ExpiresAt != nil && !now.Before(*grant.ExpiresAt) {
				continue
			}
			if requested.ExpiresAt != nil && grant.ExpiresAt != nil && requested.ExpiresAt.After(*grant.ExpiresAt) {
				continue
			}
			if scopeAllows(grant.Scope, requested.Scope) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func domainFor(kind string) string {
	switch kind {
	case "fs.read_file", "fs.list_directory":
		return "fs.read"
	case "fs.write_file":
		return "fs.write"
	case "process.exec":
		return "process.exec"
	case "network.request":
		return "network"
	default:
		return "pure"
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func scopeAllows(scope, resource string) bool {
	if scope == "*" {
		return true
	}
	if resource == "" {
		return scope == ""
	}
	base := filepath.Clean(scope)
	target := filepath.Clean(resource)
	rel, err := filepath.Rel(base, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
