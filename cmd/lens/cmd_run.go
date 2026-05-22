package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mgm702/lens/internal/adapters"
	"github.com/mgm702/lens/internal/config"
	"github.com/mgm702/lens/internal/judge"
	"github.com/mgm702/lens/internal/progress"
	"github.com/mgm702/lens/internal/results"
	"github.com/mgm702/lens/internal/runner"
	"github.com/mgm702/lens/pkg/types"
	"github.com/spf13/cobra"
)

type runFlags struct {
	personas   []string
	scenarios  []string
	infoLevels []string
	reps       int
	workers    int
	outputDir  string
	dryRun     bool
	noProgress bool
}

func newRunCmd() *cobra.Command {
	var flags runFlags

	cmd := &cobra.Command{
		Use:   "run <experiment.yaml>",
		Short: "Run an evaluation experiment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExperiment(args[0], flags)
		},
	}

	cmd.Flags().StringSliceVar(&flags.personas, "personas", nil, "override personas list (comma-separated IDs)")
	cmd.Flags().StringSliceVar(&flags.scenarios, "scenarios", nil, "override scenarios list (comma-separated IDs)")
	cmd.Flags().StringSliceVar(&flags.infoLevels, "info-levels", nil, "override info levels (e.g. full,partial)")
	cmd.Flags().IntVar(&flags.reps, "reps", 0, "override rep count (0 = use config)")
	cmd.Flags().IntVar(&flags.workers, "workers", 0, "override worker count (0 = use config)")
	cmd.Flags().StringVar(&flags.outputDir, "output-dir", "", "override output directory")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "expand grid and print cases, no LLM calls")
	cmd.Flags().BoolVar(&flags.noProgress, "no-progress", false, "disable progress bar")

	return cmd
}

func runExperiment(experimentFile string, flags runFlags) error {
	// ── 1. Load and validate config ───────────────────────────────────────────
	cfg, err := config.Load(experimentFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	applyRunFlags(cfg, flags)

	if errs := config.Validate(cfg); len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "Config validation failed:")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  • %s\n", e)
		}
		return fmt.Errorf("invalid configuration")
	}

	// ── 2. Load experiment data ───────────────────────────────────────────────
	personas, err := config.LoadPersonas(cfg.PersonasFile)
	if err != nil {
		return fmt.Errorf("loading personas: %w", err)
	}
	scenarios, err := config.LoadScenarios(cfg.ScenariosFile)
	if err != nil {
		return fmt.Errorf("loading scenarios: %w", err)
	}
	rubric, err := config.LoadRubric(cfg.Judge.RubricFile)
	if err != nil {
		return fmt.Errorf("loading rubric: %w", err)
	}

	// Filter personas/scenarios by ID if flags were provided.
	if len(flags.personas) > 0 {
		idSet := makeIDSet(flags.personas)
		var filtered []types.Persona
		for _, p := range personas {
			if idSet[p.ID] {
				filtered = append(filtered, p)
			}
		}
		personas = filtered
	}
	if len(flags.scenarios) > 0 {
		idSet := makeIDSet(flags.scenarios)
		var filtered []types.Scenario
		for _, s := range scenarios {
			if idSet[s.ID] {
				filtered = append(filtered, s)
			}
		}
		scenarios = filtered
	}

	// ── 3. Expand grid ────────────────────────────────────────────────────────
	cases := runner.ExpandGrid(cfg, personas, scenarios)

	if flags.dryRun {
		fmt.Printf("Dry run: %d cases\n\n", len(cases))
		for _, c := range cases {
			fmt.Printf("  %s\n", c.Label)
		}
		return nil
	}

	// ── 4. Create adapter and judge ───────────────────────────────────────────
	adapter, err := adapters.New(cfg.Target)
	if err != nil {
		return fmt.Errorf("creating adapter: %w", err)
	}

	j, err := judge.New(cfg.Judge, rubric)
	if err != nil {
		return fmt.Errorf("creating judge: %w", err)
	}

	// ── 5. Set up output store ────────────────────────────────────────────────
	outDir := cfg.OutputDir
	if flags.outputDir != "" {
		outDir = flags.outputDir
	}
	if outDir == "" {
		outDir = filepath.Join("results", cfg.Name+"-"+time.Now().Format("20060102-150405"))
	}
	store, err := results.NewStore(outDir)
	if err != nil {
		return fmt.Errorf("creating results store: %w", err)
	}

	// ── 6. Set up progress renderer ───────────────────────────────────────────
	eventCh := make(chan progress.Event, len(cases)*3)
	renderer := progress.New(len(cases), eventCh, os.Stdout, flags.noProgress)

	// ── 7. Run with graceful shutdown ─────────────────────────────────────────
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("Lens  ▸  %s  ▸  %d cases  ▸  %d workers\n\n",
		cfg.Name, len(cases), cfg.Workers)

	r := runner.New(cfg, adapter, j, eventCh)

	rendererDone := make(chan struct{})
	go func() {
		renderer.Run()
		close(rendererDone)
	}()

	allResults, err := r.Run(ctx, cases)
	close(eventCh)
	<-rendererDone

	if err != nil {
		return fmt.Errorf("running experiment: %w", err)
	}

	// ── 8. Write results ──────────────────────────────────────────────────────
	var writeErrs []error
	for _, res := range allResults {
		if wErr := store.WriteCase(res); wErr != nil {
			writeErrs = append(writeErrs, wErr)
		}
	}
	if err := store.WriteCSV(allResults); err != nil {
		writeErrs = append(writeErrs, err)
	}

	// Count outcomes.
	var passed, failed, errored int
	for _, res := range allResults {
		switch {
		case res.Err != nil:
			errored++
		case res.Verdict != nil && res.Verdict.Composite >= 0.5:
			passed++
		default:
			failed++
		}
	}

	fmt.Printf("\nResults written to: %s\n", outDir)
	fmt.Printf("  Passed: %d  Failed: %d  Errors: %d\n", passed, failed, errored)

	if len(writeErrs) > 0 {
		for _, e := range writeErrs {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", e)
		}
	}

	return nil
}

// applyRunFlags overrides experiment config fields with non-zero flag values.
func applyRunFlags(cfg *config.ExperimentConfig, flags runFlags) {
	if len(flags.infoLevels) > 0 {
		cfg.InfoLevels = flags.infoLevels
	}
	if flags.reps > 0 {
		cfg.Reps = flags.reps
	}
	if flags.workers > 0 {
		cfg.Workers = flags.workers
	}
}

// makeIDSet converts a slice of IDs to a fast-lookup map.
func makeIDSet(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
