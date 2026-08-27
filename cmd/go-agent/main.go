package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Methamorphe/go-agent/internal/app"
	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/control"
	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/logging"
)

const commandTimeout = 10 * time.Second

func main() {
	os.Exit(
		run(os.Args[1:]),
	)
}

func run(args []string) int {
	flags := flag.NewFlagSet(
		"go-agent",
		flag.ContinueOnError,
	)

	flags.SetOutput(os.Stderr)

	var configPath string
	var dataDir string
	var logLevel string
	var controlAddress string
	var pprofAddress string

	flags.StringVar(
		&configPath,
		"config",
		"",
		"path to JSON config file",
	)

	flags.StringVar(
		&dataDir,
		"data-dir",
		"",
		"runtime data directory",
	)

	flags.StringVar(
		&logLevel,
		"log-level",
		"",
		"debug|info|warn|error",
	)

	flags.StringVar(
		&controlAddress,
		"control-address",
		"",
		"unix socket or Windows named pipe",
	)

	flags.StringVar(
		&pprofAddress,
		"pprof-address",
		"",
		"explicit loopback address, e.g. 127.0.0.1:6060",
	)

	if err := flags.Parse(args); err != nil {
		return 2
	}

	/*
		Le package standard flag s'arrête au premier argument
		qui n'est pas un flag.

		Donc :

		    go-agent --data-dir ./data process inspect agt_xxx

		fonctionne.

		Alors que les flags globaux placés après "process"
		seront considérés comme appartenant à la sous-commande.
	*/
	command := flags.Args()

	visited := map[string]bool{}

	flags.Visit(
		func(flag *flag.Flag) {
			visited[flag.Name] = true
		},
	)

	overrides := config.Overrides{}

	if visited["data-dir"] {
		overrides.DataDir = &dataDir
	}

	if visited["log-level"] {
		overrides.LogLevel = &logLevel
	}

	if visited["control-address"] {
		overrides.ControlAddress =
			&controlAddress
	}

	if visited["pprof-address"] {
		overrides.PprofAddress =
			&pprofAddress
	}

	cfg, err := config.Load(
		configPath,
		overrides,
	)
	if err != nil {
		fallback := slog.New(
			slog.NewTextHandler(
				os.Stderr,
				nil,
			),
		)

		fallback.Error(
			"configuration error",
			"error",
			err,
		)

		return 2
	}

	logger, err := logging.New(
		os.Stderr,
		cfg.LogLevel,
	)
	if err != nil {
		fmt.Fprintln(
			os.Stderr,
			err,
		)

		return 2
	}

	ctx, stop :=
		signal.NotifyContext(
			context.Background(),
			os.Interrupt,
			syscall.SIGTERM,
		)

	defer stop()

	/*
		Aucune sous-commande signifie :

		    go-agent

		=> démarrage du daemon.

		"serve" est simplement une forme explicite :

		    go-agent serve
	*/
	if len(command) == 0 {
		return runDaemon(
			ctx,
			logger,
			cfg,
		)
	}

	if command[0] == "serve" {
		if len(command) != 1 {
			fmt.Fprintln(
				os.Stderr,
				"usage: go-agent serve",
			)

			return 2
		}

		return runDaemon(
			ctx,
			logger,
			cfg,
		)
	}

	/*
		À partir d'ici nous sommes dans le mode CLI client.

		Le CLI ne touche jamais directement SQLite.

		Il parle au daemon via :

		    Unix Domain Socket
		ou
		    Windows Named Pipe

		C'est important : il n'existe qu'un seul runtime qui
		mute l'état canonique.
	*/
	client := control.NewClient(
		cfg.ControlAddress,
		cfg.MaxFrameBytes,
		id.NewGenerator(),
	)

	commandCtx, cancel :=
		context.WithTimeout(
			ctx,
			commandTimeout,
		)

	defer cancel()

	if err := runCommand(
		commandCtx,
		client,
		command,
	); err != nil {
		fmt.Fprintln(
			os.Stderr,
			"error:",
			err,
		)

		return 1
	}

	return 0
}

func runDaemon(
	ctx context.Context,
	logger *slog.Logger,
	cfg config.Config,
) int {
	application := app.New(
		logger,
		cfg,
	)

	if err := application.Run(ctx); err != nil {
		logger.Error(
			"application stopped with an error",
			"error",
			err,
		)

		return 1
	}

	return 0
}

/*
caller est une petite interface locale.

Pourquoi ne pas prendre directement *control.Client ?

Parce que cette fonction n'a besoin que d'UNE capacité :

	Call(...)

Elle ne se soucie pas de savoir comment Client fonctionne.

Cela nous permettra également de tester runCommand avec un
faux client sans ouvrir réellement un socket.
*/
type caller interface {
	Call(
		context.Context,
		string,
		any,
		any,
	) error
}

func runCommand(
	ctx context.Context,
	client caller,
	args []string,
) error {
	if len(args) == 0 {
		return fmt.Errorf(
			"missing command",
		)
	}

	switch args[0] {
	case "process":
		return runProcessCommand(
			ctx,
			client,
			args[1:],
		)

	case "events":
		return runEventsCommand(
			ctx,
			client,
			args[1:],
		)

	default:
		return fmt.Errorf(
			"unknown command %q",
			args[0],
		)
	}
}

func runProcessCommand(
	ctx context.Context,
	client caller,
	args []string,
) error {
	if len(args) == 0 {
		return fmt.Errorf(
			"usage: go-agent process <create|inspect|suspend|resume>",
		)
	}

	switch args[0] {
	case "create":
		return runProcessCreate(
			ctx,
			client,
			args[1:],
		)

	case "inspect":
		return runProcessInspect(
			ctx,
			client,
			args[1:],
		)

	case "suspend":
		return runProcessSuspend(
			ctx,
			client,
			args[1:],
		)

	case "resume":
		return runProcessResume(
			ctx,
			client,
			args[1:],
		)

	default:
		return fmt.Errorf(
			"unknown process command %q",
			args[0],
		)
	}
}

func runProcessCreate(
	ctx context.Context,
	client caller,
	args []string,
) error {
	flags := flag.NewFlagSet(
		"process create",
		flag.ContinueOnError,
	)

	flags.SetOutput(os.Stderr)

	var intent string

	flags.StringVar(
		&intent,
		"intent",
		"",
		"root intent of the agent process",
	)

	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return fmt.Errorf(
			"usage: go-agent process create --intent <text>",
		)
	}

	if intent == "" {
		return fmt.Errorf(
			"--intent is required",
		)
	}

	request := controlapi.ProcessCreateRequest{
		Intent: intent,
	}

	var response controlapi.ProcessResponse

	if err := client.Call(
		ctx,
		controlapi.MessageProcessCreate,
		request,
		&response,
	); err != nil {
		return fmt.Errorf(
			"create process: %w",
			err,
		)
	}

	return writeJSON(
		os.Stdout,
		response,
	)
}

func runProcessInspect(
	ctx context.Context,
	client caller,
	args []string,
) error {
	if len(args) != 1 {
		return fmt.Errorf(
			"usage: go-agent process inspect <agent-id>",
		)
	}

	request := controlapi.ProcessIDRequest{
		AgentID: id.AgentID(args[0]),
	}

	var response controlapi.ProcessResponse

	if err := client.Call(
		ctx,
		controlapi.MessageProcessInspect,
		request,
		&response,
	); err != nil {
		return fmt.Errorf(
			"inspect process: %w",
			err,
		)
	}

	return writeJSON(
		os.Stdout,
		response,
	)
}

func runProcessSuspend(
	ctx context.Context,
	client caller,
	args []string,
) error {
	if len(args) == 0 {
		return fmt.Errorf(
			"usage: go-agent process suspend <agent-id> [--reason <text>] [--expected-version <n>]",
		)
	}

	agentID := id.AgentID(
		args[0],
	)

	flags := flag.NewFlagSet(
		"process suspend",
		flag.ContinueOnError,
	)

	flags.SetOutput(os.Stderr)

	var reason string
	var expectedVersion uint64

	flags.StringVar(
		&reason,
		"reason",
		"",
		"suspension reason",
	)

	flags.Uint64Var(
		&expectedVersion,
		"expected-version",
		0,
		"expected current process version",
	)

	if err := flags.Parse(
		args[1:],
	); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return fmt.Errorf(
			"usage: go-agent process suspend <agent-id> [--reason <text>] [--expected-version <n>]",
		)
	}

	var expectedVersionPointer *uint64

	flags.Visit(
		func(flag *flag.Flag) {
			if flag.Name ==
				"expected-version" {
				expectedVersionPointer =
					&expectedVersion
			}
		},
	)

	request :=
		controlapi.ProcessTransitionRequest{
			AgentID: agentID,

			ExpectedVersion: expectedVersionPointer,

			Reason: reason,
		}

	var response controlapi.ProcessResponse

	if err := client.Call(
		ctx,
		controlapi.MessageProcessSuspend,
		request,
		&response,
	); err != nil {
		return fmt.Errorf(
			"suspend process: %w",
			err,
		)
	}

	return writeJSON(
		os.Stdout,
		response,
	)
}

func runProcessResume(
	ctx context.Context,
	client caller,
	args []string,
) error {
	if len(args) == 0 {
		return fmt.Errorf(
			"usage: go-agent process resume <agent-id> [--reason <text>] [--expected-version <n>]",
		)
	}

	agentID := id.AgentID(
		args[0],
	)

	flags := flag.NewFlagSet(
		"process resume",
		flag.ContinueOnError,
	)

	flags.SetOutput(os.Stderr)

	var reason string
	var expectedVersion uint64

	flags.StringVar(
		&reason,
		"reason",
		"",
		"resume reason",
	)

	flags.Uint64Var(
		&expectedVersion,
		"expected-version",
		0,
		"expected current process version",
	)

	if err := flags.Parse(
		args[1:],
	); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return fmt.Errorf(
			"usage: go-agent process resume <agent-id> [--reason <text>] [--expected-version <n>]",
		)
	}

	var expectedVersionPointer *uint64

	flags.Visit(
		func(flag *flag.Flag) {
			if flag.Name ==
				"expected-version" {
				expectedVersionPointer =
					&expectedVersion
			}
		},
	)

	request :=
		controlapi.ProcessTransitionRequest{
			AgentID: agentID,

			ExpectedVersion: expectedVersionPointer,

			Reason: reason,
		}

	var response controlapi.ProcessResponse

	if err := client.Call(
		ctx,
		controlapi.MessageProcessResume,
		request,
		&response,
	); err != nil {
		return fmt.Errorf(
			"resume process: %w",
			err,
		)
	}

	return writeJSON(
		os.Stdout,
		response,
	)
}

func runEventsCommand(
	ctx context.Context,
	client caller,
	args []string,
) error {
	if len(args) == 0 {
		return fmt.Errorf(
			"usage: go-agent events <agent-id> [--after <version>] [--limit <n>]",
		)
	}

	agentID := id.AgentID(
		args[0],
	)

	flags := flag.NewFlagSet(
		"events",
		flag.ContinueOnError,
	)

	flags.SetOutput(os.Stderr)

	var afterVersion uint64
	var limit int

	flags.Uint64Var(
		&afterVersion,
		"after",
		0,
		"return events after this process version",
	)

	flags.IntVar(
		&limit,
		"limit",
		100,
		"maximum number of events",
	)

	if err := flags.Parse(
		args[1:],
	); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return fmt.Errorf(
			"usage: go-agent events <agent-id> [--after <version>] [--limit <n>]",
		)
	}

	if limit <= 0 {
		return fmt.Errorf(
			"--limit must be greater than zero",
		)
	}

	request :=
		controlapi.ProcessEventsRequest{
			AgentID:      agentID,
			AfterVersion: afterVersion,
			Limit:        limit,
		}

	var response controlapi.ProcessEventsResponse

	if err := client.Call(
		ctx,
		controlapi.MessageProcessEvents,
		request,
		&response,
	); err != nil {
		return fmt.Errorf(
			"list process events: %w",
			err,
		)
	}

	return writeJSON(
		os.Stdout,
		response,
	)
}

func writeJSON(
	file *os.File,
	value any,
) error {
	encoder :=
		json.NewEncoder(file)

	encoder.SetIndent(
		"",
		"  ",
	)

	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(
		value,
	); err != nil {
		return fmt.Errorf(
			"encode output: %w",
			err,
		)
	}

	return nil
}
