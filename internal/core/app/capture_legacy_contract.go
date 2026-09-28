package app

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"github.com/m0n0x41d/haft/internal/core/check"
)

// Historical expected parts were JSON objects assembled from maps, while the
// current digest covers check.Contract's struct encoding. Decode only the
// contract shape whose complete meaning this reader knows, then hash the same
// representation used by captureBasis. A missing or extra semantic field is
// not evidence that the old and current contracts are equal.
func legacyExpectedContractDigest(raw []byte) string {
	top, ok := knownLegacyObject(raw,
		[]string{"ref", "selector", "basis", "scope"},
		[]string{"ref", "selector", "basis", "scope", "failure_contract", "failure_pattern"})
	if !ok {
		return ""
	}
	_, ok = knownLegacyObject(top["selector"],
		[]string{"package", "test"},
		[]string{"package", "test"})
	if !ok {
		return ""
	}
	_, ok = knownLegacyObject(top["basis"],
		[]string{"claim", "code", "check", "dependencies", "conditions", "environment"},
		[]string{"claim", "code", "check", "dependencies", "conditions", "environment", "seed"})
	if !ok {
		return ""
	}
	var expected check.Contract
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&expected); err != nil {
		return ""
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ""
	}
	if !supportedLegacyExpectedContract(expected) {
		return ""
	}
	canonical, err := json.Marshal(expected)
	if err != nil {
		return ""
	}
	return carrier.Digest(canonical)
}

func knownLegacyObject(raw []byte, required, allowed []string) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, false
	}
	known := map[string]bool{}
	for _, name := range allowed {
		known[name] = true
	}
	for name, value := range fields {
		if !known[name] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, false
		}
	}
	for _, name := range required {
		if _, present := fields[name]; !present {
			return nil, false
		}
	}
	return fields, true
}

func supportedLegacyExpectedContract(expected check.Contract) bool {
	claim, err := carrier.ParseRef(expected.Basis.Claim)
	if err != nil || !claim.Pinned() || claim.ClaimID == "" {
		return false
	}
	if expected.Ref == "" || expected.Selector.Package == "" || expected.Selector.Test == "" || expected.Scope == "" {
		return false
	}
	if !carrier.ValidDigest(expected.Basis.Code) || !carrier.ValidDigest(expected.Basis.Check) || !carrier.ValidDigest(expected.Basis.Dependencies) {
		return false
	}
	if len(expected.Basis.Conditions) == 0 || len(expected.Basis.Environment) == 0 {
		return false
	}
	_, hasBuildTags := expected.Basis.Environment["build_tags"]
	return hasBuildTags
}
