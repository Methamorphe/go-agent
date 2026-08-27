package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/control"
	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/id"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	global := flag.NewFlagSet("go-agentctl", flag.ContinueOnError)
	global.SetOutput(os.Stderr)

	var dataDir string
	var controlAddress string
	global.StringVar(&dataDir, "data-dir", "", "runtime data directory")
	global.StringVar(&controlAddress, "control-address", "", "unix socket or Windows named pipe")

	if err := global.Parse(args); err != nil {
		return 2
	}

	visited := map[string]bool{}
	global.Visit(func(value *flag.Flag) { visited[value.Name] = true })
	overrides := config.Overrides{}
	if visited["data-dir"] {
		overrides.DataDir = &dataDir
	}
	if visited["control-address"] {
		overrides.ControlAddress = &controlAddress
	}

	cfg, err := config.Load("", overrides)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := control.NewClient(cfg.ControlAddress, cfg.MaxFrameBytes, id.NewGenerator())
	command := global.Args()
	if len(command) == 0 {
		fmt.Fprintln(os.Stderr, "usage: go-agentctl [global flags] <run|attach> ...")
		return 2
	}

	switch command[0] {
	case "run":
		if err := runAgent(ctx, client, command[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
	case "attach":
		if err := attachAgent(ctx, client, command[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command[0])
		return 2
	}

	return 0
}

func runAgent(ctx context.Context, client *control.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-agentctl run <agent-id> --model <model> [flags]")
	}

	agentID := id.AgentID(args[0])
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	var providerName string
	var model string
	var baseURL string
	var apiKeyEnv string
	var workspace string
	var maxSteps int
	flags.StringVar(&providerName, "provider", "openai-compatible", "openai or openai-compatible")
	flags.StringVar(&model, "model", "", "model identifier")
	flags.StringVar(&baseURL, "base-url", "", "provider API base URL")
	flags.StringVar(&apiKeyEnv, "api-key-env", "OPENAI_API_KEY", "API key environment variable read by the daemon")
	flags.StringVar(&workspace, "workspace", ".", "workspace root")
	flags.IntVar(&maxSteps, "max-steps", 24, "maximum model steps")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if model == "" {
		return fmt.Errorf("--model is required")
	}

	workspace, err := filepath.Abs(filepath.Clean(workspace))
	if err != nil {
		return fmt.Errorf("normalize workspace: %w", err)
	}

	request := controlapi.AgentRunRequest{
		AgentID:   agentID,
		Provider:  providerName,
		Model:     model,
		BaseURL:   baseURL,
		APIKeyEnv: apiKeyEnv,
		Workspace: workspace,
		MaxSteps:  maxSteps,
	}
	var response controlapi.AgentRunResponse
	return client.Call(ctx, controlapi.MessageAgentRun, request, &response)
}

func attachAgent(ctx context.Context, client *control.Client, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: go-agentctl attach <agent-id>")
	}

	agentID := id.AgentID(args[0])
	var cursor int64
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var response controlapi.AgentLiveResponse
		err := client.Call(
			requestCtx,
			controlapi.MessageAgentLive,
			controlapi.AgentLiveRequest{AgentID: agentID},
			&response,
		)
		cancel()
		if err != nil {
			return err
		}

		stream := response.Stream
		if cursor < stream.StartOffset {
			fmt.Fprintln(os.Stdout, "\n[... live stream gap; durable final artifact is retained ...]")
			cursor = stream.StartOffset
		}
		if cursor < stream.EndOffset {
			data := []byte(stream.Data)
			start := cursor - stream.StartOffset
			if start >= 0 && start <= int64(len(data)) {
				if _, err := os.Stdout.Write(data[start:]); err != nil {
					return err
				}
			}
			cursor = stream.EndOffset
		}

		switch stream.Status {
		case "completed":
			fmt.Fprintln(os.Stdout)
			return nil
		case "failed":
			return fmt.Errorf("agent failed: %s", stream.Error)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
