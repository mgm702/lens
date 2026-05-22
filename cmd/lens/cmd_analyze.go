package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/mgm702/lens/internal/results"
	"github.com/spf13/cobra"
)

type analyzeFlags struct {
	format  string
	groupBy []string
}

func newAnalyzeCmd() *cobra.Command {
	var flags analyzeFlags

	cmd := &cobra.Command{
		Use:   "analyze <results-dir>",
		Short: "Aggregate and display eval results",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return analyzeResults(args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.format, "format", "table", "output format: table|json|csv")
	cmd.Flags().StringSliceVar(&flags.groupBy, "group-by", []string{"info_level"}, "grouping dimensions (persona, scenario, info_level)")

	return cmd
}

func analyzeResults(dir string, flags analyzeFlags) error {
	res, err := loadResultsDir(dir)
	if err != nil {
		return err
	}
	if len(res) == 0 {
		fmt.Println("No results found in", dir)
		return nil
	}

	// Build key functions from --group-by flags.
	keyFns := make([]func(results.CaseResult) string, 0, len(flags.groupBy))
	for _, g := range flags.groupBy {
		switch g {
		case "persona":
			keyFns = append(keyFns, results.ByPersona)
		case "scenario":
			keyFns = append(keyFns, results.ByScenario)
		case "info_level":
			keyFns = append(keyFns, results.ByInfoLevel)
		default:
			return fmt.Errorf("unknown group-by dimension %q (valid: persona, scenario, info_level)", g)
		}
	}

	groups := results.GroupBy(res, keyFns...)

	switch flags.format {
	case "json":
		return printAnalysisJSON(groups)
	case "csv":
		return printAnalysisCSV(groups, flags.groupBy)
	default:
		return printAnalysisTable(groups, flags.groupBy, len(res))
	}
}

func printAnalysisTable(groups map[string][]results.CaseResult, dims []string, total int) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	defer w.Flush()

	header := strings.Join(dims, " / ") + "\t  cases\t  composite\t  done%\t  errors"
	fmt.Fprintln(w, header)
	fmt.Fprintln(w, strings.Repeat("-", 70))

	for _, key := range results.SortedKeys(groups) {
		group := groups[key]
		composite := meanComposite(group)
		doneCount := countEndReason(group, "DONE")
		errCount := countErrors(group)
		n := len(group)

		fmt.Fprintf(w, "%s\t  %d\t  %.3f\t  %.0f%%\t  %d\n",
			key, n, composite,
			float64(doneCount)/float64(n)*100,
			errCount,
		)
	}

	fmt.Fprintf(w, "\nTotal: %d cases\n", total)
	return nil
}

func printAnalysisJSON(groups map[string][]results.CaseResult) error {
	type row struct {
		Group     string  `json:"group"`
		Cases     int     `json:"cases"`
		Composite float64 `json:"composite"`
		DonePct   float64 `json:"done_pct"`
		Errors    int     `json:"errors"`
	}

	var rows []row
	for _, key := range results.SortedKeys(groups) {
		group := groups[key]
		n := len(group)
		rows = append(rows, row{
			Group:     key,
			Cases:     n,
			Composite: meanComposite(group),
			DonePct:   float64(countEndReason(group, "DONE")) / float64(n) * 100,
			Errors:    countErrors(group),
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func printAnalysisCSV(groups map[string][]results.CaseResult, dims []string) error {
	header := strings.Join(dims, ",") + ",cases,composite,done_pct,errors"
	fmt.Println(header)
	for _, key := range results.SortedKeys(groups) {
		group := groups[key]
		n := len(group)
		fmt.Printf("%s,%d,%.4f,%.2f,%d\n",
			key, n, meanComposite(group),
			float64(countEndReason(group, "DONE"))/float64(n)*100,
			countErrors(group),
		)
	}
	return nil
}

// loadResultsDir reads all .json CaseResult files from dir.
func loadResultsDir(dir string) ([]results.CaseResult, error) {
	pattern := filepath.Join(dir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("reading results dir: %w", err)
	}

	// Exclude results.csv-adjacent metadata files.
	var res []results.CaseResult
	for _, f := range files {
		if filepath.Base(f) == "results.json" {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f, err)
		}
		var r results.CaseResult
		if err := json.Unmarshal(data, &r); err != nil {
			// Skip files that don't parse as CaseResult.
			continue
		}
		res = append(res, r)
	}

	// Sort for deterministic output.
	sort.Slice(res, func(i, j int) bool {
		return res[i].Label < res[j].Label
	})

	return res, nil
}

func meanComposite(res []results.CaseResult) float64 {
	var sum float64
	var n int
	for _, r := range res {
		if r.Verdict != nil {
			sum += r.Verdict.Composite
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func countEndReason(res []results.CaseResult, reason string) int {
	var n int
	for _, r := range res {
		if r.EndReason == reason {
			n++
		}
	}
	return n
}

func countErrors(res []results.CaseResult) int {
	var n int
	for _, r := range res {
		if r.Err != nil {
			n++
		}
	}
	return n
}
