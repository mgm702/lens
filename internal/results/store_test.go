package results_test

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgm702/lens/internal/judge"
	"github.com/mgm702/lens/internal/results"
	"github.com/mgm702/lens/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func mkResult(label, personaID, scenarioID, infoLevel string, rep int, composite float64) results.CaseResult {
	return results.CaseResult{
		Label:      label,
		PersonaID:  personaID,
		ScenarioID: scenarioID,
		InfoLevel:  infoLevel,
		Rep:        rep,
		EndReason:  types.EndReasonDone,
		Transcript: types.Transcript{
			Turns:     []types.Turn{{Role: "user", Text: "hi"}, {Role: "assistant", Text: "hello"}},
			EndReason: types.EndReasonDone,
		},
		Verdict: &judge.Verdict{
			HardGates:  map[string]bool{"no_harm": true},
			Outcomes:   map[string]judge.OutcomeResult{"empathy": {Pass: true}},
			Indicators: map[string]int{"clarity": 4},
			Composite:  composite,
		},
		Perf: results.CasePerf{WallMs: 1500},
	}
}

// ── Store tests ───────────────────────────────────────────────────────────────

func TestStore_WriteCase_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	store, err := results.NewStore(dir)
	require.NoError(t, err)

	r := mkResult("P1_T1_full_r0", "P1", "T1", "full", 0, 0.9)
	require.NoError(t, store.WriteCase(r))

	path := filepath.Join(dir, "P1_T1_full_r0.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var got results.CaseResult
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "P1_T1_full_r0", got.Label)
	assert.Equal(t, "P1", got.PersonaID)
	assert.InDelta(t, 0.9, got.Verdict.Composite, 0.001)
}

func TestStore_WriteCSV_Header(t *testing.T) {
	dir := t.TempDir()
	store, err := results.NewStore(dir)
	require.NoError(t, err)

	res := []results.CaseResult{mkResult("lbl", "P1", "T1", "full", 0, 1.0)}
	require.NoError(t, store.WriteCSV(res))

	f, err := os.Open(filepath.Join(dir, "results.csv"))
	require.NoError(t, err)
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	header := rows[0]
	assert.Contains(t, header, "label")
	assert.Contains(t, header, "composite")
	assert.Contains(t, header, "end_reason")
	assert.Contains(t, header, "gate_no_harm")
	assert.Contains(t, header, "outcome_empathy")
}

func TestStore_WriteCSV_DataRow(t *testing.T) {
	dir := t.TempDir()
	store, err := results.NewStore(dir)
	require.NoError(t, err)

	res := []results.CaseResult{mkResult("P1_T1_full_r0", "P1", "T1", "full", 0, 0.75)}
	require.NoError(t, store.WriteCSV(res))

	f, err := os.Open(filepath.Join(dir, "results.csv"))
	require.NoError(t, err)
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2) // header + 1 data row

	data := rows[1]
	assert.Equal(t, "P1_T1_full_r0", data[0])
	assert.Equal(t, "P1", data[1])
	assert.Equal(t, "DONE", data[5])
	assert.Equal(t, "2", data[6]) // 2 turns
}

func TestStore_WriteCSV_Empty(t *testing.T) {
	dir := t.TempDir()
	store, err := results.NewStore(dir)
	require.NoError(t, err)
	require.NoError(t, store.WriteCSV(nil))
	// No file should be created when there's nothing to write.
	_, err = os.Stat(filepath.Join(dir, "results.csv"))
	assert.True(t, os.IsNotExist(err))
}

func TestStore_WriteCSV_MultipleResults_StableColumns(t *testing.T) {
	dir := t.TempDir()
	store, err := results.NewStore(dir)
	require.NoError(t, err)

	// Two results with different but overlapping outcome keys.
	r1 := mkResult("r1", "P1", "T1", "full", 0, 1.0)
	r2 := mkResult("r2", "P2", "T1", "full", 0, 0.0)
	r2.Verdict.Outcomes["concrete_step"] = judge.OutcomeResult{Pass: false}

	require.NoError(t, store.WriteCSV([]results.CaseResult{r1, r2}))

	f, err := os.Open(filepath.Join(dir, "results.csv"))
	require.NoError(t, err)
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	require.NoError(t, err)

	// All rows must have the same number of columns.
	require.Greater(t, len(rows), 1)
	for i, row := range rows[1:] {
		assert.Len(t, row, len(rows[0]), "row %d column count mismatch", i)
	}
}

// ── Aggregate tests ───────────────────────────────────────────────────────────

func TestMicroAverage_AllPass(t *testing.T) {
	res := []results.CaseResult{
		mkResult("a", "P1", "T1", "full", 0, 1.0),
		mkResult("b", "P2", "T1", "full", 0, 1.0),
	}
	assert.InDelta(t, 1.0, results.MicroAverage(res, "empathy"), 0.001)
}

func TestMicroAverage_HalfPass(t *testing.T) {
	r1 := mkResult("a", "P1", "T1", "full", 0, 1.0)
	r2 := mkResult("b", "P2", "T1", "full", 0, 0.0)
	r2.Verdict.Outcomes["empathy"] = judge.OutcomeResult{Pass: false}
	assert.InDelta(t, 0.5, results.MicroAverage([]results.CaseResult{r1, r2}, "empathy"), 0.001)
}

func TestMicroAverage_UnknownOutcome(t *testing.T) {
	res := []results.CaseResult{mkResult("a", "P1", "T1", "full", 0, 1.0)}
	assert.Equal(t, 0.0, results.MicroAverage(res, "nonexistent"))
}

func TestCompositeByCondition(t *testing.T) {
	res := []results.CaseResult{
		mkResult("a", "P1", "T1", "full", 0, 1.0),
		mkResult("b", "P1", "T1", "full", 1, 0.5),
		mkResult("c", "P1", "T1", "partial", 0, 0.0),
	}
	byLevel := results.CompositeByCondition(res, results.ByInfoLevel)
	assert.InDelta(t, 0.75, byLevel["full"], 0.001)
	assert.InDelta(t, 0.0, byLevel["partial"], 0.001)
}

func TestGroupBy(t *testing.T) {
	res := []results.CaseResult{
		mkResult("a", "P1", "T1", "full", 0, 1.0),
		mkResult("b", "P1", "T2", "full", 0, 0.5),
		mkResult("c", "P2", "T1", "full", 0, 0.0),
	}
	groups := results.GroupBy(res, results.ByPersona)
	assert.Len(t, groups["P1"], 2)
	assert.Len(t, groups["P2"], 1)
}
