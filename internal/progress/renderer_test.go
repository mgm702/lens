package progress_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/mgm702/lens/internal/progress"
	"github.com/stretchr/testify/assert"
)

// sendEvents feeds events into the channel and then closes it.
func sendEvents(events chan<- progress.Event, evs ...progress.Event) {
	for _, e := range evs {
		events <- e
	}
	close(events)
}

func TestRenderer_CompletedCaseLogged(t *testing.T) {
	var buf bytes.Buffer
	events := make(chan progress.Event, 10)
	r := progress.New(1, events, &buf, true) // noProgress=true → no bar

	sendEvents(events,
		progress.Event{Type: progress.EventCaseStarted, CaseLabel: "P1_T1_full_r0"},
		progress.Event{Type: progress.EventCaseCompleted, CaseLabel: "P1_T1_full_r0", EndReason: "DONE", Turns: 4, WallMs: 2100},
	)

	r.Run()

	out := buf.String()
	assert.Contains(t, out, "P1_T1_full_r0")
	assert.Contains(t, out, "DONE")
	assert.Contains(t, out, "4 turns")
}

func TestRenderer_FailedCaseLogged(t *testing.T) {
	var buf bytes.Buffer
	events := make(chan progress.Event, 10)
	r := progress.New(1, events, &buf, true)

	sendEvents(events,
		progress.Event{Type: progress.EventCaseFailed, CaseLabel: "P1_T1_full_r0", Err: fmt.Errorf("network timeout"), WallMs: 500},
	)

	r.Run()

	out := buf.String()
	assert.Contains(t, out, "✗")
	assert.Contains(t, out, "ERROR")
	assert.Contains(t, out, "network timeout")
}

func TestRenderer_StartedEventLogged_NoProgress(t *testing.T) {
	var buf bytes.Buffer
	events := make(chan progress.Event, 10)
	r := progress.New(2, events, &buf, true)

	sendEvents(events,
		progress.Event{Type: progress.EventCaseStarted, CaseLabel: "P1_T1_full_r0"},
		progress.Event{Type: progress.EventCaseCompleted, CaseLabel: "P1_T1_full_r0", EndReason: "DONE", Turns: 2},
	)

	r.Run()
	assert.Contains(t, buf.String(), "running")
}

func TestRenderer_MultipleCases(t *testing.T) {
	var buf bytes.Buffer
	events := make(chan progress.Event, 20)
	r := progress.New(3, events, &buf, true)

	sendEvents(events,
		progress.Event{Type: progress.EventCaseCompleted, CaseLabel: "A", EndReason: "DONE", Turns: 2},
		progress.Event{Type: progress.EventCaseCompleted, CaseLabel: "B", EndReason: "MAX_TURNS", Turns: 10},
		progress.Event{Type: progress.EventCaseFailed, CaseLabel: "C", Err: fmt.Errorf("boom")},
	)

	r.Run()

	out := buf.String()
	assert.Contains(t, out, "A")
	assert.Contains(t, out, "B")
	assert.Contains(t, out, "C")
}

func TestRenderer_EmptyEventStream(t *testing.T) {
	var buf bytes.Buffer
	events := make(chan progress.Event)
	r := progress.New(0, events, &buf, true)

	close(events)
	r.Run() // should return immediately
	// No panic, no deadlock.
}
