package change

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func t01br4RevisionSource(t *testing.T, taggedField, state string) ([]byte, []byte, string, map[string]Basis) {
	t.Helper()
	d, _, bases, c := setup(t)
	modified := changeClaim(d)
	c.State = state
	c.Intent = "Preserve the original change intent"
	c.Tasks = []Task{{ID: "one", Text: "Preserve task metadata", Extra: carrier.Extra{"x-owner": "abc"}}}
	c.Patches[0].Operations = []Operation{{Op: "MODIFIED", ClaimID: modified.ID, Claim: &modified, Reason: "Clarify the claim"}}
	c.Patches[0].Extra = carrier.Extra{"x-owner": "abc"}
	raw, err := Encode(c, []byte("Original change prose.\n"))
	if err != nil {
		t.Fatal(err)
	}
	var original, tagged []byte
	switch taggedField {
	case "intent":
		original = []byte("intent: Preserve the original change intent")
		tagged = []byte("intent: !vendor Preserve the original change intent")
	case "tasks":
		original = []byte("    x-owner: abc")
		tagged = []byte("    x-owner: !vendor abc")
	case "patches":
		original = []byte("    x-owner: abc")
		tagged = []byte("    x-owner: !vendor abc")
	case "":
	default:
		t.Fatalf("unknown tagged field %q", taggedField)
	}
	if taggedField == "patches" {
		// The task extension occurs first in the encoded source.
		first := bytes.Index(raw, original)
		if first < 0 {
			t.Fatal("missing first extension")
		}
		second := bytes.Index(raw[first+len(original):], original)
		if second < 0 {
			t.Fatal("missing patch extension")
		}
		at := first + len(original) + second
		raw = append(append(bytes.Clone(raw[:at]), tagged...), raw[at+len(original):]...)
	} else if taggedField != "" {
		if !bytes.Contains(raw, original) {
			t.Fatalf("fixture lacks %q", original)
		}
		raw = bytes.Replace(raw, original, tagged, 1)
	}
	if parsed := Parse(raw); carrier.HasErrors(parsed.Diagnostics) {
		t.Fatalf("historically readable source rejected: %+v", parsed.Diagnostics)
	}
	_, snapshot, hash, err := carrier.NewSnapshot(raw, carrier.InterpretationBasis{})
	if err != nil {
		t.Fatal(err)
	}
	return raw, snapshot, c.ID + "@" + hash, bases
}

func t01br4RevisionEdit(action, currentRef string) Revision {
	return Revision{
		Action:     action,
		ID:         "chg-20260923-00000002",
		CreatedAt:  "2026-09-23T11:00:00Z",
		Reason:     "Record the requested lifecycle action",
		CurrentRef: currentRef,
	}
}

func TestT01BR4IgnoredRevisionContentCannotExemptRetainedTags(t *testing.T) {
	for _, action := range []string{"archive", "reopen"} {
		state := "open"
		if action == "reopen" {
			state = "archived"
		}
		for _, field := range []string{"intent", "tasks", "patches"} {
			t.Run(action+"/"+field, func(t *testing.T) {
				_, snapshot, ref, bases := t01br4RevisionSource(t, field, state)
				originalSnapshot := bytes.Clone(snapshot)
				edit := t01br4RevisionEdit(action, ref)
				path := field
				switch field {
				case "intent":
					replacement := "Ignored intent"
					edit.Intent = &replacement
				case "tasks":
					empty := []Task{}
					edit.Tasks = &empty
					path = "tasks[0].x-owner"
				case "patches":
					edit.Patches = []SectionPatch{}
					path = "patches[0].x-owner"
				}
				got := Revise(ref, snapshot, edit, bases)
				if got.Kind != "conflict" || len(got.Raw) != 0 {
					t.Fatalf("ignored %s changed retained semantics: %+v", field, got)
				}
				found := false
				for _, diagnostic := range got.Diagnostics {
					if diagnostic.Code == "unsupported_yaml_tag_conversion" && diagnostic.Path == path {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing exact retained-value refusal for %s: %+v", path, got.Diagnostics)
				}
				if !bytes.Equal(snapshot, originalSnapshot) {
					t.Fatal("failed revision changed predecessor bytes")
				}
			})
		}
	}
}

func TestT01BR4OrdinaryLifecycleRetainsIgnoredRevisionFields(t *testing.T) {
	for _, action := range []string{"archive", "reopen"} {
		t.Run(action, func(t *testing.T) {
			state := "open"
			wantState := "archived"
			if action == "reopen" {
				state = "archived"
				wantState = "open"
			}
			raw, snapshot, ref, bases := t01br4RevisionSource(t, "", state)
			originalSnapshot := bytes.Clone(snapshot)
			ed := t01br4RevisionEdit(action, ref)
			irrelevantIntent := "Ignored replacement"
			irrelevantTasks := []Task{}
			ed.Intent = &irrelevantIntent
			ed.Tasks = &irrelevantTasks
			ed.Patches = []SectionPatch{}
			got := Revise(ref, snapshot, ed, bases)
			if got.Kind != "ready" || got.Change.State != wantState {
				t.Fatalf("ordinary lifecycle rejected: %+v", got)
			}
			previous := Parse(raw).Change
			if got.Change.Intent != previous.Intent || !reflect.DeepEqual(got.Change.Tasks, previous.Tasks) || !reflect.DeepEqual(got.Change.Patches, previous.Patches) {
				t.Fatalf("ignored fields changed authored content: %+v", got.Change)
			}
			if !bytes.Equal(snapshot, originalSnapshot) || !bytes.Equal(Parse(got.Raw).Body, []byte("Original change prose.\n")) {
				t.Fatal("lifecycle revision changed predecessor or body bytes")
			}
		})
	}
}

func TestT01BR4ActualUpdateAndRebaseCanReplaceTaggedContent(t *testing.T) {
	for _, field := range []string{"intent", "tasks", "patches"} {
		t.Run("update/"+field, func(t *testing.T) {
			raw, snapshot, ref, bases := t01br4RevisionSource(t, field, "open")
			ed := t01br4RevisionEdit("update", ref)
			switch field {
			case "intent":
				replacement := "Explicitly changed intent"
				ed.Intent = &replacement
			case "tasks":
				replacement := []Task{{ID: "one", Text: "Explicitly changed task", Done: true}}
				ed.Tasks = &replacement
			case "patches":
				replacement := Parse(raw).Change.Patches
				replacement[0].Extra = carrier.Extra{"x-owner": "explicit replacement"}
				ed.Patches = replacement
			}
			got := Revise(ref, snapshot, ed, bases)
			if got.Kind != "ready" {
				t.Fatalf("actual update rejected: %+v", got)
			}
		})
	}
	raw, snapshot, ref, bases := t01br4RevisionSource(t, "patches", "open")
	ed := t01br4RevisionEdit("rebase", ref)
	ed.Patches = Parse(raw).Change.Patches
	ed.Patches[0].Extra = carrier.Extra{"x-owner": "explicit rebase"}
	got := Revise(ref, snapshot, ed, bases)
	if got.Kind != "ready" || got.Preview == nil || got.Preview.Kind != "ready" {
		t.Fatalf("actual rebase rejected: %+v", got)
	}
}
