package app

import (
	"context"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func TestT01BRReauthorReceiptKeepsPublishedIdentityAfterLaterContent(t *testing.T) {
	s := service(t)
	_, predecessor := seedT01BActiveSpec(t, s)
	preview := public(t, s, Request{Operation: "change", Action: "reauthor_preview", Ref: predecessor})
	if preview.Kind != "ready" {
		t.Fatalf("preview: %+v", preview)
	}
	apply := Request{Operation: "change", Action: "reauthor_apply", Ref: predecessor, RequestID: "reauthor-receipt",
		ExpectedGeneration: preview.Basis["memory_generation"], PreviewDigest: preview.Basis["preview_digest"]}
	written := public(t, s, apply)
	if written.Kind != "written" || written.IsError {
		t.Fatalf("apply: %+v", written)
	}
	data := object(written.Data)
	ref := str(data["published_successor_ref"])
	parsed, err := carrier.ParseRef(ref)
	if err != nil || !parsed.Pinned() || parsed.ClaimID != "" {
		t.Fatalf("apply omitted exact published ref: %+v", data)
	}
	request := object(data["exact_read_request"])
	if request["format"] != delivery.Format || request["operation"] != "recall" || request["ref"] != ref {
		t.Fatalf("apply omitted executable exact-read route: %+v", request)
	}
	if object(data["published_content"])["live_currentness"] != "not_reassessed" || object(data["decision_authority"])["assessment"] != "not_assessed" {
		t.Fatalf("content was presented as current authority: %+v", data)
	}
	read := public(t, s, Request{Operation: "recall", Ref: ref})
	if read.Kind != "found" || read.IsError {
		t.Fatalf("published exact ref is not readable: %+v", read)
	}

	var published carrier.Document
	for _, document := range readView(t, s).Documents {
		if document.Record.ID == parsed.RecordID {
			published = document
		}
	}
	if published.Record.ID == "" {
		t.Fatal("published identity absent from canonical view")
	}
	later := published.Record
	later.ID = ""
	later.CreatedAt = ""
	later.UpdatedAt = ""
	later.WriteReceipt = nil
	later.Supersedes = []string{ref}
	later.SupersedeReason = "Later authored content revision"
	later.Claims[0].Text = "Later content supersedes the reauthored edition"
	run(t, s, Request{Operation: "remember", RequestID: "later-spec-content", Carrier: encode(t, later, published.Body), ExpectedHeads: []string{ref}}, "written")
	if heads := readView(t, s).Projection.Heads(parsed.RecordID); len(heads) != 1 || heads[0] == ref {
		t.Fatalf("later content did not supersede original output: %+v", heads)
	}

	fresh := Service{Root: s.Root, Now: s.Now}
	replayed := public(t, fresh, apply)
	if replayed.Kind != "replayed" || replayed.IsError {
		t.Fatalf("replay: %+v", replayed)
	}
	replayData := object(replayed.Data)
	if replayData["published_successor_ref"] != ref || object(replayData["exact_read_request"])["ref"] != ref {
		t.Fatalf("replay selected a later head or lost output identity: %+v", replayData)
	}
	if !strings.Contains(str(object(replayData["decision_authority"])["meaning"]), "does not accept") {
		t.Fatalf("replay lost authority distinction: %+v", replayData)
	}
	read = fresh.Call(context.Background(), Request{Format: delivery.Format, Operation: "recall", Ref: ref})
	if read.Kind != "found" || read.IsError {
		t.Fatalf("replayed exact route failed: %+v", read)
	}
}
