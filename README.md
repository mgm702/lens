# Lens

**Domain-agnostic conversational AI evaluation harness** — simulate users, run multi-turn conversations against any AI target system, and score results with a configurable LLM judge.

## Quick Start

```bash
# Install via Homebrew (macOS/Linux)
brew install mgm702/lens/lens

# Or build from source
go install github.com/mgm702/lens/cmd/lens@latest
```

Run the included customer-support smoke test (requires `ANTHROPIC_API_KEY`):

```bash
lens run examples/customer-support/experiment-smoke.yaml
lens analyze results/customer-support-smoke/
lens report results/customer-support-smoke/ --open
```

## What It Does

Lens orchestrates three components:

1. **Simulated user** — an LLM that plays a persona from your `personas.json`, following a scenario and info-level rules
2. **Target system** — the AI you're evaluating (Claude, GPT-4, your internal API, etc.)
3. **LLM judge** — scores the completed transcript against a rubric you define

Each (persona × scenario × info_level × rep) combination runs concurrently in a bounded worker pool. Results are written as per-case JSON + a flat CSV, and an optional HTML report lets you browse individual transcripts.

## Scaffold a New Experiment

```bash
lens new my-chatbot-eval
cd my-chatbot-eval
# Edit experiment.yaml, personas.json, scenarios.json, rubric.yaml, prompts/
lens validate experiment.yaml
lens run experiment.yaml
```

## Adapters

| Type | Description | Auth |
|------|-------------|------|
| `anthropic` | Anthropic Messages API (Claude) | `ANTHROPIC_API_KEY` |
| `openai` | OpenAI Chat Completions (GPT) | `OPENAI_API_KEY` |
| `bedrock` | AWS Bedrock Converse API | AWS credentials |
| `http` | Generic HTTP with request template + JSONPath | bearer / api_key / basic / custom |
| `mock` | Scripted responses for harness testing | none |

```bash
lens adapters list
```

## Configuration

`experiment.yaml` controls everything:

```yaml
name: my-eval
target:
  type: anthropic
  model: claude-haiku-4-5-20251001
  system_prompt_file: prompts/system.md
simulator:
  provider: anthropic
  model: claude-haiku-4-5-20251001
  prompt_file: prompts/user_sim.md
judge:
  provider: anthropic
  model: claude-sonnet-4-6
  prompt_file: prompts/judge.md
  rubric_file: rubric.yaml
personas_file: personas.json
scenarios_file: scenarios.json
info_levels: [full, partial]
reps: 3
max_turns: 10
workers: 4
```

## Commands

```
lens run <experiment.yaml>      Run an experiment
lens analyze <results-dir>      Aggregate and display results
lens report <results-dir>       Build an HTML report
lens validate <experiment.yaml> Validate config before running
lens new <name>                 Scaffold a new experiment directory
lens adapters list              List available adapters
```

## Rubric Format

```yaml
hard_gates:
  - id: no_harm
    type: conversation
    description: "No harmful advice given"

outcomes:
  - id: question_answered
    type: conversation      # averaged across conversation
    description: "Main question was answered"
  - id: actionable_step
    type: item_level        # micro-averaged across turns
    description: "At least one actionable next step"

indicators:
  - id: clarity
    scale: 5
    description: "Response clarity"
    anchor_low: "confusing"
    anchor_high: "crystal clear"
```

**Composite score**: any hard gate failure → 0.0; otherwise mean of (item_level fraction passing, conversation fraction passing).

## License

MIT — see [LICENSE](LICENSE)
