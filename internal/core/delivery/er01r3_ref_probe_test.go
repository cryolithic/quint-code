package delivery

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func TestER01R3ShortKeyFallbackBeforeExtraRoute(t *testing.T) {
	for _, width := range []int{78, 360, 430} {
		d := exampleDocument(JSON("large", map[string]any{"a": strings.Repeat("<", 100), "z": strings.Repeat("x", 9000)}, true))
		d.Parts = append(d.Parts, JSON("basis", d.Basis, false))
		d.Ref += strings.Repeat("r", width-len(d.Ref))
		page := Present(d, readRequest(d, d.Parts[0], "detail"))
		wireCheck(t, page)
		if page.Kind != d.Kind || page.Delivery.Encoding != "json_members" {
			t.Fatalf("ref=%d: short key route refused: %s", width, page.Kind)
		}
		var body struct {
			Members []memberPageItem `json:"members"`
		}
		raw, err := json.Marshal(page.Data)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &body); err != nil || len(body.Members) == 0 {
			t.Fatalf("ref=%d: no readable member: %v", width, err)
		}
		first := body.Members[0]
		if first.Key != "a" || !first.KeyComplete || first.KeyRequest != nil {
			t.Fatalf("ref=%d: full short key acquired an unnecessary route: %+v", width, first)
		}
		if !first.Complete && len(first.Value) != 0 {
			t.Fatalf("ref=%d: incomplete value still displayed as complete", width)
		}
		if width == 78 {
			var inline string
			if err := json.Unmarshal(first.Value, &inline); err != nil || !first.Complete || inline != strings.Repeat("<", 100) {
				t.Fatal("ordinary small value lost its complete inline form", err)
			}
		}
		if width == 430 && first.Complete {
			t.Fatal("fixture no longer distinguishes the valueless short-key fallback")
		}
		if got := er01rBytes(t, d, first.Read.Request); string(got) != strings.Repeat("<", 100) {
			t.Fatalf("ref=%d: exact child bytes changed", width)
		}
	}
}

func TestER01R3CursorRebindVerifiesOldSelector(t *testing.T) {
	d := exampleDocument(Text("body", []byte(strings.Repeat("A", 12000))))
	old := readRequest(d, d.Parts[0], "bytes")
	first := Present(d, old)
	if first.Delivery.Next == nil {
		t.Fatal("expected a byte continuation")
	}
	before := *first.Delivery.Next
	newRef := "result:sha256:" + strings.Repeat("9", 64)
	rebound, err := RebindCursor(before, newRef)
	if err != nil || rebound.Ref != newRef || rebound.Cursor == before.Cursor {
		t.Fatal("verified offset was not rebound", err)
	}
	d.Ref = newRef
	page := Present(d, rebound)
	wireCheck(t, page)
	if page.Delivery.Offset != first.Delivery.ReturnedBytes || page.Kind != d.Kind {
		t.Fatal("rebinding skipped bytes or changed outcome", page.Delivery.Offset, page.Kind)
	}
	changed := before
	changed.Ref = newRef
	if _, err := RebindCursor(changed, newRef); err == nil {
		t.Fatal("edited original selector accepted")
	}
}

func TestER01R3OmissionCountsDescribeAllReplies(t *testing.T) {
	d := exampleDocument(Text("body", []byte("complete")))
	d.Diagnostics = []carrier.Diagnostic{{Code: "first"}, {Code: "second"}}
	d.Limits = []string{"one", "two"}
	page := Present(d, Request{})
	wireCheck(t, page)
	if page.Delivery.Omissions.Diagnostics != 0 || page.Delivery.Omissions.Limits != 0 {
		t.Fatal("displayed diagnostics or limits counted as omissions", page.Delivery.Omissions)
	}
	d.Diagnostics = append(d.Diagnostics, carrier.Diagnostic{Code: "third"})
	d.Limits = append(d.Limits, "three")
	page = Present(d, Request{})
	wireCheck(t, page)
	if page.Delivery.Omissions.Diagnostics != 1 || page.Delivery.Omissions.Limits != 1 {
		t.Fatal("hidden diagnostics or limits were not counted", page.Delivery.Omissions)
	}
}
