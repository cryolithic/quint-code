package app

import (
	"sort"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

// A candidate is selected by an authored relation only. The relation does not
// establish that the decision governs this spec or resolve a contested choice.
type specDecisionCandidate struct {
	Entry      carrier.Entry
	MatchKinds []string
}

func specDecisionCandidates(spec carrier.Record, projection carrier.Projection) []specDecisionCandidate {
	selectors := map[string]bool{}
	for _, selector := range spec.Constrains {
		selectors[selector] = true
	}
	candidates := []specDecisionCandidate{}
	for _, entry := range projection.Entries {
		decision := entry.Document.Record
		if decision.Kind != "decision" || decision.Disposition != "choose_now" || entry.State != carrier.Active && entry.State != carrier.Contested {
			continue
		}
		matches := []string{}
		if spec.About != "" && decision.About == spec.About {
			matches = append(matches, "same_about")
		}
		for _, selector := range decision.Constrains {
			if selectors[selector] {
				matches = append(matches, "shared_declared_selector")
				break
			}
		}
		if len(matches) == 0 {
			continue
		}
		sort.Strings(matches)
		candidates = append(candidates, specDecisionCandidate{Entry: entry, MatchKinds: matches})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Entry.Ref < candidates[j].Entry.Ref })
	return candidates
}

func decisionCandidateAssessment(coverage string, count int, selection string) map[string]any {
	return map[string]any{
		"assessment":         "not_assessed",
		"candidate_coverage": coverage,
		"candidate_count":    count,
		"selection_rule":     selection,
		"meaning":            "Matches are advisory review candidates; semantic applicability and authority are not assessed. An empty candidate band does not establish absence of a governing decision.",
	}
}
