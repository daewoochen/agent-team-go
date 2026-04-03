# Example Cases

These examples are designed to show what an agent team can look like before you plug in real models.

## Runnable examples

- `software-team`
  Ship a public MVP with planner, researcher, coder, and reviewer roles.
- `assistant-team`
  Route incoming requests, retry critical work items, and prepare channel-aware delivery previews.
- `deep-research-team`
  Produce a research package and keep Feishu delivery in the loop.
- `incident-response-team`
  Coordinate incident triage, evidence gathering, and stakeholder communication.
- `content-studio-team`
  Plan and review launch content with structured delegation.
- `openai-launch-team`
  A real-provider configuration example that uses `OPENAI_API_KEY`.
- `manual-approval-team`
  Pause the run until a human approves protected actions, then resume from checkpoint.
- `release-memory-team`
  Reuse recent launch history so the team carries forward open risks and lessons across runs.

## Run one

```bash
go run ./cmd/agentteam run --team ./examples/deep-research-team/team.yaml --task "Compare the top Go agent runtimes and propose our launch angle"
```

Or let the CLI choose a team for you:

```bash
go run ./cmd/agentteam auto --task "Prepare an incident response brief for the sev1 outage"
```

Or run the webhook gateway:

```bash
go run ./cmd/agentteam serve --listen :8080 --deliver
```

## Inspect the team before you run it

```bash
go run ./cmd/agentteam inspect team --team ./examples/software-team/team.yaml
go run ./cmd/agentteam inspect team --team ./examples/software-team/team.yaml --format mermaid
```

## Try the approval flow

```bash
go run ./cmd/agentteam run --team ./examples/manual-approval-team/team.yaml --task "Prepare the launch response and guarded rollout plan"
go run ./cmd/agentteam approvals show --checkpoint ./.agentteam/checkpoints/<run-id>.json
go run ./cmd/agentteam approvals approve --checkpoint ./.agentteam/checkpoints/<run-id>.json --all
go run ./cmd/agentteam resume --team ./examples/manual-approval-team/team.yaml --checkpoint ./.agentteam/checkpoints/<run-id>.json
```

To stop the run instead of resuming it:

```bash
go run ./cmd/agentteam approvals reject --checkpoint ./.agentteam/checkpoints/<run-id>.json --id approval-outbound-message --note "Need a safer rollout first"
```

To request changes and re-review the revised draft:

```bash
go run ./cmd/agentteam approvals request-changes --checkpoint ./.agentteam/checkpoints/<run-id>.json --id approval-outbound-message --note "Add rollback guidance and make the message more conservative"
go run ./cmd/agentteam resume --team ./examples/manual-approval-team/team.yaml --checkpoint ./.agentteam/checkpoints/<run-id>.json
```

## Try persistent team memory

```bash
go run ./cmd/agentteam run --team ./examples/release-memory-team/team.yaml --task "Plan the v0.2 launch and capture the open risks"
go run ./cmd/agentteam run --team ./examples/release-memory-team/team.yaml --task "Prepare the follow-up release after last week's launch issues"
go run ./cmd/agentteam memory show --team ./examples/release-memory-team/team.yaml
```

## Switch to a real model provider

The examples use `mock/*` models so they are runnable without credentials.

When you are ready to use a real provider:

1. Add a provider under `models.providers`
2. Point `api_key_env` at an environment variable such as `OPENAI_API_KEY`
3. Change agent models from `mock/...` to `openai/...` or your provider prefix
4. Optionally place the key in a `.env` file in the repo root or next to the team spec

## Deliver to real channels

```bash
go run ./cmd/agentteam run --team ./examples/assistant-team/team.yaml --task "Draft the launch update" --deliver
go run ./cmd/agentteam channels deliver --team ./examples/assistant-team/team.yaml --run ./.agentteam/runs/<run-id>.json
```
