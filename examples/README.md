# Example Cases

These examples are designed to show what an agent team can look like before you plug in real models.

## Runnable examples

- `software-team`
  Ship a public MVP with planner, researcher, coder, and reviewer roles.
- `assistant-team`
  Route incoming requests and prepare channel-aware responses.
- `deep-research-team`
  Produce a research package and keep Feishu delivery in the loop.
- `incident-response-team`
  Coordinate incident triage, evidence gathering, and stakeholder communication.
- `content-studio-team`
  Plan and review launch content with structured delegation.
- `openai-launch-team`
  A real-provider configuration example that uses `OPENAI_API_KEY`.

## Run one

```bash
go run ./cmd/agentteam run --team ./examples/deep-research-team/team.yaml --task "Compare the top Go agent runtimes and propose our launch angle"
```

## Inspect the team before you run it

```bash
go run ./cmd/agentteam inspect team --team ./examples/software-team/team.yaml
go run ./cmd/agentteam inspect team --team ./examples/software-team/team.yaml --format mermaid
```

## Switch to a real model provider

The examples use `mock/*` models so they are runnable without credentials.

When you are ready to use a real provider:

1. Add a provider under `models.providers`
2. Point `api_key_env` at an environment variable such as `OPENAI_API_KEY`
3. Change agent models from `mock/...` to `openai/...` or your provider prefix
4. Optionally place the key in a `.env` file in the repo root or next to the team spec
