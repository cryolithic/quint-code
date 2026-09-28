package delivery

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

// MCPResult is both the measured envelope and the actual adapter output. Text
// clients and structured clients receive the same JSON value, including errors.
func MCPResult(r Response) map[string]any {
	raw, _ := json.Marshal(r)
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": string(raw)}}, "structuredContent": r, "isError": r.IsError}
}
func Size(r Response) int { raw, _ := json.Marshal(MCPResult(r)); return len(raw) }
func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
func Compact(v any, depth, width int) any {
	switch x := v.(type) {
	case string:
		if len(x) <= width {
			return x
		}
		return map[string]any{"excerpt": short(x, width), "complete": false, "total_bytes": len(x)}
	case []any:
		if depth <= 0 {
			return map[string]any{"items": len(x), "complete": len(x) == 0}
		}
		a := []any{}
		for _, child := range x[:min(3, len(x))] {
			a = append(a, Compact(child, depth-1, width))
		}
		if len(x) > 3 {
			return map[string]any{"first_items": a, "total_items": len(x), "complete": false}
		}
		return a
	case map[string]any:
		if depth <= 0 {
			return map[string]any{"members": len(x), "complete": len(x) == 0}
		}
		keys := keys(x)
		m := map[string]any{}
		for _, k := range keys[:min(16, len(keys))] {
			m[short(k, 80)] = Compact(x[k], depth-1, width)
		}
		if len(keys) > 16 {
			m["_delivery_omitted_members"] = len(keys) - 16
		}
		return m
	default:
		return v
	}
}
func keys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
func readRequest(d Document, p Part, view string) Request {
	q := Request{Format: Format, Operation: "read", Ref: d.Ref, View: view, Part: p.Name, Limit: d.PageLimit, ExpectedDigest: carrier.Digest(p.Raw)}
	if p.Mutable {
		q.ExpectedGeneration = d.Basis["memory_generation"]
	}
	return q
}
func descriptor(d Document, p Part) Descriptor {
	label := p.Label
	if label == "" {
		label = p.Name
	}
	return Descriptor{Name: p.Name, Label: short(label, 100), LabelComplete: len(label) <= 100, Media: p.Media, Bytes: len(p.Raw), Digest: carrier.Digest(p.Raw), Request: readRequest(d, p, "detail")}
}
func basisOmissions(full map[string]string, shown map[string]string) (int, int) {
	truncated := 0
	for key, value := range full {
		if _, included := shown[short(key, 64)]; included && (len(key) > 64 || len(value) > 180) {
			truncated++
		}
	}
	return len(full) - len(shown), truncated
}

// trimMetadata removes one displayed field. Exact basis and result parts remain
// addressable through their returned requests; outcome and read routes stay put.
func trimMetadata(d Document, r *Response) bool {
	if len(r.Diagnostics) > 0 {
		r.Diagnostics = r.Diagnostics[:len(r.Diagnostics)-1]
		r.Delivery.Omissions.Diagnostics = len(d.Diagnostics) - len(r.Diagnostics)
		return true
	}
	if len(r.Limits) > 0 {
		r.Limits = r.Limits[:len(r.Limits)-1]
		r.Delivery.Omissions.Limits = len(d.Limits) - len(r.Limits)
		return true
	}
	ks := keysString(r.Basis)
	if len(ks) > 0 {
		remove := ks[len(ks)-1]
		for i := len(ks) - 1; i >= 0; i-- {
			if ks[i] != "memory_generation" {
				remove = ks[i]
				break
			}
		}
		delete(r.Basis, remove)
		r.Delivery.Omissions.Basis, r.Delivery.Omissions.BasisTruncated = basisOmissions(d.Basis, r.Basis)
		return true
	}
	if r.Delivery.Omissions.Note != "" {
		r.Delivery.Omissions.Note = ""
		return true
	}
	return false
}
func fitMetadata(d Document, r *Response, budget int) bool {
	for Size(*r) > budget {
		if !trimMetadata(d, r) {
			return false
		}
	}
	return true
}
func initial(d Document, q Request, p Part) Response {
	r := Response{Format: Format, Operation: short(d.Operation, 64), Kind: short(d.Kind, 64), IsError: d.IsError, Data: nil, Coverage: short(d.Coverage, 48), Basis: map[string]string{}, Diagnostics: []carrier.Diagnostic{}, Limits: []string{}}
	// Exact metadata remains available as named parts; these are visibly excerpts.
	if generation, ok := d.Basis["memory_generation"]; ok {
		r.Basis["memory_generation"] = short(generation, 180)
	}
	for _, k := range keysString(d.Basis) {
		if len(r.Basis) >= 6 {
			break
		}
		if k == "memory_generation" {
			continue
		}
		r.Basis[short(k, 64)] = short(d.Basis[k], 180)
	}
	for _, x := range d.Diagnostics[:min(2, len(d.Diagnostics))] {
		r.Diagnostics = append(r.Diagnostics, carrier.Diagnostic{Code: short(x.Code, 64), Path: short(x.Path, 80), Message: short(x.Message, 160), Severity: short(x.Severity, 16)})
	}
	for _, s := range d.Limits[:min(2, len(d.Limits))] {
		r.Limits = append(r.Limits, short(s, 180))
	}
	omitted, truncated := basisOmissions(d.Basis, r.Basis)
	r.Delivery = State{ReadTool: "haft_read", View: q.View, Part: p.Name, Lifetime: d.Lifetime, Digest: carrier.Digest(p.Raw), TotalBytes: len(p.Raw), Budget: Budget, Omissions: Omission{Parts: len(d.Parts), Diagnostics: len(d.Diagnostics) - len(r.Diagnostics), Limits: len(d.Limits) - len(r.Limits), Basis: omitted, BasisTruncated: truncated, Note: "Summary/metadata may be excerpts; exact parts are separately readable when a result ref is available. Delivery is not domain coverage."}}
	if d.Ref != "" && d.Unavailable == "" {
		catalog, _ := d.part("parts")
		c := readRequest(d, catalog, "detail")
		r.Delivery.Catalog = &c
		if basis, err := d.part("basis"); err == nil {
			request := readRequest(d, basis, "detail")
			r.Delivery.BasisRequest = &request
		}
	} else {
		r.Delivery.NoNext = short(d.Unavailable, 180)
	}
	// Escape-heavy diagnostic text can expand several times in the duplicated
	// envelope. Reserve space for at least one useful member/continuation.
	fitMetadata(d, &r, Budget/2)
	return r
}
func keysString(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
func Error(kind, message string) Response {
	note := "No successful read"
	if len(message) > 300 {
		note = fmt.Sprintf("Diagnostic excerpt; original has %d bytes and digest %s. Invalid input has no continuation", len(message), carrier.Digest([]byte(message)))
	}
	return Response{Format: Format, Operation: "read", Kind: short(kind, 64), IsError: true, Data: nil, Diagnostics: []carrier.Diagnostic{{Code: short(kind, 64), Message: short(message, 300), Severity: "error"}}, Basis: map[string]string{}, Coverage: "unavailable", Limits: []string{}, Delivery: State{View: "summary", Lifetime: "none", Complete: len(message) <= 300, Budget: Budget, NoNext: "Correct the request or repeat the original operation; no replacement bytes were supplied", Omissions: Omission{Note: note}}}
}
func cursor(q Request, offset int) string {
	q.Cursor = ""
	raw, _ := json.Marshal(q)
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(offset))
	sum := sha256.Sum256(append(raw, b...))
	b = append(b, sum[:]...)
	return "d1." + base64.RawURLEncoding.EncodeToString(b)
}
func position(q Request) (int, error) {
	if q.Cursor == "" {
		return 0, nil
	}
	if len(q.Cursor) < 4 || q.Cursor[:3] != "d1." {
		return 0, fmt.Errorf("invalid_cursor")
	}
	b, err := base64.RawURLEncoding.DecodeString(q.Cursor[3:])
	if err != nil || len(b) != 40 {
		return 0, fmt.Errorf("invalid_cursor")
	}
	n := binary.BigEndian.Uint64(b[:8])
	if n > uint64(^uint(0)>>1) || cursor(q, int(n)) != q.Cursor {
		return 0, fmt.Errorf("cursor_parameters_changed")
	}
	return int(n), nil
}
func continuation(d Document, p Part, view string, offset int) *Request {
	q := readRequest(d, p, view)
	q.Cursor = cursor(q, offset)
	return &q
}

// RebindCursor keeps a verified member or byte offset when the application
// replaces an exact persisted selector with a disposable result selector.
// The old cursor is checked against every original request parameter first.
func RebindCursor(q Request, ref string) (Request, error) {
	offset, err := position(q)
	if err != nil {
		return Request{}, err
	}
	withRef := q
	withRef.Ref = ref
	if q.Cursor != "" {
		withRef.Cursor = cursor(withRef, offset)
	}
	return withRef, nil
}

type child struct {
	key  string
	part Part
}

func children(p Part) []child {
	var out []child
	add := func(k string, v any) {
		name := p.Name + "/" + strconv.Itoa(len(out))
		next := JSON(name, v, p.Mutable)
		if s, ok := v.(string); ok {
			next = Text(name, []byte(s))
			next.Mutable = p.Mutable
		}
		next.Label = k
		out = append(out, child{k, next})
	}
	switch v := Value(p.Raw).(type) {
	case map[string]any:
		for _, k := range keys(v) {
			add(k, v[k])
		}
	case []any:
		for i, x := range v {
			add(strconv.Itoa(i), x)
		}
	}
	return out
}

func Present(d Document, q Request) Response {
	if q.View == "" {
		q.View = "summary"
	}
	if q.Part == "" {
		q.Part = "summary"
	}
	if q.View != "summary" && q.View != "detail" && q.View != "bytes" {
		return Error("invalid_view", "Use summary, detail or bytes")
	}
	p, err := d.part(q.Part)
	if err != nil {
		return Error(err.Error(), "Select a returned part request")
	}
	if q.ExpectedDigest != "" && q.ExpectedDigest != carrier.Digest(p.Raw) {
		return Error("stale", "The selected member changed; repeat the original query")
	}
	if q.ExpectedGeneration != "" && q.ExpectedGeneration != d.Basis["memory_generation"] {
		return Error("stale", "Memory generation changed; repeat the original query")
	}
	start, err := position(q)
	if err != nil {
		return Error(err.Error(), "Use the unchanged next_request; do not edit cursor parameters")
	}
	if d.Ref == "" && d.Unavailable == "" {
		d.Unavailable = "Expansion unavailable: no read selector was published"
	}
	if d.Ref == "" && (q.Part != "summary" || q.View != "summary") {
		return Error("delivery_unavailable", d.Unavailable)
	}
	r := initial(d, q, p)
	if q.Part == "summary" && q.View == "summary" {
		if start != 0 {
			return Error("invalid_cursor", "Summary does not have byte offsets")
		}
		for depth := 3; depth >= 0; depth-- {
			r.Data = Compact(Value(p.Raw), depth, 180)
			r.Delivery.Complete = depth == 3 && sameJSON(r.Data, Value(p.Raw))
			r.Delivery.Encoding = "summary"
			r.Delivery.ReturnedBytes = 0
			r.Delivery.Available = nil
			if d.Ref != "" && d.Unavailable == "" {
				for _, part := range d.Parts[:min(4, len(d.Parts))] {
					r.Delivery.Available = append(r.Delivery.Available, descriptor(d, part))
				}
			}
			for Size(r) > Budget && len(r.Delivery.Available) > 0 {
				r.Delivery.Available = r.Delivery.Available[:len(r.Delivery.Available)-1]
			}
			if Size(r) <= Budget {
				return r
			}
		}
		return Error("delivery_unavailable", "Metadata exceeds the delivery budget")
	}
	if q.Part == "parts" && q.View != "bytes" {
		return directory(d, q, p, r, start)
	}
	if q.View == "bytes" || p.Media != "json" {
		return chunks(d, q, p, r, start)
	}
	fullAllowed := true
	if values, ok := Value(p.Raw).([]any); ok && d.PageLimit > 0 && len(values) > d.PageLimit {
		fullAllowed = false
	}
	if start == 0 && fullAllowed {
		r.Data = json.RawMessage(p.Raw)
		r.Delivery.Complete = true
		r.Delivery.Encoding = "json"
		r.Delivery.ReturnedBytes = len(p.Raw)
		r.Delivery.NoNext = "Selected member delivered in full"
		if Size(r) <= Budget {
			return r
		}
	}
	members := children(p)
	if len(members) == 0 {
		return chunks(d, q, p, r, start)
	}
	if start >= len(members) {
		return Error("invalid_cursor", "Member offset is outside the selected structure")
	}
	type item struct {
		Key         string     `json:"key"`
		KeyComplete bool       `json:"key_complete"`
		Value       any        `json:"value,omitempty"`
		Complete    bool       `json:"complete"`
		Read        Descriptor `json:"read"`
		KeyRequest  *Request   `json:"key_request,omitempty"`
	}
	items := []item{}
	r.Delivery.Encoding = "json_members"
	r.Delivery.Offset = start
	r.Delivery.TotalItems = len(members)
	r.Delivery.ReturnedBytes = 0
	r.Delivery.Complete = false
	for i := start; i < len(members) && (d.PageLimit <= 0 || i-start < d.PageLimit); i++ {
		c := members[i]
		x := item{Key: short(c.key, 100), KeyComplete: len(c.key) <= 100, Read: descriptor(d, c.part)}
		value := Value(c.part.Raw)
		if c.part.Media == "text" {
			value = string(c.part.Raw)
		}
		x.Complete = len(c.part.Raw) <= 300
		if x.Complete {
			x.Value = value
		} else {
			x.Value = Compact(value, 1, 100)
		}
		if !x.KeyComplete {
			k := Text(c.part.Name+"/key", []byte(c.key))
			k.Mutable = p.Mutable
			rq := readRequest(d, k, "detail")
			x.KeyRequest = &rq
		}
		candidate := append(append([]item{}, items...), x)
		r.Data = map[string]any{"members": candidate}
		r.Delivery.Next = nil
		r.Delivery.NoNext = "Read incomplete members using their requests, or use view=bytes to reconstruct the entire structure"
		if i+1 < len(members) {
			r.Delivery.Next = continuation(d, p, q.View, i+1)
			r.Delivery.NoNext = ""
		}
		if Size(r) > Budget {
			if len(items) > 0 {
				break
			}
			update := func() {
				candidate[0] = x
				r.Data = map[string]any{"members": candidate}
			}
			// Value and label are optional displays; the child descriptor and
			// exact read request are not. Keep a small complete value if it fits.
			if !x.Complete {
				x.Value = nil
				update()
			}
			if Size(r) > Budget {
				x.Read.Label = ""
				x.Read.LabelComplete = false
				update()
			}
			if Size(r) > Budget {
				fitMetadata(d, &r, Budget)
				if Size(r) > Budget && x.Complete {
					x.Value = nil
					x.Complete = false
					update()
				}
				if Size(r) > Budget {
					// A full short key without its optional value is a smaller
					// executable representation. Only add a key route if this form
					// still cannot fit and the displayed key must be shortened.
					if x.KeyRequest == nil {
						k := Text(c.part.Name+"/key", []byte(c.key))
						k.Mutable = p.Mutable
						rq := readRequest(d, k, "detail")
						x.KeyRequest = &rq
					}
					x.Key = ""
					x.KeyComplete = false
					update()
					if Size(r) > Budget {
						return Error("delivery_unavailable", "Cannot fit an exact member route")
					}
					// Grow a UTF-8 prefix only after the exact key route fits.
					low, high := 0, min(len(c.key), 100)
					for low < high {
						mid := (low + high + 1) / 2
						x.Key = short(c.key, mid)
						update()
						if Size(r) <= Budget {
							low = mid
						} else {
							high = mid - 1
						}
					}
					x.Key = short(c.key, low)
					if low == len(c.key) {
						x.KeyComplete = true
						x.KeyRequest = nil
					}
					update()
				}
			}
		}
		items = candidate
	}
	if len(items) == 0 {
		return Error("delivery_unavailable", "Cannot fit a member descriptor")
	}
	r.Data = map[string]any{"members": items}
	end := start + len(items)
	r.Delivery.Next = nil
	r.Delivery.NoNext = "Read incomplete members using their requests, or use view=bytes to reconstruct the entire structure"
	if end < len(members) {
		r.Delivery.Next = continuation(d, p, q.View, end)
		r.Delivery.NoNext = ""
	}
	return r
}
func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func directory(d Document, q Request, p Part, r Response, start int) Response {
	if start > len(d.Parts) {
		return Error("invalid_cursor", "Part offset outside directory")
	}
	list := []Descriptor{}
	r.Delivery.Encoding = "part_directory"
	r.Delivery.Offset = start
	r.Delivery.TotalItems = len(d.Parts)
	for i := start; i < len(d.Parts) && (d.PageLimit <= 0 || i-start < d.PageLimit); i++ {
		candidate := append(append([]Descriptor{}, list...), descriptor(d, d.Parts[i]))
		r.Data = map[string]any{"parts": candidate}
		r.Delivery.Next = nil
		r.Delivery.NoNext = "All parts listed"
		r.Delivery.Complete = i+1 == len(d.Parts)
		if i+1 < len(d.Parts) {
			r.Delivery.Next = continuation(d, p, q.View, i+1)
			r.Delivery.NoNext = ""
		}
		if Size(r) > Budget {
			if len(list) > 0 {
				break
			}
			candidate[0].Label = ""
			candidate[0].LabelComplete = false
			r.Data = map[string]any{"parts": candidate}
			fitMetadata(d, &r, Budget)
			if Size(r) > Budget {
				return Error("delivery_unavailable", "Cannot fit an exact part route")
			}
		}
		list = candidate
	}
	if len(list) == 0 && start < len(d.Parts) {
		return Error("delivery_unavailable", "Cannot fit a part descriptor")
	}
	r.Data = map[string]any{"parts": list}
	end := start + len(list)
	r.Delivery.Complete = end == len(d.Parts)
	r.Delivery.Next = nil
	r.Delivery.NoNext = "All parts listed"
	if end < len(d.Parts) {
		r.Delivery.Next = continuation(d, p, q.View, end)
		r.Delivery.NoNext = ""
	}
	return r
}
func chunks(d Document, q Request, p Part, r Response, start int) Response {
	textMode := q.View != "bytes" && p.Media == "text" && utf8.Valid(p.Raw)
	if start > len(p.Raw) || (textMode && start < len(p.Raw) && !utf8.RuneStart(p.Raw[start])) {
		return Error("invalid_cursor", "Offset outside member or not a UTF-8 boundary")
	}
	build := func(n int) Response {
		end := start + n
		if textMode {
			for end > start && end < len(p.Raw) && !utf8.RuneStart(p.Raw[end]) {
				end--
			}
		}
		x := r
		x.Delivery.Offset = start
		x.Delivery.ReturnedBytes = end - start
		x.Delivery.Complete = end == len(p.Raw)
		x.Delivery.Next = nil
		x.Delivery.NoNext = "Member delivered through final byte; verify the complete member digest"
		x.Delivery.Encoding = "base64"
		x.Data = map[string]any{"bytes_base64": p.Raw[start:end]}
		if textMode {
			x.Delivery.Encoding = "utf-8"
			x.Data = map[string]any{"text": string(p.Raw[start:end])}
		}
		if end < len(p.Raw) {
			x.Delivery.Next = continuation(d, p, q.View, end)
			x.Delivery.NoNext = ""
		}
		return x
	}
	base := build(0)
	if !fitMetadata(d, &base, Budget) {
		return Error("delivery_unavailable", "Cannot fit an exact byte route")
	}
	r = base
	low, high := 0, min(len(p.Raw)-start, Budget)
	for low < high {
		mid := (low + high + 1) / 2
		if Size(build(mid)) <= Budget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	result := build(low)
	if Size(result) > Budget || (result.Delivery.ReturnedBytes == 0 && start < len(p.Raw)) {
		return Error("delivery_unavailable", "Cannot fit a byte chunk")
	}
	return result
}
