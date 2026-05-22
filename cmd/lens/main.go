// Command lens is the CLI entry point for the Lens eval harness.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "lens",
		Short: "Domain-agnostic conversational AI evaluation harness",
		Long: `Lens runs multi-turn conversational AI evaluations against any target system
using configurable personas, scenarios, rubrics, and pluggable LLM adapters.`,
	}

	root.AddCommand(
		newRunCmd(),
		newValidateCmd(),
		newAnalyzeCmd(),
		newNewCmd(),
		newAdaptersCmd(),
		newReportCmd(),
	)

	return root
}
