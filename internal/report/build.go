// Package report builds static HTML reports from eval results.
package report

import (
	_ "embed"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgm702/lens/internal/results"
	"github.com/mgm702/lens/pkg/types"
)

//go:embed templates/index.html.tmpl
var indexTmpl string

//go:embed templates/transcript.html.tmpl
var transcriptTmpl string

// indexData is the template data for index.html.
type indexData struct {
	ExperimentName string
	GeneratedAt    string
	TotalCases     int
	Workers        int
	MeanComposite  float64
	DonePct        float64
	Errors         int
	Cases          []caseRow
}

type caseRow struct {
	Label      string
	PersonaID  string
	ScenarioID string
	InfoLevel  string
	Rep        int
	EndReason  string
	Turns      int
	Verdict    *judgeVerdict
}

// judgeVerdict is a minimal copy of judge.Verdict fields used in templates.
type judgeVerdict struct {
	Composite float64
}

// Build generates an HTML report from the given results into outputDir.
func Build(res []results.CaseResult, outputDir string) error {
	if err := os.MkdirAll(filepath.Join(outputDir, "cases"), 0o755); err != nil {
		return fmt.Errorf("report: creating output dirs: %w", err)
	}

	funcs := template.FuncMap{
		"endReasonClass": endReasonClass,
		"compositeWidth": func(v float64) int {
			return int(math.Round(v * 100))
		},
	}

	indexT, err := template.New("index").Funcs(funcs).Parse(indexTmpl)
	if err != nil {
		return fmt.Errorf("report: parsing index template: %w", err)
	}
	txT, err := template.New("transcript").Funcs(funcs).Parse(transcriptTmpl)
	if err != nil {
		return fmt.Errorf("report: parsing transcript template: %w", err)
	}

	// Build index page data.
	var totalComposite float64
	var verdictCount, doneCount, errCount int
	rows := make([]caseRow, 0, len(res))

	for _, r := range res {
		row := caseRow{
			Label:      r.Label,
			PersonaID:  r.PersonaID,
			ScenarioID: r.ScenarioID,
			InfoLevel:  r.InfoLevel,
			Rep:        r.Rep,
			EndReason:  r.EndReason,
			Turns:      len(r.Transcript.Turns),
		}
		if r.Verdict != nil {
			row.Verdict = &judgeVerdict{Composite: r.Verdict.Composite}
			totalComposite += r.Verdict.Composite
			verdictCount++
		}
		if r.EndReason == types.EndReasonDone {
			doneCount++
		}
		if r.Err != nil {
			errCount++
		}
		rows = append(rows, row)
	}

	meanComposite := 0.0
	if verdictCount > 0 {
		meanComposite = totalComposite / float64(verdictCount)
	}
	donePct := 0.0
	if len(res) > 0 {
		donePct = float64(doneCount) / float64(len(res)) * 100
	}

	iData := indexData{
		ExperimentName: "Eval Results",
		GeneratedAt:    time.Now().Format("2006-01-02 15:04:05"),
		TotalCases:     len(res),
		MeanComposite:  meanComposite,
		DonePct:        donePct,
		Errors:         errCount,
		Cases:          rows,
	}

	// Write index.html.
	indexFile, err := os.Create(filepath.Join(outputDir, "index.html"))
	if err != nil {
		return fmt.Errorf("report: creating index.html: %w", err)
	}
	defer indexFile.Close()
	if err := indexT.Execute(indexFile, iData); err != nil {
		return fmt.Errorf("report: rendering index.html: %w", err)
	}

	// Write per-case transcript pages.
	for _, r := range res {
		casePath := filepath.Join(outputDir, "cases", r.Label+".html")
		f, err := os.Create(casePath)
		if err != nil {
			return fmt.Errorf("report: creating case %s: %w", r.Label, err)
		}
		if err := txT.Execute(f, r); err != nil {
			f.Close()
			return fmt.Errorf("report: rendering case %s: %w", r.Label, err)
		}
		f.Close()
	}

	return nil
}

func endReasonClass(reason string) string {
	switch strings.ToUpper(reason) {
	case "DONE":
		return "done"
	case "DROPOUT":
		return "dropout"
	case "MAX_TURNS":
		return "maxturns"
	default:
		return "error"
	}
}
