package change

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/m0n0x41d/haft/internal/core/carrier"
	"gopkg.in/yaml.v3"
)

// CreatedAtTimestampNormalization is the one known change-carrier coercion:
// a valid unquoted RFC3339 timestamp becomes the same typed string on write.
// Other tags on this control field remain subject to semantic comparison.
func CreatedAtTimestampNormalization(source, normalized []byte) bool {
	before := changeYAMLField(source, "created_at")
	after := changeYAMLField(normalized, "created_at")
	if before == nil || after == nil || before.Kind != yaml.ScalarNode || after.Kind != yaml.ScalarNode {
		return false
	}
	if before.Tag != "!!timestamp" || after.Tag != "!!str" || before.Value != after.Value {
		return false
	}
	_, err := time.Parse(time.RFC3339, before.Value)
	return err == nil
}

func changeYAMLField(raw []byte, key string) *yaml.Node {
	front, _, err := carrier.SplitFrontmatter(raw)
	if err != nil {
		return nil
	}
	node, diagnostics := carrier.ParseYAML(front)
	if carrier.HasErrors(diagnostics) {
		return nil
	}
	for index := 0; index+1 < len(node.Content[0].Content); index += 2 {
		if node.Content[0].Content[index].Value == key {
			return node.Content[0].Content[index+1]
		}
	}
	return nil
}

// sectionSemanticLosses compares the exact authored base with the proposed
// publication. The patch names deliberate rewrites; every other YAML value is
// retained, including unknown fields inside claims and examples.
func sectionSemanticLosses(base carrier.Document, next carrier.Record, body []byte, patch SectionPatch) []carrier.Diagnostic {
	normalized, err := carrier.Encode(next, body)
	if err != nil {
		return []carrier.Diagnostic{diag("spec_normalization_unavailable", patch.Base, err.Error())}
	}
	ambiguous := extensionSequenceMoves(base.Record, next)
	if len(ambiguous) > 0 {
		return ambiguous
	}
	retention := sectionRetention(base.Record, next, patch)
	return carrier.SemanticYAMLLosses(base.Raw, normalized, retention)
}

func extensionSequenceMoves(before, after carrier.Record) []carrier.Diagnostic {
	byID := map[string]carrier.Claim{}
	for _, claim := range after.Claims {
		byID[claim.ID] = claim
	}
	var paths []string
	for index, claim := range before.Claims {
		next, retained := byID[claim.ID]
		if !retained {
			continue
		}
		prefix := fmt.Sprintf("claims[%d]", index)
		paths = append(paths, extraSequenceMoves(prefix, claim.Extra, next.Extra)...)
		paths = append(paths, bindingSequenceMoves(prefix+".checks", claim.Checks, next.Checks)...)
		paths = append(paths, bindingSequenceMoves(prefix+".implemented_by", claim.ImplementedBy, next.ImplementedBy)...)
		paths = append(paths, evidenceSequenceMoves(prefix+".evidence_inputs", claim.EvidenceInputs, next.EvidenceInputs)...)
		examples := map[string]carrier.Example{}
		for _, example := range next.Examples {
			examples[example.ID] = example
		}
		for oldIndex, example := range claim.Examples {
			if updated, exists := examples[example.ID]; exists {
				path := fmt.Sprintf("%s.examples[%d]", prefix, oldIndex)
				paths = append(paths, extraSequenceMoves(path, example.Extra, updated.Extra)...)
			}
		}
	}
	sort.Strings(paths)
	var diagnostics []carrier.Diagnostic
	for _, path := range paths {
		diagnostics = append(diagnostics, diag("ambiguous_yaml_correspondence", path, "A sequence value moved between positions; the edited element cannot be identified by stable position"))
	}
	return diagnostics
}

func bindingSequenceMoves(path string, before, after []carrier.Binding) []string {
	if !uniqueBindingRefs(before) || !uniqueBindingRefs(after) {
		var paths []string
		for index := 0; index < min(len(before), len(after)); index++ {
			if before[index].Ref == after[index].Ref {
				paths = append(paths, extraSequenceMoves(fmt.Sprintf("%s[%d]", path, index), before[index].Extra, after[index].Extra)...)
			}
		}
		return paths
	}
	byRef := map[string]carrier.Binding{}
	for _, binding := range after {
		byRef[binding.Ref] = binding
	}
	var paths []string
	for index, binding := range before {
		if next, exists := byRef[binding.Ref]; exists {
			paths = append(paths, extraSequenceMoves(fmt.Sprintf("%s[%d]", path, index), binding.Extra, next.Extra)...)
		}
	}
	return paths
}

func evidenceSequenceMoves(path string, before, after []carrier.EvidenceInput) []string {
	if !uniqueEvidenceRefs(before) || !uniqueEvidenceRefs(after) {
		var paths []string
		for index := 0; index < min(len(before), len(after)); index++ {
			if before[index].Ref == after[index].Ref {
				paths = append(paths, extraSequenceMoves(fmt.Sprintf("%s[%d]", path, index), before[index].Extra, after[index].Extra)...)
			}
		}
		return paths
	}
	byRef := map[string]carrier.EvidenceInput{}
	for _, input := range after {
		byRef[input.Ref] = input
	}
	var paths []string
	for index, input := range before {
		if next, exists := byRef[input.Ref]; exists {
			paths = append(paths, extraSequenceMoves(fmt.Sprintf("%s[%d]", path, index), input.Extra, next.Extra)...)
		}
	}
	return paths
}

func uniqueEvidenceRefs(inputs []carrier.EvidenceInput) bool {
	seen := map[string]bool{}
	for _, input := range inputs {
		if input.Ref == "" || seen[input.Ref] {
			return false
		}
		seen[input.Ref] = true
	}
	return true
}

func extraSequenceMoves(path string, before, after carrier.Extra) []string {
	var paths []string
	for key, value := range before {
		if next, retained := after[key]; retained {
			paths = append(paths, valueSequenceMoves(semanticPath(path, key), value, next)...)
		}
	}
	return paths
}

func valueSequenceMoves(path string, before, after any) []string {
	left, leftMap := stringMap(before)
	right, rightMap := stringMap(after)
	if leftMap && rightMap {
		var paths []string
		for key, value := range left {
			if next, retained := right[key]; retained {
				paths = append(paths, valueSequenceMoves(semanticPath(path, key), value, next)...)
			}
		}
		return paths
	}
	oldSequence, oldOK := valueSequence(before)
	newSequence, newOK := valueSequence(after)
	if leftMap && newOK || rightMap && oldOK {
		return []string{path}
	}
	if !oldOK || !newOK {
		return nil
	}
	if sequenceMoved(oldSequence, newSequence) {
		return []string{path}
	}
	var paths []string
	for index := 0; index < min(len(oldSequence), len(newSequence)); index++ {
		child := fmt.Sprintf("%s[%d]", path, index)
		paths = append(paths, valueSequenceMoves(child, oldSequence[index], newSequence[index])...)
	}
	return paths
}

func sectionRetention(before, after carrier.Record, patch SectionPatch) carrier.SemanticRetention {
	rewritten := map[string]bool{}
	modified := map[string]bool{}
	for _, path := range []string{"id", "created_at", "updated_at", "write_receipt", "status", "origin", "operator_confirmed", "supersedes", "supersede_reason"} {
		rewritten[path] = true
	}
	original := map[string]int{}
	current := map[string]int{}
	finalID := map[int]string{}
	for index, claim := range before.Claims {
		original[claim.ID] = index
		current[claim.ID] = index
		finalID[index] = claim.ID
	}
	for _, op := range patch.Operations {
		index, retained := current[op.ClaimID]
		switch op.Op {
		case "ADDED":
			if _, exists := current[op.Claim.ID]; !exists {
				current[op.Claim.ID] = -1
			}
		case "REMOVED":
			if retained && index >= 0 {
				rewritten[fmt.Sprintf("claims[%d]", index)] = true
				rewritten["retired_claim_ids"] = true
				delete(finalID, index)
			}
			delete(current, op.ClaimID)
		case "RENAMED":
			if retained && index >= 0 {
				finalID[index] = op.NewID
				rewritten["retired_claim_ids"] = true
				for originalIndex, claim := range before.Claims {
					if contains(claim.Refs, op.ClaimID) {
						rewritten[fmt.Sprintf("claims[%d].refs", originalIndex)] = true
					}
				}
			}
			delete(current, op.ClaimID)
			current[op.NewID] = index
		case "MODIFIED":
			if !retained || index < 0 || op.Claim == nil {
				continue
			}
			prefix := fmt.Sprintf("claims[%d]", index)
			claimKnownRewrites(modified, prefix, before.Claims[index], *op.Claim)
			for key, supplied := range op.Claim.Extra {
				changedValuePaths(modified, semanticPath(prefix, key), before.Claims[index].Extra[key], supplied)
			}
			for _, key := range op.RemoveFields {
				rewritten[semanticPath(prefix, key)] = true
			}
			for exampleIndex, old := range before.Claims[index].Examples {
				path := fmt.Sprintf("%s.examples[%d]", prefix, exampleIndex)
				for _, removed := range op.RemoveExamples {
					if removed == old.ID {
						rewritten[path] = true
					}
				}
				for _, supplied := range op.Claim.Examples {
					if supplied.ID != old.ID {
						continue
					}
					for key, value := range supplied.Extra {
						changedValuePaths(modified, semanticPath(path, key), old.Extra[key], value)
					}
				}
			}
		}
	}
	if len(before.Claims) > 0 && len(after.Claims) == 0 {
		allRemoved := true
		for index := range before.Claims {
			allRemoved = allRemoved && rewritten[fmt.Sprintf("claims[%d]", index)]
		}
		if allRemoved {
			rewritten["claims"] = true
			rewritten["retirement"] = true
		}
	}
	renames := map[string]string{}
	byID := map[string]carrier.Claim{}
	for _, claim := range after.Claims {
		byID[claim.ID] = claim
	}
	for oldID, index := range original {
		prefix := fmt.Sprintf("claims[%d]", index)
		newID, retained := finalID[index]
		if !retained {
			continue
		}
		if oldID != newID {
			renames[oldID] = newID
			rewritten[prefix+".id"] = true
		}
		old := before.Claims[index]
		next, exists := byID[newID]
		if !exists {
			continue
		}
		actual := map[string]bool{}
		claimKnownRewrites(actual, prefix, old, next)
		for path := range actual {
			if modified[path] {
				rewritten[path] = true
			}
		}
		for key, oldValue := range old.Extra {
			path := semanticPath(prefix, key)
			actual := map[string]bool{}
			changedValuePaths(actual, path, oldValue, next.Extra[key])
			for leaf := range actual {
				if modified[leaf] {
					rewritten[leaf] = true
				}
			}
		}
		nextExamples := map[string]carrier.Example{}
		for _, example := range next.Examples {
			nextExamples[example.ID] = example
		}
		for exampleIndex, oldExample := range old.Examples {
			nextExample, exists := nextExamples[oldExample.ID]
			if !exists {
				continue
			}
			for key, oldValue := range oldExample.Extra {
				path := semanticPath(fmt.Sprintf("%s.examples[%d]", prefix, exampleIndex), key)
				actual := map[string]bool{}
				changedValuePaths(actual, path, oldValue, nextExample.Extra[key])
				for leaf := range actual {
					if modified[leaf] {
						rewritten[leaf] = true
					}
				}
			}
		}
		if len(old.Examples) > 0 && len(next.Examples) == 0 {
			allRemoved := true
			for exampleIndex := range old.Examples {
				path := fmt.Sprintf("%s.examples[%d]", prefix, exampleIndex)
				allRemoved = allRemoved && rewritten[path]
			}
			if allRemoved {
				rewritten[prefix+".examples"] = true
			}
		}
	}
	paths := make([]string, 0, len(rewritten))
	for path := range rewritten {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return carrier.SemanticRetention{RewrittenPaths: paths, RenamedIDs: map[string]map[string]string{"claims": renames}}
}

func claimKnownRewrites(paths map[string]bool, path string, before, after carrier.Claim) {
	fields := []struct {
		name string
		old  any
		new  any
	}{
		{"kind", before.Kind, after.Kind},
		{"text", before.Text, after.Text},
		{"refs", before.Refs, after.Refs},
		{"terms", before.Terms, after.Terms},
		{"unchecked", before.Unchecked, after.Unchecked},
	}
	for _, field := range fields {
		if !reflect.DeepEqual(field.old, field.new) {
			paths[path+"."+field.name] = true
		}
	}
	bindingRewrites(paths, path+".checks", before.Checks, after.Checks)
	bindingRewrites(paths, path+".implemented_by", before.ImplementedBy, after.ImplementedBy)
	evidenceInputRewrites(paths, path+".evidence_inputs", before.EvidenceInputs, after.EvidenceInputs)
	examples := map[string]carrier.Example{}
	for _, example := range after.Examples {
		examples[example.ID] = example
	}
	for index, old := range before.Examples {
		next, exists := examples[old.ID]
		if !exists {
			continue
		}
		prefix := fmt.Sprintf("%s.examples[%d]", path, index)
		for _, field := range []struct {
			name string
			old  string
			new  string
		}{
			{"given", old.Given, next.Given},
			{"when", old.When, next.When},
			{"then", old.Then, next.Then},
			{"text", old.Text, next.Text},
		} {
			if field.old != field.new {
				paths[prefix+"."+field.name] = true
			}
		}
	}
}

// Unique refs identify retained bindings through reordering. Duplicate refs
// permit positional edits of plain declared fields; retained extensions and
// authored tags still require unambiguous correspondence in the comparator.
func bindingRewrites(paths map[string]bool, path string, before, after []carrier.Binding) {
	if len(before) > 0 && len(after) == 0 {
		paths[path] = true
		return
	}
	current := make(map[string]carrier.Binding, len(after))
	seen := map[string]bool{}
	unique := true
	for _, item := range before {
		if item.Ref == "" || seen[item.Ref] {
			unique = false
		}
		seen[item.Ref] = true
	}
	for _, item := range after {
		if item.Ref == "" || current[item.Ref].Ref != "" {
			unique = false
		}
		current[item.Ref] = item
	}
	if !unique {
		bindingPositionRewrites(paths, path, before, after)
		return
	}
	for index, old := range before {
		itemPath := fmt.Sprintf("%s[%d]", path, index)
		next, retained := current[old.Ref]
		if !retained {
			paths[itemPath] = true
			continue
		}
		if old.Covers != next.Covers {
			paths[itemPath+".covers"] = true
		}
		if old.Conditions != next.Conditions {
			paths[itemPath+".conditions"] = true
		}
		changedValuePaths(paths, itemPath+".interpretation_basis", old.InterpretationBasis, next.InterpretationBasis)
		changedExtraPaths(paths, itemPath, old.Extra, next.Extra)
	}
}

func bindingPositionRewrites(paths map[string]bool, path string, before, after []carrier.Binding) {
	for index, old := range before {
		itemPath := fmt.Sprintf("%s[%d]", path, index)
		if index >= len(after) || old.Ref != after[index].Ref {
			paths[itemPath] = true
			continue
		}
		next := after[index]
		if old.Covers != next.Covers {
			paths[itemPath+".covers"] = true
		}
		if old.Conditions != next.Conditions {
			paths[itemPath+".conditions"] = true
		}
		changedValuePaths(paths, itemPath+".interpretation_basis", old.InterpretationBasis, next.InterpretationBasis)
		changedExtraPaths(paths, itemPath, old.Extra, next.Extra)
	}
}

func evidenceInputRewrites(paths map[string]bool, path string, before, after []carrier.EvidenceInput) {
	if len(before) > 0 && len(after) == 0 {
		paths[path] = true
		return
	}
	current := make(map[string]carrier.EvidenceInput, len(after))
	seen := map[string]bool{}
	unique := true
	for _, item := range before {
		if item.Ref == "" || seen[item.Ref] {
			unique = false
		}
		seen[item.Ref] = true
	}
	for _, item := range after {
		if item.Ref == "" || current[item.Ref].Ref != "" {
			unique = false
		}
		current[item.Ref] = item
	}
	if !unique {
		evidencePositionRewrites(paths, path, before, after)
		return
	}
	for index, old := range before {
		itemPath := fmt.Sprintf("%s[%d]", path, index)
		next, retained := current[old.Ref]
		if !retained {
			paths[itemPath] = true
			continue
		}
		if old.Applicability != next.Applicability {
			paths[itemPath+".applicability"] = true
		}
		changedExtraPaths(paths, itemPath, old.Extra, next.Extra)
	}
}

func evidencePositionRewrites(paths map[string]bool, path string, before, after []carrier.EvidenceInput) {
	for index, old := range before {
		itemPath := fmt.Sprintf("%s[%d]", path, index)
		if index >= len(after) || old.Ref != after[index].Ref {
			paths[itemPath] = true
			continue
		}
		next := after[index]
		if old.Applicability != next.Applicability {
			paths[itemPath+".applicability"] = true
		}
		changedExtraPaths(paths, itemPath, old.Extra, next.Extra)
	}
}

func changedExtraPaths(paths map[string]bool, parent string, before, after carrier.Extra) {
	for key, supplied := range after {
		original, retained := before[key]
		if retained {
			changedValuePaths(paths, semanticPath(parent, key), original, supplied)
		}
	}
}

// A nested map's changed leaf is an authored edit. Siblings remain guarded,
// including a copied custom tag that typed decoding represented as a string.
// Omitted nested keys are not silently treated as a requested removal.
func changedValuePaths(paths map[string]bool, path string, before, after any) {
	if reflect.DeepEqual(before, after) {
		return
	}
	left, leftMap := stringMap(before)
	right, rightMap := stringMap(after)
	if leftMap && rightMap {
		for key, value := range right {
			if old, retained := left[key]; retained {
				changedValuePaths(paths, semanticPath(path, key), old, value)
			}
		}
		return
	}
	leftSequence, leftIsSequence := valueSequence(before)
	rightSequence, rightIsSequence := valueSequence(after)
	if leftIsSequence && rightIsSequence {
		if sequenceMoved(leftSequence, rightSequence) {
			return
		}
		for index := 0; index < min(len(leftSequence), len(rightSequence)); index++ {
			changedValuePaths(paths, fmt.Sprintf("%s[%d]", path, index), leftSequence[index], rightSequence[index])
		}
		for index := len(rightSequence); index < len(leftSequence); index++ {
			paths[fmt.Sprintf("%s[%d]", path, index)] = true
		}
		return
	}
	// A different container shape cannot identify retained children. Replacing
	// one complete extension with a scalar is an addressed field replacement;
	// moves between two containers remain ambiguous.
	if ambiguousContainer(before) && ambiguousContainer(after) {
		return
	}
	paths[path] = true
}

func valueSequence(value any) ([]any, bool) {
	shape := reflect.ValueOf(value)
	if !shape.IsValid() || shape.Kind() != reflect.Slice && shape.Kind() != reflect.Array {
		return nil, false
	}
	values := make([]any, shape.Len())
	for index := range values {
		values[index] = shape.Index(index).Interface()
	}
	return values, true
}

func sequenceMoved(before, after []any) bool {
	for oldIndex, value := range before {
		if oldIndex < len(after) && reflect.DeepEqual(value, after[oldIndex]) {
			continue
		}
		for newIndex, candidate := range after {
			if newIndex != oldIndex && reflect.DeepEqual(value, candidate) &&
				(newIndex >= len(before) || !reflect.DeepEqual(value, before[newIndex])) {
				return true
			}
		}
	}
	return false
}

func ambiguousContainer(value any) bool {
	shape := reflect.ValueOf(value)
	if !shape.IsValid() {
		return false
	}
	return shape.Kind() == reflect.Map || shape.Kind() == reflect.Slice || shape.Kind() == reflect.Array
}

func stringMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case carrier.Extra:
		return typed, true
	default:
		return nil, false
	}
}

func semanticPath(parent, key string) string {
	if strings.ContainsAny(key, ".[]") {
		return parent + "[" + strconv.Quote(key) + "]"
	}
	return parent + "." + key
}
