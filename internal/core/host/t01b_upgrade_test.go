package host

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var t01rFixtures = map[string]string{
	"AGENTS.md":                        "agents.md",
	".agents/skills/h-decide/SKILL.md": "h-decide.md",
	".agents/skills/h-reason/SKILL.md": "h-reason.md",
	".agents/skills/h-spec/SKILL.md":   "h-spec.md",
	".agents/skills/h-verify/SKILL.md": "h-verify.md",
}

func t01rContent(t *testing.T, name string) []byte {
	t.Helper()
	fixture, ok := t01rFixtures[name]
	if !ok {
		t.Fatalf("unknown T01R fixture %s", name)
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "t01r", fixture))
	if err != nil {
		t.Fatal(err)
	}
	if !generatedDigestMatches(t01rGeneratedDigests, name, raw) {
		t.Fatalf("T01R generated fixture digest changed: %s", name)
	}
	return raw
}

func TestT01BInitUpgradesExactPriorGeneratedAssets(t *testing.T) {
	c := config(t)
	prefix := "# Operator instructions\n\nKeep this prelude.\n\n"
	suffix := "\nKeep this suffix.\n"
	for name := range t01rFixtures {
		content := string(t01rContent(t, name))
		if name == "AGENTS.md" {
			content = prefix + content + suffix
		}
		write(t, c.Root, name, content)
	}
	first, err := Init(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Created) != 1 || len(first.Updated) != 5 {
		t.Fatalf("expected config creation and five exact upgrades: %+v", first)
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
	if err != nil || len(second.Updated) != 0 || len(second.Created) != 0 {
		t.Fatalf("rerun changed upgraded assets: %+v, %v", second, err)
	}
}

func TestT01BInitPreservesEditedPriorGeneratedSkill(t *testing.T) {
	c := config(t)
	before := map[string][]byte{}
	for name := range t01rFixtures {
		raw := t01rContent(t, name)
		if name == ".agents/skills/h-spec/SKILL.md" {
			raw = bytes.Replace(raw, []byte("A proposed spec uses"), []byte("A locally edited spec uses"), 1)
		}
		before[name] = raw
		write(t, c.Root, name, string(raw))
	}
	_, err := Init(c)
	if err == nil || !strings.Contains(err.Error(), "host_conflict") {
		t.Fatalf("edited prior h-spec was upgraded: %v", err)
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
