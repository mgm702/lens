# Example: Customer Support

Evaluates a customer support AI across common service scenarios — returns,
refunds, and order issues. A good starting point for teams building support bots.

## What's included

- 2 personas (frustrated customer, first-time buyer)
- 3 scenarios (return request, refund status, damaged item)
- Rubric covering issue resolution, policy explanation, empathy, and tone

## Run it

Copy `.env.example` to `.env` and fill in your API key:

```bash
cp .env.example .env
# edit .env and set ANTHROPIC_API_KEY
lens run experiment-smoke.yaml
lens analyze results/customer-support-smoke/
lens report results/customer-support-smoke/ --open
```

## Adapt it for your own bot

1. Replace `prompts/system.md` with your system prompt
2. Edit `personas.json` and `scenarios.json` to match your users and use cases
3. Adjust `rubric.yaml` to reflect what good looks like for your product
