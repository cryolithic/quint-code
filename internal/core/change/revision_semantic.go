package change

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"

	"github.com/m0n0x41d/haft/internal/core/carrier"
)

// Revision correspondence is internal to one requested successor. The keys
// remain source indexes so diagnostics and exemptions address saved YAML.
type revisionItems struct {
	tasks      map[int]int
	patches    map[int]int
	operations map[int]map[int]int
}

func matchRevisionItems(before, after Change, bases map[string]Basis) (revisionItems, []carrier.Diagnostic) {
	items := revisionItems{tasks: map[int]int{}, patches: map[int]int{}, operations: map[int]map[int]int{}}
	taskIndex := map[string]int{}
	for index, task := range after.Tasks {
		taskIndex[task.ID] = index
	}
	for index, task := range before.Tasks {
		if next, found := taskIndex[task.ID]; found {
			items.tasks[index] = next
		}
	}
	used := map[int]bool{}
	exactOld := map[int][]int{}
	exactNew := map[int][]int{}
	for oldIndex, old := range before.Patches {
		for newIndex, next := range after.Patches {
			if old.Base != next.Base {
				continue
			}
			exactOld[oldIndex] = append(exactOld[oldIndex], newIndex)
			exactNew[newIndex] = append(exactNew[newIndex], oldIndex)
		}
	}
	var diagnostics []carrier.Diagnostic
	for oldIndex, candidates := range exactOld {
		if len(candidates) != 1 || len(exactNew[candidates[0]]) != 1 {
			diagnostics = append(diagnostics, diag("ambiguous_revision_correspondence", fmt.Sprintf("patches[%d]", oldIndex), "Repeated exact bases do not identify one patch owner"))
			continue
		}
		items.patches[oldIndex] = candidates[0]
		used[candidates[0]] = true
	}
	unmatchedOld := map[string]bool{}
	for oldIndex, old := range before.Patches {
		if _, matched := items.patches[oldIndex]; !matched {
			unmatchedOld[old.Base] = true
		}
	}
	oldCandidates := map[int][]int{}
	newCandidates := map[int][]int{}
	// A rebase can change every operation context. Exact captured ancestry
	// establishes patch identity independently of claim IDs or array order.
	selected := map[int]Basis{}
	for newIndex, next := range after.Patches {
		if used[newIndex] || len(unmatchedOld) == 0 {
			continue
		}
		selected[newIndex] = bases[next.Base]
	}
	lineages := capturedRevisionLineages(after.Patches, selected, unmatchedOld)
	for newIndex, lineage := range lineages {
		for oldIndex, old := range before.Patches {
			if !unmatchedOld[old.Base] || lineage.problem != "" || !lineage.ancestors[old.Base] {
				continue
			}
			oldCandidates[oldIndex] = append(oldCandidates[oldIndex], newIndex)
			newCandidates[newIndex] = append(newCandidates[newIndex], oldIndex)
		}
	}
	for oldIndex, candidates := range oldCandidates {
		if len(candidates) != 1 || len(newCandidates[candidates[0]]) != 1 {
			diagnostics = append(diagnostics, diag("ambiguous_revision_correspondence", fmt.Sprintf("patches[%d]", oldIndex), "Several exact ancestry paths claim one patch correspondence"))
			continue
		}
		items.patches[oldIndex] = candidates[0]
		used[candidates[0]] = true
	}
	for newIndex, lineage := range lineages {
		if !lineage.known || lineage.problem == "" || used[newIndex] {
			continue
		}
		for oldIndex, old := range before.Patches {
			if !unmatchedOld[old.Base] {
				continue
			}
			diagnostics = append(diagnostics, diag("ambiguous_revision_correspondence", fmt.Sprintf("patches[%d]", oldIndex), lineage.problem))
		}
	}
	// Overlapping operation context alone does not prove section identity,
	// even when only one old and one new patch remain. Refuse that unresolved
	// association rather than copying extensions into an unrelated section.
	for oldIndex, old := range before.Patches {
		if _, matched := items.patches[oldIndex]; matched {
			continue
		}
		for newIndex, next := range after.Patches {
			lineage := lineages[newIndex]
			if !lineage.known || used[newIndex] {
				continue
			}
			if lineage.problem != "" {
				break
			}
			if sharedOperationContext(old.Operations, next.Operations) > 0 {
				diagnostics = append(diagnostics, diag("ambiguous_revision_correspondence", fmt.Sprintf("patches[%d]", oldIndex), "Changed patch base overlaps an operation context without exact ancestry; retained correspondence is unproven"))
				break
			}
		}
	}
	for oldIndex, newIndex := range items.patches {
		oldOperations := before.Patches[oldIndex].Operations
		newOperations := after.Patches[newIndex].Operations
		if (duplicateOperationContext(oldOperations) || duplicateOperationContext(newOperations)) && !reflect.DeepEqual(oldOperations, newOperations) {
			path := fmt.Sprintf("patches[%d].operations", oldIndex)
			diagnostics = append(diagnostics, diag("ambiguous_revision_correspondence", path, "Repeated operation contexts cannot identify a changed retained operation"))
			continue
		}
		items.operations[oldIndex] = matchRevisionOperations(oldOperations, newOperations)
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].Path != diagnostics[j].Path {
			return diagnostics[i].Path < diagnostics[j].Path
		}
		if diagnostics[i].Code != diagnostics[j].Code {
			return diagnostics[i].Code < diagnostics[j].Code
		}
		return diagnostics[i].Message < diagnostics[j].Message
	})
	return items, diagnostics
}

type revisionLineage struct {
	ancestors map[string]bool
	known     bool
	problem   string
}

type revisionWalkItem struct {
	ref  string
	path []string
}

type revisionLineageWalker struct {
	ref          string
	basis        Basis
	lineage      revisionLineage
	queue        []revisionWalkItem
	frontiers    []revisionWalkItem
	seen         map[string]bool
	head         int
	frontierHead int
	nodes        int
	edges        int
	limited      bool
}

func newRevisionLineageWalker(ref string, basis Basis) *revisionLineageWalker {
	walker := &revisionLineageWalker{
		ref: ref, basis: basis,
		lineage: revisionLineage{ancestors: map[string]bool{}},
		seen:    map[string]bool{},
	}
	if len(basis.Contested) > 1 || basis.CurrentRef != ref {
		return walker
	}
	walker.queue = []revisionWalkItem{{ref: ref, path: []string{ref}}}
	walker.seen[ref] = true
	return walker
}

// A found old base is a provisional frontier. Finish all non-frontier branches
// before extending behind old bases to seek an owner still unaccounted for.
// Non-frontier branches can still conceal a competing owner of another patch.
// Each walker spends at most 256 verified snapshots and 1024 inspected links.
func capturedRevisionLineages(patches []SectionPatch, selected map[int]Basis, oldBases map[string]bool) map[int]revisionLineage {
	walkers := map[int]*revisionLineageWalker{}
	indexes := make([]int, 0, len(selected))
	for index, basis := range selected {
		indexes = append(indexes, index)
		walkers[index] = newRevisionLineageWalker(patches[index].Base, basis)
	}
	sort.Ints(indexes)
	for {
		for {
			advanced := false
			for _, index := range indexes {
				walker := walkers[index]
				// With one old owner proved for this new patch, its other
				// branches cannot introduce a different owner. Other new
				// patches still walk to exclude a competing claim.
				if len(oldBases) == 1 && len(walker.lineage.ancestors) == 1 {
					continue
				}
				if walker.advance(oldBases) {
					advanced = true
				}
			}
			if !advanced {
				break
			}
		}
		owners := map[string][]int{}
		for _, index := range indexes {
			for old := range walkers[index].lineage.ancestors {
				owners[old] = append(owners[old], index)
			}
		}
		if len(owners) == len(oldBases) || ambiguousRevisionOwners(owners, walkers) {
			break
		}
		promoted := false
		for _, index := range indexes {
			if walkers[index].promoteFrontiers() {
				promoted = true
			}
		}
		if !promoted {
			break
		}
	}
	lineages := map[int]revisionLineage{}
	for _, index := range indexes {
		lineage := walkers[index].lineage
		if len(oldBases) == 1 && len(lineage.ancestors) == 1 {
			lineage.problem = ""
		}
		lineages[index] = lineage
	}
	return lineages
}

func ambiguousRevisionOwners(owners map[string][]int, walkers map[int]*revisionLineageWalker) bool {
	for _, indexes := range owners {
		if len(indexes) != 1 {
			return true
		}
	}
	for _, walker := range walkers {
		if len(walker.lineage.ancestors) > 1 {
			return true
		}
	}
	return false
}

func (walker *revisionLineageWalker) advance(oldBases map[string]bool) bool {
	if walker.limited {
		return false
	}
	for walker.head < len(walker.queue) {
		item := walker.queue[walker.head]
		walker.head++
		walker.visit(item, oldBases)
		return true
	}
	return false
}

func (walker *revisionLineageWalker) promoteFrontiers() bool {
	if walker.limited || walker.frontierHead == len(walker.frontiers) {
		return false
	}
	walker.queue = append(walker.queue, walker.frontiers[walker.frontierHead:]...)
	walker.frontierHead = len(walker.frontiers)
	return true
}

func (walker *revisionLineageWalker) visit(item revisionWalkItem, oldBases map[string]bool) {
	if walker.nodes == 256 {
		walker.recordProblem("Exact ancestry exceeds the 256-snapshot bound")
		walker.limited = true
		return
	}
	walker.nodes++
	parsed, err := carrier.ParseRef(item.ref)
	if err != nil || parsed.Digest == "" || parsed.ClaimID != "" {
		walker.recordProblem("Exact ancestry contains an invalid predecessor ref: " + item.ref)
		return
	}
	blob := walker.basis.CapturedSnapshots[parsed.Digest]
	if item.ref == walker.ref {
		blob = walker.basis.Snapshot
	}
	snapshot, err := carrier.ReadSnapshot(blob, parsed.Digest)
	if err != nil {
		walker.recordProblem("Exact ancestor snapshot is missing or invalid: " + item.ref)
		return
	}
	doc := snapshot.Document()
	if !doc.Valid() || doc.Record.Kind != "spec" || doc.Record.ID != parsed.RecordID {
		walker.recordProblem("Exact ancestor snapshot does not match its ref: " + item.ref)
		return
	}
	if item.ref == walker.ref {
		walker.lineage.known = true
	}
	parents := slices.Clone(doc.Record.Supersedes)
	sort.Strings(parents)
	for _, parent := range parents {
		if walker.edges == 1024 {
			walker.recordProblem("Exact ancestry exceeds the 1024-link bound")
			walker.limited = true
			return
		}
		walker.edges++
		if slices.Contains(item.path, parent) {
			walker.recordProblem("Exact ancestry contains a cycle at " + parent)
			continue
		}
		if oldBases[parent] {
			walker.lineage.ancestors[parent] = true
		}
		// This checks the first-discovery path. Convergence can conceal
		// cycles on another path; seen still prevents repeated traversal.
		if walker.seen[parent] {
			continue
		}
		path := append(slices.Clone(item.path), parent)
		item := revisionWalkItem{ref: parent, path: path}
		if oldBases[parent] {
			walker.frontiers = append(walker.frontiers, item)
		} else {
			walker.queue = append(walker.queue, item)
		}
		walker.seen[parent] = true
	}
}

func (walker *revisionLineageWalker) recordProblem(problem string) {
	if walker.lineage.problem == "" {
		walker.lineage.problem = problem
	}
}

func duplicateOperationContext(operations []Operation) bool {
	seen := map[string]bool{}
	for _, operation := range operations {
		key := operationContext(operation)
		if seen[key] {
			return true
		}
		seen[key] = true
	}
	return false
}

func operationContext(op Operation) string {
	claimID := ""
	if op.Claim != nil {
		claimID = op.Claim.ID
	}
	return op.Op + "\x00" + op.ClaimID + "\x00" + claimID + "\x00" + op.NewID
}

func sharedOperationContext(before, after []Operation) int {
	counts := map[string]int{}
	for _, op := range after {
		counts[operationContext(op)]++
	}
	score := 0
	for _, op := range before {
		key := operationContext(op)
		if counts[key] == 0 {
			continue
		}
		counts[key]--
		score++
	}
	return score
}

func matchRevisionOperations(before, after []Operation) map[int]int {
	matches := map[int]int{}
	used := map[int]bool{}
	for oldIndex, old := range before {
		if oldIndex < len(after) && operationContext(old) == operationContext(after[oldIndex]) {
			matches[oldIndex] = oldIndex
			used[oldIndex] = true
		}
	}
	for oldIndex, old := range before {
		if _, matched := matches[oldIndex]; matched {
			continue
		}
		candidate := -1
		for newIndex, next := range after {
			if used[newIndex] || operationContext(old) != operationContext(next) {
				continue
			}
			if candidate >= 0 {
				candidate = -1
				break
			}
			candidate = newIndex
		}
		if candidate >= 0 {
			matches[oldIndex] = candidate
			used[candidate] = true
		}
	}
	return matches
}

func retainRevisionExtras(before, after Change, items revisionItems) Change {
	after.Tasks = slices.Clone(after.Tasks)
	for oldIndex, newIndex := range items.tasks {
		if !taskContentChanged(before.Tasks[oldIndex], after.Tasks[newIndex]) || after.Tasks[newIndex].Extra != nil {
			after.Tasks[newIndex].Extra = retainExtra(before.Tasks[oldIndex].Extra, after.Tasks[newIndex].Extra)
		}
	}
	after.Patches = slices.Clone(after.Patches)
	for oldIndex, newIndex := range items.patches {
		old := before.Patches[oldIndex]
		next := after.Patches[newIndex]
		next.Extra = retainExtra(old.Extra, next.Extra)
		next.Operations = slices.Clone(next.Operations)
		for oldOperation, newOperation := range items.operations[oldIndex] {
			previous := old.Operations[oldOperation]
			current := next.Operations[newOperation]
			current.Extra = retainExtra(previous.Extra, current.Extra)
			if previous.Claim != nil && current.Claim != nil && previous.Claim.ID == current.Claim.ID {
				claim := retainRevisionClaim(*previous.Claim, *current.Claim)
				current.Claim = &claim
			}
			next.Operations[newOperation] = current
		}
		after.Patches[newIndex] = next
	}
	return after
}

func taskContentChanged(before, after Task) bool {
	return before.Text != after.Text || before.Done != after.Done || !slices.Equal(before.Results, after.Results)
}

func retainExtra(before, after carrier.Extra) carrier.Extra {
	if before == nil && after == nil {
		return nil
	}
	retained := carrier.Extra{}
	for key, value := range after {
		retained[key] = value
	}
	for key, value := range before {
		if _, supplied := retained[key]; !supplied {
			retained[key] = value
		}
	}
	return retained
}

func retainRevisionClaim(before, after carrier.Claim) carrier.Claim {
	after.Extra = retainExtra(before.Extra, after.Extra)
	after.Checks = slices.Clone(after.Checks)
	after.ImplementedBy = slices.Clone(after.ImplementedBy)
	after.EvidenceInputs = slices.Clone(after.EvidenceInputs)
	for index := range after.Checks {
		after.Checks[index].Extra = retainExtra(nil, after.Checks[index].Extra)
	}
	for index := range after.ImplementedBy {
		after.ImplementedBy[index].Extra = retainExtra(nil, after.ImplementedBy[index].Extra)
	}
	for index := range after.EvidenceInputs {
		after.EvidenceInputs[index].Extra = retainExtra(nil, after.EvidenceInputs[index].Extra)
	}
	after.Checks = retainBindingExtras(before.Checks, after.Checks)
	after.ImplementedBy = retainBindingExtras(before.ImplementedBy, after.ImplementedBy)
	after.EvidenceInputs = retainEvidenceInputExtras(before.EvidenceInputs, after.EvidenceInputs)
	after.Examples = slices.Clone(after.Examples)
	oldExamples := map[string]carrier.Example{}
	for _, example := range before.Examples {
		oldExamples[example.ID] = example
	}
	for index := range after.Examples {
		if old, found := oldExamples[after.Examples[index].ID]; found {
			after.Examples[index].Extra = retainExtra(old.Extra, after.Examples[index].Extra)
		}
	}
	return after
}

func revisionRetention(before, after Change, action string, items revisionItems, bases map[string]Basis) carrier.SemanticRetention {
	paths := map[string]bool{}
	for _, path := range []string{"id", "created_at", "supersedes", "supersede_reason", "write_receipt"} {
		paths[path] = true
	}
	if action == "archive" || action == "reopen" {
		paths["state"] = true
	}
	retention := carrier.SemanticRetention{Schema: Change{}}
	if action != "update" && action != "rebase" {
		retention.RewrittenPaths = sortedPaths(paths)
		return retention
	}
	if before.Intent != after.Intent {
		paths["intent"] = true
	}
	retention.SequenceMatches = map[string]map[int]int{"tasks": items.tasks, "patches": items.patches}
	if len(before.Tasks) > 0 && len(after.Tasks) == 0 {
		paths["tasks"] = true
	}
	for oldIndex, old := range before.Tasks {
		path := fmt.Sprintf("tasks[%d]", oldIndex)
		newIndex, matched := items.tasks[oldIndex]
		if !matched {
			paths[path] = true
			continue
		}
		next := after.Tasks[newIndex]
		if taskContentChanged(old, next) && next.Extra == nil {
			paths[path] = true
			continue
		}
		if old.Text != next.Text {
			paths[path+".text"] = true
		}
		if old.Done != next.Done {
			paths[path+".done"] = true
		}
		declaredStringListRewrites(paths, retention.SequenceMatches, path+".results", old.Results, next.Results)
		changedExtraPaths(paths, path, old.Extra, next.Extra)
	}
	if len(before.Patches) > 0 && len(after.Patches) == 0 {
		paths["patches"] = true
	}
	for oldIndex, old := range before.Patches {
		path := fmt.Sprintf("patches[%d]", oldIndex)
		newIndex, matched := items.patches[oldIndex]
		if !matched {
			paths[path] = true
			continue
		}
		next := after.Patches[newIndex]
		if old.Base != next.Base {
			paths[path+".base"] = true
		}
		for _, field := range []struct {
			name string
			old  any
			new  any
		}{
			{"body", old.Body, next.Body},
			{"expected_body_digest", old.ExpectedBodyDigest, next.ExpectedBodyDigest},
			{"body_change_reason", old.BodyChangeReason, next.BodyChangeReason},
			{"retire_reason", old.RetireReason, next.RetireReason},
		} {
			if !reflect.DeepEqual(field.old, field.new) {
				paths[path+"."+field.name] = true
			}
		}
		changedExtraPaths(paths, path, old.Extra, next.Extra)
		operationPath := path + ".operations"
		matches := items.operations[oldIndex]
		retention.SequenceMatches[operationPath] = matches
		if len(old.Operations) > 0 && len(next.Operations) == 0 {
			paths[operationPath] = true
		}
		for oldOperation, previous := range old.Operations {
			itemPath := operationPath + "[" + strconv.Itoa(oldOperation) + "]"
			newOperation, found := matches[oldOperation]
			if !found {
				paths[itemPath] = true
				continue
			}
			baseExamples, baseKnown := revisionSelectedBaseExamples(next.Base, next.Operations[newOperation], bases)
			operationRewrites(paths, retention.SequenceMatches, itemPath, previous, next.Operations[newOperation], baseExamples, baseKnown)
		}
	}
	retention.RewrittenPaths = sortedPaths(paths)
	return retention
}

func revisionSelectedBaseExamples(ref string, operation Operation, bases map[string]Basis) (map[string]bool, bool) {
	if operation.Op == "ADDED" {
		return map[string]bool{}, true
	}
	if operation.Op != "MODIFIED" {
		return nil, false
	}
	parsed, err := carrier.ParseRef(ref)
	if err != nil {
		return nil, false
	}
	snapshot, err := carrier.ReadSnapshot(bases[ref].Snapshot, parsed.Digest)
	if err != nil {
		return nil, false
	}
	doc := snapshot.Document()
	if !doc.Valid() || doc.Record.ID != parsed.RecordID {
		return nil, false
	}
	for _, claim := range doc.Record.Claims {
		if claim.ID != operation.ClaimID {
			continue
		}
		examples := map[string]bool{}
		for _, example := range claim.Examples {
			examples[example.ID] = true
		}
		return examples, true
	}
	return nil, false
}

func operationRewrites(paths map[string]bool, matches map[string]map[int]int, path string, before, after Operation, baseExamples map[string]bool, baseKnown bool) {
	if before.Reason != after.Reason {
		paths[path+".reason"] = true
	}
	declaredStringListRewrites(paths, matches, path+".remove_examples", before.RemoveExamples, after.RemoveExamples)
	declaredStringListRewrites(paths, matches, path+".remove_fields", before.RemoveFields, after.RemoveFields)
	changedExtraPaths(paths, path, before.Extra, after.Extra)
	if before.Claim == nil || after.Claim == nil {
		return
	}
	claimPath := path + ".claim"
	claimKnownRewrites(paths, claimPath, *before.Claim, *after.Claim)
	changedExtraPaths(paths, claimPath, before.Claim.Extra, after.Claim.Extra)
	examples := map[string]carrier.Example{}
	for _, example := range after.Claim.Examples {
		examples[example.ID] = example
	}
	for index, old := range before.Claim.Examples {
		path := fmt.Sprintf("%s.examples[%d]", claimPath, index)
		next, exists := examples[old.ID]
		if !exists && (contains(after.RemoveExamples, old.ID) || after.Claim.Examples != nil && baseKnown && !baseExamples[old.ID]) {
			paths[path] = true
			continue
		}
		if exists {
			changedExtraPaths(paths, path, old.Extra, next.Extra)
		}
	}
	if len(before.Claim.Examples) > 0 && after.Claim.Examples == nil {
		allRemoved := true
		for _, old := range before.Claim.Examples {
			allRemoved = allRemoved && contains(after.RemoveExamples, old.ID)
		}
		if allRemoved {
			paths[claimPath+".examples"] = true
		}
	}
}

// Declared string lists have positional edits and stable equal-value moves.
// Preserve surviving source scalars through reorder; only values genuinely
// removed or replaced are exempt. An empty result is omitted by YAML encoding.
func declaredStringListRewrites(paths map[string]bool, matches map[string]map[int]int, path string, before, after []string) {
	if slices.Equal(before, after) {
		return
	}
	if len(before) > 0 && len(after) == 0 {
		paths[path] = true
		return
	}
	used := map[int]bool{}
	paired := map[int]int{}
	for oldIndex, value := range before {
		for newIndex, next := range after {
			if used[newIndex] || value != next {
				continue
			}
			paired[oldIndex] = newIndex
			used[newIndex] = true
			break
		}
		if _, retained := paired[oldIndex]; !retained {
			paths[fmt.Sprintf("%s[%d]", path, oldIndex)] = true
		}
	}
	matches[path] = paired
}

func sortedPaths(paths map[string]bool) []string {
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}
