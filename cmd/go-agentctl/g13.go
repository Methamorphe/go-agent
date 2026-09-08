package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Methamorphe/go-agent/internal/agent"
	"github.com/Methamorphe/go-agent/internal/control"
	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

func runG13ControlCommand(ctx context.Context, client *control.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-agentctl <workspace|message|process> ...")
	}
	switch args[0] {
	case "workspace":
		return runWorkspaceCommand(ctx, client, args[1:])
	case "message":
		return runMessageCommand(ctx, client, args[1:])
	case "process":
		return runProcessControlCommand(ctx, client, args[1:])
	default:
		return fmt.Errorf("unknown G13 command %q", args[0])
	}
}

func runWorkspaceCommand(ctx context.Context, client *control.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-agentctl workspace <attach|refresh|history|search> ...")
	}
	switch args[0] {
	case "attach":
		flags := flag.NewFlagSet("workspace attach", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		var agentID string
		var mode string
		var historyLimit int
		var treeLimit int
		flags.StringVar(&agentID, "agent", "", "focused Agent Process; latest root when omitted")
		flags.StringVar(&mode, "mode", "ACT", "ASK|PLAN|ACT|REVIEW|OBSERVE")
		flags.IntVar(&historyLimit, "history-limit", workspace.DefaultPageSize, "bounded history viewport")
		flags.IntVar(&treeLimit, "tree-limit", workspace.DefaultTreeLimit, "bounded process tree projection")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		workspaceMode := workspace.Mode(strings.ToUpper(mode))
		if !workspaceMode.Valid() {
			return fmt.Errorf("invalid workspace mode %q", mode)
		}
		var response controlapi.WorkspaceAttachResponse
		err := client.Call(ctx, controlapi.MessageWorkspaceAttach, controlapi.WorkspaceAttachRequest{
			AgentID: id.AgentID(agentID), Mode: workspaceMode, HistoryLimit: historyLimit, TreeLimit: treeLimit,
		}, &response)
		if err != nil {
			return err
		}
		return writeG13JSON(response)

	case "refresh":
		flags := flag.NewFlagSet("workspace refresh", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		var rootID, focusedID string
		var after uint64
		var limit int
		var inspector bool
		flags.StringVar(&rootID, "root", "", "root Agent ID")
		flags.StringVar(&focusedID, "agent", "", "focused Agent ID")
		flags.Uint64Var(&after, "after", 0, "global presentation sequence cursor")
		flags.IntVar(&limit, "limit", workspace.DefaultPageSize, "maximum update events")
		flags.BoolVar(&inspector, "inspector", false, "include inspector projection")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if rootID == "" || focusedID == "" || flags.NArg() != 0 {
			return fmt.Errorf("workspace refresh requires --root and --agent")
		}
		var response controlapi.WorkspaceRefreshResponse
		err := client.Call(ctx, controlapi.MessageWorkspaceRefresh, controlapi.WorkspaceRefreshRequest{
			RootAgentID: id.AgentID(rootID), FocusedAgentID: id.AgentID(focusedID), AfterSequence: after, Limit: limit, Inspector: inspector,
		}, &response)
		if err != nil {
			return err
		}
		return writeG13JSON(response)

	case "history":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-agentctl workspace history <agent-id> [--before n] [--limit n]")
		}
		agentID := id.AgentID(args[1])
		flags := flag.NewFlagSet("workspace history", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		var before uint64
		var limit int
		flags.Uint64Var(&before, "before", 0, "exclusive process version cursor; 0 means latest")
		flags.IntVar(&limit, "limit", workspace.DefaultPageSize, "bounded page size")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		var response controlapi.WorkspaceHistoryResponse
		err := client.Call(ctx, controlapi.MessageWorkspaceHistory, controlapi.WorkspaceHistoryRequest{AgentID: agentID, Before: before, Limit: limit}, &response)
		if err != nil {
			return err
		}
		return writeG13JSON(response)

	case "search":
		if len(args) < 3 {
			return fmt.Errorf("usage: go-agentctl workspace search <root-agent-id> <query>")
		}
		rootID := id.AgentID(args[1])
		query := strings.TrimSpace(strings.Join(args[2:], " "))
		var response controlapi.WorkspaceSearchResponse
		err := client.Call(ctx, controlapi.MessageWorkspaceSearch, controlapi.WorkspaceSearchRequest{RootAgentID: rootID, Query: query, Limit: 100}, &response)
		if err != nil {
			return err
		}
		return writeG13JSON(response)
	default:
		return fmt.Errorf("unknown workspace command %q", args[0])
	}
}

func runMessageCommand(ctx context.Context, client *control.Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: go-agentctl message <agent-id> <text> [--queue steer|follow_up]")
	}
	agentID := id.AgentID(args[0])
	queue := agent.QueueSteer
	textArgs := args[1:]
	if len(textArgs) >= 2 && textArgs[len(textArgs)-2] == "--queue" {
		queue = agent.MessageQueue(textArgs[len(textArgs)-1])
		textArgs = textArgs[:len(textArgs)-2]
	}
	text := strings.TrimSpace(strings.Join(textArgs, " "))
	if text == "" {
		return fmt.Errorf("message text is required")
	}
	var response controlapi.AgentMessageResponse
	err := client.Call(ctx, controlapi.MessageAgentMessage, controlapi.AgentMessageRequest{AgentID: agentID, Text: text, Queue: queue}, &response)
	if err != nil {
		return err
	}
	return writeG13JSON(response)
}

func runProcessControlCommand(ctx context.Context, client *control.Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: go-agentctl process <suspend|resume> <agent-id> [--expected-version n] [--reason text]")
	}
	operation := args[0]
	agentID := id.AgentID(args[1])
	flags := flag.NewFlagSet("process "+operation, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var expected uint64
	var reason string
	flags.Uint64Var(&expected, "expected-version", 0, "expected canonical process version")
	flags.StringVar(&reason, "reason", "go-agentctl_operator", "operator reason")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	var expectedPtr *uint64
	flags.Visit(func(value *flag.Flag) {
		if value.Name == "expected-version" {
			expectedPtr = &expected
		}
	})
	request := controlapi.ProcessTransitionRequest{AgentID: agentID, ExpectedVersion: expectedPtr, Reason: reason}
	var response controlapi.ProcessResponse
	var message string
	switch operation {
	case "suspend":
		message = controlapi.MessageProcessSuspend
	case "resume":
		message = controlapi.MessageProcessResume
	default:
		return fmt.Errorf("unknown process command %q", operation)
	}
	if err := client.Call(ctx, message, request, &response); err != nil {
		return err
	}
	return writeG13JSON(response)
}

func writeG13JSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
