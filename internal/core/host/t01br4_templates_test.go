package host

import (
	"strings"
	"testing"
)

func TestT01BR4GeneratedSkillsDescribeFullPreservationGuard(t *testing.T) {
	for name, skill := range Skills() {
		for _, term := range []string{"Remember across carrier kinds", "change create/revise/preview/apply/sync/reauthor", "extensions retain raw YAML meaning", "exact affected paths"} {
			if !strings.Contains(skill, term) {
				t.Fatalf("%s omits shared guard term %q", name, term)
			}
		}
	}
	spec := Skills()["h-spec"]
	for _, term := range []string{"Remember across carrier kinds", "selected reauthor", "Known schema fields", "extensions at any depth", "requested effect", "same-ref bindings", "original bytes", "returned read route"} {
		if !strings.Contains(spec, term) {
			t.Fatalf("h-spec omits %q", term)
		}
	}
	if strings.Contains(agents, "Spec writers refuse") || strings.Contains(spec, "Spec writing and change preview refuse") {
		t.Fatal("generated instructions retain the former spec-only preservation claim")
	}
}
