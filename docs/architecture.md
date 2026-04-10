# MezaMozg Architecture

## Design Principles

- Safe by default: typed operations instead of arbitrary shell execution.
- Fleet-first orchestration: staged rollouts and pause-on-error behavior.
- Auditability: each action must be attributable to an operator and policy context.
- Extensibility: control plane and agent communication should evolve toward streaming gRPC with mTLS.

## Components

### Meza-Core

The control plane is responsible for:

- node registry and inventory metadata
- job planning and rollout strategy selection
- audit event creation
- AI intent interpretation
- future certificate issuance and artifact signing

The current MVP is implemented as a Go HTTP API with an in-memory store. It is structured so we can later swap the store and planner implementations without changing handlers.

### Meza-Panel

The web UI focuses on:

- dashboard snapshots
- node inventory and tagging
- job creation and monitoring
- AI operations copilot UX

The initial panel is a static Next.js shell that mirrors the domain concepts and gives us a layout to wire to the API next.

### Meza-Node

The agent will eventually provide:

- secure enrollment
- heartbeat and metric streaming
- typed job execution
- artifact-based self-update

The current Rust scaffold is intentionally small and prepares the repo for the real agent implementation.

## Runtime Flow

1. An operator creates a job manually or asks the AI copilot to perform an operation.
2. Meza-Core resolves the target nodes and validates the requested action against policy.
3. The system converts the request into a typed job with a rollout strategy.
4. Nodes execute the job only after receiving a validated instruction from Meza-Core.
5. Logs, status transitions, and metrics are streamed back into the control plane.

## Future Target Stack

- API and scheduler: Go
- Agent: Rust
- Web: Next.js + TypeScript
- Transport: gRPC streaming over mTLS
- Storage: PostgreSQL + Redis + object storage
- Metrics and logs: Prometheus, Grafana, Loki
- Events: NATS

