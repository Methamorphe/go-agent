package world

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type LocalWorld struct {
	root string
}

func NewLocalWorld(root string) (*LocalWorld, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("world root is required")
	}
	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &LocalWorld{root: resolved}, nil
}

func (w *LocalWorld) Profile() Profile {
	return Profile{Name: "local", SupportsStreaming: true, SupportsCancellation: true, SupportsProcessTreeKill: true}
}

func (w *LocalWorld) Execute(ctx context.Context, action Action) (Result, error) {
	switch action.Kind {
	case "fs.read_file":
		path, err := w.resolve(action.Resource, true)
		if err != nil {
			return Result{Status: ResultFailed, Error: err.Error()}, err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return Result{Status: ResultFailed, Error: err.Error()}, err
		}
		return Result{Status: ResultSucceeded, Data: body}, nil
	case "fs.list_directory":
		path, err := w.resolve(action.Resource, true)
		if err != nil {
			return Result{Status: ResultFailed, Error: err.Error()}, err
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return Result{Status: ResultFailed, Error: err.Error()}, err
		}
		data, err := json.Marshal(entriesToNames(entries))
		if err != nil {
			return Result{Status: ResultFailed, Error: err.Error()}, err
		}
		return Result{Status: ResultSucceeded, Data: data}, nil
	case "fs.write_file":
		path, err := w.resolve(action.Resource, false)
		if err != nil {
			return Result{Status: ResultFailed, Error: err.Error()}, err
		}
		var params struct{ Content string `json:"content"` }
		if err := json.Unmarshal(action.Params, &params); err != nil {
			return Result{Status: ResultFailed, Error: err.Error()}, err
		}
		if err := os.WriteFile(path, []byte(params.Content), 0o644); err != nil {
			return Result{Status: ResultFailed, Error: err.Error()}, err
		}
		return Result{Status: ResultSucceeded}, nil
	case "process.exec":
		var params struct {
			Executable string   `json:"executable"`
			Args       []string `json:"args"`
			Cwd        string   `json:"cwd"`
			TimeoutMS  int      `json:"timeout_ms"`
		}
		if err := json.Unmarshal(action.Params, &params); err != nil {
			return Result{Status: ResultFailed, Error: err.Error()}, err
		}
		cwd := w.root
		if params.Cwd != "" {
			var err error
			cwd, err = w.resolve(params.Cwd, true)
			if err != nil {
				return Result{Status: ResultFailed, Error: err.Error()}, err
			}
		}
		timeout := 30 * time.Second
		if params.TimeoutMS > 0 {
			timeout = time.Duration(params.TimeoutMS) * time.Millisecond
		}
		commandCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		cmd := exec.CommandContext(commandCtx, params.Executable, params.Args...)
		cmd.Dir = cwd
		configureProcessTree(cmd)
		output, err := cmd.CombinedOutput()
		if commandCtx.Err() != nil {
			_ = killProcessTree(cmd)
		}
		code := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			}
			return Result{Status: ResultFailed, Data: output, ExitCode: &code, Error: err.Error()}, err
		}
		return Result{Status: ResultSucceeded, Data: output, ExitCode: &code}, nil
	default:
		return Result{Status: ResultFailed, Error: "unsupported action"}, fmt.Errorf("unsupported action %q", action.Kind)
	}
}

func (w *LocalWorld) resolve(resource string, mustExist bool) (string, error) {
	path := resource
	if !filepath.IsAbs(path) {
		path = filepath.Join(w.root, path)
	}
	path = filepath.Clean(path)
	if mustExist {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		path = resolved
	}
	rel, err := filepath.Rel(w.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resource escapes world root")
	}
	return path, nil
}

func entriesToNames(entries []os.DirEntry) []string {
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Name())
	}
	return result
}
