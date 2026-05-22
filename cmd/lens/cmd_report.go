package main

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/mgm702/lens/internal/report"
	"github.com/spf13/cobra"
)

type reportFlags struct {
	output string
	open   bool
}

func newReportCmd() *cobra.Command {
	var flags reportFlags

	cmd := &cobra.Command{
		Use:   "report <results-dir>",
		Short: "Build an HTML report from eval results",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return buildReport(args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.output, "output", "./site", "output directory for HTML files")
	cmd.Flags().BoolVar(&flags.open, "open", false, "open report in browser after building")

	return cmd
}

func buildReport(resultsDir string, flags reportFlags) error {
	res, err := loadResultsDir(resultsDir)
	if err != nil {
		return err
	}

	if err := report.Build(res, flags.output); err != nil {
		return fmt.Errorf("building report: %w", err)
	}

	fmt.Printf("Report written to: %s\n", flags.output)

	if flags.open {
		openBrowser(flags.output + "/index.html")
	}

	return nil
}

func openBrowser(path string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "start"
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, path).Start()
}
