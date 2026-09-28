package host

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestT02RHistoricalGeneratedConfigForms(t *testing.T) {
	cases := []struct {
		name   string
		digest string
	}{
		{"b1", "ab8c91386b9626e86aba0074c0cc0458cf41a93554865f1e671e6dd0331a4d44"},
		{"t01ar", "febae23d3a327ae841de969271dcd99f78274c7fee73a9c546956cc419a7f51b"},
		{"t01r2", "731178616995c10c9def4248f083fa41344dcf1826b8194d188731e38a1beb6b"},
		{"t01br", "24bd4ee2a494d83eea98d0aec3164f6475e0f37f755548bee1805b1c0cca67a2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := filepath.Join("testdata", "t02r", tc.name+"-config.toml")
			raw, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(raw)
			if hex.EncodeToString(digest[:]) != tc.digest {
				t.Fatal("public synthetic historical config bytes changed")
			}
			_, _, block, _, err := managedSlice(raw, configStart, configEnd)
			if err != nil {
				t.Fatal(err)
			}
			prior, err := parseManagedConfig(block)
			if err != nil || !bytes.Equal(prior.block(), block) || prior.SourceRepository != "fixture-source" {
				t.Fatalf("authentic predecessor config is not exactly recognized: %+v, %v", prior, err)
			}

			base := t.TempDir()
			original := filepath.Join(base, "original")
			copied := filepath.Join(base, "copy")
			oldBinary := filepath.Join(base, "old-bin", "haft10")
			newBinary := filepath.Join(base, "new-bin", "haft10")
			oldSource := filepath.Join(base, "source-old")
			newSource := filepath.Join(base, "source-new")
			fakeExecutable(t, oldBinary)
			fakeExecutable(t, newBinary)
			for _, path := range []string{original, oldSource, newSource} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			addressed := historicalConfigAddress(t, raw, prior, managedConfig{
				Root: original, Binary: oldBinary, SourceRoot: oldSource,
			})
			configPath := filepath.Join(original, ".codex", "config.toml")
			if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(configPath, addressed, 0600); err != nil {
				t.Fatal(err)
			}
			originalBefore := projectFiles(t, original)
			copyProject(t, original, copied)
			_, _, _, outsideBefore, err := managedSlice(addressed, configStart, configEnd)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Init(Config{Root: copied, Binary: newBinary, SourceRoot: newSource, Codex: true})
			if err != nil || !reflect.DeepEqual(result.Updated, []string{".codex/config.toml"}) || len(result.Created) != 5 {
				t.Fatalf("historical config did not relocate: %+v, %v", result, err)
			}
			current := installedConfig(t, copied)
			physicalRoot, err := filepath.EvalSymlinks(copied)
			if err != nil {
				t.Fatal(err)
			}
			physicalBinary, err := filepath.EvalSymlinks(newBinary)
			if err != nil {
				t.Fatal(err)
			}
			physicalSource, err := filepath.EvalSymlinks(newSource)
			if err != nil {
				t.Fatal(err)
			}
			if current.Root != physicalRoot || current.Binary != physicalBinary || current.SourceRoot != physicalSource || current.SourceRepository != prior.SourceRepository || current.Profile != prior.Profile {
				t.Fatalf("relocation changed source repository, profile or destination: %+v", current)
			}
			updated, err := os.ReadFile(filepath.Join(copied, ".codex", "config.toml"))
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, outsideAfter, err := managedSlice(updated, configStart, configEnd)
			if err != nil || !bytes.Equal(outsideBefore, outsideAfter) {
				t.Fatal("historical foreign config changed during relocation", err)
			}
			if !reflect.DeepEqual(projectFiles(t, original), originalBefore) {
				t.Fatal("relocation changed original historical project")
			}
			retry, err := Init(Config{Root: copied, Binary: newBinary, SourceRoot: newSource, Codex: true})
			if err != nil || len(retry.Created) != 0 || len(retry.Updated) != 0 || len(retry.Unchanged) != 6 {
				t.Fatalf("historical config retry was not a no-op: %+v, %v", retry, err)
			}
		})
	}
}

func historicalConfigAddress(t *testing.T, raw []byte, prior, local managedConfig) []byte {
	t.Helper()
	addressed := bytes.Clone(raw)
	for _, pair := range [][2]string{{prior.Root, local.Root}, {prior.Binary, local.Binary}, {prior.SourceRoot, local.SourceRoot}} {
		oldJSON, err := json.Marshal(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		newJSON, err := json.Marshal(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Count(addressed, oldJSON) != 1 {
			t.Fatalf("historical address %q was not a unique generated field", pair[0])
		}
		addressed = bytes.Replace(addressed, oldJSON, newJSON, 1)
	}
	return addressed
}
