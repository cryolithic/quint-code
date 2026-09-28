package carrier

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// SemanticRetention names only fields deliberately changed by the operation.
// Paths address source nodes, including their original sequence indexes.
// RenamedIDs pairs renamed items without exempting their retained fields.
type SemanticRetention struct {
	RewrittenPaths []string
	RenamedIDs     map[string]map[string]string
	// SequenceMatches pairs source indexes with successor indexes only where an
	// effect has established item correspondence without a shared YAML key.
	SequenceMatches map[string]map[int]int
	// Schema is the authored frontmatter type. A nil Schema uses Record.
	// Only declared fields gain their type and omitempty semantics; inline
	// extension maps remain raw YAML values.
	Schema any
}

// SemanticYAMLLosses checks that retained YAML meaning survives authored
// normalization. Node comparison precedes typed decoding, which may round
// numbers, discard tags, or omit unknown fields. Old carriers remain readable.
func SemanticYAMLLosses(source, normalized []byte, retention SemanticRetention) []Diagnostic {
	before, err := frontmatterNode(source)
	if err != nil {
		return []Diagnostic{diagnostic("semantic_yaml_check_unavailable", "", err.Error())}
	}
	after, err := frontmatterNode(normalized)
	if err != nil {
		return []Diagnostic{diagnostic("semantic_yaml_check_unavailable", "", err.Error())}
	}
	rewritten := make(map[string]bool, len(retention.RewrittenPaths))
	for _, path := range retention.RewrittenPaths {
		if path != "" {
			rewritten[path] = true
		}
	}
	schema := reflect.TypeOf(retention.Schema)
	if schema == nil {
		schema = reflect.TypeOf(Record{})
	}
	return compareRetainedYAML(before, after, "", rewritten, retention.RenamedIDs, retention.SequenceMatches, schema, false)
}

func frontmatterNode(raw []byte) (*yaml.Node, error) {
	front, _, err := SplitFrontmatter(raw)
	if err != nil {
		return nil, err
	}
	node, ds := ParseYAML(front)
	if HasErrors(ds) {
		return nil, fmt.Errorf("cannot compare YAML values: %v", ds)
	}
	return node.Content[0], nil
}

func compareRetainedYAML(before, after *yaml.Node, path string, rewritten map[string]bool, renamed map[string]map[string]string, matches map[string]map[int]int, schema reflect.Type, optional bool) []Diagnostic {
	if rewritten[path] {
		return nil
	}
	if after == nil {
		if omittedKnownZero(before, schema, optional) {
			return nil
		}
		return []Diagnostic{omittedYAMLValue(path, before)}
	}
	if before.Kind != after.Kind || before.Tag != after.Tag {
		if knownStringScalar(before, after, schema) {
			return nil
		}
		return []Diagnostic{tagLoss(path, before.Tag, after.Tag)}
	}
	if before.Kind == yaml.ScalarNode {
		if sameYAMLScalar(before, after) {
			return nil
		}
		return []Diagnostic{valueLoss(path, before.Tag)}
	}
	if before.Kind == yaml.MappingNode {
		return compareYAMLMapping(before, after, path, rewritten, renamed, matches, schema)
	}
	if before.Kind == yaml.SequenceNode {
		return compareYAMLSequence(before, after, path, rewritten, renamed, matches, schema)
	}
	return nil
}

func compareYAMLMapping(before, after *yaml.Node, path string, rewritten map[string]bool, renamed map[string]map[string]string, matches map[string]map[int]int, schema reflect.Type) []Diagnostic {
	var losses []Diagnostic
	for i := 0; i+1 < len(before.Content); i += 2 {
		key := before.Content[i].Value
		childPath := yamlValuePath(path, key)
		other := mappedValue(after, key)
		childSchema, optional := declaredYAMLField(schema, key)
		losses = append(losses, compareRetainedYAML(before.Content[i+1], other, childPath, rewritten, renamed, matches, childSchema, optional)...)
	}
	return losses
}

func compareYAMLSequence(before, after *yaml.Node, path string, rewritten map[string]bool, renamed map[string]map[string]string, matches map[string]map[int]int, schema reflect.Type) []Diagnostic {
	key := sequenceIdentityField(path, schema)
	byID, keyed := sequenceItemsByKey(before, after, key)
	matched := matches[path]
	if key == "ref" && !keyed && hasItemRewrite(rewritten, path) &&
		bindingSequenceHasRetainedMeaning(before, sequenceElementType(schema)) &&
		sharedSequenceKey(before, after, key) {
		return []Diagnostic{diagnostic("ambiguous_yaml_correspondence", path,
			"Binding refs are not unique and complete; retained tagged or extension values need exact item correspondence")}
	}
	itemSchema := sequenceElementType(schema)
	var losses []Diagnostic
	for i, value := range before.Content {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		if matched != nil {
			var other *yaml.Node
			if next, exists := matched[i]; exists && next >= 0 && next < len(after.Content) {
				other = after.Content[next]
			}
			losses = append(losses, compareRetainedYAML(value, other, childPath, rewritten, renamed, matches, itemSchema, false)...)
			continue
		}
		if keyed {
			sourceID := mappedValue(value, key).Value
			id := sourceID
			if key == "id" && renamed[path] != nil && renamed[path][id] != "" {
				nextID := renamed[path][id]
				id = nextID
			}
			if id != sourceID {
				renamedFields := make(map[string]bool, len(rewritten)+1)
				for field := range rewritten {
					renamedFields[field] = true
				}
				renamedFields[childPath+"."+key] = true
				losses = append(losses, compareRetainedYAML(value, byID[id], childPath, renamedFields, renamed, matches, itemSchema, false)...)
				continue
			}
			losses = append(losses, compareRetainedYAML(value, byID[id], childPath, rewritten, renamed, matches, itemSchema, false)...)
			continue
		}
		var other *yaml.Node
		if i < len(after.Content) {
			other = after.Content[i]
		}
		losses = append(losses, compareRetainedYAML(value, other, childPath, rewritten, renamed, matches, itemSchema, false)...)
	}
	return losses
}

func sharedSequenceKey(before, after *yaml.Node, key string) bool {
	seen := map[string]bool{}
	for _, item := range before.Content {
		if ref := mappedValue(item, key); ref != nil {
			seen[ref.Value] = true
		}
	}
	for _, item := range after.Content {
		if ref := mappedValue(item, key); ref != nil && seen[ref.Value] {
			return true
		}
	}
	return false
}

func bindingSequenceHasRetainedMeaning(sequence *yaml.Node, itemSchema reflect.Type) bool {
	for _, item := range sequence.Content {
		if item.Kind != yaml.MappingNode {
			return true
		}
		for index := 0; index+1 < len(item.Content); index += 2 {
			key := item.Content[index].Value
			value := item.Content[index+1]
			field, _ := declaredYAMLField(itemSchema, key)
			if field == nil || key == "interpretation_basis" || explicitYAMLTag(value) {
				return true
			}
		}
	}
	return false
}

func explicitYAMLTag(node *yaml.Node) bool {
	if node.Style&yaml.TaggedStyle != 0 {
		return true
	}
	for _, child := range node.Content {
		if explicitYAMLTag(child) {
			return true
		}
	}
	return false
}

func hasItemRewrite(rewritten map[string]bool, path string) bool {
	prefix := path + "["
	for candidate := range rewritten {
		if strings.HasPrefix(candidate, prefix) {
			return true
		}
	}
	return false
}

// Claims, scenarios and a claim's bindings have source-owned correspondence.
// Other sequences retain authored order and are compared positionally.
func sequenceIdentityField(path string, schema reflect.Type) string {
	element := sequenceElementType(schema)
	if path == "claims" && element == reflect.TypeOf(Claim{}) {
		return "id"
	}
	if strings.HasSuffix(path, ".examples") && element == reflect.TypeOf(Example{}) {
		return "id"
	}
	if element == reflect.TypeOf(Binding{}) &&
		(strings.HasSuffix(path, ".checks") || strings.HasSuffix(path, ".implemented_by")) {
		return "ref"
	}
	if strings.HasSuffix(path, ".evidence_inputs") && element == reflect.TypeOf(EvidenceInput{}) {
		return "ref"
	}
	return ""
}

func sequenceItemsByKey(before, after *yaml.Node, key string) (map[string]*yaml.Node, bool) {
	if key == "" {
		return nil, false
	}
	if !allUniqueKeys(before, key) || !allUniqueKeys(after, key) {
		return nil, false
	}
	items := make(map[string]*yaml.Node, len(after.Content))
	for _, value := range after.Content {
		items[mappedValue(value, key).Value] = value
	}
	return items, true
}

func allUniqueKeys(sequence *yaml.Node, key string) bool {
	seen := map[string]bool{}
	for _, item := range sequence.Content {
		if item.Kind != yaml.MappingNode {
			return false
		}
		id := mappedValue(item, key)
		if !knownStringIdentity(id) || seen[id.Value] {
			return false
		}
		seen[id.Value] = true
	}
	return true
}

func knownStringIdentity(value *yaml.Node) bool {
	if value == nil || value.Kind != yaml.ScalarNode || value.Value == "" {
		return false
	}
	if value.Tag == "!!str" {
		return true
	}
	if value.Style&yaml.TaggedStyle != 0 {
		return false
	}
	switch value.Tag {
	case "!!timestamp", "!!int", "!!float", "!!bool":
		return true
	default:
		return false
	}
}

func sequenceElementType(schema reflect.Type) reflect.Type {
	schema = dereferenceSchema(schema)
	if schema == nil || schema.Kind() != reflect.Slice {
		return nil
	}
	return dereferenceSchema(schema.Elem())
}

func dereferenceSchema(schema reflect.Type) reflect.Type {
	for schema != nil && schema.Kind() == reflect.Pointer {
		schema = schema.Elem()
	}
	return schema
}

func declaredYAMLField(schema reflect.Type, key string) (reflect.Type, bool) {
	schema = dereferenceSchema(schema)
	if schema == nil || schema.Kind() != reflect.Struct {
		return nil, false
	}
	for i := 0; i < schema.NumField(); i++ {
		field := schema.Field(i)
		name, options, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if name != key || name == "" {
			continue
		}
		return field.Type, strings.Contains(","+options+",", ",omitempty,")
	}
	return nil, false
}

func knownStringScalar(before, after *yaml.Node, schema reflect.Type) bool {
	schema = dereferenceSchema(schema)
	if schema == nil || schema.Kind() != reflect.String || before.Kind != yaml.ScalarNode || after.Kind != yaml.ScalarNode {
		return false
	}
	if before.Style&yaml.TaggedStyle != 0 || after.Tag != "!!str" {
		return false
	}
	switch before.Tag {
	case "!!timestamp", "!!int", "!!float", "!!bool":
		return before.Value == after.Value
	default:
		return false
	}
}

func omittedKnownZero(before *yaml.Node, schema reflect.Type, optional bool) bool {
	if !optional || before == nil || before.Kind == yaml.AliasNode {
		return false
	}
	if schema == nil {
		return false
	}
	if before.Kind == yaml.ScalarNode && before.Tag == "!!null" {
		return true
	}
	// A non-null pointer is present even when its pointed-to value is zero.
	// In particular, body: "" is an authored change, not omitted *string.
	if schema.Kind() == reflect.Pointer {
		return false
	}
	switch schema.Kind() {
	case reflect.String:
		return before.Kind == yaml.ScalarNode && before.Tag == "!!str" && before.Value == ""
	case reflect.Bool:
		return before.Kind == yaml.ScalarNode && before.Tag == "!!bool" && strings.EqualFold(before.Value, "false")
	case reflect.Slice:
		return before.Kind == yaml.SequenceNode && before.Tag == "!!seq" && len(before.Content) == 0
	case reflect.Map:
		return before.Kind == yaml.MappingNode && before.Tag == "!!map" && len(before.Content) == 0
	default:
		return false
	}
}

func mappedValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func sameYAMLScalar(before, after *yaml.Node) bool {
	if before.Tag != after.Tag {
		return false
	}
	switch before.Tag {
	case "!!null":
		return true
	case "!!bool":
		return strings.EqualFold(before.Value, after.Value)
	case "!!int":
		left, leftOK := new(big.Int).SetString(strings.ReplaceAll(before.Value, "_", ""), 0)
		right, rightOK := new(big.Int).SetString(strings.ReplaceAll(after.Value, "_", ""), 0)
		return leftOK && rightOK && left.Cmp(right) == 0
	case "!!float":
		return sameYAMLFloat(before.Value, after.Value)
	case "!!timestamp":
		left, leftOK := yamlTimestamp(before.Value)
		right, rightOK := yamlTimestamp(after.Value)
		return leftOK && rightOK && left.Equal(right)
	case "!!binary":
		left, leftErr := base64.StdEncoding.DecodeString(removeYAMLWhitespace(before.Value))
		right, rightErr := base64.StdEncoding.DecodeString(removeYAMLWhitespace(after.Value))
		return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
	default:
		return before.Value == after.Value
	}
}

func sameYAMLFloat(before, after string) bool {
	left := strings.ToLower(strings.ReplaceAll(before, "_", ""))
	right := strings.ToLower(strings.ReplaceAll(after, "_", ""))
	if left == "+.inf" {
		left = ".inf"
	}
	if right == "+.inf" {
		right = ".inf"
	}
	if left == ".nan" || left == ".inf" || left == "-.inf" ||
		right == ".nan" || right == ".inf" || right == "-.inf" {
		return left == right
	}
	a, aOK := new(big.Rat).SetString(left)
	b, bOK := new(big.Rat).SetString(right)
	return aOK && bOK && a.Cmp(b) == 0
}

func yamlTimestamp(value string) (time.Time, bool) {
	// Keep this set aligned with yaml.v3's timestamp resolver. It accepts
	// short date/time fields, lower-case t, and a zone-free space separator.
	for _, layout := range []string{
		"2006-1-2T15:4:5.999999999Z07:00",
		"2006-1-2t15:4:5.999999999Z07:00",
		"2006-1-2 15:4:5.999999999",
		"2006-1-2",
	} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func removeYAMLWhitespace(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
}

func yamlValuePath(parent, key string) string {
	if strings.ContainsAny(key, ".[]") {
		return parent + "[" + strconv.Quote(key) + "]"
	}
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func omittedYAMLValue(path string, before *yaml.Node) Diagnostic {
	if before.Style&yaml.TaggedStyle != 0 {
		return tagLoss(path, before.Tag, "<omitted>")
	}
	return diagnostic("unsupported_yaml_content_conversion", path,
		"Retained YAML value would be omitted by authored normalization")
}

func tagLoss(path, original, normalized string) Diagnostic {
	return diagnostic("unsupported_yaml_tag_conversion", path,
		fmt.Sprintf("YAML value tag/type %s would become %s; authored normalization cannot preserve this value's meaning", original, normalized))
}

func valueLoss(path, tag string) Diagnostic {
	return diagnostic("unsupported_yaml_value_conversion", path,
		fmt.Sprintf("YAML %s scalar value would change; authored normalization cannot preserve this value's meaning", tag))
}
