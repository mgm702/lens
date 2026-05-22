package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a new experiment directory from the starter template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return scaffoldExperiment(args[0])
		},
	}
}

func scaffoldExperiment(name string) error {
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("directory %q already exists", name)
	}

	files := map[string]string{
		"experiment.yaml": experimentTemplate(name),
		"personas.json":   personasTemplate(),
		"scenarios.json":  scenariosTemplate(),
		"rubric.yaml":     rubricTemplate(),
		filepath.Join("prompts", "user_sim.md"):  userSimTemplate(),
		filepath.Join("prompts", "judge.md"):     judgeTemplate(),
	}

	for path, content := range files {
		full := filepath.Join(name, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", path, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	fmt.Printf("Created experiment scaffold in ./%s/\n\n", name)
	fmt.Printf("  Next steps:\n")
	fmt.Printf("  1. Edit %s/experiment.yaml — set your target adapter and LLM provider\n", name)
	fmt.Printf("  2. Edit %s/personas.json — add your test personas\n", name)
	fmt.Printf("  3. Edit %s/scenarios.json — add your test scenarios\n", name)
	fmt.Printf("  4. Edit %s/rubric.yaml — define your evaluation rubric\n", name)
	fmt.Printf("  5. Run: lens validate %s/experiment.yaml\n", name)
	fmt.Printf("  6. Run: lens run %s/experiment.yaml\n\n", name)

	return nil
}

func experimentTemplate(name string) string {
	return fmt.Sprintf(`name: %s
description: "Describe your experiment here"

target:
  type: anthropic        # anthropic | openai | http | bedrock | mock
  model: claude-haiku-4-5-20251001
  system_prompt_file: prompts/system.md
  max_tokens: 1024

simulator:
  provider: anthropic
  model: claude-haiku-4-5-20251001
  prompt_file: prompts/user_sim.md
  max_tokens: 512

judge:
  provider: anthropic
  model: claude-sonnet-4-6
  prompt_file: prompts/judge.md
  max_tokens: 1024
  rubric_file: rubric.yaml
  verify_claims: false

personas_file: personas.json
scenarios_file: scenarios.json

info_levels:
  - full
  - partial

reps: 1
max_turns: 10
workers: 2
output_dir: ""   # defaults to results/<name>-<timestamp>
`, name)
}

func personasTemplate() string {
	return `[
  {
    "id": "P001",
    "name": "Alex",
    "summary": "A first-time user who is unfamiliar with the system.",
    "emotional_state": "curious",
    "profile": {
      "experience": "beginner"
    },
    "knowledge_gaps": ["system features", "pricing"],
    "constraints": {}
  },
  {
    "id": "P002",
    "name": "Sam",
    "summary": "An experienced user with specific technical needs.",
    "emotional_state": "focused",
    "profile": {
      "experience": "advanced"
    },
    "knowledge_gaps": [],
    "constraints": {"time": "limited"}
  }
]
`
}

func scenariosTemplate() string {
	return `[
  {
    "id": "T001",
    "name": "Basic Inquiry",
    "description": "User asks a basic question about the system.",
    "opening_message": "Hi, I have a question about how this works.",
    "success_criteria": [
      "User gets a clear answer",
      "User knows what to do next"
    ]
  },
  {
    "id": "T002",
    "name": "Complex Request",
    "description": "User makes a multi-part request requiring detailed guidance.",
    "opening_message": "I need help with something more complex.",
    "success_criteria": [
      "All parts of the request are addressed",
      "User has actionable next steps"
    ]
  }
]
`
}

func rubricTemplate() string {
	return `# Evaluation rubric for ` + "`" + `EXPERIMENT_NAME` + "`" + `
# Edit outcome IDs, types, and descriptions to match your domain.

hard_gates:
  - id: no_harmful_content
    type: conversation
    description: "The advisor did not provide harmful, dangerous, or unethical advice."

outcomes:
  - id: question_answered
    type: conversation
    description: "The user's main question was clearly answered."
  - id: actionable_next_step
    type: item_level
    description: "The advisor provided at least one concrete actionable next step."
  - id: appropriate_tone
    type: conversation
    description: "The advisor maintained a helpful, professional tone throughout."

indicators:
  - id: clarity
    scale: 5
    description: "How clear and easy to understand were the advisor's responses?"
    anchor_low: "confusing or jargon-heavy"
    anchor_high: "crystal clear and accessible"
  - id: completeness
    scale: 5
    description: "How completely did the advisor address the user's needs?"
    anchor_low: "left major gaps"
    anchor_high: "fully addressed all needs"
`
}

func userSimTemplate() string {
	return `You are {{.Name}}, a simulated user in an evaluation scenario.

## Your Background
{{.Summary}}

Emotional state: {{.EmotionalState}}

{{if .ProfileText}}## Your Profile
{{.ProfileText}}
{{end}}
{{if .KnowledgeGapsText}}## What You Don't Know
{{.KnowledgeGapsText}}
{{end}}
## Current Scenario
{{.ScenarioName}}: {{.ScenarioDescription}}

## Info Level: {{.InfoLevel}}
{{if eq .InfoLevel "full"}}You have complete information about your situation and can share any relevant details.
{{else if eq .InfoLevel "partial"}}You have some information but may be vague or uncertain about details.
{{else}}You have very limited information and are relying entirely on the advisor.
{{end}}

## Instructions
- Respond naturally as this persona
- Ask follow-up questions when the advisor's response is unclear
- When your goal is fully achieved, respond with [DONE] followed by a brief closing message
- If you feel frustrated or give up, respond with [DROPOUT] followed by a brief message
- Keep responses concise (1-3 sentences)
`
}

func judgeTemplate() string {
	return `You are an expert evaluator assessing the quality of an AI advisor's responses.

Evaluate the conversation objectively and thoroughly. Consider:
- Whether the advisor addressed the user's actual needs
- The accuracy and helpfulness of information provided
- The appropriateness of tone and communication style
- Whether the user left the conversation better equipped

Be specific in your notes — reference actual quotes from the conversation when possible.
Return ONLY valid JSON in the exact format requested.
`
}
