package types

// Rubric defines what the judge should evaluate. HardGates are binary
// pass/fail criteria — failing any one zeroes the composite score.
// Outcomes are the primary scored dimensions. Indicators are diagnostic
// scales (1–N) used to understand *why* a score landed where it did.
type Rubric struct {
	HardGates  []OutcomeDef   `yaml:"hard_gates"`
	Outcomes   []OutcomeDef   `yaml:"outcomes"`
	Indicators []IndicatorDef `yaml:"indicators"`
}

// OutcomeDef describes a single scoreable outcome.
// Type is either "item_level" (scored per item, then micro-averaged)
// or "conversation" (scored once for the whole conversation).
type OutcomeDef struct {
	ID          string `yaml:"id"`
	Type        string `yaml:"type"`        // "item_level" | "conversation"
	Description string `yaml:"description"`
}

// IndicatorDef describes a diagnostic rating scale.
// AnchorLow and AnchorHigh describe what score 1 and score Scale look like.
type IndicatorDef struct {
	ID          string `yaml:"id"`
	Scale       int    `yaml:"scale"`
	Description string `yaml:"description"`
	AnchorLow   string `yaml:"anchor_low"`
	AnchorHigh  string `yaml:"anchor_high"`
}
