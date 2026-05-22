package main

import (
	"fmt"
	"os"

	"github.com/mgm702/lens/internal/config"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <experiment.yaml>",
		Short: "Validate an experiment config and its referenced files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateConfig(args[0])
		},
	}
}

func validateConfig(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return err
	}

	errs := config.Validate(cfg)
	if len(errs) == 0 {
		fmt.Println("✓ Config is valid")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Config validation failed (%d error(s)):\n", len(errs))
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "  • %s\n", e)
	}
	return fmt.Errorf("invalid configuration")
}
