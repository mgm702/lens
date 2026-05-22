package types

// Persona represents a simulated user with a profile, constraints, and
// knowledge gaps. The Profile and Constraints fields are intentionally
// open (map[string]any) so any domain can add fields without changing
// the core type.
type Persona struct {
	ID             string         `yaml:"id"               json:"id"`
	Name           string         `yaml:"name"             json:"name"`
	Summary        string         `yaml:"summary"          json:"summary"`
	Profile        map[string]any `yaml:"profile"          json:"profile"`
	Constraints    map[string]any `yaml:"constraints"      json:"constraints"`
	KnowledgeGaps  []string       `yaml:"knowledge_gaps"   json:"knowledge_gaps"`
	EmotionalState string         `yaml:"emotional_state"  json:"emotional_state"`
}
