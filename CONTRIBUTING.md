# Contributing

Thanks for helping build `agent-team-go`.

## Development loop

```bash
make fmt
make test
make build
```

## Principles

- Keep public types stable and explicit
- Prefer clear extension points over clever abstractions
- Make examples runnable before adding more features
- Preserve replayability and observability when adding runtime behavior

## Pull request checklist

- Add or update tests for behavior changes
- Keep README or docs aligned with user-facing changes
- Explain why the change matters for real agent teams
- Avoid adding heavy dependencies without a strong reason
