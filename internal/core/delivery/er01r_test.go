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

type memberPageItem struct {
	Key         string          `json:"key"`
	KeyComplete bool            `json:"key_complete"`
	Value       json.RawMessage `json:"value"`
	Complete    bool            `json:"complete"`
	Read        Descriptor      `json:"read"`
	KeyRequest  *Request        `json:"key_request"`
}

func er01rBytes(t *testing.T, d Document, request Request) []byte {
	t.Helper()
	request.View = "bytes"
	var result []byte
	for {
		page := Present(d, request)
		wireCheck(t, page)
		if page.Kind != d.Kind || page.IsError != d.IsError || page.Delivery.Offset != len(result) || page.Delivery.Digest != request.ExpectedDigest {
			t.Fatalf("byte route lost its exact basis: %+v", page)
		}
		chunk := page.Data.(map[string]any)["bytes_base64"].([]byte)
		if len(chunk) != page.Delivery.ReturnedBytes {
			t.Fatal("byte route reported a different chunk length")
		}
		result = append(result, chunk...)
		if page.Delivery.Next == nil {
			if !page.Delivery.Complete || len(result) != page.Delivery.TotalBytes || carrier.Digest(result) != request.ExpectedDigest {
				t.Fatal("byte route ended without the complete selected member")
			}
			return result
		}
		request = *page.Delivery.Next
	}
}

func er01rText(t *testing.T, d Document, request Request) string {
	t.Helper()
	var result strings.Builder
	for {
		page := Present(d, request)
		wireCheck(t, page)
		if page.Kind != d.Kind || page.IsError != d.IsError || page.Delivery.Offset != result.Len() {
			t.Fatalf("text route lost its offset: %+v", page)
		}
		result.WriteString(page.Data.(map[string]any)["text"].(string))
		if page.Delivery.Next == nil {
			if !page.Delivery.Complete || carrier.Digest([]byte(result.String())) != request.ExpectedDigest {
				t.Fatal("text route ended without its exact key")
			}
			return result.String()
		}
		request = *page.Delivery.Next
	}
}

func TestMemberPreviewNeverBlocksExactChildRoute(t *testing.T) {
	wide := map[string]any{}
	for i := 0; i < 16; i++ {
		wide[fmt.Sprintf("nested-%02d", i)] = strings.Repeat("Иван\\\"\n<&🌱", 120)
	}
	longKey := "zz" + strings.Repeat("🔑\\", 650)
	for _, tc := range []struct {
		name    string
		members map[string]any
	}{
		{"wide-first", map[string]any{"a_large": wide, "b_small": "ok", "c_tail": 42}},
		{"escaped-after-small", map[string]any{"a_small": "ok", "b_large": strings.Repeat("Иван\\\"\n<&🌱", 1600), longKey: "key value"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys := make([]string, 0, len(tc.members))
			for key := range tc.members {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			d := exampleDocument(JSON("large", tc.members, true))
			q := readRequest(d, d.Parts[0], "detail")
			seen := []string{}
			omittedPreview := false
			for pageNumber := 0; pageNumber < len(keys); pageNumber++ {
				page := Present(d, q)
				wireCheck(t, page)
				if page.Kind != d.Kind || page.IsError != d.IsError || page.Delivery.Encoding != "json_members" || page.Delivery.Complete || page.Delivery.Offset != len(seen) || page.Delivery.TotalItems != len(keys) {
					t.Fatalf("member page changed its meaning: %+v", page)
				}
				var body struct {
					Members []memberPageItem `json:"members"`
				}
				raw, _ := json.Marshal(page.Data)
				if err := json.Unmarshal(raw, &body); err != nil || len(body.Members) == 0 {
					t.Fatalf("empty or undecodable member page: %v %s", err, raw)
				}
				for _, member := range body.Members {
					key := keys[len(seen)]
					if member.Key != short(key, 100) || member.KeyComplete != (len(key) <= 100) {
						t.Fatal("member key identity changed", member.Key)
					}
					value := tc.members[key]
					want, isText := value.(string)
					var rawValue []byte
					if isText {
						rawValue = []byte(want)
					} else {
						rawValue, _ = json.Marshal(value)
					}
					if member.Read.Name != fmt.Sprintf("large/%d", len(seen)) || member.Read.Digest != carrier.Digest(rawValue) || member.Read.Request.ExpectedDigest != member.Read.Digest {
						t.Fatal("child route changed its exact identity", member.Read)
					}
					if got := er01rBytes(t, d, member.Read.Request); !bytes.Equal(got, rawValue) {
						t.Fatal("exact child bytes changed")
					}
					if len(rawValue) > 300 && member.Complete {
						t.Fatal("large member preview declared complete")
					}
					if len(rawValue) > 300 && len(member.Value) == 0 {
						omittedPreview = true
					}
					if want == "ok" {
						child := Present(d, member.Read.Request)
						wireCheck(t, child)
						if !child.Delivery.Complete || child.Data.(map[string]any)["text"] != "ok" || !member.Complete {
							t.Fatal("small member lost direct complete read")
						}
					}
					if len(key) > 100 {
						if member.KeyRequest == nil || er01rText(t, d, *member.KeyRequest) != key {
							t.Fatal("long key lost its returned exact route")
						}
					}
					seen = append(seen, key)
				}
				if page.Delivery.Next == nil {
					break
				}
				bad := *page.Delivery.Next
				bad.Limit++
				if refused := Present(d, bad); refused.Kind != "cursor_parameters_changed" {
					t.Fatal("edited cursor parameters accepted", refused.Kind)
				}
				bad = *page.Delivery.Next
				bad.ExpectedDigest = "sha256:" + strings.Repeat("0", 64)
				if refused := Present(d, bad); refused.Kind != "stale" {
					t.Fatal("mismatched member digest accepted", refused.Kind)
				}
				bad = *page.Delivery.Next
				bad.ExpectedGeneration = "sha256:" + strings.Repeat("0", 64)
				if refused := Present(d, bad); refused.Kind != "stale" {
					t.Fatal("mismatched generation accepted", refused.Kind)
				}
				q = *page.Delivery.Next
			}
			if !slices.Equal(seen, keys) {
				t.Fatalf("member order, omission or duplication: got %d want %d", len(seen), len(keys))
			}
			if tc.name == "wide-first" && !omittedPreview {
				t.Fatal("fixture did not exercise an oversized optional preview")
			}
		})
	}
}
