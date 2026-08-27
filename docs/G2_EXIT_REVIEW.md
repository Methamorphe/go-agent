# G2 Exit Review — Minimal Agent Loop + Agent Syscalls

## Status

**PASS — G2 is complete.**

Implementation commit:

```text
19f3fadcf0343cc53910dd52144bf3dc35d95bcb
feat(g2): implement minimal agent loop and syscalls
```

Validation run `33083660845` passed on August 27, 2026:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

The platform jobs passed `go test ./...`, `go vet ./...` and the daemon build. The race job passed `go test -race ./...`. The close-out CI additionally builds both `go-agent` and `go-agentctl` explicitly.

---

## 1. Goal review

G2 had one goal:

> Run the smallest useful intelligent process through kernel boundaries.

That goal is satisfied.

A durable G1 Agent Process can now:

1. be activated without changing Agent identity;
2. invoke a model through a provider-independent streaming ABI;
3. inspect a workspace through `observe`;
4. execute an argv-based command through `execute` with timeout and output limits;
5. create a durable checkpoint through `checkpoint`;
6. feed syscall results back into the next model invocation;
7. persist final model output and large command output in the Object Store;
8. attribute model/syscall lifecycle facts in the Event Ledger;
9. expose a bounded live stream to an attachable CLI;
10. complete the durable Agent Process with the final response object reference.

The deterministic fake-provider test demonstrates the minimal loop:

```text
model
  → observe
  → model
  → execute
  → model
  → final answer
```

without network access or nondeterministic model behavior.

---

## 2. Provider boundary

G2 introduces `internal/provider` as the provider-independent ABI.

The canonical Agent Process never stores proprietary provider conversation/thread identity.

The request model carries:

- `AgentID`;
- `InvocationID`;
- model identifier;
- typed messages/content parts;
- provider-neutral tool definitions;
- JSON tool arguments/results.

The streaming contract is callback/sink based:

```go
type Provider interface {
    Name() string
    Stream(context.Context, Request, EventSink) (Result, error)
}
```

This deliberately avoids requiring a goroutine/channel per invocation. A slow sink naturally applies backpressure.

Implemented adapters:

- deterministic `Fake` provider;
- OpenAI Responses streaming adapter;
- OpenAI-compatible `/chat/completions` streaming adapter for local stacks such as vLLM/Ollama-style endpoints.

Provider cancellation is driven by `context.Context`; cancellation/deadline identity remains detectable.

Usage supports provider-reported input/output/total token counts. Monetary pricing is not fabricated when the provider does not return it; model pricing/routing belongs to later scheduler/economy work.

---

## 3. Invocation durability and streaming

`internal/invocation` separates a model invocation from the Agent Process itself.

Before network/model execution:

```text
provider request JSON
      ↓
Object Store
      ↓
ModelInvocationStarted(request_ref)
```

During generation:

```text
provider chunks
   ├─→ bounded live presentation stream
   ├─→ bounded head/tail preview
   └─→ io.Pipe → Object Store
```

After generation:

```text
ModelInvocationCompleted(
    response_ref,
    finish_reason,
    usage,
    tool_call_count
)
```

A failed invocation records `ModelInvocationFailed`, including a partial response reference when one was finalized.

Critical invariant proven by test: many model chunks produce invocation lifecycle records, **not one Ledger event per chunk/token**.

---

## 4. Agent Syscalls

G2 exposes exactly the initial syscall vocabulary planned for this generation:

```text
observe
execute
checkpoint
```

### `observe`

Initial operations:

```text
read_file
list_directory
```

Workspace path resolution is bounded to the configured workspace and resolves symlinks before accepting an existing path. Traversal outside the workspace is rejected.

### `execute`

The process execution request is structured as:

```text
executable
argv[]
cwd
timeout
```

There is no implicit unrestricted shell-string API.

Command execution has:

- a default and maximum timeout;
- bounded stdout/stderr preview;
- a hard aggregate output limit;
- streaming stdout/stderr persistence into the Object Store;
- captured exit status;
- cancellation through the invocation context.

### `checkpoint`

The syscall reuses the G1 durable `CheckpointCreated` transition rather than introducing a second checkpoint semantic.

---

## 5. Syscall auditability

Syscall arguments are persisted to the Object Store before the request event:

```text
arguments
   ↓
Object Store
   ↓
SyscallRequested(arguments_ref)
```

Terminal syscall metadata is then recorded as one of:

```text
SyscallCompleted(result_ref)
SyscallFailed(failure)
```

Large bodies remain outside inline Ledger payloads.

Operation events advance the Agent Process stream version while leaving the primary process status `RUNNING`.

This preserves one causally ordered durable process history without conflating an Agent Process with a model invocation or tool execution.

---

## 6. Minimal agent loop

`internal/agent.Runner` owns the G2 harness loop.

Important bounds:

```text
default model steps        24
maximum model steps        64
maximum tool calls/step     8
G2 working context       2 MiB
```

The context cap is intentionally a hard G2 safety bound, not a fake Cognitive MMU. G4 will replace this temporary bounded transcript strategy with Context Pages/working-set construction.

A successful run transitions:

```text
READY
  ↓
RUNNING
  ↓
model/syscalls/model/...
  ↓
COMPLETED(response_ref)
```

If a transient invocation/run fails before terminal task completion, the runner attempts to yield the process back to `READY` rather than silently manufacturing success.

---

## 7. Live attachable CLI

G2 adds the small `go-agentctl` client rather than prematurely building the final TUI.

Run example:

```bash
go-agentctl --control-address ./runtime-data/control.sock run <agent-id> \
  --provider openai-compatible \
  --base-url http://127.0.0.1:8000/v1 \
  --model <served-model-name> \
  --workspace .
```

OpenAI example:

```bash
export OPENAI_API_KEY=...

go-agentctl --control-address ./runtime-data/control.sock run <agent-id> \
  --provider openai \
  --model <model> \
  --workspace .
```

Attach from another terminal:

```bash
go-agentctl --control-address ./runtime-data/control.sock attach <agent-id>
```

The live bus is presentation-only and bounded. It keeps offsets so a slow attach client can detect a gap. The durable final response remains in the Object Store even if presentation frames were dropped.

---

## 8. Memory/backpressure review

G2 does not introduce an unbounded model/output buffer.

Boundaries include:

- callback-driven provider backpressure;
- SSE event size limit;
- tool-argument size limit;
- bounded model preview;
- bounded live stream bytes and stream count;
- `io.Pipe` streaming into Object Store;
- bounded command stdout/stderr preview;
- hard command-output quota;
- bounded model-step/tool-call/context limits.

This satisfies the generation requirement that large output cannot grow hot memory without bound.

---

## 9. Crash and interruption semantics

G2 preserves the G1 crash-safe canonical-state model.

A model invocation or syscall request is durably visible before the corresponding external/model work proceeds. High-volume stream chunks are not canonical state.

If the daemon dies while a process is `RUNNING`, G1 startup recovery reconciles stale `RUNNING` state back to `READY`; the already committed operation-start event remains in history. G2 does **not** implicitly replay an interrupted command or model call and does not synthesize a successful terminal event.

This deliberately avoids unsafe retry semantics before G3 introduces the full World/effect/certainty/reconciliation model. Strong platform process-tree termination and explicit World-level `OutcomeUnknown` semantics remain G3 responsibilities.

---

## 10. Scope boundaries preserved

G2 deliberately does **not** implement G3 under different names.

Still deferred:

- formal `World` API and `LocalWorld`;
- capability grants/leases;
- Intent-Based Authority enforcement;
- Effect classification;
- process-tree platform adapter;
- network policy;
- secrets binding;
- reversible writes;
- transaction/fork semantics.

The G2 workspace containment and argv execution rules are correctness/safety bounds for the minimal loop, not claims of a complete security sandbox.

---

## 11. Validation coverage

Tests cover at least:

- deterministic fake-provider event order;
- cancellation and deadline propagation;
- sink-error propagation;
- provider-neutral JSON tool calls;
- malformed roles/schemas;
- OpenAI Responses SSE mapping;
- OpenAI-compatible SSE/tool-call mapping;
- bounded head/tail preview;
- bounded live stream with offsets;
- lifecycle-only model recording despite many chunks;
- workspace read/list and traversal denial;
- bounded command previews;
- hard command-output quota;
- checkpoint reuse;
- operation events requiring `RUNNING`;
- model → observe → model → execute → model → answer loop;
- cross-platform test/vet/build;
- race detector.

---

## 12. Exit decision

**G2 PASS.**

The completion criterion is met: a durable Agent Process can inspect a small repository, execute a bounded command and produce a final answer through provider/syscall boundaries; meaningful actions are attributable in the Ledger and large outputs are streamed/reference-backed rather than accumulated without bound in hot memory.

G3 can now start without reopening G2 semantics.
