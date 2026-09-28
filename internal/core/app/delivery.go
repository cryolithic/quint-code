package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/delivery"
	"github.com/m0n0x41d/haft/internal/core/source"
)

type capturedResult struct {
	Format   string             `json:"format"`
	Request  Request            `json:"request"`
	Result   Result             `json:"result"`
	Document *delivery.Document `json:"document,omitempty"`
}

// Call is the bounded public API shared by CLI and MCP. Execute remains the
// internal complete domain operation, including raw inputs and outputs.
func (s Service) Call(ctx context.Context, q Request) delivery.Response {
	if q.Format != delivery.Format {
		return delivery.Error("unsupported_format", "Public delivery requires haft.api/2. Carrier/history formats are unchanged; use summary/detail/bytes and returned next_request")
	}
	if q.Offset != 0 {
		return delivery.Error("invalid_offset", "haft.api/2 uses returned cursors instead of caller-selected offsets")
	}
	if q.Limit < 0 || q.Limit > 500 {
		return delivery.Error("invalid_limit", "limit must be between zero and 500")
	}
	if len(q.Part) > 512 || len(q.Cursor) > 256 || len(q.ExpectedDigest) > 80 {
		return delivery.Error("invalid_read_request", "Read controls exceed their bounds")
	}
	if q.View != "" && q.View != "summary" && q.View != "detail" && q.View != "bytes" {
		return delivery.Error("invalid_view", "Use summary, detail or bytes")
	}
	if q.Operation == "read" {
		return s.readDelivery(ctx, q)
	}
	mutation := q.Operation == "remember" || q.Operation == "recover" || q.Operation == "change" && q.Action != "preview" && q.Action != "reauthor_preview" && q.Action != "list" && q.Action != "show" || q.Operation == "check" && q.Action == "capture"
	if mutation && (q.Part != "" || q.ExpectedDigest != "" || q.View == "detail" || q.View == "bytes") {
		return delivery.Error("invalid_read_request", "Mutations return a receipt summary. Select its returned read requests for detail; no effect was performed")
	}
	if q.Cursor != "" {
		return delivery.Error("invalid_cursor", "Continuation requires operation read and the supplied request")
	}
	view, part, digest := q.View, q.Part, q.ExpectedDigest
	q.Format = Format
	q.View = ""
	q.Part = ""
	q.ExpectedDigest = ""
	q.forDelivery = true
	if q.Operation == "check" && q.Action == "capture" {
		return s.captureCall(ctx, q)
	}
	// Receipt identity covers the original logical inputs, including retention,
	// independent of presentation, expansion and disposable cache availability.
	if len(q.Retain) > 0 {
		if q.Operation != "remember" || q.Action != "" {
			return delivery.Error("invalid_retention", "retain is supported only by an ordinary remember")
		}
		raw, _ := json.Marshal(q)
		if prior, found, err := s.memory().Replay(ctx, q.RequestID, carrier.Digest(raw)); err != nil {
			return delivery.Error("unavailable", err.Error())
		} else if found {
			r := published(Result{Format: Format, Operation: q.Operation, Basis: map[string]string{}, Diagnostics: []carrier.Diagnostic{}, Limits: []string{}, Coverage: "publication_only"}, prior)
			return s.deliver(ctx, q, r, view, part, digest)
		}
		q.payload = raw
		for _, keep := range q.Retain {
			d, err := s.loadTransient(ctx, keep.Ref, false, "")
			if err != nil {
				return transientFailure(err)
			}
			p, err := d.Member(keep.Part)
			if err != nil {
				return delivery.Error("invalid_retention", err.Error())
			}
			if incomplete, err := incompleteCaptureRetention(d, p); err != nil {
				return delivery.Error("invalid_retention", err.Error())
			} else if incomplete {
				replacement, _ := json.Marshal([]Retention{{Ref: keep.Ref, Part: "result"}})
				message := fmt.Sprintf("Capture %s is incomplete; no remember was written. Retry the remember request with retain set to %s so the result preserves incompleteness and full observed byte counts and digests", keep.Part, replacement)
				return delivery.Error("invalid_retention", message)
			}
			attachment, _ := json.Marshal(struct {
				Format string `json:"format"`
				Ref    string `json:"result_ref"`
				Part   string `json:"part"`
				Digest string `json:"digest"`
				Media  string `json:"media"`
				Raw    []byte `json:"bytes_base64"`
			}{"haft.retained-part/1", keep.Ref, keep.Part, carrier.Digest(p.Raw), p.Media, p.Raw})
			q.Carrier += "\n\nRetained captured result (data, not independent attestation):\n```json\n" + string(attachment) + "\n```\n"
		}
		q.Retain = nil
	}
	r := s.Execute(ctx, q)
	return s.deliver(ctx, q, r, view, part, digest)
}

func incompleteCaptureRetention(d delivery.Document, p delivery.Part) (bool, error) {
	if p.Name != "stdout" && p.Name != "stderr" {
		return false, nil
	}
	process, err := d.Member("process")
	if err != nil {
		return false, nil
	}
	var facts captureStreamAccounting
	if err := json.Unmarshal(process.Raw, &facts); err != nil {
		return false, fmt.Errorf("capture process accounting is unreadable; retain result instead")
	}
	return incompleteCaptureStream(facts, p.Name, len(p.Raw)), nil
}
func (s Service) deliver(ctx context.Context, q Request, r Result, view, part, digest string) delivery.Response {
	d := makeDelivery(q, r)
	request := delivery.Request{Format: delivery.Format, Operation: "read", Ref: d.Ref, View: view, Part: part, ExpectedDigest: digest}
	return s.presentDelivery(ctx, q, r, d, request)
}

// A persisted selector is retained while its actual reply fits. If the
// presenter cannot fit an exact route, the same captured document is given a
// bounded disposable selector. This also applies to later read pages: merely
// fitting the first page cannot promise that a later member will fit.
func (s Service) presentDelivery(ctx context.Context, origin Request, r Result, d delivery.Document, q delivery.Request) delivery.Response {
	if d.Ref != "" {
		response := delivery.Present(d, q)
		if strings.HasPrefix(d.Ref, "result:") || len(d.Ref) <= 512 && (response.Kind != "delivery_unavailable" || response.Operation != "read") {
			return response
		}
		if response.IsError && response.Operation == "read" && response.Kind != "delivery_unavailable" {
			return response
		}
	}
	// The cache locator is a read selector, not an immutable edition or an
	// authority token. The captured request/result and document retain the
	// original exact identity and bytes; cache expiry remains explicit.
	metadata := r
	metadata.Data = nil
	raw, err := json.Marshal(capturedResult{Format: "haft.transient-result/1", Request: origin, Result: metadata, Document: &d})
	if err == nil {
		var ref string
		ref, err = s.memory().PutTransient(ctx, raw)
		if err == nil {
			q, err = delivery.RebindCursor(q, ref)
			d.Ref = ref
		}
	}
	d.Lifetime = "transient_cache"
	if err != nil {
		d.Ref = ""
		d.Unavailable = "Expansion unavailable: transient cache publication failed; the shown outcome is unchanged. Repeat reads; retry effects only with the same request_id"
	}
	return delivery.Present(d, q)
}
func readError(err error) string {
	for _, code := range []string{"stale", "expired", "corrupt", "invalid_result_ref"} {
		if strings.HasPrefix(err.Error(), code) {
			return code
		}
	}
	return "unavailable"
}

func transientFailure(err error) delivery.Response {
	if readError(err) == "expired" {
		response := delivery.Error("expired", "Result bytes expired; read/retain cannot restore them. Repeat non-executing recall, context, check/prepare or check/observe. For check/capture, retry the same request_id for status; use a new ID only for a deliberate new run after inspecting the original. Inspect other effectful receipts first.")
		response.Delivery.NoNext = "Follow the operation-specific recovery diagnostic; the original result bytes are unavailable"
		return response
	}
	if readError(err) == "corrupt" {
		response := delivery.Error("corrupt", "Result bytes are corrupt; read/retain cannot repair them. Repeat non-executing recall, context, check/prepare or check/observe. For check/capture, retry the same request_id for status; use a new ID only for a deliberate new run after inspecting the original. Inspect other effectful receipts first.")
		response.Delivery.NoNext = "Follow the operation-specific recovery diagnostic; the original result bytes are unavailable"
		return response
	}
	return delivery.Error(readError(err), err.Error())
}
func (s Service) loadTransient(ctx context.Context, ref string, checkBasis bool, selectedPart string) (delivery.Document, error) {
	raw, err := s.memory().ReadTransient(ctx, ref)
	if err != nil {
		return delivery.Document{}, err
	}
	var captured capturedResult
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&captured); err != nil || captured.Format != "haft.transient-result/1" {
		return delivery.Document{}, fmt.Errorf("corrupt: transient envelope")
	}
	q, r := captured.Request, captured.Result
	d := delivery.Document{}
	if captured.Document != nil {
		d = *captured.Document
	} else {
		d = makeDelivery(q, r)
	}
	markRecallCompositeBasis(q, &d)
	// Observations and receipts retain their original outcome. A read is not a
	// new check of currentness, and cannot turn an old pass into a current pass.
	mutable := q.Operation == "recall" || q.Operation == "context" || q.Operation == "impact" || q.Operation == "check" && q.Action == "prepare" || q.Operation == "change" && (q.Action == "list" || q.Action == "show" || q.Action == "preview" || q.Action == "reauthor_preview")
	if q.Operation == "recall" && q.Ref != "" {
		part := selectedPart
		if part == "" {
			part = "summary"
		}
		selected, err := d.Member(part)
		mutable = err == nil && selected.Mutable
	}
	if checkBasis && mutable {
		if gen := r.Basis["memory_generation"]; gen != "" {
			now, e := s.memory().Read(ctx)
			if e != nil {
				return delivery.Document{}, e
			}
			if now.Generation != gen {
				return delivery.Document{}, fmt.Errorf("stale: memory generation changed; repeat original operation")
			}
		}
		if basis := r.Basis["code_basis"]; basis != "" {
			index, _, e := s.codeIndex(q)
			if e != nil {
				return delivery.Document{}, e
			}
			if index.Basis != basis {
				return delivery.Document{}, fmt.Errorf("stale: code basis changed; repeat original operation")
			}
		}
	}
	if checkBasis && (q.Operation == "source" || q.Operation == "fpf") && r.Basis["source_tree"] != "" {
		now, e := source.Capture(s.SourceRoot, s.SourceRepository, s.SourceLimits)
		if e != nil {
			return delivery.Document{}, e
		}
		if now.Status().Revision.TreeDigest != r.Basis["source_tree"] {
			return delivery.Document{}, fmt.Errorf("stale: source tree changed; repeat source query")
		}
	}
	d.Ref = ref
	d.Lifetime = "transient_cache"
	return d, nil
}
func (s Service) readDelivery(ctx context.Context, q Request) delivery.Response {
	// A read may never replay a mutation or accept hidden authoring controls.
	if q.Action != "" || q.Carrier != "" || q.Query != "" || q.RequestID != "" || q.Observation != nil || q.Capture != nil || q.Revision != nil || len(q.Retain) > 0 || len(q.Snapshots) > 0 || q.CaptureCode || q.PriorCode != nil || q.Offset != 0 || len(q.ExpectedHeads) > 0 || len(q.ExpectedTargets) > 0 || len(q.Metadata) > 0 || q.CodeConfig != nil || q.CheckRef != "" || q.Scope != "" || q.FailureContract != "" || q.FailurePattern != "" || q.Seed != nil || q.Strict || q.PreviewDigest != "" {
		return delivery.Error("invalid_read_request", "Read accepts ref, view, part, cursor, expected_digest and expected_generation only")
	}
	var d delivery.Document
	if strings.HasPrefix(q.Ref, "result:") {
		var err error
		d, err = s.loadTransient(ctx, q.Ref, true, q.Part)
		if err != nil {
			return transientFailure(err)
		}
	} else {
		ref, err := carrier.ParseRef(q.Ref)
		if err != nil || !ref.Pinned() {
			return delivery.Error("invalid_read_ref", "Use a returned exact persisted ref or transient result ref")
		}
		r := s.Execute(ctx, Request{Format: Format, Operation: "recall", Ref: q.Ref, forDelivery: true})
		if r.Kind != "found" {
			kind := "missing"
			for _, diag := range r.Diagnostics {
				if strings.Contains(diag.Path, strings.TrimPrefix(ref.Digest, "sha256:")) && (diag.Code == "corrupt_snapshot" || diag.Code == "invalid_snapshot" || diag.Code == "snapshot_unavailable") {
					kind = "corrupt"
				}
			}
			if r.Kind == "unavailable" {
				kind = "unavailable"
			}
			return delivery.Error(kind, "Exact snapshot cannot be resolved; no current-head fallback. Repeat recall for the original diagnostics")
		}
		origin := Request{Format: Format, Operation: "recall", Ref: q.Ref, Limit: q.Limit, forDelivery: true}
		d = makeDelivery(origin, r)
		if d.Ref == "" {
			return delivery.Error("missing", "No durable snapshot for this ref")
		}
		request := delivery.Request{Format: q.Format, Operation: q.Operation, Ref: q.Ref, View: q.View, Part: q.Part, Cursor: q.Cursor, ExpectedDigest: q.ExpectedDigest, ExpectedGeneration: q.ExpectedGeneration, Limit: q.Limit}
		return s.presentDelivery(ctx, origin, r, d, request)
	}
	return delivery.Present(d, delivery.Request{Format: q.Format, Operation: q.Operation, Ref: q.Ref, View: q.View, Part: q.Part, Cursor: q.Cursor, ExpectedDigest: q.ExpectedDigest, ExpectedGeneration: q.ExpectedGeneration, Limit: q.Limit})
}
