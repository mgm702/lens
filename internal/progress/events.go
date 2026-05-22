// Package progress defines the event types emitted by the runner during an
// experiment run. A Renderer (or any other consumer) receives these from a
// channel and updates the UI or log output.
package progress

// EventType identifies the lifecycle stage of an eval case.
type EventType int

const (
	EventCaseStarted   EventType = iota // case has been picked up by a worker
	EventCaseCompleted                  // case finished successfully
	EventCaseFailed                     // case failed due to an infrastructure error
)

// Event is emitted by the runner as each case progresses.
// Consumers receive these from the channel passed to runner.New.
type Event struct {
	Type      EventType
	CaseLabel string
	EndReason string // populated on EventCaseCompleted
	Turns     int    // total turns in the transcript
	USD       float64
	WallMs    int64
	Err       error // populated on EventCaseFailed
}
