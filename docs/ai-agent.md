# AI Operations Copilot

## Goal

Allow an operator to write natural language such as:

`обнови docker на ноде argentina-17`

The AI layer should interpret the request and convert it into a typed operation that Meza-Core can validate, approve, and execute.

## Safe Execution Model

The AI assistant must not:

- send raw shell directly to nodes
- bypass rollout policy
- bypass approval rules
- issue privileged operations without an explicit capability check

The AI assistant may:

- resolve entities like node names, tags, and environments
- infer the right typed job
- ask for confirmation when a task is ambiguous or privileged
- summarize the expected rollout and risk

## Gemini Integration

Use Gemini function calling so the model chooses from backend tools such as:

- `resolve_node`
- `get_node_facts`
- `create_job`
- `request_approval`
- `run_job`
- `get_job_status`

Gemini should return a structured function call, not a shell command. Meza-Core remains the execution authority.

The backend now includes an optional Gemini-backed planner behind `MEZA_AI_PROVIDER=gemini`. If the key is missing or the Gemini request fails, Meza-Core falls back to the deterministic local planner.

## Suggested Function Shape

```json
{
  "name": "create_job",
  "arguments": {
    "job_type": "update_docker",
    "target": {
      "kind": "node",
      "selector": "argentina-17"
    },
    "strategy": {
      "mode": "rolling",
      "batches": [10, 25, 50, 100]
    }
  }
}
```

## Planner Strategy

The repo now includes both planners:

- a deterministic `stub` planner for development and offline safety
- a Gemini planner that calls `generateContent` with a `plan_typed_job` function declaration

Both emit the same `AIPlannedOperation` contract, so the rest of the control plane stays provider-agnostic.
