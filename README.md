# agent-team-go

[![CI](https://github.com/daewoochen/agent-team-go/actions/workflows/ci.yml/badge.svg)](https://github.com/daewoochen/agent-team-go/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/daewoochen/agent-team-go)](./LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/daewoochen/agent-team-go)](./go.mod)
[![Stars](https://img.shields.io/github/stars/daewoochen/agent-team-go?style=social)](https://github.com/daewoochen/agent-team-go/stargazers)

Build and run AI-native agent teams in Go.

`agent-team-go` is a Go-first platform skeleton for teams of agents that can coordinate work, install the skills they need, and connect to real delivery channels like Feishu and Telegram.

![Social preview](./assets/social-preview.svg)

[Chinese docs](./docs/zh-cn/README.md) · [Contributing](./CONTRIBUTING.md) · [Security](./SECURITY.md)

## Why this exists

Most agent frameworks stop at orchestration demos. Production teams need more:

- Structured delegation instead of prompt-only handoffs
- Custom skills with auto-install from local, registry, or git sources
- Channel adapters for Feishu, Telegram, and CLI-first workflows
- Replayable runs, artifacts, and event logs
- A clean Go codebase that is simple to deploy and extend

This repository is the first public release of that direction.

## Core promises

- `Custom Skills`: define your own skill packages and keep them versioned
- `Auto Skill Install`: missing skills are resolved and installed before a run
- `Feishu / Telegram Gateway`: channel adapters are first-class, not an afterthought
- `Structured Delegation`: captain, planner, researcher, coder, reviewer all work through typed work items
- `Replay Logs`: every run emits events and artifacts that can be replayed later
- `Checkpoints + Approvals`: runs persist checkpoints and approval events for safer execution
- `Team Memory`: compact history from earlier runs feeds into future planning and synthesis
- `Revision Loop`: operators can request changes, resume the run, and re-review the revised draft
- `Model Bindings`: each agent can declare its own model while providers are configured once at the team level
- `Retry-Aware Execution`: work items can retry and surface blocked dependencies instead of failing silently
- `Real Delivery`: enabled channels can send real Telegram and Feishu messages, not only previews
- `Incoming Gateway`: Telegram and Feishu webhook events can trigger auto-generated teams directly
- `Session-Aware Bots`: every chat keeps profile preference, recent turns, and last run summary
- `Pause / Resume`: manual approval mode can pause a run, persist state, and resume after a human decision

## Quick start

### 1. Run the example

```bash
git clone git@github.com:daewoochen/agent-team-go.git
cd agent-team-go
go run ./cmd/agentteam run \
  --team ./examples/software-team/team.yaml \
  --task "Launch the public MVP and de-risk the first release"
```

### 1.5 Just give it a task

```bash
go run ./cmd/agentteam auto \
  --task "Compare the top Go agent runtimes and propose our launch angle"
```

### 1.6 Run it as a bot backend

```bash
go run ./cmd/agentteam serve --listen :8080 --deliver
```

Then talk to the bot with simple control commands:

```text
/help
/memory
/reset
/profile research
/profile assistant Draft the launch update
```

### 2. Validate channels

```bash
go run ./cmd/agentteam channels validate --team ./examples/software-team/team.yaml
```

### 3. Install a skill manually

```bash
go run ./cmd/agentteam skills install \
  --name github \
  --source local \
  --path ./skills/github
```

### 4. Scaffold a custom skill

```bash
go run ./cmd/agentteam skills scaffold \
  --name launch-writer \
  --dir ./skills/launch-writer \
  --description "Draft release-ready launch notes"
```

### 5. Browse the skill catalog

```bash
go run ./cmd/agentteam skills search --query messenger
go run ./cmd/agentteam skills list --workdir .
```

### 6. Bootstrap your own team

```bash
go run ./cmd/agentteam init --name my-team --dir ./demo
```

### 7. Explain model setup

```bash
go run ./cmd/agentteam models explain --team ./examples/software-team/team.yaml
```

### 8. Inspect the team topology

```bash
go run ./cmd/agentteam inspect team --team ./examples/software-team/team.yaml
go run ./cmd/agentteam inspect team --team ./examples/software-team/team.yaml --format mermaid
```

### 9. Inspect a replay

```bash
go run ./cmd/agentteam replay show --run ./.agentteam/runs/<run-id>.json
```

### 10. Inspect persistent team memory

```bash
go run ./cmd/agentteam memory show --team ./examples/software-team/team.yaml
```

### 11. Pause for approval and resume

```bash
go run ./cmd/agentteam run \
  --team ./examples/manual-approval-team/team.yaml \
  --task "Prepare the launch response and guarded rollout plan"

go run ./cmd/agentteam approvals show --checkpoint ./.agentteam/checkpoints/<run-id>.json
go run ./cmd/agentteam approvals approve --checkpoint ./.agentteam/checkpoints/<run-id>.json --all
go run ./cmd/agentteam resume --team ./examples/manual-approval-team/team.yaml --checkpoint ./.agentteam/checkpoints/<run-id>.json
```

If the operator wants the team to revise the draft instead of approving immediately:

```bash
go run ./cmd/agentteam approvals request-changes \
  --checkpoint ./.agentteam/checkpoints/<run-id>.json \
  --id approval-outbound-message \
  --note "Add rollback guidance and make the customer message more conservative"

go run ./cmd/agentteam resume --team ./examples/manual-approval-team/team.yaml --checkpoint ./.agentteam/checkpoints/<run-id>.json
```

If the operator wants to stop the run instead of continuing:

```bash
go run ./cmd/agentteam approvals reject \
  --checkpoint ./.agentteam/checkpoints/<run-id>.json \
  --id approval-outbound-message \
  --note "Need a safer rollout and external review first"
```

## What the MVP already does

- Parses a declarative `team.yaml`
- Validates channel configuration
- Validates model provider configuration and API key env bindings
- Ensures required skills are installed before a run
- Runs a hierarchical team loop with structured delegations, retries, and dependency-aware scheduling
- Produces work items, approvals, artifacts, checkpoints, replay logs, compact team memory, real or preview deliveries, and resumable paused runs

## Real Telegram and Feishu delivery

Use environment bindings in `team.yaml`:

```yaml
channels:
  - kind: telegram
    enabled: true
    token: env:TELEGRAM_BOT_TOKEN
    allow_from: [env:TELEGRAM_CHAT_ID]
  - kind: feishu
    enabled: true
    app_id: env:FEISHU_APP_ID
    app_secret: env:FEISHU_APP_SECRET
    allow_from: [env:FEISHU_CHAT_ID]
```

Then send the prepared deliveries:

```bash
go run ./cmd/agentteam run --team ./examples/assistant-team/team.yaml --task "Draft the launch update" --deliver
go run ./cmd/agentteam channels deliver --team ./examples/assistant-team/team.yaml --run ./.agentteam/runs/<run-id>.json
```

`token`, `app_id`, `app_secret`, and `allow_from` entries all support `env:VAR_NAME` so you can keep secrets out of committed YAML.

## Incoming webhook gateway

If you want a dumb-simple bot backend, run:

```bash
go run ./cmd/agentteam serve --listen :8080 --deliver
```

Endpoints:

```text
POST /webhooks/telegram
POST /webhooks/feishu
GET  /healthz
```

What happens on each incoming message:

1. The gateway normalizes the message into a task.
2. `agent-team-go` auto-selects a team profile such as research, incident, or software.
3. The team runs with memory enabled.
4. The final summary is sent back to the source Telegram chat or Feishu chat when `--deliver` is enabled.

Each chat also gets its own lightweight session state under `.agentteam/sessions/`, so follow-up messages can reuse the recent conversation, remember a preferred profile, and keep the bot feeling continuous instead of stateless.

Supported chat commands:

```text
/help
/memory
/reset
/profile <auto|software|research|incident|content|assistant>
/profile <profile> <task>
```

This means a non-technical user can do things like:

1. Send `/profile incident`
2. Ask "Summarize the sev1 blast radius"
3. Ask "Now turn that into a stakeholder update"

The second and third messages stay in the same session, and the saved profile keeps the team composition stable for that chat.

## Inspect and reset bot sessions

```bash
go run ./cmd/agentteam sessions list --workdir .
go run ./cmd/agentteam sessions show --channel telegram --target 12345
go run ./cmd/agentteam sessions reset --channel telegram --target 12345
```

Telegram example:

```bash
curl -X POST http://127.0.0.1:8080/webhooks/telegram \
  -H 'Content-Type: application/json' \
  -d '{"message":{"text":"Prepare an incident response brief","chat":{"id":12345},"from":{"id":7}}}'
```

Feishu example:

```bash
curl -X POST http://127.0.0.1:8080/webhooks/feishu \
  -H 'Content-Type: application/json' \
  -d '{"header":{"event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_x"}},"message":{"chat_id":"oc_123","message_type":"text","content":"{\"text\":\"Draft the launch update\"}"}}}'
```

## Persistent team memory

Agent teams should not lose every lesson after a run finishes.

Enable file-backed memory in `team.yaml`:

```yaml
memory:
  backend: file
  path: .agentteam/memory/release-history.json
  max_entries: 8
```

Then inspect it:

```bash
go run ./cmd/agentteam memory show --team ./examples/release-memory-team/team.yaml
```

This is useful for recurring cases such as release management, incident follow-up, customer support triage, and weekly research programs.

## Configure model API keys

Model providers live under `models.providers` in `team.yaml`. The recommended pattern is:

1. Put the real secret in an environment variable
2. Reference that variable with `api_key_env`
3. Point each agent at a model like `openai/gpt-4.1-mini`

Example:

```yaml
models:
  default_model: openai/gpt-4.1-mini
  providers:
    openai:
      kind: openai-compatible
      base_url: https://api.openai.com/v1
      api_key_env: OPENAI_API_KEY

agents:
  - name: captain
    role: captain
    model: openai/gpt-4.1
```

Then export the key before you run the team:

```bash
export OPENAI_API_KEY=your_api_key
go run ./cmd/agentteam models validate --team ./team.yaml
```

`agentteam` will also auto-load a `.env` file from the current working directory and the team spec directory when present. The repo ships an [.env.example](./.env.example) file with common variable names.

## Auto-generated teams

If you want the easiest path, use the auto mode.

```bash
go run ./cmd/agentteam auto --task "Prepare an incident response brief for the sev1 outage"
```

The CLI will:

1. Classify the task into a built-in team profile such as software, research, incident, content, or assistant.
2. Build a ready-to-run team with captain/planner/specialists.
3. Enable persistent memory by default.
4. Automatically use OpenAI if `OPENAI_API_KEY` is present, otherwise fall back to deterministic mock providers.
5. Auto-enable Telegram or Feishu delivery when the relevant env vars are present.

## Example architecture

```mermaid
flowchart TD
    User["User / Trigger"] --> CLI["agentteam CLI"]
    CLI --> Loader["TeamSpec Loader"]
    Loader --> Skills["Skill Resolver + Installer"]
    Loader --> Policy["Policy Gate"]
    Skills --> Runtime["Hierarchical Runtime"]
    Runtime --> Captain["Captain"]
    Captain --> Planner["Planner"]
    Captain --> Researcher["Researcher"]
    Captain --> Coder["Coder"]
    Captain --> Reviewer["Reviewer"]
    Runtime --> Channels["CLI / Telegram / Feishu"]
    Runtime --> Replay["Replay Log + Artifacts"]
```

## Typical launch-worthy scenarios

1. `Software Team`
   Captain coordinates Planner, Researcher, Coder, and Reviewer to ship a feature or release.
2. `Assistant Team`
   Coordinator receives incoming requests, routes them to specialists, and reports progress back to Feishu or Telegram.
3. `Ops Team`
   A captain agent validates channel access, installs missing skills, and assembles a safe execution plan.
4. `Manual Approval Team`
   A run pauses for human approval before protected actions, then resumes from checkpoint.
5. `Deep Research Team`
   Researcher and Reviewer build a fact package while the captain prepares a final synthesis.
6. `Incident Response Team`
   Captain coordinates evidence gathering and approval-aware stakeholder updates.
7. `Content Studio Team`
   A small team plans, drafts, and reviews launch assets using reusable skills.
8. `Release Memory Team`
   A recurring release team remembers prior risks, decisions, and follow-up tasks across runs.
9. `Auto Team`
   Give the CLI a task and let it choose the agent mix for you.

More example specs live in [examples/README.md](./examples/README.md).
If you want a real provider example, start from [examples/openai-launch-team/team.yaml](./examples/openai-launch-team/team.yaml).

## Why Go

- Single binary distribution
- Strong typing for specs, work items, and delegation contracts
- Great fit for concurrent run orchestration
- Friendly to platform teams that want predictable operations

## Why not another agent framework

This repo is intentionally opinionated:

- It starts from team execution, not just model orchestration
- It treats skills and channels as platform primitives
- It keeps the code small enough to learn, fork, and ship

## Roadmap

- `v0.1`: CLI, TeamSpec, skill resolver, local runtime, CLI channel
- `v0.2`: richer Telegram and Feishu adapters, stronger policy hooks
- `v0.3`: MCP bridge, better artifact handling, richer replay visualization
- `v0.4`: A2A bridge, sandbox execution, web console

## Repo layout

```text
cmd/agentteam          # CLI entrypoint
pkg/spec               # TeamSpec, AgentSpec, SkillManifest, channel config
pkg/runtime            # Run loop, delegation events, replay model
pkg/skills             # Skill resolver, installer, registry placeholder
pkg/channels           # CLI / Telegram / Feishu adapters
pkg/agents             # Role helpers
pkg/policy             # Download / install policy hooks
pkg/observe            # Replay log writer
examples/              # Runnable team templates
skills/                # Bundled skills
docs/                  # Extra documentation
```

## New in this iteration

- Team-level model provider config with per-agent model selection
- real `openai-compatible` provider support alongside deterministic mock providers
- `agentteam models explain` and `agentteam models validate`
- `agentteam skills scaffold`, `skills search`, and `skills list`
- `agentteam inspect team --format text|mermaid`
- retry-aware work items with blocked-dependency events
- prepared channel delivery previews in run output and replay logs
- manual approval mode with checkpoint-backed `approvals show/approve` and `resume`
- approval rejection and operator notes that flow back into resumed runs
- request-changes approval loops that revise the draft and reopen approvals for re-review
- replay inspection via `agentteam replay show`
- persistent team memory with `agentteam memory show`
- auto-generated teams via `agentteam auto`
- real Telegram and Feishu delivery via `--deliver` and `channels deliver`
- incoming webhook gateway via `agentteam serve`
- checkpoint persistence under `.agentteam/checkpoints/`
- richer example cases for research, incident response, and content teams

## Current status

This is a polished MVP skeleton. It is meant to be runnable, readable, and easy to extend. The next step after the initial launch is to replace placeholder integrations with full production adapters while keeping the public interfaces stable.

## Contributing

Issues and pull requests are welcome. Good first contributions:

- richer skill manifests
- more realistic delegation strategies
- deeper Telegram / Feishu validation
- replay visualizers
- MCP and sandbox integrations

If this direction resonates with you, give the repo a star and share it with one builder who is tired of fragile agent demos.
