# G10 Exit Review — Context Faults + Cognitive MMU v2

Status: **PASS**

## Scope delivered

G10 turns missing cognitive state into an explicit, typed and bounded runtime paging event rather than an implicit prompt/cache concern.

### Stable cognitive references

The MMU accepts provider-independent semantic references using the following schemes:

- `ctx://`
- `belief://`
- `evidence://`
- `object://`
- `event://`
- `checkpoint://`
- `agent://`

References are parsed and validated before resolution. Non-context schemes are resolved through registered `CognitiveResolver` implementations, so the core MMU does not depend on a model/provider protocol.

### Typed faults

The runtime exposes all six A0 fault classes:

- `ReferenceFault`
- `RecallFault`
- `EvidenceFault`
- `FreshnessFault`
- `DependencyFault`
- `RepresentationFault`

`ResolveFault` validates Agent/Invocation identity, scope visibility, fault shape and budget before granting any context lease.

### Paging semantics

Resolved pages receive one-build leases and therefore enter the next bounded working set through the existing MMU lease tier. Fault resolution never widens scope or authority.

`DependencyFault` performs bounded dependency traversal. Resolver-returned dependencies can themselves be cognitive references and are deduplicated before materialization.

`FreshnessFault` follows supersession chains and rejects cycles as corruption.

`EvidenceFault` reverses structured compaction: when a summary page carries `SummaryOf`, its raw source pages can be paged back for verification without deleting or rewriting the summary.

`RepresentationFault` explicitly returns `NEEDS_COMPACTION_OR_PROJECTION` when the requested representation cannot fit its fault token budget.

### Fault budgets and storm protection

The G10 fault budget limits:

- faults per invocation/turn;
- faults per task window;
- total materialized tokens;
- resolution time;
- repeated request/page-set hits.

`ProgressEpoch` resets loop fingerprints only after observable caller progress, allowing legitimate repeated work while stopping no-progress thrashing.

### Durable observability and replay

Migration `0011_context_faults.sql` persists fault records independently from context pages.

Resolved faults remain pending until the next invocation manifest is persisted. G4 attaches pending resolved faults to the next `ContextManifest`; the SQLite manifest trigger then atomically binds those pending faults to the persisted invocation. This gives replay a durable chain:

`source invocation -> fault -> resolved pages/refs -> leases -> next manifest`.

The handoff is crash-safe in the useful direction: a crash before manifest persistence leaves the fault pending rather than silently losing it.

## Compatibility

G10 deliberately does not widen `mmu.IDGenerator`; fault IDs are derived from the existing cryptographically generated lease-ID source. Existing G4/G6/G9 test doubles therefore remain source-compatible.

The existing MMU page repository contract is also unchanged. Fault persistence is an optional `FaultJournal` capability implemented by SQLite, preserving compatibility for lightweight/in-memory repositories.

## Tests

New G10 contract coverage includes:

- semantic reference validation;
- ReferenceFault -> next-build lease;
- EvidenceFault -> raw compacted sources;
- FreshnessFault -> supersession follow;
- resolver-independent dependency paging;
- repeated-fault storm rejection;
- RepresentationFault projection requirement;
- durable resolved-fault -> next-manifest claiming.

During the G10 gate, the tagged soak compile exposed stale calls to the old SQLite `Checkpoint` name. Those harnesses now use the existing `CheckpointWAL` API, removing the G8-era method-collision residue.

## Exit invariants

- context faults are explicit and typed;
- paging is bounded by scope, time, tokens and repetition limits;
- context leases remain finite;
- raw evidence survives compaction and can be paged back;
- freshness follows durable supersession metadata;
- dependency traversal is bounded and deduplicated;
- faults cannot mint capabilities or widen visible scope;
- resolved faults are observable in the next manifest and durably attributable to an invocation;
- provider/model APIs are not embedded in the MMU fault contract.

## CI gate

The implementation was validated through the repository CI matrix. Linux passed `go test ./...`, tagged soak compilation, `go vet ./...`, and builds of `cmd/go-agent` and `cmd/go-agentctl`; the final branch CI run is the authoritative gate recorded in GitHub Actions.
