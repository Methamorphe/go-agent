package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Methamorphe/go-agent/internal/workspace"
)

type reviewSurface string

const (
	reviewConversation reviewSurface = "conversation"
	reviewPlan         reviewSurface = "plan"
	reviewDiff         reviewSurface = "diff"
)

type planStep struct {
	ID     string
	Title  string
	State  string
	Detail string
}

type planReview struct {
	BlockID   string
	ObjectRef string
	Title     string
	Steps     []planStep
}

type diffHunk struct {
	Header string
	Body   string
}

type diffFile struct {
	Path      string
	Status    string
	Additions int
	Deletions int
	Hunks     []diffHunk
}

type diffReview struct {
	BlockID   string
	ObjectRef string
	BaseRef   string
	SourceRef string
	TargetRef string
	Files     []diffFile
}

func (m Model) latestPlan() (planReview, bool) {
	for index := len(m.blocks) - 1; index >= 0; index-- {
		if m.blocks[index].Kind != workspace.BlockPlan {
			continue
		}
		return decodePlanBlock(m.blocks[index]), true
	}
	return planReview{}, false
}

func (m Model) latestDiff() (diffReview, bool) {
	for index := len(m.blocks) - 1; index >= 0; index-- {
		if m.blocks[index].Kind != workspace.BlockDiff {
			continue
		}
		return decodeDiffBlock(m.blocks[index]), true
	}
	return diffReview{}, false
}

func decodePlanBlock(block workspace.Block) planReview {
	view := planReview{BlockID: block.ID, ObjectRef: block.ObjectRef, Title: block.Title}
	if strings.TrimSpace(view.Title) == "" {
		view.Title = "Agent plan"
	}
	var raw any
	if len(block.Payload) > 0 && json.Unmarshal(block.Payload, &raw) == nil {
		body := unwrapObject(raw, "plan")
		if object, ok := body.(map[string]any); ok {
			if title := textField(object, "title", "goal", "objective"); title != "" {
				view.Title = title
			}
			if values, ok := sliceField(object, "steps", "items", "tasks"); ok {
				view.Steps = decodePlanSteps(values)
			}
		}
	}
	if len(view.Steps) == 0 {
		preview := strings.TrimSpace(block.Preview)
		if preview == "" {
			preview = "Plan artifact available at " + block.ObjectRef
		}
		view.Steps = []planStep{{ID: "1", Title: preview, State: "proposed"}}
	}
	return view
}

func decodePlanSteps(values []any) []planStep {
	steps := make([]planStep, 0, len(values))
	for index, value := range values {
		step := planStep{ID: fmt.Sprintf("%d", index+1), State: "proposed"}
		switch item := value.(type) {
		case string:
			step.Title = item
		case map[string]any:
			if id := textField(item, "id", "step_id", "key"); id != "" {
				step.ID = id
			}
			step.Title = textField(item, "title", "summary", "task", "name")
			step.Detail = textField(item, "detail", "description", "reason")
			if state := textField(item, "state", "status"); state != "" {
				step.State = state
			}
		}
		if strings.TrimSpace(step.Title) == "" {
			step.Title = fmt.Sprintf("Step %d", index+1)
		}
		steps = append(steps, step)
	}
	return steps
}

func decodeDiffBlock(block workspace.Block) diffReview {
	view := diffReview{BlockID: block.ID, ObjectRef: block.ObjectRef}
	var raw any
	if len(block.Payload) > 0 && json.Unmarshal(block.Payload, &raw) == nil {
		body := unwrapObject(raw, "diff")
		if object, ok := body.(map[string]any); ok {
			view.BaseRef = textField(object, "base_ref", "base", "base_identity")
			view.SourceRef = textField(object, "source_ref", "source", "source_identity")
			view.TargetRef = textField(object, "target_ref", "target", "target_identity")
			if values, ok := sliceField(object, "files", "changes"); ok {
				view.Files = decodeDiffFiles(values)
			}
		}
	}
	if len(view.Files) == 0 {
		path := block.ObjectRef
		if path == "" {
			path = block.Title
		}
		if path == "" {
			path = "workspace diff"
		}
		view.Files = []diffFile{{Path: path, Status: "changed", Hunks: []diffHunk{{Header: "projected diff", Body: block.Preview}}}}
	}
	return view
}

func decodeDiffFiles(values []any) []diffFile {
	files := make([]diffFile, 0, len(values))
	for _, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		file := diffFile{
			Path: textField(object, "path", "file", "name"),
			Status: textField(object, "status", "state"),
			Additions: intField(object, "additions", "added"),
			Deletions: intField(object, "deletions", "removed"),
		}
		if file.Status == "" {
			file.Status = "changed"
		}
		if values, ok := sliceField(object, "hunks", "patches"); ok {
			for _, rawHunk := range values {
				switch hunk := rawHunk.(type) {
				case string:
					file.Hunks = append(file.Hunks, diffHunk{Header: "hunk", Body: hunk})
				case map[string]any:
					file.Hunks = append(file.Hunks, diffHunk{
						Header: textField(hunk, "header", "range", "title"),
						Body: textField(hunk, "body", "patch", "diff", "text"),
					})
				}
			}
		}
		if file.Path != "" {
			files = append(files, file)
		}
	}
	return files
}

func unwrapObject(value any, key string) any {
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if nested, ok := object[key]; ok {
		return nested
	}
	return value
}

func textField(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return typed
				}
			case json.Number:
				return typed.String()
			case float64:
				return fmt.Sprintf("%.0f", typed)
			}
		}
	}
	return ""
}

func sliceField(object map[string]any, keys ...string) ([]any, bool) {
	for _, key := range keys {
		if value, ok := object[key].([]any); ok {
			return value, true
		}
	}
	return nil, false
}

func intField(object map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := object[key].(type) {
		case float64:
			return int(value)
		case json.Number:
			parsed, _ := value.Int64()
			return int(parsed)
		}
	}
	return 0
}

func reviewReference(kind, blockID, objectRef string) string {
	ref := blockID
	if objectRef != "" {
		ref = objectRef
	}
	if ref == "" {
		ref = "latest"
	}
	return kind + ":" + ref
}
