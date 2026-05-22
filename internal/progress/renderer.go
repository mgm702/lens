package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/sys/unix"
)

// Renderer drives a live progress bar and per-case log during a lens run.
// It consumes events from the channel provided at construction time and updates
// the display until the channel is closed.
//
// When the output writer is not a TTY (or when noProgress is true) the progress
// bar is suppressed and only line-by-line log output is written.
type Renderer struct {
	events     <-chan Event
	total      int
	out        io.Writer
	noProgress bool
}

// New creates a Renderer.
//
//   - total is the number of cases that will be run (used to size the bar).
//   - events is the channel the runner emits to; the renderer consumes it.
//   - out is the writer for log lines (usually os.Stdout).
//   - noProgress suppresses the bar regardless of TTY state.
func New(total int, events <-chan Event, out io.Writer, noProgress bool) *Renderer {
	return &Renderer{
		events:     events,
		total:      total,
		out:        out,
		noProgress: noProgress,
	}
}

// Run blocks until the events channel is closed or an error occurs.
// Call it in a goroutine alongside the runner worker pool.
func (r *Renderer) Run() {
	showBar := !r.noProgress && isTTY(r.out)

	var (
		p   *mpb.Progress
		bar *mpb.Bar
	)

	if showBar {
		p = mpb.New(mpb.WithOutput(r.out))
		bar = p.AddBar(int64(r.total),
			mpb.PrependDecorators(
				decor.Name("Running "),
				decor.CountersNoUnit("%d/%d", decor.WCSyncWidth),
			),
			mpb.AppendDecorators(
				decor.Percentage(decor.WC{W: 5}),
				decor.Name("  ETA "),
				decor.EwmaETA(decor.ET_STYLE_HHMMSS, 0),
			),
		)
	}

	for e := range r.events {
		switch e.Type {
		case EventCaseStarted:
			if !showBar {
				fmt.Fprintf(r.out, "  · %s  running…\n", e.CaseLabel)
			}

		case EventCaseCompleted:
			if bar != nil {
				bar.EwmaIncrement(milliDuration(e.WallMs))
			}
			marker := "✓"
			fmt.Fprintf(r.out, "  %s  %-32s  %-8s  %d turns  %.3fs\n",
				marker, e.CaseLabel, e.EndReason, e.Turns,
				float64(e.WallMs)/1000,
			)

		case EventCaseFailed:
			if bar != nil {
				bar.EwmaIncrement(milliDuration(e.WallMs))
			}
			errStr := ""
			if e.Err != nil {
				errStr = truncate(e.Err.Error(), 60)
			}
			fmt.Fprintf(r.out, "  ✗  %-32s  ERROR    %s\n", e.CaseLabel, errStr)
		}
	}

	if p != nil {
		p.Wait()
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// isTTY reports whether w is a terminal.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	return err == nil
}

// milliDuration converts milliseconds to time.Duration for mpb's EWMA ETA estimator.
func milliDuration(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
