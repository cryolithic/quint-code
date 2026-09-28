package transport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/app"
	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/delivery"
)

func TestER01R3LongPersistedRefRoutesAcrossStdioMCP(t *testing.T) {
	for _, profile := range []struct{ name, write, read string }{
		{ProfileDefault, "haft_write", "haft_read"},
		{ProfileLegacy, "haft", "haft"},
	} {
		for _, variant := range []struct{ name, key, value string }{
			{"short", "a", strings.Repeat("<", 100)},
			{"long", strings.Repeat("a", 101), strings.Repeat("x", 9000)},
		} {
			t.Run(profile.name+"/"+variant.name, func(t *testing.T) {
				client := t01aStartClient(t, app.Service{Root: t.TempDir()}, profile.name)
				client.list(t)
				claimID := strings.Repeat("c", 335)
				fields := map[string]any{
					"kind": "spec", "title": "Long-ref MCP fixture", "about": "domain:Delivery", "slug": "long-ref", "receiving_use": "Exact public read routes",
					"claims": []any{map[string]any{"id": claimID, "kind": "definition", "text": "Fixture claim.", "x-wide": map[string]any{variant.key: variant.value, "z": strings.Repeat("y", 9000)}}},
				}
				header, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				carrierText := "---\n" + string(header) + "\n---\nOwned fixture only.\n"
				written := client.mustCall(t, profile.write, app.Request{Format: delivery.Format, Operation: "remember", RequestID: "long-ref-fixture", Carrier: carrierText})
				if written.Kind != "written" || written.IsError {
					t.Fatal("fixture write failed", written.Kind)
				}
				summary := client.mustCall(t, profile.read, app.Request{Format: delivery.Format, Operation: "recall", Ref: "spec:long-ref#" + claimID})
				if summary.Kind != "found" || summary.IsError {
					t.Fatal("fixture recall failed", summary.Kind)
				}
				claim, ok := t01aParts(t, client, summary)["claim"]
				if !ok || len(claim.Request.Ref) != 430 {
					t.Fatal("exact 430-byte claim route missing", len(claim.Request.Ref))
				}
				var wide delivery.Request
				for request := &claim.Request; request != nil; {
					page := t01aRead(t, client, profile.read, *request)
					if page.Kind != "found" || page.Delivery.Encoding != "json_members" {
						t.Fatal("claim member route failed", page.Kind)
					}
					for _, member := range er01r3MCPMembers(t, page) {
						if member.Key == "x-wide" {
							wide = member.Read.Request
						}
					}
					request = page.Delivery.Next
				}
				if wide.Part == "" {
					t.Fatal("nested extension route missing")
				}
				seen := map[string]bool{}
				for request := &wide; request != nil; {
					page := t01aRead(t, client, profile.read, *request)
					if page.Kind != "found" || page.Delivery.Encoding != "json_members" {
						t.Fatal("extension member route failed", page.Kind, page.Delivery.Encoding)
					}
					for _, member := range er01r3MCPMembers(t, page) {
						key := member.Key
						if !member.KeyComplete {
							if member.KeyRequest == nil {
								t.Fatal("shortened key has no exact request")
							}
							keyPart := delivery.Descriptor{Name: "key", Bytes: len(variant.key), Digest: member.KeyRequest.ExpectedDigest, Request: *member.KeyRequest}
							key = string(er01r3MCPText(t, client, profile.read, keyPart))
						}
						if seen[key] || key != variant.key && key != "z" {
							t.Fatal("key lost or duplicated", key)
						}
						want := strings.Repeat("y", 9000)
						if key == variant.key {
							want = variant.value
						}
						if !bytes.Equal(er01r3MCPText(t, client, profile.read, member.Read), []byte(want)) || member.Read.Digest != carrier.Digest([]byte(want)) {
							t.Fatal("exact child changed", key)
						}
						if key == "a" && member.KeyRequest != nil {
							t.Fatal("short key has redundant request")
						}
						seen[key] = true
					}
					request = page.Delivery.Next
				}
				if len(seen) != 2 {
					t.Fatal("nested members not fully recovered", seen)
				}
			})
		}
	}
}

type er01r3MCPMember struct {
	Key         string              `json:"key"`
	KeyComplete bool                `json:"key_complete"`
	Read        delivery.Descriptor `json:"read"`
	KeyRequest  *delivery.Request   `json:"key_request"`
}

func er01r3MCPMembers(t *testing.T, page delivery.Response) []er01r3MCPMember {
	t.Helper()
	raw, err := json.Marshal(page.Data)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Members []er01r3MCPMember `json:"members"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body.Members
}

func er01r3MCPText(t *testing.T, client *t01aProtocolClient, tool string, part delivery.Descriptor) []byte {
	t.Helper()
	got, _ := t01aTextPart(t, client, tool, part)
	if carrier.Digest(got) != part.Digest {
		t.Fatal("text part digest changed", part.Name)
	}
	return got
}
