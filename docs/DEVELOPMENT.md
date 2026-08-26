# Development

## Requirements

- Go 1.27+
- Git

## Setup

```bash
git clone https://github.com/Methamorphe/go-agent.git
cd go-agent
go mod download
```

## Run

```bash
make run
```

Or directly:

```bash
go run ./cmd/go-agent
```

## Tests

```bash
make test
```

Or:

```bash
go test ./...
```

## Race detector

```bash
make test-race
```

## Full local check

```bash
make check
```

## Development rule

Architecture contracts in `docs/` take precedence over implementation shortcuts.

High-coupling semantic changes must update the corresponding architecture documentation before or with the code change.

## G0 discipline

G0 implements foundations only. It must preserve the A0 invariants:

- no TUI-owned canonical state;
- no unbounded hot-path queues;
- large I/O must be streamable;
- durable facts must cross a persistence boundary before the runtime relies on them;
- process identity must remain independent from goroutine/model/provider lifetime;
- failure behavior must be explicit and testable.
