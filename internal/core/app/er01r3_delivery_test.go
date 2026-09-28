package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

type er01r3Member struct {
	Key         string              `json:"key"`
	KeyComplete bool                `json:"key_complete"`
	Value       json.RawMessage     `json:"value"`
	Complete    bool                `json:"complete"`
	Read        delivery.Descriptor `json:"read"`
	KeyRequest  *delivery.Request   `json:"key_request"`
}

func er01r3Members(t *testing.T, page delivery.Response) []er01r3Member {
	t.Helper()
	raw, err := json.Marshal(page.Data)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Members []er01r3Member `json:"members"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body.Members
}

func TestER01R3PersistedRefAdaptsAtLaterMemberPage(t *testing.T) {
	s := service(t)
	claimID := strings.Repeat("c", 400)
	longKey := "b" + strings.Repeat("\x01", 100)
	values := map[string]any{"a": strings.Repeat("<", 100), longKey: strings.Repeat("x", 9000), "z": strings.Repeat("y", 9000)}
	record := carrier.Record{Kind: "spec", Title: "Long ref disclosure", About: "domain:Delivery", Slug: "long-ref", ReceivingUse: "Exercise exact returned routes", Claims: []carrier.Claim{{ID: claimID, Kind: "definition", Text: "Fixture claim.", Extra: carrier.Extra{"x-wide": values}}}}
	written := public(t, s, Request{Operation: "remember", RequestID: "er01r3-ref", Carrier: encode(t, record, nil)})
	if written.Kind != "written" || written.IsError {
		t.Fatal("fixture was not remembered", written.Kind)
	}
	summary := public(t, s, Request{Operation: "recall", Ref: "spec:long-ref#" + claimID})
	if summary.Kind != "found" || summary.IsError {
		t.Fatal("exact claim was not recalled", summary.Kind)
	}
	claim := deliveredPart(t, s, summary, "claim")
	if len(claim.Ref) != 495 || summary.Delivery.Lifetime != "persisted_snapshot" {
		t.Fatal("fixture did not exercise a retained 495-byte selector", len(claim.Ref), summary.Delivery.Lifetime)
	}
	var wide delivery.Request
	for q := &claim; q != nil; {
		page := public(t, s, nextApp(*q))
		if page.Kind != "found" || page.Delivery.Encoding != "json_members" {
			t.Fatal("claim member page failed", page.Kind, page.Delivery.Encoding)
		}
		for _, member := range er01r3Members(t, page) {
			if member.Key == "x-wide" {
				wide = member.Read.Request
			}
		}
		q = page.Delivery.Next
	}
	if wide.Part == "" {
		t.Fatal("nested extension was not addressable")
	}
	seen := map[string]bool{}
	adapted := false
	var transientRead delivery.Request
	var transientDirectory delivery.Request
	for q := &wide; q != nil; {
		page := public(t, s, nextApp(*q))
		if page.Kind != "found" || page.Delivery.Encoding != "json_members" {
			t.Fatal("nested member page failed", page.Kind, page.Delivery.Encoding)
		}
		if page.Delivery.Lifetime == "transient_cache" {
			adapted = true
			if page.Delivery.Catalog != nil {
				transientDirectory = *page.Delivery.Catalog
			}
		}
		for _, member := range er01r3Members(t, page) {
			if page.Delivery.Lifetime == "transient_cache" && transientRead.Ref == "" {
				transientRead = member.Read.Request
			}
			key := member.Key
			if !member.KeyComplete {
				if member.KeyRequest == nil {
					t.Fatal("shortened key has no exact route")
				}
				key = string(collectPart(t, s, *member.KeyRequest))
			}
			if seen[key] || values[key] == nil {
				t.Fatal("member skipped, duplicated or misidentified", key)
			}
			want := []byte(values[key].(string))
			if !bytes.Equal(collectPart(t, s, member.Read.Request), want) || member.Read.Digest != carrier.Digest(want) {
				t.Fatal("exact child bytes/digest changed", key)
			}
			if key == "a" && member.KeyRequest != nil {
				t.Fatal("full short key acquired a redundant route")
			}
			seen[key] = true
		}
		if page.Delivery.Next != nil && page.Delivery.Lifetime == "persisted_snapshot" {
			changedCursor := *page.Delivery.Next
			changedCursor.Limit++
			if rejected := public(t, s, nextApp(changedCursor)); rejected.Kind != "cursor_parameters_changed" {
				t.Fatal("edited persisted cursor accepted", rejected.Kind)
			}
			wrongDigest := *page.Delivery.Next
			wrongDigest.ExpectedDigest = "sha256:" + strings.Repeat("0", 64)
			if rejected := public(t, s, nextApp(wrongDigest)); rejected.Kind != "stale" {
				t.Fatal("wrong persisted digest accepted", rejected.Kind)
			}
			wrongGeneration := *page.Delivery.Next
			wrongGeneration.ExpectedGeneration = "sha256:" + strings.Repeat("0", 64)
			if rejected := public(t, s, nextApp(wrongGeneration)); rejected.Kind != "stale" {
				t.Fatal("wrong persisted generation accepted", rejected.Kind)
			}
		}
		q = page.Delivery.Next
	}
	if len(seen) != len(values) || !adapted {
		t.Fatal("later page was not recovered through bounded selector", seen, adapted)
	}
	if !strings.HasPrefix(transientRead.Ref, "result:") || !strings.HasPrefix(transientDirectory.Ref, "result:") {
		t.Fatal("adapted page did not return disposable routes")
	}
	changed := public(t, s, Request{Operation: "remember", RequestID: "next-generation", Carrier: "---\nkind: note\ntitle: Next generation\nabout: domain:Delivery\n---\nAnother fact.\n"})
	if changed.Kind != "written" {
		t.Fatal("new generation fixture failed", changed.Kind)
	}
	if got := collectPart(t, s, transientRead); len(got) == 0 {
		t.Fatal("immutable captured child became stale after a new generation")
	}
	if stale := public(t, s, nextApp(transientDirectory)); stale.Kind != "stale" {
		t.Fatal("mutable transient directory accepted changed generation", stale.Kind)
	}
	if err := os.RemoveAll(filepath.Join(s.Root, ".haft", ".cache", "disclosure")); err != nil {
		t.Fatal(err)
	}
	if expired := public(t, s, nextApp(transientRead)); expired.Kind != "expired" {
		t.Fatal("lost disposable selector was not reported expired", expired.Kind)
	}
	exactRead := transientRead
	exactRead.Ref = claim.Ref
	if got := collectPart(t, s, exactRead); len(got) == 0 {
		t.Fatal("original pinned selector could not reconstruct exact bytes")
	}
}

func TestER01R3LegalRefWidthsKeepExecutableChildRoutes(t *testing.T) {
	for _, idWidth := range []int{1, 335, 417, 418, 700, 10000} {
		t.Run(strconv.Itoa(idWidth), func(t *testing.T) {
			s := service(t)
			claimID := strings.Repeat("c", idWidth)
			key := strings.Repeat("a", 101)
			values := map[string]any{key: strings.Repeat("x", 9000), "z": strings.Repeat("y", 9000)}
			record := carrier.Record{Kind: "spec", Title: "Ref width disclosure", About: "domain:Delivery", Slug: "ref-width", ReceivingUse: "Exercise returned routes", Claims: []carrier.Claim{{ID: claimID, Kind: "definition", Text: "Fixture claim.", Extra: carrier.Extra{"x-wide": values}}}}
			written := public(t, s, Request{Operation: "remember", RequestID: "ref-width", Carrier: encode(t, record, nil)})
			if written.Kind != "written" {
				t.Fatal("fixture write failed", written.Kind)
			}
			summary := public(t, s, Request{Operation: "recall", Ref: "spec:ref-width#" + claimID})
			if summary.Kind != "found" {
				t.Fatal("fixture recall failed", summary.Kind)
			}
			fullResult := collectPart(t, s, deliveredPart(t, s, summary, "result"))
			var captured struct {
				Data struct {
					ExactRef string `json:"exact_ref"`
				} `json:"data"`
			}
			if err := json.Unmarshal(fullResult, &captured); err != nil {
				t.Fatal(err)
			}
			exact := captured.Data.ExactRef
			if len(exact) != idWidth+95 {
				t.Fatal("exact selector width changed", len(exact), idWidth)
			}
			claim := deliveredPart(t, s, summary, "claim")
			var wide delivery.Request
			for q := &claim; q != nil; {
				page := public(t, s, nextApp(*q))
				if page.Kind != "found" || page.Delivery.Encoding != "json_members" {
					t.Fatal("claim page failed", page.Kind)
				}
				for _, member := range er01r3Members(t, page) {
					if member.Key == "x-wide" {
						wide = member.Read.Request
					}
				}
				q = page.Delivery.Next
			}
			if wide.Part == "" {
				t.Fatal("extension route missing")
			}
			if idWidth > 417 {
				wide.Ref = exact
			}
			seen := map[string]bool{}
			for q := &wide; q != nil; {
				page := public(t, s, nextApp(*q))
				if page.Kind != "found" || page.Delivery.Encoding != "json_members" {
					t.Fatal("extension page failed", page.Kind, page.Delivery.Encoding)
				}
				for _, member := range er01r3Members(t, page) {
					fullKey := member.Key
					if !member.KeyComplete {
						if member.KeyRequest == nil {
							t.Fatal("incomplete key has no route")
						}
						fullKey = string(collectPart(t, s, *member.KeyRequest))
					}
					if seen[fullKey] || values[fullKey] == nil {
						t.Fatal("key lost or duplicated", fullKey)
					}
					want := []byte(values[fullKey].(string))
					if !bytes.Equal(collectPart(t, s, member.Read.Request), want) {
						t.Fatal("child bytes changed", fullKey)
					}
					seen[fullKey] = true
				}
				q = page.Delivery.Next
			}
			if len(seen) != len(values) {
				t.Fatal("not all members recovered", len(seen))
			}
		})
	}
}
