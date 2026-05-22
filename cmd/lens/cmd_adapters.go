package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newAdaptersCmd() *cobra.Command {
	adapters := &cobra.Command{
		Use:   "adapters",
		Short: "Adapter management commands",
	}
	adapters.AddCommand(newAdaptersListCmd())
	return adapters
}

func newAdaptersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered target adapters",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listAdapters()
		},
	}
}

var adapterDescriptions = []struct {
	name        string
	description string
	auth        string
}{
	{
		name:        "anthropic",
		description: "Direct Anthropic Messages API (Claude models)",
		auth:        "ANTHROPIC_API_KEY env var",
	},
	{
		name:        "openai",
		description: "Direct OpenAI Chat Completions API (GPT models)",
		auth:        "OPENAI_API_KEY env var",
	},
	{
		name:        "bedrock",
		description: "AWS Bedrock Converse API (Claude, Llama, etc.)",
		auth:        "AWS credentials (env vars or ~/.aws/credentials)",
	},
	{
		name:        "http",
		description: "Generic HTTP adapter with request template and JSONPath extraction",
		auth:        "bearer | api_key | basic | custom | none",
	},
	{
		name:        "mock",
		description: "Scripted adapter for testing (no LLM calls, deterministic responses)",
		auth:        "none",
	},
}

func listAdapters() error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(w, "TYPE\t  DESCRIPTION\t  AUTH")
	fmt.Fprintln(w, "----\t  -----------\t  ----")
	for _, a := range adapterDescriptions {
		fmt.Fprintf(w, "%s\t  %s\t  %s\n", a.name, a.description, a.auth)
	}
	return nil
}
