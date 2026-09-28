package carrier

import (
	"strings"
	"testing"
)

func yamlDoc(front string) []byte {
	return []byte("---\n" + front + "---\nBody.\n")
}

func TestSemanticYAMLLossesDistinguishesValueTypesFromLexicalChanges(t *testing.T) {
	cases := []struct {
		name       string
		source     string
		normalized string
		path       string
		tag        string
	}{
		{"root_custom", "x-vendor: !vendor abc\n", "x-vendor: abc\n", "x-vendor", "!vendor"},
		{"nested_custom", "x-vendor:\n  meaning: !currency 12\n", "x-vendor:\n  meaning: \"12\"\n", "x-vendor.meaning", "!currency"},
		{"claim_binary", "claims:\n  - id: rule\n    x-proof: !!binary aGVsbG8=\n", "claims:\n  - id: rule\n    x-proof: hello\n", "claims[0].x-proof", "!!binary"},
		{"container_custom", "x-vendor: !vendor [one, two]\n", "x-vendor: [one, two]\n", "x-vendor", "!vendor"},
		{"numeric_lexical", "# source comment\nx-vendor: 0x1F\n", "x-vendor: 31\n", "", ""},
		{"explicit_preserved_str", "x-vendor: !!str 12\n", "x-vendor: \"12\"\n", "", ""},
		{"ordinary_container", "x-vendor: {owner: Billing, values: [1, 2]}\n", "x-vendor:\n  owner: Billing\n  values:\n    - 1\n    - 2\n", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte("---\n" + tc.source + "---\nBody.\n")
			normalized := []byte("---\n" + tc.normalized + "---\nBody.\n")
			losses := SemanticYAMLLosses(source, normalized, SemanticRetention{})
			if tc.path == "" {
				if len(losses) != 0 {
					t.Fatalf("harmless normalization called a semantic loss: %+v", losses)
				}
				return
			}
			if len(losses) != 1 || losses[0].Code != "unsupported_yaml_tag_conversion" || losses[0].Path != tc.path || !strings.Contains(losses[0].Message, tc.tag) || !strings.Contains(losses[0].Message, "would become") {
				t.Fatalf("missing exact tag conversion refusal: %+v", losses)
			}
		})
	}
}

func TestSemanticYAMLLossesComparesExactScalars(t *testing.T) {
	cases := []struct {
		name   string
		before string
		after  string
		code   string
		path   string
	}{
		{"large_implicit_number", "x-value: 123456789012345678901234\n", "x-value: 1.2345678901234569e+23\n", "unsupported_yaml_value_conversion", "x-value"},
		{"float_to_integer", "x-value: 1.0\n", "x-value: 1\n", "unsupported_yaml_tag_conversion", "x-value"},
		{"claim_large_number", "claims:\n  - id: rule\n    x-value: 123456789012345678901234\n", "claims:\n  - id: rule\n    x-value: 1.2345678901234569e+23\n", "unsupported_yaml_value_conversion", "claims[0].x-value"},
		{"scenario_float_to_integer", "claims:\n  - id: rule\n    examples:\n      - id: ex\n        x-value: 1.0\n", "claims:\n  - id: rule\n    examples:\n      - id: ex\n        x-value: 1\n", "unsupported_yaml_tag_conversion", "claims[0].examples[0].x-value"},
		{"explicit_tag_same_type_different_value", "x-value: !!str old\n", "x-value: new\n", "unsupported_yaml_value_conversion", "x-value"},
		{"dropped_plain_extension", "x-value: retained\n", "other: present\n", "unsupported_yaml_content_conversion", "x-value"},
		{"hex_decimal", "x-value: 0x1F\n", "x-value: 31\n", "", ""},
		{"float_exponent", "x-value: 1e3\n", "x-value: 1000.0\n", "", ""},
		{"date_midnight", "x-value: 2026-09-25\n", "x-value: 2026-09-25T00:00:00Z\n", "", ""},
		{"bool_case", "x-value: TRUE\n", "x-value: true\n", "", ""},
		{"null_spelling", "x-value: ~\n", "x-value: null\n", "", ""},
		{"implicit_null_absent", "x-value: null\n", "other: present\n", "unsupported_yaml_content_conversion", "x-value"},
		{"explicit_string", "x-value: !!str 12\n", "x-value: \"12\"\n", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			losses := SemanticYAMLLosses(yamlDoc(tc.before), yamlDoc(tc.after), SemanticRetention{})
			if tc.code == "" {
				if len(losses) != 0 {
					t.Fatalf("harmless conversion reported loss: %+v", losses)
				}
				return
			}
			if len(losses) != 1 || losses[0].Code != tc.code || losses[0].Path != tc.path {
				t.Fatalf("want %s at %s, got %+v", tc.code, tc.path, losses)
			}
		})
	}
}

func TestSemanticYAMLLossesMatchesStableClaimAndScenarioIDs(t *testing.T) {
	before := yamlDoc("claims:\n  - id: first\n    x-value: 0x1F\n    examples:\n      - id: a\n        x-value: !vendor retained\n      - id: b\n        x-value: 1.0\n  - id: second\n    x-note: !!str known\n")
	after := yamlDoc("claims:\n  - id: second\n    x-note: known\n  - id: first\n    x-value: 31\n    examples:\n      - id: b\n        x-value: 1\n      - id: a\n        x-value: retained\n")
	losses := SemanticYAMLLosses(before, after, SemanticRetention{})
	if len(losses) != 2 || losses[0].Path != "claims[0].examples[0].x-value" || losses[1].Path != "claims[0].examples[1].x-value" {
		t.Fatalf("identity matching must report source paths, got %+v", losses)
	}
}

func TestSemanticYAMLLossesExemptsOnlyAuthorizedChanges(t *testing.T) {
	before := yamlDoc("status: !!str proposed\nclaims:\n  - id: old\n    text: Before\n    x-value: !vendor retained\n")
	after := yamlDoc("claims:\n  - id: new\n    text: After\n    x-value: retained\n")
	policy := SemanticRetention{
		RewrittenPaths: []string{"status", "claims[0].text"},
		RenamedIDs:     map[string]map[string]string{"claims": {"old": "new"}},
	}
	losses := SemanticYAMLLosses(before, after, policy)
	if len(losses) != 1 || losses[0].Path != "claims[0].x-value" || losses[0].Code != "unsupported_yaml_tag_conversion" {
		t.Fatalf("authorized edits must not exempt retained extension: %+v", losses)
	}
}

func TestSemanticYAMLLossesClaimAdditionAndExplicitRemoval(t *testing.T) {
	base := yamlDoc("claims:\n  - id: retained\n    x-value: 0x1F\n")
	added := yamlDoc("claims:\n  - id: added\n    x-value: !vendor new\n  - id: retained\n    x-value: 31\n")
	if losses := SemanticYAMLLosses(base, added, SemanticRetention{}); len(losses) != 0 {
		t.Fatalf("new claim must not become a false retained-base loss: %+v", losses)
	}
	removed := yamlDoc("claims: []\n")
	if losses := SemanticYAMLLosses(base, removed, SemanticRetention{}); len(losses) != 1 || losses[0].Path != "claims[0]" {
		t.Fatalf("unaccounted claim removal must be visible: %+v", losses)
	}
	policy := SemanticRetention{RewrittenPaths: []string{"claims[0]"}}
	if losses := SemanticYAMLLosses(base, removed, policy); len(losses) != 0 {
		t.Fatalf("exact intentional claim removal must be allowed: %+v", losses)
	}
}

func TestSemanticYAMLLossesUsesDeclaredStringMeaning(t *testing.T) {
	cases := []struct {
		name   string
		before string
		after  string
		path   string
	}{
		{"observed_timestamp", "observed_at: 2026-09-26T10:11:12Z\n", "observed_at: \"2026-09-26T10:11:12Z\"\n", ""},
		{"updated_timestamp", "updated_at: 2026-09-26T10:11:12Z\n", "updated_at: \"2026-09-26T10:11:12Z\"\n", ""},
		{"reopen_date", "reopen_when: 2026-09-26\n", "reopen_when: \"2026-09-26\"\n", ""},
		{"numeric_title", "title: 0012\n", "title: \"0012\"\n", ""},
		{"nested_claim_text", "claims:\n  - id: rule\n    text: 42\n", "claims:\n  - id: rule\n    text: \"42\"\n", ""},
		{"nested_evidence_reopen", "uses:\n  - id: use\n    reopen_when: 2026-09-26\n", "uses:\n  - id: use\n    reopen_when: \"2026-09-26\"\n", ""},
		{"unknown_timestamp", "x-value: 2026-09-26T10:11:12Z\n", "x-value: \"2026-09-26T10:11:12Z\"\n", "x-value"},
		{"unknown_number", "x-value: 0012\n", "x-value: \"0012\"\n", "x-value"},
		{"custom_tag_on_known_string", "title: !vendor 42\n", "title: \"42\"\n", "title"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			losses := SemanticYAMLLosses(yamlDoc(tc.before), yamlDoc(tc.after), SemanticRetention{})
			if tc.path == "" && len(losses) == 0 {
				return
			}
			if tc.path != "" && len(losses) == 1 && losses[0].Path == tc.path && losses[0].Code == "unsupported_yaml_tag_conversion" {
				return
			}
			t.Fatalf("want path %q, got %+v", tc.path, losses)
		})
	}
}

func TestSemanticYAMLLossesUsesOnlyDeclaredOptionalAbsence(t *testing.T) {
	cases := []struct {
		name   string
		before string
		after  string
		path   string
	}{
		{"empty_list", "supersedes: []\n", "title: present\n", ""},
		{"null_list", "supersedes: null\n", "title: present\n", ""},
		{"false_bool", "operator_confirmed: false\n", "title: present\n", ""},
		{"empty_string", "reopen_when: \"\"\n", "title: present\n", ""},
		{"null_string", "reopen_when: null\n", "title: present\n", ""},
		{"nested_empty_list", "claims:\n  - id: rule\n    refs: []\n", "claims:\n  - id: rule\n", ""},
		{"nested_empty_string", "links:\n  - kind: supports\n    reason: \"\"\n", "links:\n  - kind: supports\n", ""},
		{"nested_null_string", "sources:\n  - ref: item\n    source_revision:\n      kind: git\n      reason: null\n", "sources:\n  - ref: item\n    source_revision:\n      kind: git\n", ""},
		{"unknown_empty_list", "x-value: []\n", "title: present\n", "x-value"},
		{"unknown_null", "x-value: null\n", "title: present\n", "x-value"},
		{"unknown_false", "x-value: false\n", "title: present\n", "x-value"},
		{"custom_empty_string", "reopen_when: !vendor \"\"\n", "title: present\n", "reopen_when"},
		{"nonempty_known_string", "reopen_when: tomorrow\n", "title: present\n", "reopen_when"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			losses := SemanticYAMLLosses(yamlDoc(tc.before), yamlDoc(tc.after), SemanticRetention{})
			if tc.path == "" && len(losses) == 0 {
				return
			}
			if tc.path != "" && len(losses) == 1 && losses[0].Path == tc.path {
				return
			}
			t.Fatalf("want path %q, got %+v", tc.path, losses)
		})
	}
}

func TestSemanticYAMLLossesAcceptsSelectedChangeSchema(t *testing.T) {
	type task struct {
		Text string `yaml:"text"`
	}
	type change struct {
		Title string  `yaml:"title"`
		Tasks []task  `yaml:"tasks,omitempty"`
		Body  *string `yaml:"body,omitempty"`
	}
	before := yamlDoc("title: 42\ntasks: []\n")
	after := yamlDoc("title: \"42\"\n")
	if losses := SemanticYAMLLosses(before, after, SemanticRetention{}); len(losses) == 0 {
		t.Fatal("default carrier schema must not exempt unknown change tasks")
	}
	if losses := SemanticYAMLLosses(before, after, SemanticRetention{Schema: change{}}); len(losses) != 0 {
		t.Fatalf("selected change schema must interpret its declared fields: %+v", losses)
	}
	before = yamlDoc("tasks:\n  - text: 123\n    x-owner: !vendor abc\n")
	after = yamlDoc("tasks:\n  - text: \"123\"\n    x-owner: abc\n")
	losses := SemanticYAMLLosses(before, after, SemanticRetention{Schema: change{}})
	if len(losses) != 1 || losses[0].Path != "tasks[0].x-owner" {
		t.Fatalf("declared nested string must not waive unknown extension: %+v", losses)
	}
	before = yamlDoc("body: \"\"\n")
	after = yamlDoc("title: present\n")
	losses = SemanticYAMLLosses(before, after, SemanticRetention{Schema: change{}})
	if len(losses) != 1 || losses[0].Path != "body" {
		t.Fatalf("non-null pointer to empty string is an authored value: %+v", losses)
	}
}

func TestSemanticYAMLLossesMatchesClaimBindingsByUniqueRef(t *testing.T) {
	cases := []string{"checks", "implemented_by", "evidence_inputs"}
	for _, field := range cases {
		t.Run(field, func(t *testing.T) {
			knownField := "covers"
			if field == "evidence_inputs" {
				knownField = "applicability"
			}
			before := yamlDoc("claims:\n  - id: rule\n    " + field + ":\n      - ref: first\n        " + knownField + ": old\n        x-owner: !vendor abc\n      - ref: second\n        " + knownField + ": stable\n        x-owner: 42\n")
			after := yamlDoc("claims:\n  - id: rule\n    " + field + ":\n      - ref: second\n        " + knownField + ": stable\n        x-owner: 42\n      - ref: first\n        " + knownField + ": new\n        x-owner: abc\n")
			changed := "claims[0]." + field + "[0]." + knownField
			losses := SemanticYAMLLosses(before, after, SemanticRetention{RewrittenPaths: []string{changed}})
			path := "claims[0]." + field + "[0].x-owner"
			if len(losses) != 1 || losses[0].Path != path || losses[0].Code != "unsupported_yaml_tag_conversion" {
				t.Fatalf("same-ref edit must retain unrelated extension at source index: %+v", losses)
			}
			after = yamlDoc("claims:\n  - id: rule\n    " + field + ":\n      - ref: second\n        " + knownField + ": stable\n        x-owner: 42\n      - ref: first\n        " + knownField + ": new\n        x-owner: !vendor abc\n")
			if losses := SemanticYAMLLosses(before, after, SemanticRetention{RewrittenPaths: []string{changed}}); len(losses) != 0 {
				t.Fatalf("same-ref reorder and exact retained extension must pass: %+v", losses)
			}
		})
	}
}

func TestSemanticYAMLLossesMatchesNumericLookingBindingRefAsDeclaredString(t *testing.T) {
	before := yamlDoc("claims:\n  - id: rule\n    checks:\n      - ref: 42\n        covers: first\n        x-owner: !vendor abc\n      - ref: second\n        covers: second\n")
	after := yamlDoc("claims:\n  - id: rule\n    checks:\n      - ref: second\n        covers: second\n      - ref: \"42\"\n        covers: first\n        x-owner: !vendor abc\n")
	if losses := SemanticYAMLLosses(before, after, SemanticRetention{}); len(losses) != 0 {
		t.Fatalf("binding ref is a declared string with stable lexical identity: %+v", losses)
	}
}

func TestSemanticYAMLLossesDoesNotExemptAmbiguousBindingRefs(t *testing.T) {
	before := yamlDoc("claims:\n  - id: rule\n    checks:\n      - ref: duplicate\n        covers: first\n        x-owner: !vendor abc\n      - ref: duplicate\n        covers: second\n        x-owner: !vendor xyz\n")
	after := yamlDoc("claims:\n  - id: rule\n    checks:\n      - ref: duplicate\n        covers: second\n        x-owner: !vendor xyz\n      - ref: duplicate\n        covers: first\n        x-owner: !vendor abc\n")
	rewritten := []string{"claims[0].checks[0].covers", "claims[0].checks[1].covers"}
	losses := SemanticYAMLLosses(before, after, SemanticRetention{RewrittenPaths: rewritten})
	if len(losses) != 1 || losses[0].Code != "ambiguous_yaml_correspondence" || losses[0].Path != "claims[0].checks" {
		t.Fatalf("duplicate refs cannot grant item exemptions: %+v", losses)
	}
	if losses := SemanticYAMLLosses(before, before, SemanticRetention{}); len(losses) != 0 {
		t.Fatalf("unchanged duplicate list remains readable: %+v", losses)
	}
}

func TestSemanticYAMLLossesMatchesNestedChangeClaimBindings(t *testing.T) {
	type operation struct {
		Claim *Claim `yaml:"claim,omitempty"`
	}
	type patch struct {
		Operations []operation `yaml:"operations,omitempty"`
	}
	type change struct {
		Patches []patch `yaml:"patches,omitempty"`
	}
	before := yamlDoc("patches:\n  - operations:\n      - claim:\n          id: rule\n          checks:\n            - ref: first\n              covers: old\n              x-owner: !vendor abc\n            - ref: second\n              covers: stable\n")
	after := yamlDoc("patches:\n  - operations:\n      - claim:\n          id: rule\n          checks:\n            - ref: second\n              covers: stable\n            - ref: first\n              covers: old\n              x-owner: !vendor abc\n")
	if losses := SemanticYAMLLosses(before, after, SemanticRetention{Schema: change{}}); len(losses) != 0 {
		t.Fatalf("nested declared binding list should retain ref correspondence: %+v", losses)
	}
}

func TestSemanticYAMLLossesUsesReaderTimestampAndInfinitySpelling(t *testing.T) {
	cases := []struct {
		name   string
		before string
		after  string
		loss   bool
	}{
		{"space_without_zone", "2026-09-26 10:11:12", "2026-09-26T10:11:12Z", false},
		{"lower_t", "2026-09-26t10:11:12Z", "2026-09-26T10:11:12Z", false},
		{"short_components", "2026-9-6T1:2:3Z", "2026-09-06T01:02:03Z", false},
		{"positive_inf", "+.inf", ".inf", false},
		{"negative_inf", "-.inf", ".inf", true},
		{"distinct_time", "2026-09-26 10:11:12", "2026-09-26T10:11:13Z", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			losses := SemanticYAMLLosses(yamlDoc("x-value: "+tc.before+"\n"), yamlDoc("x-value: "+tc.after+"\n"), SemanticRetention{})
			if tc.loss && len(losses) == 1 && losses[0].Code == "unsupported_yaml_value_conversion" && losses[0].Path == "x-value" {
				return
			}
			if !tc.loss && len(losses) == 0 {
				return
			}
			t.Fatalf("incorrect value comparison: %+v", losses)
		})
	}
}

func TestSemanticYAMLLossesMatchesActualRecordCodec(t *testing.T) {
	for _, source := range []string{
		"observed_at: 2026-09-26T10:11:12Z\n",
		"updated_at: 2026-09-26T10:11:12Z\n",
		"reopen_when: 2026-09-26\n",
		"title: 0012\n",
		"supersedes: []\n",
		"supersedes: null\n",
		"operator_confirmed: false\n",
		"reopen_when: \"\"\n",
		"x-value: 2026-09-26 10:11:12\n",
		"x-value: 2026-09-26t10:11:12Z\n",
		"x-value: 2026-9-6T1:2:3Z\n",
		"x-value: +.inf\n",
	} {
		t.Run(strings.TrimSpace(source), func(t *testing.T) {
			before := yamlDoc(source)
			node, err := frontmatterNode(before)
			if err != nil {
				t.Fatal(err)
			}
			var record Record
			if err := node.Decode(&record); err != nil {
				t.Fatalf("existing reader rejects fixture: %v", err)
			}
			after, err := Encode(record, []byte("Body.\n"))
			if err != nil {
				t.Fatal(err)
			}
			if losses := SemanticYAMLLosses(before, after, SemanticRetention{}); len(losses) != 0 {
				t.Fatalf("actual schema codec falsely loses value: %+v\nnormalized:\n%s", losses, after)
			}
		})
	}
}

func TestSemanticYAMLLossesStillRefusesActualCodecExtensionLoss(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{"custom_tag", "x-value: !vendor abc\n"},
		{"large_integer", "x-value: 123456789012345678901234\n"},
		{"integral_float", "x-value: 1.0\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := yamlDoc(tc.source)
			node, err := frontmatterNode(before)
			if err != nil {
				t.Fatal(err)
			}
			var record Record
			if err := node.Decode(&record); err != nil {
				t.Fatalf("historical reader rejects fixture: %v", err)
			}
			after, err := Encode(record, []byte("Body.\n"))
			if err != nil {
				t.Fatal(err)
			}
			losses := SemanticYAMLLosses(before, after, SemanticRetention{})
			if len(losses) == 0 || losses[0].Path != "x-value" {
				t.Fatalf("actual codec changed retained extension without refusal: %+v\nnormalized:\n%s", losses, after)
			}
		})
	}
}
