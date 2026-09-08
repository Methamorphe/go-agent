package world

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

type OCIMount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

type OCISecretBinding struct {
	Ref    string `json:"ref"`
	Target string `json:"target"`
}

type SecretResolver interface {
	ResolveSecret(context.Context, string) ([]byte, error)
}

type OCICommandRunner interface {
	Run(context.Context, string, []string) ([]byte, int, error)
}

type OCIConfig struct {
	WorldID       id.WorldID
	Runtime       string
	Image         string
	Workspace     string
	WorkspacePath string
	Mounts        []OCIMount
	Network       NetworkMode
	AllowNetwork  bool
	CPUs          float64
	MemoryBytes   int64
	PIDs          int
	WallTime      time.Duration
	WritableRoot  bool
	User          string
	Secrets       []OCISecretBinding
	SecretResolver SecretResolver
	Runner         OCICommandRunner
}

type OCIWorld struct {
	id             id.WorldID
	runtime        string
	image          string
	workspace      string
	workspacePath  string
	mounts         []OCIMount
	network        NetworkMode
	cpus           float64
	memoryBytes    int64
	pids           int
	wallTime       time.Duration
	readOnlyRoot   bool
	user           string
	secrets        []OCISecretBinding
	secretResolver SecretResolver
	runner          OCICommandRunner
}

func NewOCIWorld(cfg OCIConfig) (*OCIWorld, error) {
	if cfg.WorldID == "" {
		return nil, fmt.Errorf("oci world id is required")
	}
	if strings.TrimSpace(cfg.Image) == "" {
		return nil, fmt.Errorf("oci image is required")
	}
	runtimeName := strings.TrimSpace(cfg.Runtime)
	if runtimeName == "" {
		runtimeName = "docker"
	}
	if runtimeName != "docker" && runtimeName != "podman" {
		return nil, fmt.Errorf("unsupported oci runtime %q", runtimeName)
	}
	network := cfg.Network
	if network == "" {
		network = NetworkNone
	}
	if network != NetworkNone && !cfg.AllowNetwork {
		return nil, fmt.Errorf("oci network access requires explicit opt-in")
	}
	if network != NetworkNone && network != NetworkFullOutbound {
		return nil, fmt.Errorf("oci network mode %q is not enforceable by the v0 adapter", network)
	}
	if cfg.CPUs < 0 || cfg.MemoryBytes < 0 || cfg.PIDs < 0 || cfg.WallTime < 0 {
		return nil, fmt.Errorf("oci resource limits must be non-negative")
	}

	workspace := strings.TrimSpace(cfg.Workspace)
	workspacePath := strings.TrimSpace(cfg.WorkspacePath)
	if workspacePath == "" {
		workspacePath = "/workspace"
	}
	if workspace != "" {
		abs, err := filepath.Abs(filepath.Clean(workspace))
		if err != nil {
			return nil, fmt.Errorf("normalize oci workspace: %w", err)
		}
		workspace = abs
		if err := validateOCIMount(OCIMount{Source: workspace, Target: workspacePath}); err != nil {
			return nil, err
		}
	}
	mounts := append([]OCIMount(nil), cfg.Mounts...)
	for i := range mounts {
		abs, err := filepath.Abs(filepath.Clean(mounts[i].Source))
		if err != nil {
			return nil, fmt.Errorf("normalize oci mount: %w", err)
		}
		mounts[i].Source = abs
		if err := validateOCIMount(mounts[i]); err != nil {
			return nil, err
		}
	}
	for _, secret := range cfg.Secrets {
		if strings.TrimSpace(secret.Ref) == "" || !strings.HasPrefix(filepath.ToSlash(secret.Target), "/run/secrets/") {
			return nil, fmt.Errorf("oci secret bindings require a ref and /run/secrets target")
		}
	}
	if len(cfg.Secrets) > 0 && cfg.SecretResolver == nil {
		return nil, fmt.Errorf("oci secret resolver is required")
	}
	runner := cfg.Runner
	if runner == nil {
		runner = execOCICommandRunner{}
	}
	wallTime := cfg.WallTime
	if wallTime == 0 {
		wallTime = 5 * time.Minute
	}
	return &OCIWorld{
		id:             cfg.WorldID,
		runtime:        runtimeName,
		image:          cfg.Image,
		workspace:      workspace,
		workspacePath:  workspacePath,
		mounts:         mounts,
		network:        network,
		cpus:           cfg.CPUs,
		memoryBytes:    cfg.MemoryBytes,
		pids:           cfg.PIDs,
		wallTime:       wallTime,
		readOnlyRoot:   !cfg.WritableRoot,
		user:           strings.TrimSpace(cfg.User),
		secrets:        append([]OCISecretBinding(nil), cfg.Secrets...),
		secretResolver: cfg.SecretResolver,
		runner:          runner,
	}, nil
}

func (w *OCIWorld) Profile() Profile {
	return Profile{
		Name:             "oci",
		Type:             TypeOCI,
		EnforcementLevel: EnforcementIsolated,
		Filesystem: FilesystemGuarantees{
			WorldRelativeRoot: true,
		},
		Network: w.network,
		Secrets: SecretGuarantees{
			ExplicitBinding: w.secretResolver != nil,
			ModelOpaque:     w.secretResolver != nil,
		},
		Snapshot:  SnapshotGuarantees{Supported: false},
		Fork:      ForkGuarantees{Supported: false},
		Promotion: PromotionGuarantees{Supported: false},
		ResourceLimits: ResourceLimitGuarantees{
			WallTime: true,
			CPU:      w.cpus > 0,
			Memory:   w.memoryBytes > 0,
			PIDs:     w.pids > 0,
		},
		ProfileVersion:       1,
		SupportsCancellation: true,
	}
}

func (w *OCIWorld) Execute(ctx context.Context, action Action) (Result, error) {
	if action.Kind != "process.exec" {
		err := fmt.Errorf("oci world supports process.exec only, got %q", action.Kind)
		return Result{Status: ResultFailed, Error: err.Error()}, err
	}
	var params struct {
		Executable string   `json:"executable"`
		Args       []string `json:"args"`
		Cwd        string   `json:"cwd"`
		TimeoutMS  int      `json:"timeout_ms"`
	}
	if err := json.Unmarshal(action.Params, &params); err != nil {
		return Result{Status: ResultFailed, Error: err.Error()}, err
	}
	if strings.TrimSpace(params.Executable) == "" {
		err := fmt.Errorf("oci process executable is required")
		return Result{Status: ResultFailed, Error: err.Error()}, err
	}

	args, cleanup, err := w.commandArgs(ctx, params.Cwd, params.Executable, params.Args)
	if err != nil {
		return Result{Status: ResultFailed, Error: err.Error()}, err
	}
	defer cleanup()

	timeout := w.wallTime
	if params.TimeoutMS > 0 {
		requested := time.Duration(params.TimeoutMS) * time.Millisecond
		if requested < timeout {
			timeout = requested
		}
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, exitCode, err := w.runner.Run(commandCtx, w.runtime, args)
	if err != nil {
		return Result{Status: ResultFailed, Data: output, ExitCode: &exitCode, Error: err.Error()}, err
	}
	return Result{Status: ResultSucceeded, Data: output, ExitCode: &exitCode}, nil
}

func (w *OCIWorld) commandArgs(ctx context.Context, cwd, executable string, commandArgs []string) ([]string, func(), error) {
	args := []string{"run", "--rm", "--network", runtimeNetworkMode(w.network), "--cap-drop=ALL", "--security-opt=no-new-privileges"}
	if w.readOnlyRoot {
		args = append(args, "--read-only")
	}
	if w.cpus > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(w.cpus, 'f', -1, 64))
	}
	if w.memoryBytes > 0 {
		args = append(args, "--memory", strconv.FormatInt(w.memoryBytes, 10))
	}
	if w.pids > 0 {
		args = append(args, "--pids-limit", strconv.Itoa(w.pids))
	}
	if w.user != "" {
		args = append(args, "--user", w.user)
	}
	if w.workspace != "" {
		args = append(args, "--mount", renderOCIMount(OCIMount{Source: w.workspace, Target: w.workspacePath}))
	}
	for _, mount := range w.mounts {
		args = append(args, "--mount", renderOCIMount(mount))
	}

	cleanup := func() {}
	if len(w.secrets) > 0 {
		directory, err := os.MkdirTemp("", "go-agent-oci-secrets-")
		if err != nil {
			return nil, cleanup, fmt.Errorf("create oci secret directory: %w", err)
		}
		cleanup = func() { _ = os.RemoveAll(directory) }
		for i, binding := range w.secrets {
			value, err := w.secretResolver.ResolveSecret(ctx, binding.Ref)
			if err != nil {
				cleanup()
				return nil, func() {}, fmt.Errorf("resolve oci secret %q: %w", binding.Ref, err)
			}
			path := filepath.Join(directory, "secret-"+strconv.Itoa(i))
			if err := os.WriteFile(path, value, 0o600); err != nil {
				cleanup()
				return nil, func() {}, fmt.Errorf("write oci secret binding: %w", err)
			}
			args = append(args, "--mount", renderOCIMount(OCIMount{Source: path, Target: binding.Target, ReadOnly: true}))
		}
	}

	containerCWD, err := w.containerCWD(cwd)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if containerCWD != "" {
		args = append(args, "--workdir", containerCWD)
	}
	args = append(args, w.image, executable)
	args = append(args, commandArgs...)
	return args, cleanup, nil
}

func (w *OCIWorld) containerCWD(cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		if w.workspace != "" {
			return filepath.ToSlash(w.workspacePath), nil
		}
		return "", nil
	}
	if w.workspace == "" {
		if !strings.HasPrefix(filepath.ToSlash(cwd), "/") {
			return "", fmt.Errorf("relative oci cwd requires a workspace mount")
		}
		return filepath.ToSlash(filepath.Clean(cwd)), nil
	}
	if strings.HasPrefix(filepath.ToSlash(cwd), "/") {
		clean := filepath.ToSlash(filepath.Clean(cwd))
		root := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(w.workspacePath)), "/")
		if clean != root && !strings.HasPrefix(clean, root+"/") {
			return "", fmt.Errorf("oci cwd escapes workspace")
		}
		return clean, nil
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.Join(w.workspacePath, cwd)))
	root := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(w.workspacePath)), "/")
	if clean != root && !strings.HasPrefix(clean, root+"/") {
		return "", fmt.Errorf("oci cwd escapes workspace")
	}
	return clean, nil
}

func validateOCIMount(mount OCIMount) error {
	if strings.TrimSpace(mount.Source) == "" || strings.TrimSpace(mount.Target) == "" {
		return fmt.Errorf("oci mount source and target are required")
	}
	if !strings.HasPrefix(filepath.ToSlash(mount.Target), "/") {
		return fmt.Errorf("oci mount target must be absolute")
	}
	if isHostControlSocket(mount.Source) || isHostControlSocket(mount.Target) {
		return fmt.Errorf("oci host control socket mount is forbidden by default")
	}
	return nil
}

func isHostControlSocket(path string) bool {
	name := strings.ToLower(filepath.Base(filepath.Clean(path)))
	switch name {
	case "docker.sock", "podman.sock", "containerd.sock", "crio.sock":
		return true
	default:
		return false
	}
}

func renderOCIMount(mount OCIMount) string {
	parts := []string{"type=bind", "src=" + mount.Source, "dst=" + filepath.ToSlash(mount.Target)}
	if mount.ReadOnly {
		parts = append(parts, "ro")
	}
	return strings.Join(parts, ",")
}

func runtimeNetworkMode(mode NetworkMode) string {
	if mode == NetworkNone {
		return "none"
	}
	return "bridge"
}

type execOCICommandRunner struct{}

func (execOCICommandRunner) Run(ctx context.Context, executable string, args []string) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return output, 0, nil
	}
	code := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	}
	return output, code, err
}
