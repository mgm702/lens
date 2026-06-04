# Contributing to Lens

Thanks for your interest in contributing. This guide covers how to get set up, run tests, and add new adapters.

## Prerequisites

- Go 1.26+
- `golangci-lint` for linting (`brew install golangci-lint`)

## Setup

```bash
git clone https://github.com/mgm702/lens
cd lens
go mod download
make build
```

Verify the build works:

```bash
make smoke   # builds lens and runs the customer-support example (requires ANTHROPIC_API_KEY)
```

## Running Tests

```bash
make test    # unit tests only (no credentials required)
make lint    # run golangci-lint
```

Integration tests hit live APIs and are skipped by default. To run them:

```bash
LENS_INTEGRATION_TESTS=1 ANTHROPIC_API_KEY=... go test ./internal/adapters/anthropic/...
LENS_INTEGRATION_TESTS=1 OPENAI_API_KEY=... go test ./internal/adapters/openai/...
LENS_INTEGRATION_TESTS=1 AWS_PROFILE=... go test ./internal/adapters/bedrock/...
```

## Project Structure

```
cmd/lens/           CLI entry point (Cobra commands)
internal/
  adapters/         One package per adapter type (anthropic, openai, bedrock, http, mock)
  config/           experiment.yaml parsing and validation
  judge/            LLM judge and rubric scoring
  llm/              Shared LLM caller (used by simulator and judge)
  runner/           Experiment grid expansion and concurrent execution
  simulator/        Simulated user turn generation
  progress/         Terminal progress bar
  report/           HTML report builder
  results/          JSON + CSV result writing
pkg/types/          Public interfaces (Adapter, Session)
examples/           Runnable example experiments
```

## Adding a New Adapter

1. Create a new package under `internal/adapters/<name>/`
2. Implement the `types.Adapter` interface from `pkg/types/`:

```go
type Adapter interface {
    CreateSession(ctx context.Context, cfg SessionConfig) (Session, error)
    Send(ctx context.Context, session Session, message string) (string, PerfStats, error)
    CloseSession(ctx context.Context, session Session) error
}
```

3. Register it in the factory at `internal/adapters/adapter.go`:

```go
case "myname":
    return myadapter.New(cfg)
```

4. Add a row to the adapters table in `README.md`
5. Add an integration test following the pattern in `internal/adapters/anthropic/adapter_test.go`

## Submitting a PR

- Keep PRs focused — one adapter, one fix, one feature
- Run `make test && make lint` before opening a PR
- If you're adding an adapter, include a minimal working example under `examples/`
