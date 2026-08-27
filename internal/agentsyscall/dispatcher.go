package agentsyscall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/bounded"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
)

const (
	DefaultPreviewBytes   = 16 << 10
	DefaultMaxOutputBytes = 64 << 20
	DefaultCommandTimeout = 30 * time.Second
	MaximumCommandTimeout = 10 * time.Minute
)

type IDGenerator interface {
	Action() (id.ActionID, error)
}

type ProcessAPI interface {
	SyscallRequested(context.Context, id.AgentID, *uint64, process.SyscallRequestedPayload, process.CommandMeta) (process.State, error)
	SyscallCompleted(context.Context, id.AgentID, *uint64, process.SyscallCompletedPayload, process.CommandMeta) (process.State, error)
	SyscallFailed(context.Context, id.AgentID, *uint64, process.SyscallFailedPayload, process.CommandMeta) (process.State, error)
	Checkpoint(context.Context, id.AgentID, *uint64, process.CommandMeta) (process.State, error)
}

type ObjectStore interface {
	Put(context.Context, io.Reader) (objectstore.Meta, error)
}

type Dispatcher struct {
	processes ProcessAPI
	objects   ObjectStore
	ids       IDGenerator
	root      string
	preview   int
	maxOutput int64
}

type Result struct {
	ActionID   id.ActionID `json:"action_id"`
	OK         bool        `json:"ok"`
	Kind       string      `json:"kind"`
	ContentRef string      `json:"content_ref,omitempty"`
	StdoutRef  string      `json:"stdout_ref,omitempty"`
	StderrRef  string      `json:"stderr_ref,omitempty"`
	Preview    string      `json:"preview,omitempty"`
	Stdout     string      `json:"stdout_preview,omitempty"`
	Stderr     string      `json:"stderr_preview,omitempty"`
	Truncated  bool        `json:"truncated,omitempty"`
	ExitCode   *int        `json:"exit_code,omitempty"`
	Checkpoint string      `json:"checkpoint_id,omitempty"`
	Error      string      `json:"error,omitempty"`
}

var tools = []provider.ToolDefinition{
	{
		Name:        "observe",
		Description: "Read a file or list a directory inside the configured workspace.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"operation":{"type":"string","enum":["read_file","list_directory"]},"path":{"type":"string"}},"required":["operation","path"],"additionalProperties":false}`),
	},
	{
		Name:        "execute",
		Description: "Run one executable with argv in the configured workspace. This is argv execution, not an implicit shell.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"executable":{"type":"string"},"args":{"type":"array","items":{"type":"string"}},"cwd":{"type":"string"},"timeout_ms":{"type":"integer","minimum":1,"maximum":600000}},"required":["executable"],"additionalProperties":false}`),
	},
	{
		Name:        "checkpoint",
		Description: "Create a durable Agent Process checkpoint marker.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	},
}

func Tools() []provider.ToolDefinition {
	return append([]provider.ToolDefinition(nil), tools...)
}

func New(processes ProcessAPI, objects ObjectStore, ids IDGenerator, root string) (*Dispatcher, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errs.New(errs.CodeInvalidArgument, "syscall.new", "workspace root is required")
	}
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidArgument, "syscall.new", "normalize workspace root", err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidArgument, "syscall.new", "resolve workspace root", err)
	}
	return &Dispatcher{
		processes: processes,
		objects:   objects,
		ids:       ids,
		root:      resolved,
		preview:   DefaultPreviewBytes,
		maxOutput: DefaultMaxOutputBytes,
	}, nil
}

func (d *Dispatcher) Call(
	ctx context.Context,
	state process.State,
	call provider.ToolCall,
	meta process.CommandMeta,
) (Result, process.State, error) {
	if err := call.Validate(); err != nil {
		return Result{}, state, err
	}

	actionID, err := d.ids.Action()
	if err != nil {
		return Result{}, state, errs.Wrap(errs.CodeInternal, "syscall.call", "generate action id", err)
	}
	argumentsObject, err := d.objects.Put(ctx, bytes.NewReader(call.Arguments))
	if err != nil {
		return Result{}, state, err
	}

	expected := state.Version
	started, err := d.processes.SyscallRequested(
		ctx,
		state.AgentID,
		&expected,
		process.SyscallRequestedPayload{
			ActionID:     actionID,
			Name:         call.Name,
			ArgumentsRef: string(argumentsObject.Ref),
		},
		meta,
	)
	if err != nil {
		return Result{}, state, err
	}

	result := Result{ActionID: actionID, Kind: call.Name}
	switch call.Name {
	case "observe":
		result, err = d.observe(ctx, actionID, call.Arguments)
	case "execute":
		result, err = d.execute(ctx, actionID, call.Arguments)
	case "checkpoint":
		result, started, err = d.checkpoint(ctx, started, actionID, call.Arguments, meta)
	default:
		err = errs.New(errs.CodeNotFound, "syscall.call", "unknown syscall")
	}

	if err != nil {
		message := err.Error()
		if len(message) > 2048 {
			message = message[:2048]
		}
		result = Result{ActionID: actionID, OK: false, Kind: call.Name, Error: message}
		expected = started.Version
		failed, recordErr := d.processes.SyscallFailed(
			ctx,
			state.AgentID,
			&expected,
			process.SyscallFailedPayload{ActionID: actionID, Name: call.Name, Failure: message},
			meta,
		)
		if recordErr != nil {
			return result, started, errors.Join(err, recordErr)
		}
		return result, failed, nil
	}

	body, err := json.Marshal(result)
	if err != nil {
		return Result{}, started, errs.Wrap(errs.CodeInternal, "syscall.call", "encode result", err)
	}
	resultObject, err := d.objects.Put(ctx, bytes.NewReader(body))
	if err != nil {
		return Result{}, started, err
	}

	expected = started.Version
	completed, err := d.processes.SyscallCompleted(
		ctx,
		state.AgentID,
		&expected,
		process.SyscallCompletedPayload{
			ActionID:  actionID,
			Name:      call.Name,
			Status:    syscallStatus(result),
			ResultRef: string(resultObject.Ref),
		},
		meta,
	)
	if err != nil {
		return result, started, err
	}
	return result, completed, nil
}

func syscallStatus(result Result) string {
	if result.OK {
		return "known_succeeded"
	}
	return "known_failed"
}

type observeArgs struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
}

func (d *Dispatcher) observe(ctx context.Context, actionID id.ActionID, raw json.RawMessage) (Result, error) {
	var args observeArgs
	if err := strictJSON(raw, &args); err != nil {
		return Result{}, err
	}
	path, err := d.resolveExisting(args.Path)
	if err != nil {
		return Result{}, err
	}

	switch args.Operation {
	case "read_file":
		return d.readFile(ctx, actionID, path)
	case "list_directory":
		return d.listDirectory(ctx, actionID, path)
	default:
		return Result{}, errs.New(errs.CodeInvalidArgument, "syscall.observe", "operation must be read_file or list_directory")
	}
}

func (d *Dispatcher) readFile(ctx context.Context, actionID id.ActionID, path string) (Result, error) {
	file, err := os.Open(path)
	if err != nil {
		return Result{}, errs.Wrap(errs.CodeNotFound, "syscall.read_file", "open file", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Result{}, errs.Wrap(errs.CodeUnavailable, "syscall.read_file", "stat file", err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, errs.New(errs.CodeInvalidArgument, "syscall.read_file", "path is not a regular file")
	}

	preview := bounded.NewPreview(d.preview)
	object, err := d.objects.Put(ctx, io.TeeReader(file, preview))
	if err != nil {
		return Result{}, err
	}
	return Result{
		ActionID:   actionID,
		OK:         true,
		Kind:       "observe.read_file",
		ContentRef: string(object.Ref),
		Preview:    preview.String(),
		Truncated:  preview.Truncated(),
	}, nil
}

type directoryEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
	Mode  string `json:"mode"`
}

func (d *Dispatcher) listDirectory(ctx context.Context, actionID id.ActionID, path string) (Result, error) {
	dir, err := os.Open(path)
	if err != nil {
		return Result{}, errs.Wrap(errs.CodeNotFound, "syscall.list_directory", "open directory", err)
	}
	defer dir.Close()

	preview := bounded.NewPreview(d.preview)
	reader, writer := io.Pipe()
	type stored struct {
		meta objectstore.Meta
		err  error
	}
	storedCh := make(chan stored, 1)
	persistCtx, cancelPersist := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelPersist()
	go func() {
		meta, putErr := d.objects.Put(persistCtx, reader)
		_ = reader.CloseWithError(putErr)
		storedCh <- stored{meta: meta, err: putErr}
	}()

	encoder := json.NewEncoder(io.MultiWriter(writer, preview))
	var produceErr error
	for {
		entries, readErr := dir.Readdir(256)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				produceErr = err
				break
			}
			item := directoryEntry{Name: entry.Name(), IsDir: entry.IsDir(), Size: entry.Size(), Mode: entry.Mode().String()}
			if err := encoder.Encode(item); err != nil {
				produceErr = err
				break
			}
		}
		if produceErr != nil || readErr == io.EOF {
			break
		}
		if readErr != nil {
			produceErr = readErr
			break
		}
	}
	_ = writer.CloseWithError(produceErr)
	persisted := <-storedCh
	_ = reader.Close()
	if produceErr != nil {
		return Result{}, produceErr
	}
	if persisted.err != nil {
		return Result{}, persisted.err
	}
	return Result{
		ActionID:   actionID,
		OK:         true,
		Kind:       "observe.list_directory",
		ContentRef: string(persisted.meta.Ref),
		Preview:    preview.String(),
		Truncated:  preview.Truncated(),
	}, nil
}

type executeArgs struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
	Cwd        string   `json:"cwd,omitempty"`
	TimeoutMS  int      `json:"timeout_ms,omitempty"`
}

func (d *Dispatcher) execute(ctx context.Context, actionID id.ActionID, raw json.RawMessage) (Result, error) {
	var args executeArgs
	if err := strictJSON(raw, &args); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(args.Executable) == "" {
		return Result{}, errs.New(errs.CodeInvalidArgument, "syscall.execute", "executable is required")
	}

	cwd := d.root
	var err error
	if args.Cwd != "" {
		cwd, err = d.resolveExisting(args.Cwd)
		if err != nil {
			return Result{}, err
		}
	}

	timeout := DefaultCommandTimeout
	if args.TimeoutMS > 0 {
		timeout = time.Duration(args.TimeoutMS) * time.Millisecond
	}
	if timeout > MaximumCommandTimeout {
		return Result{}, errs.New(errs.CodeInvalidArgument, "syscall.execute", "timeout exceeds maximum")
	}

	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, args.Executable, args.Args...)
	cmd.Dir = cwd

	stdout := newCapture(d.objects, d.preview, d.maxOutput, cancel)
	stderr := newCapture(d.objects, d.preview, d.maxOutput, cancel)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)
	stdoutMeta, stdoutErr := stdout.Close()
	stderrMeta, stderrErr := stderr.Close()
	if stdoutErr != nil || stderrErr != nil {
		return Result{}, errors.Join(stdoutErr, stderrErr)
	}

	result := Result{
		ActionID:  actionID,
		OK:        true,
		Kind:      "execute.run_command",
		StdoutRef: string(stdoutMeta.Ref),
		StderrRef: string(stderrMeta.Ref),
		Stdout:    stdout.Preview(),
		Stderr:    stderr.Preview(),
		Truncated: stdout.Truncated() || stderr.Truncated(),
	}

	if stdout.Exceeded() || stderr.Exceeded() {
		result.OK = false
		result.Error = fmt.Sprintf("command output exceeded %d bytes per stream", d.maxOutput)
	}
	if commandCtx.Err() == context.DeadlineExceeded {
		result.OK = false
		result.Error = fmt.Sprintf("command timed out after %s", timeout)
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			code := exitErr.ExitCode()
			result.ExitCode = &code
			if result.Error == "" {
				result.Error = "command exited with non-zero status"
			}
			result.OK = false
		} else if result.Error == "" {
			result.OK = false
			result.Error = runErr.Error()
		}
	} else {
		code := 0
		result.ExitCode = &code
	}

	if result.Error != "" {
		result.Error = fmt.Sprintf("%s (duration %s)", result.Error, duration.Round(time.Millisecond))
	}
	return result, nil
}

func (d *Dispatcher) checkpoint(
	ctx context.Context,
	state process.State,
	actionID id.ActionID,
	raw json.RawMessage,
	meta process.CommandMeta,
) (Result, process.State, error) {
	var empty struct{}
	if err := strictJSON(raw, &empty); err != nil {
		return Result{}, state, err
	}
	expected := state.Version
	next, err := d.processes.Checkpoint(ctx, state.AgentID, &expected, meta)
	if err != nil {
		return Result{}, state, err
	}
	return Result{
		ActionID:   actionID,
		OK:         true,
		Kind:       "checkpoint",
		Checkpoint: next.LastCheckpointID.String(),
	}, next, nil
}

func (d *Dispatcher) resolveExisting(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(d.root, candidate)
	}
	candidate, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", errs.Wrap(errs.CodeInvalidArgument, "syscall.resolve", "normalize path", err)
	}
	candidate, err = filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", errs.Wrap(errs.CodeNotFound, "syscall.resolve", "resolve path", err)
	}
	rel, err := filepath.Rel(d.root, candidate)
	if err != nil {
		return "", errs.Wrap(errs.CodeInvalidArgument, "syscall.resolve", "relativize path", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errs.New(errs.CodePermissionDenied, "syscall.resolve", "path escapes workspace root")
	}
	return candidate, nil
}

func strictJSON(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errs.Wrap(errs.CodeInvalidArgument, "syscall.arguments", "decode arguments", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errs.New(errs.CodeInvalidArgument, "syscall.arguments", "multiple JSON values")
		}
		return errs.Wrap(errs.CodeInvalidArgument, "syscall.arguments", "decode trailing arguments", err)
	}
	return nil
}
