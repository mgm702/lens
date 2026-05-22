package types

// Scenario represents a single advising task or test case goal. RequiresMetadata
// lists keys that must be present in Metadata before the case can run (e.g.
// "opportunity_id" for job-evaluation tasks that need a real record ID).
type Scenario struct {
	ID               string         `yaml:"id"                json:"id"`
	Name             string         `yaml:"name"              json:"name"`
	Phase            string         `yaml:"phase"             json:"phase"`
	Description      string         `yaml:"description"       json:"description"`
	OpeningMessage   string         `yaml:"opening_message"   json:"opening_message"`
	SuccessCriteria  []string       `yaml:"success_criteria"  json:"success_criteria"`
	RequiresMetadata []string       `yaml:"requires_metadata" json:"requires_metadata"`
	Metadata         map[string]any `yaml:"metadata"          json:"metadata"`
}
