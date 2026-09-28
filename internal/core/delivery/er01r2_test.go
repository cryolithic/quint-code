package delivery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

func TestMemberEnvelopeFitsEscapedKeysAndHeaderBoundary(t *testing.T) {
	controlKey := strings.Repeat("\x01", 101)
	for _, tc := range []struct {
		name       string
		basisWidth int
		largeKey   string
		otherKey   string
		otherValue string
	}{
		{"reviewer-header-omitted", 180, controlKey, "z", strings.Repeat("y", 9000)},
		{"controller-header-retained", 100, controlKey, "z", strings.Repeat("y", 9000)},
		{"large-first-small", 100, controlKey, "z", "ok"},
		{"large-after-small", 100, "b" + controlKey, "a", "ok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]any{tc.largeKey: strings.Repeat("x", 9000), tc.otherKey: tc.otherValue}
			d := exampleDocument(JSON("large", values, true))
			for i := 0; i < 5; i++ {
				d.Basis[fmt.Sprintf("k%d", i)] = strings.Repeat("<", tc.basisWidth)
			}
			q := readRequest(d, d.Parts[0], "detail")
			header := initial(d, q, d.Parts[0])
			if tc.basisWidth == 180 && len(header.Basis) != 1 {
				t.Fatal("reviewer control no longer omits its large basis values")
			}
			if tc.basisWidth == 100 && len(header.Basis) <= 1 {
				t.Fatal("controller control no longer retains a larger header")
			}
			shown := []string{}
			keys := []string{tc.largeKey, tc.otherKey}
			sort.Strings(keys)
			for pageNumber := 0; pageNumber < len(keys); pageNumber++ {
				page := Present(d, q)
				wireCheck(t, page)
				if page.Kind != d.Kind || page.IsError != d.IsError || page.Delivery.Encoding != "json_members" || page.Delivery.Offset != len(shown) || page.Delivery.TotalItems != len(keys) {
					t.Fatalf("member page changed outcome or offset: %+v", page)
				}
				if page.Delivery.Omissions.Basis+len(page.Basis) != len(d.Basis) {
					t.Fatal("basis omission count changed", page.Delivery.Omissions)
				}
				var body struct {
					Members []memberPageItem `json:"members"`
				}
				raw, err := json.Marshal(page.Data)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(raw, &body); err != nil || len(body.Members) == 0 {
					t.Fatalf("member page unreadable: %v", err)
				}
				for _, member := range body.Members {
					key := keys[len(shown)]
					if !strings.HasPrefix(key, member.Key) {
						t.Fatal("displayed key is not an exact prefix")
					}
					if !member.KeyComplete && (member.KeyRequest == nil || er01rText(t, d, *member.KeyRequest) != key) {
						t.Fatal("incomplete key has no executable exact route")
					}
					if member.KeyComplete && member.Key != key {
						t.Fatal("shortened key declared complete")
					}
					want := []byte(values[key].(string))
					if member.Read.Digest != carrier.Digest(want) || !bytes.Equal(er01rBytes(t, d, member.Read.Request), want) {
						t.Fatal("child route lost exact bytes or digest")
					}
					if key == tc.otherKey && tc.otherValue == "ok" && (!member.Complete || string(member.Value) != `"ok"`) {
						t.Fatal("ordinary small value lost its complete inline form")
					}
					if key == tc.largeKey && member.Complete {
						t.Fatal("large value declared complete")
					}
					shown = append(shown, key)
				}
				if page.Delivery.Next == nil {
					break
				}
				bad := *page.Delivery.Next
				bad.Limit++
				if refused := Present(d, bad); refused.Kind != "cursor_parameters_changed" {
					t.Fatal("edited cursor accepted", refused.Kind)
				}
				bad = *page.Delivery.Next
				bad.ExpectedDigest = "sha256:" + strings.Repeat("0", 64)
				if refused := Present(d, bad); refused.Kind != "stale" {
					t.Fatal("wrong digest accepted", refused.Kind)
				}
				bad = *page.Delivery.Next
				bad.ExpectedGeneration = "sha256:" + strings.Repeat("0", 64)
				if refused := Present(d, bad); refused.Kind != "stale" {
					t.Fatal("wrong generation accepted", refused.Kind)
				}
				q = *page.Delivery.Next
			}
			if !slices.Equal(shown, keys) {
				t.Fatal("member pagination skipped or repeated a key", shown)
			}
		})
	}
}

func TestDirectoryAndBytesFitWithEscapedLabelAndHeader(t *testing.T) {
	raw := []byte(strings.Repeat("exact <&> bytes\x01🌱", 500))
	p := Binary("large", raw)
	p.Label = strings.Repeat("\x01", 101)
	d := exampleDocument(p)
	for i := 0; i < 5; i++ {
		d.Basis[fmt.Sprintf("k%d", i)] = strings.Repeat("<", 100)
	}
	q := readRequest(d, JSON("parts", []string{"large"}, true), "detail")
	directory := Present(d, q)
	wireCheck(t, directory)
	if directory.Kind != d.Kind || directory.Delivery.Encoding != "part_directory" {
		t.Fatal("part directory did not fit")
	}
	var body struct {
		Parts []Descriptor `json:"parts"`
	}
	encoded, err := json.Marshal(directory.Data)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &body); err != nil || len(body.Parts) != 1 {
		t.Fatal("part descriptor missing", err)
	}
	if body.Parts[0].Digest != carrier.Digest(raw) || !bytes.Equal(er01rBytes(t, d, body.Parts[0].Request), raw) {
		t.Fatal("directory route or byte continuation lost exact content")
	}
}
