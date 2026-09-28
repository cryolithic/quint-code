package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func alphaR1Member(t *testing.T, s Service, start delivery.Request, key string) delivery.Request {
	t.Helper()
	for q := &start; q != nil; {
		page := public(t, s, nextApp(*q))
		if page.Kind != "found" || page.Delivery.Encoding != "json_members" {
			t.Fatalf("member directory for %q failed: %s/%s", key, page.Kind, page.Delivery.Encoding)
		}
		for _, member := range er01r3Members(t, page) {
			if member.Key == key {
				return member.Read.Request
			}
		}
		q = page.Delivery.Next
	}
	t.Fatalf("member %q absent", key)
	return delivery.Request{}
}

func alphaR1BlockDisclosure(t *testing.T, s Service) func() {
	t.Helper()
	dir := filepath.Join(s.Root, ".haft", ".cache", "disclosure")
	held := dir + "-held"
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir, held); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("Owned cache publication blocker.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		if err := os.Remove(dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(held, dir); err != nil {
			t.Fatal(err)
		}
		restored = true
	}
	t.Cleanup(restore)
	return restore
}

func alphaR1NoRoutes(t *testing.T, r delivery.Response) {
	t.Helper()
	if r.Delivery.Catalog != nil || r.Delivery.BasisRequest != nil || r.Delivery.Next != nil || len(r.Delivery.Available) != 0 {
		t.Fatalf("unavailable expansion advertised a read route: %+v", r.Delivery)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"operation":"read","ref":""`)) {
		t.Fatal("unavailable expansion advertised an empty-ref read")
	}
}

func TestAlphaR1CompositeRecallBasisAndImmutableEdition(t *testing.T) {
	s := service(t)
	id := strings.Repeat("c", 450)
	body := []byte("Original body.\n\nReport:\n```json\n{\"observation\":\"historical\"}\n```\n")
	record := carrier.Record{Format: "haft/2", Kind: "spec", Title: "Composite basis", Origin: "agent_edit", About: "domain:Delivery", Slug: "composite", ReceivingUse: "Distinguish current relations from edition content", Claims: []carrier.Claim{{ID: id, Kind: "definition", Text: "Original claim.", Extra: carrier.Extra{"x-wide": strings.Repeat("<", 9000)}}}}
	if written := public(t, s, Request{Operation: "remember", RequestID: "alpha-r1-spec", Carrier: encode(t, record, body)}); written.Kind != "written" {
		t.Fatal("fixture spec not written", written.Kind)
	}
	alias := "spec:composite#" + id
	first := public(t, s, Request{Operation: "recall", Ref: alias})
	if first.Kind != "found" || first.Delivery.Catalog == nil || first.Delivery.Lifetime != "transient_cache" {
		t.Fatalf("long selector did not receive a transient read directory: %+v", first.Delivery)
	}
	ref := first.Delivery.Catalog.Ref
	summary := delivery.Request{Format: delivery.Format, Operation: "read", Ref: ref, View: "bytes", Part: "summary"}
	if captured := collectPart(t, s, summary); !bytes.Contains(captured, []byte("content_state")) {
		t.Fatal("fixture summary did not include current spec state")
	}
	result := deliveredPart(t, s, first, "result")
	backlinks := deliveredPart(t, s, first, "backlinks")
	data := alphaR1Member(t, s, result, "data")
	deepBacklinks := alphaR1Member(t, s, data, "backlinks")
	key := delivery.Request{Format: delivery.Format, Operation: "read", Ref: ref, View: "bytes", Part: deepBacklinks.Part + "/key"}
	if got := collectPart(t, s, key); string(got) != "backlinks" {
		t.Fatal("relation key route was not readable before drift", string(got))
	}
	immutable := map[string][]byte{}
	requests := map[string]delivery.Request{}
	for _, name := range []string{"carrier", "claim", "record", "body", "snapshot", "report_1"} {
		request := deliveredPart(t, s, first, name)
		requests[name] = request
		immutable[name] = collectPart(t, s, request)
	}
	if !bytes.Equal(immutable["body"], body) || !bytes.Contains(immutable["claim"], []byte("x-wide")) || !bytes.Contains(immutable["report_1"], []byte("historical")) {
		t.Fatal("immutable fixture parts were not exact before drift")
	}
	if changed := public(t, s, Request{Operation: "remember", RequestID: "alpha-r1-unrelated", Carrier: "---\nkind: note\ntitle: Unrelated generation\nabout: domain:Delivery\n---\nOther history.\n"}); changed.Kind != "written" {
		t.Fatal("unrelated generation was not recorded", changed.Kind)
	}
	for name, request := range map[string]delivery.Request{"summary": summary, "result": result, "result_data": data, "relation_descendant": deepBacklinks, "relation_key": key, "backlinks": backlinks, "parts": *first.Delivery.Catalog} {
		if stale := public(t, s, nextApp(request)); stale.Kind != "stale" {
			t.Fatalf("%s ignored changed generation: %s", name, stale.Kind)
		}
	}
	for name, request := range requests {
		if got := collectPart(t, s, request); !bytes.Equal(got, immutable[name]) || carrier.Digest(got) != carrier.Digest(immutable[name]) {
			t.Fatalf("immutable %s changed after unrelated generation", name)
		}
	}
	fresh := public(t, s, Request{Operation: "recall", Ref: alias})
	if fresh.Kind != "found" || fresh.Delivery.Catalog == nil {
		t.Fatal("fresh recall did not recover current selection")
	}
	linked := carrier.Record{Kind: "note", Title: "Current relation", About: record.About, Links: []carrier.Link{{Kind: "relates_to", Target: alias}}}
	if added := public(t, s, Request{Operation: "remember", RequestID: "alpha-r1-relation", Carrier: encode(t, linked, nil)}); added.Kind != "written" {
		t.Fatal("relation change was not recorded", added.Kind)
	}
	if stale := public(t, s, nextApp(*fresh.Delivery.Catalog)); stale.Kind != "stale" {
		t.Fatal("current relation change did not stale prior directory", stale.Kind)
	}
	current := public(t, s, Request{Operation: "recall", Ref: alias})
	if current.Kind != "found" {
		t.Fatal("current recall failed after relation change", current.Kind)
	}
	links := collectPart(t, s, deliveredPart(t, s, current, "backlinks"))
	if !bytes.Contains(links, []byte(alias)) {
		t.Fatal("fresh recall did not expose new relation", string(links))
	}
}

func TestAlphaR1CachePublicationRefusesExpansionAndKeepsReceipt(t *testing.T) {
	s := service(t)
	id := strings.Repeat("c", 450)
	record := carrier.Record{Kind: "spec", Title: "Cache failure", About: "domain:Delivery", Slug: "cache-failure", ReceivingUse: "Exercise unavailable expansion", Claims: []carrier.Claim{{ID: id, Kind: "definition", Text: "Recorded content."}}}
	if written := public(t, s, Request{Operation: "remember", RequestID: "alpha-r1-cache-spec", Carrier: encode(t, record, nil)}); written.Kind != "written" {
		t.Fatal("fixture spec not written", written.Kind)
	}
	restore := alphaR1BlockDisclosure(t, s)
	alias := "spec:cache-failure#" + id
	failure := public(t, s, Request{Operation: "recall", Ref: alias, View: "detail", Part: "result"})
	if failure.Kind != "delivery_unavailable" || !failure.IsError {
		t.Fatalf("long selector did not refuse failed cache publication: %+v", failure)
	}
	alphaR1NoRoutes(t, failure)
	summary := public(t, s, Request{Operation: "recall", Ref: alias})
	if summary.Kind != "found" || !strings.Contains(summary.Delivery.NoNext, "Expansion unavailable") {
		t.Fatalf("read-only summary hid unavailable expansion: %+v", summary)
	}
	alphaR1NoRoutes(t, summary)
	request := Request{Operation: "remember", RequestID: "alpha-r1-cache-receipt", Carrier: "---\nkind: note\ntitle: Committed under cache failure\nabout: domain:Delivery\n---\nOne effect only.\n"}
	committed := public(t, s, request)
	if committed.Kind != "written" || committed.IsError || !strings.Contains(committed.Delivery.NoNext, "retry effects only with the same request_id") {
		t.Fatalf("committed outcome disappeared with unavailable expansion: %+v", committed)
	}
	alphaR1NoRoutes(t, committed)
	first := object(committed.Data)
	if first["transaction_id"] == nil || first["generation"] == nil {
		t.Fatalf("committed receipt identity missing: %+v", first)
	}
	replayed := public(t, s, request)
	if replayed.Kind != "replayed" || replayed.IsError || !strings.Contains(replayed.Delivery.NoNext, "same request_id") {
		t.Fatalf("same request ID did not replay the effect: %+v", replayed)
	}
	alphaR1NoRoutes(t, replayed)
	restore()
	positive := public(t, s, Request{Operation: "recall", Ref: alias, View: "detail", Part: "result"})
	if positive.Kind != "found" || positive.Delivery.Catalog == nil || !strings.HasPrefix(positive.Delivery.Catalog.Ref, "result:") {
		t.Fatalf("writable cache did not publish usable routes: %+v", positive)
	}
}

func TestAlphaR1LaterPagePublicationFailurePreservesCursor(t *testing.T) {
	s := service(t)
	id := strings.Repeat("c", 400)
	key := "b" + strings.Repeat("\x01", 100)
	values := map[string]any{"a": strings.Repeat("<", 100), key: strings.Repeat("x", 9000), "z": strings.Repeat("y", 9000)}
	record := carrier.Record{Kind: "spec", Title: "Later page cache failure", About: "domain:Delivery", Slug: "later-cache", ReceivingUse: "Exercise exact fallback", Claims: []carrier.Claim{{ID: id, Kind: "definition", Text: "Fixture claim.", Extra: carrier.Extra{"x-wide": values}}}}
	if written := public(t, s, Request{Operation: "remember", RequestID: "alpha-r1-later-spec", Carrier: encode(t, record, nil)}); written.Kind != "written" {
		t.Fatal("fixture spec not written", written.Kind)
	}
	summary := public(t, s, Request{Operation: "recall", Ref: "spec:later-cache#" + id})
	claim := deliveredPart(t, s, summary, "claim")
	wide := alphaR1Member(t, s, claim, "x-wide")
	first := public(t, s, nextApp(wide))
	if first.Kind != "found" || first.Delivery.Next == nil || first.Delivery.Lifetime != "persisted_snapshot" {
		t.Fatalf("first page did not leave a persisted cursor: %+v", first)
	}
	later := *first.Delivery.Next
	restore := alphaR1BlockDisclosure(t, s)
	failure := public(t, s, nextApp(later))
	if failure.Kind != "delivery_unavailable" || !failure.IsError {
		t.Fatalf("later-page fallback did not refuse failed publication: %+v", failure)
	}
	alphaR1NoRoutes(t, failure)
	invalid := later
	invalid.Limit++
	if rejected := public(t, s, nextApp(invalid)); rejected.Kind != "cursor_parameters_changed" {
		t.Fatal("invalid cursor lost its original refusal", rejected.Kind)
	}
	restore()
	positive := public(t, s, nextApp(later))
	if positive.Kind != "found" || positive.Delivery.Offset != first.Delivery.Offset+len(er01r3Members(t, first)) || positive.Delivery.Catalog == nil || !strings.HasPrefix(positive.Delivery.Catalog.Ref, "result:") {
		t.Fatalf("writable cache did not preserve the later offset and routes: %+v", positive)
	}
}
