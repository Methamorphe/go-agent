package world

import (
	"encoding/json"
	"fmt"
)

// WorkspaceTarget returns the canonical target repository and immutable base
// identity encoded in a WorkspaceWorld BranchRef. It intentionally does not
// expose mutable implementation internals or reopen the World.
func WorkspaceTarget(ref BranchRef) (repository string, baseIdentity string, err error) {
	if ref.Type != TypeWorkspace || ref.WorldID == "" || len(ref.Metadata) == 0 {
		return "", "", fmt.Errorf("invalid workspace branch reference")
	}
	var meta workspaceMetadata
	if err := json.Unmarshal(ref.Metadata, &meta); err != nil {
		return "", "", fmt.Errorf("decode workspace branch reference: %w", err)
	}
	if meta.Repository == "" || meta.BaseTree == "" || ref.BaseIdentity == "" {
		return "", "", fmt.Errorf("workspace branch reference is incomplete")
	}
	if meta.BaseTree != ref.BaseIdentity {
		return "", "", fmt.Errorf("workspace branch base identity mismatch")
	}
	return meta.Repository, meta.BaseTree, nil
}
