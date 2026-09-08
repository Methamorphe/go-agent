package world

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
)

type captureOCIRunner struct {
	executable string
	args       []string
}

func (r *captureOCIRunner) Run(_ context.Context, executable string, args []string) ([]byte, int, error) {
	r.executable = executable
	r.args = append([]string(nil), args...)
	return []byte("ok"), 0, nil
}

type staticSecretResolver struct{ value []byte }

func (r staticSecretResolver) ResolveSecret(context.Context, string) ([]byte, error) {
	return append([]byte(nil), r.value...), nil
}

func TestOCIWorldUsesRestrictedDefaultsAndOpaqueSecrets(t *testing.T) {
	runner := &captureOCIRunner{}
	secret := "super-secret-value"
	workspace := t.TempDir()
	oci, err := NewOCIWorld(OCIConfig{
		WorldID:       id.WorldID("wld_oci"),
		Image:         "example.invalid/agent:sha256-test",
		Workspace:     workspace,
		CPUs:          1.5,
		MemoryBytes:   256 * 1024 * 1024,
		PIDs:          64,
		Secrets:       []OCISecretBinding{{Ref: "secret/api", Target: "/run/secrets/api"}},
		SecretResolver: staticSecretResolver{value: []byte(secret)},
		Runner:         runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"executable": "sh", "args": []string{"-lc", "echo ok"}})
	result, err := oci.Execute(context.Background(), Action{Kind: "process.exec", Params: params, Effect: CanonicalEffect("process.exec")})
	if err != nil || result.Status != ResultSucceeded {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	joined := strings.Join(runner.args, " ")
	for _, required := range []string{"--network none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--cpus 1.5", "--memory 268435456", "--pids-limit 64"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("runtime args missing %q: %s", required, joined)
		}
	}
	if strings.Contains(joined, secret) {
		t.Fatal("secret plaintext leaked into OCI command arguments")
	}
	profile := oci.Profile()
	if profile.Network != NetworkNone || profile.EnforcementLevel != EnforcementIsolated || profile.Snapshot.Supported || profile.Fork.Supported {
		t.Fatalf("oci profile overclaims guarantees: %+v", profile)
	}
}

func TestOCIWorldRejectsHostControlSocketMount(t *testing.T) {
	_, err := NewOCIWorld(OCIConfig{
		WorldID: id.WorldID("wld_socket"),
		Image:   "example.invalid/agent:test",
		Mounts:  []OCIMount{{Source: "/var/run/docker.sock", Target: "/var/run/docker.sock"}},
	})
	if err == nil || !strings.Contains(err.Error(), "control socket") {
		t.Fatalf("socket mount err=%v", err)
	}
}

func TestOCIWorldRequiresExplicitNetworkOptIn(t *testing.T) {
	_, err := NewOCIWorld(OCIConfig{WorldID: id.WorldID("wld_network"), Image: "example.invalid/agent:test", Network: NetworkFullOutbound})
	if err == nil || !strings.Contains(err.Error(), "explicit opt-in") {
		t.Fatalf("network opt-in err=%v", err)
	}
}

func TestOCIWorldFailsUnsupportedActionWithoutRunner(t *testing.T) {
	runner := &captureOCIRunner{}
	oci, err := NewOCIWorld(OCIConfig{WorldID: id.WorldID("wld_action"), Image: "example.invalid/agent:test", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	_, err = oci.Execute(context.Background(), Action{Kind: "fs.write_file", Effect: CanonicalEffect("fs.write_file")})
	if err == nil {
		t.Fatal("unsupported action succeeded")
	}
	if runner.executable != "" {
		t.Fatal("unsupported action reached OCI runtime")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
}
