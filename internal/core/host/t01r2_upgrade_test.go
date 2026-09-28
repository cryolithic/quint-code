package host

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var t01bFixtures = map[string]string{
	"AGENTS.md":                        "agents.md",
	".agents/skills/h-decide/SKILL.md": "h-decide.md",
	".agents/skills/h-reason/SKILL.md": "h-reason.md",
	".agents/skills/h-spec/SKILL.md":   "h-spec.md",
	".agents/skills/h-verify/SKILL.md": "h-verify.md",
}

func t01bContent(t *testing.T, name string) []byte {
	t.Helper()
	fixture, ok := t01bFixtures[name]
	if !ok {
		t.Fatalf("unknown T01B generated asset %s", name)
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "t01b", fixture))
	if err != nil {
		t.Fatal(err)
	}
	if !generatedDigestMatches(t01bGeneratedDigests, name, raw) {
		t.Fatalf("T01B generated fixture digest changed: %s", name)
	}
	return raw
}

func TestT01R2InitUpgradesExactT01BGeneratedAssets(t *testing.T) {
	c := config(t)
	prefix := "# Operator instructions\n\nKeep this prelude.\n\n"
	suffix := "\nKeep this suffix.\n"
	for name := range t01bFixtures {
		content := string(t01bContent(t, name))
		if name == "AGENTS.md" {
			content = prefix + content + suffix
		}
		write(t, c.Root, name, content)
	}
	first, err := Init(c)
	if err != nil || len(first.Created) != 1 || len(first.Updated) != 5 {
		t.Fatalf("exact T01B generated assets did not upgrade: %+v, %v", first, err)
	}
	gotAgents, err := os.ReadFile(filepath.Join(c.Root, "AGENTS.md"))
	if err != nil || string(gotAgents) != prefix+agents+suffix {
		t.Fatalf("managed AGENTS block changed operator text: %v", err)
	}
	for name, want := range Skills() {
		got, err := os.ReadFile(filepath.Join(c.Root, ".agents", "skills", name, "SKILL.md"))
		if err != nil || string(got) != want {
			t.Fatalf("skill %s did not advance: %v", name, err)
		}
	}
	second, err := Init(c)
	if err != nil || len(second.Updated) != 0 || len(second.Created) != 0 || len(second.Unchanged) != 6 {
		t.Fatalf("rerun changed upgraded assets: %+v, %v", second, err)
	}
}

func TestT01R2InitPreservesEditedT01BGeneratedSkill(t *testing.T) {
	c := config(t)
	before := map[string][]byte{}
	for name := range t01bFixtures {
		raw := t01bContent(t, name)
		if name == ".agents/skills/h-verify/SKILL.md" {
			raw = bytes.Replace(raw, []byte("Capture runs the derived exact command"), []byte("Locally edited capture runs the derived exact command"), 1)
		}
		before[name] = raw
		write(t, c.Root, name, string(raw))
	}
	_, err := Init(c)
	if err == nil || !strings.Contains(err.Error(), "host_conflict") {
		t.Fatalf("edited T01B h-verify was upgraded: %v", err)
	}
	for name, want := range before {
		got, err := os.ReadFile(filepath.Join(c.Root, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("preflight changed %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(c.Root, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("preflight created MCP config before rejecting edited skill")
	}
}
