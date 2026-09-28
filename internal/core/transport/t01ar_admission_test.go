package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func TestT01ARObserveAdmitsAppConsumedCodeConfig(t *testing.T) {
	raw := []byte(`{"format":"haft.api/2","operation":"check","action":"observe","observation":{},"code_config":{"goos":"linux","ignore_patterns":["generated/**"]}}`)
	for _, profile := range []struct{ name, tool string }{{ProfileDefault, "haft_check"}, {ProfileLegacy, "haft"}} {
		q, err := ValidateToolCall(profile.name, profile.tool, raw)
		if err != nil {
			t.Fatalf("%s rejected app-consumed observe config: %v", profile.name, err)
		}
		if q.CodeConfig == nil || q.CodeConfig.GOOS != "linux" || !reflect.DeepEqual(q.CodeConfig.IgnorePatterns, []string{"generated/**"}) {
			t.Fatalf("%s altered observe config: %+v", profile.name, q.CodeConfig)
		}
	}
	for _, profile := range []string{ProfileDefault, ProfileLegacy} {
		definitions := ToolDefinitions(profile)
		found := false
		for _, definition := range definitions {
			schema := definition["inputSchema"].(map[string]any)
			for _, entry := range schema["oneOf"].([]any) {
				branch := entry.(map[string]any)
				if branch["title"] != "check/observe" {
					continue
				}
				found = true
				properties := branch["properties"].(map[string]any)
				if _, present := properties["code_config"]; !present {
					t.Fatalf("%s observe schema omits code_config", profile)
				}
				if properties["observation"].(map[string]any)["not"] == nil {
					t.Fatalf("%s observe schema admits required observation:null", profile)
				}
			}
		}
		if !found {
			t.Fatalf("%s observe schema branch absent", profile)
		}
	}
}

func TestT01AROptionalNullPreservesOmissionAcrossBranches(t *testing.T) {
	cases := []struct {
		name  string
		tool  string
		field string
		raw   string
	}{
		{"remember snapshots", "haft_write", "snapshots", `{"format":"haft.api/2","operation":"remember","carrier":"note","snapshots":null}`},
		{"remember expected heads", "haft_write", "expected_heads", `{"format":"haft.api/2","operation":"remember","carrier":"note","expected_heads":null}`},
		{"remember expected targets", "haft_write", "expected_target_digests", `{"format":"haft.api/2","operation":"remember","carrier":"note","expected_target_digests":null}`},
		{"remember retain", "haft_write", "retain", `{"format":"haft.api/2","operation":"remember","carrier":"note","retain":null}`},
		{"context config", "haft_read", "code_config", `{"format":"haft.api/2","operation":"context","code_config":null}`},
		{"context prior code", "haft_read", "prior_code", `{"format":"haft.api/2","operation":"context","prior_code":null}`},
		{"impact config", "haft_read", "code_config", `{"format":"haft.api/2","operation":"impact","code_config":null}`},
		{"check prepare seed", "haft_check", "seed", `{"format":"haft.api/2","operation":"check","action":"prepare","ref":"claim","check_ref":"test:file.go::TestX","scope":"package","seed":null}`},
		{"check prepare config", "haft_check", "code_config", `{"format":"haft.api/2","operation":"check","action":"prepare","ref":"claim","check_ref":"test:file.go::TestX","scope":"package","code_config":null}`},
		{"check observe config", "haft_check", "code_config", `{"format":"haft.api/2","operation":"check","action":"observe","observation":{},"code_config":null}`},
		{"change create snapshots", "haft_change", "snapshots", `{"format":"haft.api/2","operation":"change","action":"create","carrier":"change","snapshots":null}`},
		{"change apply metadata", "haft_change", "metadata", `{"format":"haft.api/2","operation":"change","action":"apply","ref":"change","request_id":"id","expected_generation":"gen","preview_digest":"sha256:x","metadata":null}`},
		{"source inspect snapshots", "haft_fpf", "snapshots", `{"format":"haft.api/2","operation":"source","action":"inspect","ref":"source","snapshots":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var supplied map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.raw), &supplied); err != nil {
				t.Fatal(err)
			}
			delete(supplied, tc.field)
			absentRaw, err := json.Marshal(supplied)
			if err != nil {
				t.Fatal(err)
			}
			for _, profile := range []struct{ name, tool string }{{ProfileDefault, tc.tool}, {ProfileLegacy, "haft"}} {
				withNull, err := ValidateToolCall(profile.name, profile.tool, []byte(tc.raw))
				if err != nil {
					t.Fatalf("%s rejected valid B1 nil-able %s: %v", profile.name, tc.field, err)
				}
				without, err := ValidateToolCall(profile.name, profile.tool, absentRaw)
				if err != nil {
					t.Fatalf("%s rejected omitted %s: %v", profile.name, tc.field, err)
				}
				withBytes, err := json.Marshal(withNull)
				if err != nil {
					t.Fatal(err)
				}
				withoutBytes, err := json.Marshal(without)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(withBytes, withoutBytes) {
					t.Fatalf("%s %s changed logical payload digest basis", profile.name, tc.field)
				}
			}
		})
	}
}

func TestT01ARFrozenB1OptionalNullReplay(t *testing.T) {
	root, basis, _, frozen := t01aFrozenB1(t)
	raw, err := os.ReadFile(filepath.Join("testdata", "b1-3d3ad688", basis.RequestFile))
	if err != nil {
		t.Fatal(err)
	}
	var supplied map[string]json.RawMessage
	if err := json.Unmarshal(raw, &supplied); err != nil {
		t.Fatal(err)
	}
	supplied["snapshots"] = json.RawMessage("null")
	nullRaw, err := json.Marshal(supplied)
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []struct{ name, tool string }{{ProfileDefault, "haft_write"}, {ProfileLegacy, "haft"}} {
		q, err := ValidateToolCall(profile.name, profile.tool, nullRaw)
		if err != nil {
			t.Fatalf("%s rejected frozen B1 replay: %v", profile.name, err)
		}
		appRequest := q
		appRequest.Format = app.Format
		encoded, err := json.Marshal(appRequest)
		if err != nil {
			t.Fatal(err)
		}
		if carrier.Digest(encoded) != basis.PayloadDigest {
			t.Fatalf("%s changed B1 payload digest: got %s want %s", profile.name, carrier.Digest(encoded), basis.PayloadDigest)
		}
		result := (app.Service{Root: root}).Call(context.Background(), q)
		if result.Kind != "replayed" || result.IsError {
			t.Fatalf("%s failed frozen B1 replay: %+v", profile.name, result)
		}
	}
	for relative, original := range frozen {
		current, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil || !bytes.Equal(current, original) {
			t.Fatalf("frozen B1 historical bytes changed at %s: %v", relative, err)
		}
	}
	transactions, err := os.ReadDir(filepath.Join(root, ".haft", "transactions"))
	if err != nil || len(transactions) != 1 || transactions[0].Name() != basis.TransactionID {
		t.Fatalf("B1 replay duplicated publication: %v, %v", transactions, err)
	}
}

func TestT01ARNullAndWrongBranchControlsStillReject(t *testing.T) {
	cases := []struct {
		name string
		tool string
		raw  string
	}{
		{"required observation", "haft_check", `{"format":"haft.api/2","operation":"check","action":"observe","observation":null}`},
		{"required revision", "haft_change", `{"format":"haft.api/2","operation":"change","action":"archive","ref":"change","revision":null}`},
		{"scalar limit", "haft_read", `{"format":"haft.api/2","operation":"recall","limit":null}`},
		{"scalar view", "haft_read", `{"format":"haft.api/2","operation":"recall","view":null}`},
		{"wrong branch optional null", "haft_read", `{"format":"haft.api/2","operation":"recall","snapshots":null}`},
		{"unknown control", "haft_read", `{"format":"haft.api/2","operation":"recall","unknown_optional":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, profile := range []struct{ name, tool string }{{ProfileDefault, tc.tool}, {ProfileLegacy, "haft"}} {
				if _, err := ValidateToolCall(profile.name, profile.tool, []byte(tc.raw)); err == nil {
					t.Fatalf("%s admitted %s", profile.name, tc.name)
				}
			}
		})
	}
}

func TestT01ARWrongToolNamesAdvertisedOwner(t *testing.T) {
	raw := []byte(`{"format":"haft.api/2","operation":"recall"}`)
	if _, err := ValidateToolCall(ProfileDefault, "haft", raw); err == nil || !strings.Contains(err.Error(), "use advertised tool haft_read") {
		t.Fatalf("default wrong-tool guidance: %v", err)
	}
	if _, err := ValidateToolCall(ProfileLegacy, "haft_read", raw); err == nil || !strings.Contains(err.Error(), "use advertised tool haft") {
		t.Fatalf("legacy wrong-tool guidance: %v", err)
	}
}

func TestT01ARSchemaMakesOnlyNilableOptionalFieldsNullable(t *testing.T) {
	properties := RequestSchema()["properties"].(map[string]any)
	for _, field := range []string{"snapshots", "expected_heads", "expected_target_digests", "metadata", "revision", "seed", "observation", "code_config", "prior_code", "retain"} {
		kind := properties[field].(map[string]any)["type"].([]string)
		if len(kind) != 2 || kind[1] != "null" {
			t.Fatalf("%s schema omitted B1 null compatibility: %v", field, kind)
		}
	}
	for _, field := range []string{"format", "operation", "carrier", "limit", "strict", "capture_code"} {
		if _, nullable := properties[field].(map[string]any)["type"].([]string); nullable {
			t.Fatalf("scalar %s schema admits null", field)
		}
	}
}
