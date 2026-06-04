// Package results handles writing eval output to disk and computing aggregates.
package results

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// Store writes per-case JSON files and a flat CSV summary to a results directory.
type Store struct {
	dir string // experiment output directory
}

// NewStore creates a Store rooted at dir, creating the directory if needed.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("results: creating output dir %q: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// WriteCase writes one CaseResult as pretty-printed JSON.
// File name is <label>.json.
func (s *Store) WriteCase(r CaseResult) error {
	path := filepath.Join(s.dir, r.Label+".json")
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("results: marshalling case %q: %w", r.Label, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("results: writing case %q: %w", r.Label, err)
	}
	return nil
}

// WriteCSV writes a flat CSV of all results to results.csv in the store dir.
// If the file already exists it is overwritten.
//
// Columns: label, persona_id, scenario_id, info_level, rep, end_reason,
//
//	turns, composite, wall_ms, <hard_gate columns>, <outcome columns>
func (s *Store) WriteCSV(results []CaseResult) error {
	if len(results) == 0 {
		return nil
	}

	path := filepath.Join(s.dir, "results.csv")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("results: creating results.csv: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Collect all outcome and hard-gate keys in a stable order.
	gateKeys := collectKeys(results, func(r CaseResult) []string {
		if r.Verdict == nil {
			return nil
		}
		keys := make([]string, 0, len(r.Verdict.HardGates))
		for k := range r.Verdict.HardGates {
			keys = append(keys, k)
		}
		return keys
	})
	outcomeKeys := collectKeys(results, func(r CaseResult) []string {
		if r.Verdict == nil {
			return nil
		}
		keys := make([]string, 0, len(r.Verdict.Outcomes))
		for k := range r.Verdict.Outcomes {
			keys = append(keys, k)
		}
		return keys
	})

	// Header row.
	header := []string{
		"label", "persona_id", "scenario_id", "info_level", "rep",
		"end_reason", "turns", "composite", "wall_ms",
	}
	for _, k := range gateKeys {
		header = append(header, "gate_"+k)
	}
	for _, k := range outcomeKeys {
		header = append(header, "outcome_"+k)
	}
	if err := w.Write(header); err != nil {
		return fmt.Errorf("results: writing CSV header: %w", err)
	}

	// Data rows.
	for _, r := range results {
		turns := len(r.Transcript.Turns)
		composite := ""
		if r.Verdict != nil {
			composite = strconv.FormatFloat(r.Verdict.Composite, 'f', 4, 64)
		}

		row := []string{
			r.Label,
			r.PersonaID,
			r.ScenarioID,
			r.InfoLevel,
			strconv.Itoa(r.Rep),
			r.EndReason,
			strconv.Itoa(turns),
			composite,
			strconv.FormatInt(r.Perf.WallMs, 10),
		}
		for _, k := range gateKeys {
			v := ""
			if r.Verdict != nil {
				v = strconv.FormatBool(r.Verdict.HardGates[k])
			}
			row = append(row, v)
		}
		for _, k := range outcomeKeys {
			v := ""
			if r.Verdict != nil {
				if o, ok := r.Verdict.Outcomes[k]; ok {
					v = strconv.FormatBool(o.Pass)
				}
			}
			row = append(row, v)
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("results: writing CSV row for %q: %w", r.Label, err)
		}
	}

	return w.Error()
}

// collectKeys gathers all unique keys returned by keyFn across all results,
// sorted alphabetically.
func collectKeys(results []CaseResult, keyFn func(CaseResult) []string) []string {
	seen := map[string]bool{}
	for _, r := range results {
		for _, k := range keyFn(r) {
			seen[k] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
