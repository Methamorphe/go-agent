# Cognitive Scheduler v0 — Runtime Configuration

## Status

G6 adds an opt-in daemon path for per-invocation cognitive scheduling. The legacy G4 path remains the default when `scheduler.enabled` is `false`.

The semantic architecture remains defined by `COGNITIVE_SCHEDULER_ARCHITECTURE.md`. This document describes the implemented v0 configuration surface.

## Activation

Scheduler configuration lives in the normal JSON runtime config under `scheduler`.

Example:

```json
{
  "scheduler": {
    "enabled": true,
    "max_profiles": 64,
    "global_slots": 8,
    "per_root_slots": 4,
    "per_provider_slots": 4,
    "reserved_output_tokens": 4096,
    "default_root_budget": {
      "money_micros": 5000000,
      "tokens": 500000
    },
    "backends": [
      {
        "provider_id": "openai-cloud",
        "type": "openai",
        "api_key_env": "OPENAI_API_KEY"
      },
      {
        "provider_id": "local-vllm",
        "type": "openai-compatible",
        "base_url": "http://127.0.0.1:8000/v1"
      }
    ],
    "profiles": [
      {
        "provider_id": "openai-cloud",
        "model_id": "frontier-model",
        "context_window": 128000,
        "max_output_tokens": 8192,
        "capabilities": ["tool-calling", "streaming", "structured-output"],
        "locality": "cloud",
        "allows_sensitive": false,
        "training_opt_out": true,
        "input_cost_micros_per_million": 2000000,
        "output_cost_micros_per_million": 8000000,
        "latency_p95_ms": 2500,
        "reliability": 0.995,
        "quality": {
          "code-generation": 0.95,
          "code-review": 0.96
        },
        "max_concurrency": 4,
        "profile_version": 1
      },
      {
        "provider_id": "local-vllm",
        "model_id": "local-coder",
        "context_window": 32768,
        "max_output_tokens": 4096,
        "capabilities": ["tool-calling", "streaming"],
        "locality": "local",
        "allows_sensitive": true,
        "training_opt_out": true,
        "input_cost_micros_per_million": 0,
        "output_cost_micros_per_million": 0,
        "latency_p95_ms": 1500,
        "reliability": 0.98,
        "quality": {
          "code-generation": 0.80,
          "code-review": 0.78
        },
        "max_concurrency": 1,
        "profile_version": 1
      }
    ]
  }
}
```

Profile metadata is configuration/benchmark evidence, not provider marketing data interpreted automatically by the kernel.

## Per-invocation flow

When enabled, the G4/MMU loop stays responsible for reconstructing the bounded working context. Each model step then becomes a fresh `CognitiveTask`:

```text
MMU working set
    ↓
CognitiveTask
    ↓
hard eligibility filters
    ↓
deterministic score
    ↓
budget + slot reservation
    ↓
CognitiveRoutingDecided event + decision artifact
    ↓
provider invocation
    ↓
actual settlement + health telemetry
    ↓
bounded fallback when transiently unavailable
```

Agent identity, root identity, Intent and durable process state never move into provider session state.

## Hard constraints

The v0 router rejects candidates before scoring when they cannot satisfy required:

- context/output capacity;
- model capabilities;
- privacy/local-only policy;
- provider/model allow/deny policy;
- health/circuit state;
- minimum quality/reliability;
- finite budget;
- deadline;
- experimental-backend policy.

A high soft score cannot rescue an ineligible profile.

## Budget and fairness

`default_root_budget` initializes the scheduler budget ledger for a durable root. All descendants use the same `RootAgentID`, so concurrent parent/child cognitive tasks contend against the same logical budget.

The scheduler also enforces:

- global model slots;
- per-root slots;
- per-provider slots;
- per-profile concurrency.

Reservations are atomic. Successful calls settle actual provider usage exactly once. Unused reservation is released. Failed attempts do not silently consume unknown cost.

## Fallback and health

Transient provider failures reroute the same `CognitiveTask` after excluding the failed model. The request is reconstructed from the canonical MMU/process state; provider thread/session state is never required for fallback.

Repeated transient failures open the bounded circuit breaker. After cooldown the profile moves through `Recovering` before returning to `Healthy` on success.

## Observability

Every accepted route records `CognitiveRoutingDecided` in the Agent Process ledger with:

- Task ID;
- Decision ID;
- selected provider/model/profile version;
- estimated money/tokens;
- reservation ID;
- Object Store reference to the full routing decision.

The full decision artifact includes candidate scores and structured rejection reasons, making routing explainable without bloating the event ledger.

## Compatibility

`scheduler.enabled=false` preserves the existing explicit `provider` + `model` execution path. G6 therefore does not force existing deployments to define model profiles before they are ready to adopt automatic routing.
