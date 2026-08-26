# Architecture Diagrams

## Purpose

This document provides **visual architecture diagrams** for the runtime.

These diagrams complement the detailed architecture contracts and make the system shape explicit.

They focus on:

- runtime boundaries;
- canonical vs ephemeral state;
- control and execution flows;
- authorization;
- cognitive context and memory;
- recursive orchestration;
- scheduling;
- transactions and forks;
- local client/runtime separation;
- storage layout.

All diagrams use Mermaid so they render directly on GitHub and remain versioned with the architecture.

---

# 1. System context

```mermaid
flowchart TB
    User[User]
    Client[Thin TUI / CLI / IDE Client]
    Daemon[Durable Go Runtime Daemon]

    SQLite[(SQLite WAL)]
    ObjectStore[(Content-Addressed Object Store)]

    Providers[Model Providers / Local Inference]
    Worlds[Execution Worlds]
    External[Repos / APIs / Filesystem / Services]

    User --> Client
    Client <-->|Local IPC| Daemon

    Daemon --> SQLite
    Daemon --> ObjectStore
    Daemon --> Providers
    Daemon --> Worlds
    Worlds --> External
```

### Reading

- the client is **not** the owner of agent state;
- the daemon owns canonical runtime state;
- SQLite stores canonical metadata, events and projections;
- large payloads live in the object store;
- models and Worlds are external execution resources.

---

# 2. Internal runtime architecture

```mermaid
flowchart LR
    subgraph Presentation[Presentation Plane]
        TUI[TUI / CLI / IDE Client]
    end

    subgraph Control[Control Plane]
        IPC[Local Control Protocol]
        Supervisor[Runtime Supervisor]
        ProcessMgr[Agent Process Manager]
        Policy[Authority / Intent / Effect Engine]
        Scheduler[Cognitive Scheduler]
    end

    subgraph Data[Canonical Data Plane]
        Ledger[Event Ledger]
        Projections[Projections]
        Snapshots[Snapshots]
        Memory[Context Pages / Beliefs]
        SQLite[(SQLite)]
        Objects[(Object Store)]
    end

    subgraph Execution[Execution Plane]
        Invoker[Invocation Engine]
        Providers[Provider Adapters]
        WorldMgr[World Manager]
        Actions[Authorized Actions]
    end

    TUI <--> IPC
    IPC <--> Supervisor

    Supervisor <--> ProcessMgr
    Supervisor <--> Scheduler
    ProcessMgr <--> Policy
    ProcessMgr <--> Ledger

    Ledger <--> Projections
    Ledger <--> Snapshots
    Ledger <--> SQLite
    Projections <--> SQLite
    Snapshots <--> SQLite

    Memory <--> SQLite
    Memory <--> Objects

    Supervisor <--> Invoker
    Invoker <--> Providers
    Invoker <--> WorldMgr
    WorldMgr <--> Actions
    Invoker --> Objects
```

### Architectural law

The runtime is explicitly separated into four concerns:

- **presentation plane** — attachable clients;
- **control plane** — state machines, scheduling and authorization;
- **canonical data plane** — durable state and evidence;
- **execution plane** — model and World execution.

---

# 3. Canonical vs ephemeral state

```mermaid
flowchart TB
    subgraph Canonical[Canonical Durable State]
        C1[Agent Process State]
        C2[Intent / Capabilities / Budgets]
        C3[Event Ledger]
        C4[Transactions / Checkpoints / Forks]
        C5[Context Page Metadata]
        C6[Beliefs / Evidence Graph]
        C7[Approvals / Wake Conditions]
    end

    subgraph DurableArtifacts[Durable Artifacts]
        D1[Model Responses]
        D2[Tool Output / Logs]
        D3[Snapshots / File Artifacts]
        D4[Context Page Bodies]
        D5[Branch Artifacts]
    end

    subgraph Ephemeral[Ephemeral Runtime State]
        E1[Goroutines]
        E2[Live Streams]
        E3[Token Coalescing Buffers]
        E4[Bounded In-Memory Caches]
        E5[TUI Viewport State]
        E6[Open Sockets / Pipes]
    end

    Canonical --> DurableArtifacts
    Ephemeral -. reconstructed from durable state .-> Canonical
```

### Invariant

If correctness depends on a fact after a crash, that fact must have crossed a durable storage boundary.

---

# 4. End-to-end interaction flow

```mermaid
sequenceDiagram
    participant U as User
    participant C as Client
    participant D as Runtime Daemon
    participant P as Agent Process
    participant M as Cognitive MMU
    participant S as Scheduler
    participant L as Model Provider
    participant W as World
    participant DB as SQLite / Object Store

    U->>C: submit input
    C->>D: input.submit
    D->>DB: persist input + event
    D->>P: wake / schedule process

    P->>M: build working set
    M->>DB: load pages / beliefs / artifacts
    M-->>P: ContextManifest

    P->>S: request execution route
    S-->>P: selected model / policy

    P->>DB: persist ModelInvocationStarted
    P->>L: invoke model
    L-->>P: streamed tokens / action requests

    P->>D: syscall request
    D->>D: validate / authorize / classify effect
    D->>W: execute AuthorizedAction
    W-->>D: output / outcome
    D->>DB: persist artifacts + lifecycle events

    L-->>P: final response
    P->>DB: persist final response
    D-->>C: projection updates
    C-->>U: render output
```

---

# 5. Authorization pipeline

```mermaid
flowchart LR
    Req[Requested Action] --> Validate[Validate Action Shape]
    Validate --> Effect[Derive Effect Descriptor]
    Effect --> Proc[Check Process Lifecycle]
    Proc --> Cap[Check Capabilities / Leases]
    Cap --> Intent[Check Intent Compatibility]
    Intent --> Budget[Check Budgets / Quotas]
    Budget --> Tx[Check Transaction / Speculation Policy]
    Tx --> Approval{Approval Required?}
    Approval -- Yes --> UserApproval[Obtain Approval Token]
    Approval -- No --> WorldPolicy[Check World Enforcement Capability]
    UserApproval --> WorldPolicy
    WorldPolicy --> Decision[AuthorizedAction or Denial]
    Decision --> Exec[World Execution]
```

### Law

The model can request an action.

It cannot bypass the kernel authorization pipeline.

---

# 6. Cognitive MMU and Context Fault flow

```mermaid
flowchart TB
    subgraph DurableKnowledge[Durable Knowledge Base]
        Pages[Context Pages]
        Beliefs[Beliefs]
        Evidence[Evidence / Artifacts]
        Index[FTS / Retrieval Index]
    end

    subgraph WorkingSet[Working Set Builder]
        T0[Tier 0 Mandatory]
        T1[Tier 1 Active State]
        T2[Tier 2 Explicit Recall]
        T3[Tier 3 Relevant Durable Pages]
        T4[Tier 4 Recent Continuity]
        Pack[Pack / Trim / Emit ContextManifest]
    end

    Invoke[Invocation Request] --> T0
    Invoke --> T1
    Pages --> T2
    Beliefs --> T2
    Evidence --> T2
    Index --> T3
    Pages --> T3
    Beliefs --> T3

    T0 --> Pack
    T1 --> Pack
    T2 --> Pack
    T3 --> Pack
    T4 --> Pack
    Pack --> Prompt[Bounded Provider Request]

    Missing[Missing Reference / recall()] --> Fault[Context Fault Resolver]
    Fault --> Index
    Fault --> Pages
    Fault --> Beliefs
    Fault --> Evidence
    Fault --> T2
```

### Principle

The LLM context window is a **working-set cache**, not the memory system itself.

---

# 7. Epistemic memory model

```mermaid
flowchart LR
    Evidence[Evidence / Source Artifact]
    Episode[Episodic Record]
    Belief[Belief]
    Derived[Derived Belief]
    Contradiction[Contradiction / Supersession]
    Review[Needs Review / Stale]

    Evidence --> Episode
    Episode --> Belief
    Belief --> Derived
    Belief --> Contradiction
    Contradiction --> Review
    Evidence -. source changed .-> Review
```

### Principle

- raw evidence remains inspectable;
- beliefs carry provenance;
- source changes downgrade dependent knowledge instead of silently deleting history.

---

# 8. Recursive orchestration and subagents

```mermaid
flowchart TB
    Root[Root Agent Process]
    Reserve[Reserve Budget + Authority Subset]
    Spawn[spawn()]

    Root --> Reserve --> Spawn

    subgraph Team[Temporary Agent Organization]
        A1[Backend Agent]
        A2[Test Agent]
        A3[Security Agent]
        A4[SQL Specialist]
    end

    Spawn --> A1
    Spawn --> A2
    Spawn --> A3
    A1 --> A4

    A1 --> Result[Evidence / Result / Status]
    A2 --> Result
    A3 --> Result
    A4 --> Result
    Result --> Root
```

### Constraints

Each child is bounded by:

- delegated authority subset;
- budget reservation;
- child-count limit;
- deadline;
- scheduler policy;
- World policy.

---

# 9. Cognitive Scheduler

```mermaid
flowchart LR
    Task[Cognitive Task] --> Hard[Hard Filters]
    Hard --> Candidates[Eligible Candidates]

    subgraph Resources[Candidate Resources]
        R1[Local Small Model]
        R2[Cloud General Model]
        R3[Frontier Reasoning Model]
        R4[Deterministic Tool Path]
        R5[Specialist Child Strategy]
    end

    Candidates --> R1
    Candidates --> R2
    Candidates --> R3
    Candidates --> R4
    Candidates --> R5

    R1 --> Score[Utility Scoring]
    R2 --> Score
    R3 --> Score
    R4 --> Score
    R5 --> Score

    Score --> Reserve[Reserve Budget + Slot]
    Reserve --> Route[Selected Execution Route]
```

### Principle

The scheduler asks:

> What is the cheapest admissible resource that can solve this task under the current constraints?

Not:

> What is the default model?

---

# 10. Transaction and fork model

```mermaid
flowchart TB
    Base[Checkpoint / Base State]
    Base --> Tx[Transaction]
    Base --> Fork[Fork Group]

    subgraph TransactionLifecycle[Transaction Lifecycle]
        T1[OPEN]
        T2[VERIFYING]
        T3[READY TO COMMIT]
        T4[COMMITTING]
        T5[COMMITTED]
        T6[ROLLING BACK]
        T7[ROLLED BACK]
        T8[NEEDS RECONCILIATION]

        T1 --> T2 --> T3 --> T4 --> T5
        T2 --> T6 --> T7
        T4 --> T8
        T6 --> T8
    end

    subgraph ForkLifecycle[Fork Lifecycle]
        F1[Branch A]
        F2[Branch B]
        F3[Branch C]
        Eval[Evaluator]
        Promote[Selective Promotion]
        Discard[Discard / Retain Losers]

        Fork --> F1
        Fork --> F2
        Fork --> F3
        F1 --> Eval
        F2 --> Eval
        F3 --> Eval
        Eval --> Promote
        Eval --> Discard
    end
```

### Important distinction

- **transaction** — staged isolated work with commit/rollback/reconciliation semantics;
- **fork** — exploration of alternative future branches.

---

# 11. Safe execution editing

```mermaid
flowchart LR
    History[Observed History]
    Frontier[Causal Frontier / Checkpoint]
    Edit[Restore / Edit Request]
    Branch[New Successor Timeline]
    Reconcile[Reconcile Unknown Effects]
    Merge[Three-Way Merge / Selective Promotion]

    History --> Frontier
    Frontier --> Edit
    Edit --> Branch
    Branch --> Reconcile
    Reconcile --> Merge
```

### Law

Restore or execution editing never erases already observed effects.

It creates a new successor timeline anchored at a safe causal frontier.

---

# 12. Attach / detach TUI protocol

```mermaid
sequenceDiagram
    participant Client as TUI Client
    participant IPC as Local Control Protocol
    participant RT as Runtime Daemon
    participant PX as Presentation Projection
    participant DB as SQLite / Object Store

    Client->>IPC: Hello
    IPC->>RT: negotiate version
    RT-->>Client: Welcome

    Client->>RT: process.attach(agent_id)
    RT->>PX: load current projection
    PX->>DB: query latest blocks + summaries
    RT-->>Client: initial viewport + status + cursors

    Client->>RT: subscribe(process:<id>:presentation)
    RT-->>Client: bounded live events

    Note over Client,RT: client may detach, crash or reconnect

    Client->>RT: conversation.before(cursor, limit)
    RT->>DB: paginated history query
    RT-->>Client: page of blocks
```

### Property

A very old session can be reattached without transferring or rendering its entire lifetime history.

---

# 13. Storage layout

```mermaid
flowchart TB
    subgraph SQLiteDomain[SQLite Domains]
        LE[ledger_events]
        AP[agent_processes]
        SN[agent_snapshots]
        IN[intents]
        CG[capability_grants]
        BG[budget_accounts / reservations]
        WV[worlds]
        TX[transactions / checkpoints / forks]
        CP[context_pages]
        BL[beliefs / belief_edges]
        CB[conversation_blocks]
        AW[approvals / wake_conditions]
        OM[objects metadata]
    end

    subgraph ObjectDomain[Object Store]
        O1[Model Bodies]
        O2[Tool Outputs]
        O3[File Snapshots]
        O4[Context Page Bodies]
        O5[Logs / Branch Artifacts]
    end

    LE --> OM
    CP --> OM
    BL --> OM
    CB --> OM
    TX --> OM
```

---

# 14. Foundation implementation map

```mermaid
flowchart LR
    G0[G0 Foundations]
    G1[G1 Durable Agent Process + Event Ledger]
    G2[G2 Minimal Agent Loop]

    G0 --> F1[Config / Logging / IDs / Errors]
    G0 --> F2[SQLite Adapter]
    G0 --> F3[Object Store]
    G0 --> F4[Migrations]
    G0 --> F5[Test / Profiling Harness]

    G1 --> P1[Agent Process State Machine]
    G1 --> P2[Event Catalog]
    G1 --> P3[Reducers + Snapshots]
    G1 --> P4[Process Projections]
    G1 --> P5[Supervisor Activation]

    G2 --> A1[Provider Interface]
    G2 --> A2[observe / execute / checkpoint]
    G2 --> A3[Streaming Persistence]
    G2 --> A4[Minimal Attachable Client]

    F2 --> P2
    F3 --> P2
    P2 --> A1
```

### Usage

This diagram is the visual bridge between A0 architecture and implementation planning.

---

# Diagram maintenance rule

Whenever a high-coupling architecture change is accepted, update:

1. the relevant subsystem contract;
2. the architecture decision log if the decision changed;
3. this document if the visual system shape changed.

These diagrams are part of the architecture baseline, not decoration.
