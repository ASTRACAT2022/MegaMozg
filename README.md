# MezaMozg

MezaMozg is a safe-by-default orchestration platform for managing distributed server fleets with staged rollouts, auditability, persistence, and an AI operations copilot.

This repository starts with an MVP scaffold for three products:

- `Meza-Core`: central control plane and API.
- `Meza-Panel`: web UI for operators.
- `Meza-Node`: lightweight agent runtime.

The initial implementation intentionally avoids an unsafe "LLM -> raw shell as root" model. AI requests are converted into typed jobs that are validated and then executed by the control plane.

## Repository Layout

```text
apps/
  meza-core/   Go API and orchestration control plane
  meza-panel/  Next.js operator interface
  meza-node/   Rust node agent scaffold
contracts/     OpenAPI contracts and shared schemas
docs/          Architecture, AI design, roadmap
```

Русскоязычный гайд по запуску и эксплуатации: [`docs/MANUAL_RU.md`](docs/MANUAL_RU.md).

## Production Baseline In This Repo

- Register and list nodes.
- Create typed jobs with rollout strategies.
- Approve and start privileged jobs with simulated rolling execution.
- Persist fleet state to disk across restarts.
- Protect operator, bootstrap, and node endpoints with bearer tokens.
- Track audit events and fleet dashboard metrics.
- Interpret operator intent into typed jobs.
- Render a dashboard-oriented UI for nodes, jobs, and AI-assisted operations.
- Package both core and panel for container deployment.

## Local Development

### Meza-Core

```bash
cd apps/meza-core
go run ./cmd/api
```

The API starts on `http://localhost:8080`.
By default it persists state to `./data/state.json`.

### Meza-Node

```bash
cd apps/meza-node
cargo run -- --node-id argentina-17
```

### Meza-Panel

Install dependencies first, then run the Next.js app:

```bash
cd apps/meza-panel
npm install
npm run dev
```

If the core API is running elsewhere, set `MEZA_CORE_BASE_URL`.

```bash
MEZA_CORE_BASE_URL=http://127.0.0.1:8080 \
MEZA_PANEL_OPERATOR_TOKEN=dev-operator-token \
npm run dev
```

## Configuration

Copy `.env.example` to `.env` and set real tokens before deploying anywhere beyond local development.

Important variables:

- `MEZA_CORE_ADDR`
- `MEZA_CORE_HOST_PORT`
- `MEZA_CORE_DATA_PATH`
- `MEZA_OPERATOR_TOKEN`
- `MEZA_BOOTSTRAP_TOKEN`
- `MEZA_NODE_TOKEN`
- `MEZA_ALLOW_ANONYMOUS_UI`
- `MEZA_AI_PROVIDER`
- `MEZA_TERMINAL_LOCAL_EXEC_ENABLED`
- `MEZA_GEMINI_API_KEY`
- `MEZA_GEMINI_MODEL`
- `MEZA_GEMINI_BASE_URL`
- `MEZA_CORE_BASE_URL`
- `MEZA_PANEL_OPERATOR_TOKEN`
- `MEZA_PANEL_TLS_HOST`

## First Implemented Endpoints

- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/dashboard`
- `GET /api/v1/nodes`
- `POST /api/v1/nodes/register`
- `POST /api/v1/nodes/heartbeat`
- `GET /api/v1/jobs`
- `GET /api/v1/jobs/{id}`
- `POST /api/v1/jobs`
- `POST /api/v1/jobs/{id}/approve`
- `POST /api/v1/jobs/{id}/start`
- `POST /api/v1/terminal/stream` (SSE terminal stream)
- `GET /api/v1/audit`
- `POST /api/v1/ai/interpret`
- `POST /api/v1/ai/plan-and-create`

## Local Demo Flow

1. Start `Meza-Core`.
2. Open `Meza-Panel`.
3. Create or AI-plan a job.
4. Approve privileged jobs.
5. Start the rollout and inspect audit output.

Sample AI flow:

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/ai/plan-and-create \
  -H 'Authorization: Bearer dev-operator-token' \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"обнови docker на ноде argentina-17"}'
```

Approve and start the created job:

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/jobs/job-2/approve \
  -H 'Authorization: Bearer dev-operator-token' \
  -H 'Content-Type: application/json' \
  -d '{"actor":"reviewer"}'

curl -s -X POST http://127.0.0.1:8080/api/v1/jobs/job-2/start \
  -H 'Authorization: Bearer dev-operator-token' \
  -H 'Content-Type: application/json' \
  -d '{"actor":"operator"}'
```

## Auto Installers (Hub + Node)

Unified installer:

```bash
curl -fsSL https://raw.githubusercontent.com/ASTRACAT2022/MegaMozg/main/scripts/install.sh | bash -s -- -install hub
curl -fsSL https://raw.githubusercontent.com/ASTRACAT2022/MegaMozg/main/scripts/install.sh | bash -s -- -install node <HUB_IP> <BOOTSTRAP_TOKEN>
```

What it does:
- `-install hub`: installs Docker stack (`meza-core` + `meza-panel` + SSL proxy), generates tokens, creates self-signed TLS cert with SAN for server IP, starts services.
- `-install node`: auto-registers node and starts heartbeat service with no extra clicks.
- If host `8080` is busy, installer auto-picks another host port for core (for example `18080`), while panel remains on `1499`.
- Hub installer also configures HTTP Basic Auth for panel access (`MEZA_PANEL_BASIC_AUTH_USER` / `MEZA_PANEL_BASIC_AUTH_PASSWORD`) and prints credentials at the end.

## Container Deployment

```bash
cp .env.example .env
docker compose up --build
```

This brings up:

- `meza-core` on `http://127.0.0.1:8080`
- `meza-panel` behind HTTPS on `https://localhost:1499` (self-signed cert from `deploy/certs/panel.crt`)

Panel login protection can be configured via `.env`:

```bash
MEZA_PANEL_BASIC_AUTH_ENABLED=true
MEZA_PANEL_BASIC_AUTH_USER=admin
MEZA_PANEL_BASIC_AUTH_PASSWORD=change-me
MEZA_PANEL_BASIC_AUTH_SESSION_TOKEN=change-me-session-token
```

## Gemini Planner

The control plane supports two planner providers:

- `stub`: deterministic local planner for development and offline fallback
- `gemini`: Gemini function-calling planner with automatic fallback to `stub`

To enable Gemini:

```bash
export MEZA_AI_PROVIDER=gemini
export MEZA_GEMINI_API_KEY=your-key
export MEZA_GEMINI_MODEL=gemini-2.5-flash
go run ./apps/meza-core/cmd/api
```

## Remaining Production Gaps

1. Replace file-backed state with PostgreSQL and Redis.
2. Add full RBAC, per-user sessions, and approval policies.
3. Add multi-step Gemini tool loops instead of single-shot typed planning.
4. Introduce gRPC streaming for node heartbeats, logs, and command execution.
5. Implement typed real execution on agents, mTLS, and signed self-update flows.
