package results

import (
	"sort"
)

// MicroAverage returns the fraction of cases in which the outcome with the
// given ID passed, across all results that have a verdict. Returns 0 if no
// results have a verdict for that outcome.
func MicroAverage(res []CaseResult, outcomeID string) float64 {
	var pass, total int
	for _, r := range res {
		if r.Verdict == nil {
			continue
		}
		o, ok := r.Verdict.Outcomes[outcomeID]
		if !ok {
			continue
		}
		total++
		if o.Pass {
			pass++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(pass) / float64(total)
}

// CompositeByCondition returns the mean composite score grouped by a condition
// key (e.g. info_level). The key function extracts the grouping dimension from
// a CaseResult.
func CompositeByCondition(res []CaseResult, key func(CaseResult) string) map[string]float64 {
	sums := map[string]float64{}
	counts := map[string]int{}
	for _, r := range res {
		if r.Verdict == nil {
			continue
		}
		k := key(r)
		sums[k] += r.Verdict.Composite
		counts[k]++
	}
	out := make(map[string]float64, len(sums))
	for k, sum := range sums {
		out[k] = sum / float64(counts[k])
	}
	return out
}

// GroupBy partitions results by the concatenated value of one or more key
// functions. For example, GroupBy(results, byPersona, byScenario) returns a
// map keyed by "personaID|scenarioID".
func GroupBy(res []CaseResult, keys ...func(CaseResult) string) map[string][]CaseResult {
	out := map[string][]CaseResult{}
	for _, r := range res {
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k(r)
		}
		key := join(parts, "|")
		out[key] = append(out[key], r)
	}
	return out
}

// SortedKeys returns the keys of a map[string][]CaseResult in alphabetical order.
func SortedKeys(m map[string][]CaseResult) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ByPersona is a key function for GroupBy that groups by persona ID.
func ByPersona(r CaseResult) string { return r.PersonaID }

// ByScenario is a key function for GroupBy that groups by scenario ID.
func ByScenario(r CaseResult) string { return r.ScenarioID }

// ByInfoLevel is a key function for GroupBy that groups by info level.
func ByInfoLevel(r CaseResult) string { return r.InfoLevel }

func join(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
