package transport

import (
	"context"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/store"
)

func TestT01BR4RememberSchemaAndExtensionValuesAcrossMCPProfiles(t *testing.T) {
	evidence := "---\nformat: haft/1\nid: ev-20260923-00000001\nkind: evidence\ntitle: Cancellation property result\nstatus: active\norigin: agent_proposal\nabout: domain:Billing.OrderCancellation\ncreated_at: \"2026-09-23T10:00:00Z\"\nclaim: The total remained unchanged in the exercised cases\nobserved_at: 2026-09-23T09:59:00Z\nmethod: Property test\nsource: reports/cancel-property.txt\nbasis:\n  kind: code\n  ref: build:cancel-fixture-001\n  conditions: Go property fixture, seed 42, new and paid orders\n---\nController observation.\n"
	note := func(field string) string {
		return "---\nkind: note\ntitle: Controller scalar\nabout: domain:Controller.Meaning\n" + field + "\n---\nRationale.\n"
	}
	cases := []struct {
		name     string
		carrier  string
		wantKind string
		lossPath string
		lossCode string
	}{
		{"known-observed-timestamp", evidence, "written", "", ""},
		{"known-updated-timestamp", note("updated_at: 2026-09-23T09:59:00Z"), "written", "", ""},
		{"known-reopen-date", note("reopen_when: 2026-09-23"), "written", "", ""},
		{"known-numeric-title", "---\nkind: note\ntitle: 12345\nabout: domain:Controller.Meaning\n---\nRationale.\n", "written", "", ""},
		{"optional-empty-supersedes", note("supersedes: []"), "written", "", ""},
		{"optional-null-supersedes", note("supersedes: null"), "written", "", ""},
		{"optional-false-confirmed", note("operator_confirmed: false"), "written", "", ""},
		{"optional-empty-reopen", note("reopen_when: \"\""), "written", "", ""},
		{"extension-space-timestamp", note("x-value: 2026-09-25 10:00:00"), "written", "", ""},
		{"extension-lowercase-t", note("x-value: 2026-09-25t10:00:00Z"), "written", "", ""},
		{"extension-short-date", note("x-value: 2026-9-5 10:00:00"), "written", "", ""},
		{"extension-positive-infinity", note("x-value: +.inf"), "written", "", ""},
		{"extension-custom-tag", note("x-value: !vendor abc"), "invalid", "x-value", "unsupported_yaml_tag_conversion"},
		{"extension-large-number", note("x-value: 123456789012345678901234"), "invalid", "x-value", "unsupported_yaml_value_conversion"},
		{"extension-integral-float", note("x-value: 1.0"), "invalid", "x-value", "unsupported_yaml_tag_conversion"},
	}
	for _, profile := range []struct {
		name string
		tool string
	}{
		{ProfileDefault, "haft_write"},
		{ProfileLegacy, "haft"},
	} {
		t.Run(profile.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					root := t.TempDir()
					client := t01aStartClient(t, app.Service{Root: root}, profile.name)
					client.list(t)
					before, err := (store.Store{Root: root}).Read(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					result := client.mustCall(t, profile.tool, app.Request{
						Format: delivery.Format, Operation: "remember", RequestID: "t01br4-" + tc.name, Carrier: tc.carrier,
					})
					if result.Kind != tc.wantKind || result.IsError != (tc.wantKind == "invalid") || delivery.Size(result) > delivery.Budget {
						t.Fatalf("unexpected or unbounded MCP result: %+v", result)
					}
					after, err := (store.Store{Root: root}).Read(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					if tc.wantKind == "written" {
						if len(after.Documents) != 1 || !after.Documents[0].Valid() || after.Generation == before.Generation {
							t.Fatalf("valid schema or equivalent extension input was not published: %+v", after)
						}
						return
					}
					if after.Generation != before.Generation || len(after.Documents) != len(before.Documents) {
						t.Fatalf("semantic refusal changed durable state: before=%+v after=%+v", before, after)
					}
					for _, diagnostic := range result.Diagnostics {
						if diagnostic.Path == tc.lossPath && diagnostic.Code == tc.lossCode {
							return
						}
					}
					t.Fatalf("semantic refusal omitted exact path/code %s/%s: %+v", tc.lossPath, tc.lossCode, result.Diagnostics)
				})
			}
		})
	}
}
