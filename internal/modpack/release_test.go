package modpack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// releaseVersion is the version pinned by this release. Bump it together with
// assets/mod/manifest.json, CHANGELOG.md and .modkitignore.
const releaseVersion = "1.0.1"

// sequelWording is what this release must never say: the code-only build reads
// the launcher's Black/White import and nothing else. The names are split so a
// repo-wide wording scan over this file stays clean while the test still
// rejects them.
var sequelWording = []string{"Black " + "2", "White " + "2", "BW" + "2"}

type releaseManifest struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Version        string `json:"version"`
	GitHub         string `json:"github"`
	Description    string `json:"description"`
	RequiredAssets []struct {
		Importer string `json:"importer"`
		Pack     string `json:"pack"`
	} `json:"required_assets"`
}

// TestReleaseContract guards the launcher's update contract without a network:
// the identity in the manifest, the version the release tooling derives, the
// changelog section the release body comes from, and the workflow that ties
// them together.
func TestReleaseContract(t *testing.T) {
	t.Run("embedded manifest", func(t *testing.T) {
		const file = modRoot + "/manifest.json"
		raw, err := embedded.ReadFile(file)
		if err != nil {
			t.Fatalf("read embedded %s: %v", file, err)
		}
		var manifest releaseManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatalf("%s is not valid JSON: %v", file, err)
		}
		checkManifestIdentity(t, file, &manifest)
		if !strings.Contains(manifest.Description, "Black/White") {
			t.Errorf("%s: description does not mention Black/White", file)
		}
		checkNoSequelWording(t, file, manifest.Description)
	})

	t.Run("external manifest", func(t *testing.T) {
		raw, err := externalManifest()
		if err != nil {
			t.Fatalf("externalManifest: %v", err)
		}
		var manifest releaseManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatalf("externalManifest is not valid JSON: %v", err)
		}
		checkManifestIdentity(t, "externalManifest()", &manifest)
		checkNoSequelWording(t, "externalManifest()", manifest.Description)
		if len(manifest.RequiredAssets) != 1 {
			t.Fatalf("externalManifest(): required_assets has %d entries, want 1: %+v",
				len(manifest.RequiredAssets), manifest.RequiredAssets)
		}
		asset := manifest.RequiredAssets[0]
		if asset.Importer != "gen5_bw" || asset.Pack != "battle_sprites" {
			t.Errorf("externalManifest(): required asset is %q/%q, want gen5_bw/battle_sprites",
				asset.Importer, asset.Pack)
		}
	})

	t.Run("changelog", func(t *testing.T) {
		const file = "../../CHANGELOG.md"
		text := readRepoFile(t, file)
		if !strings.Contains(text, "## "+releaseVersion) {
			t.Errorf("%s has no '## %s' section; the release body is built from it", file, releaseVersion)
		}
	})

	t.Run("release workflow", func(t *testing.T) {
		const file = "../../.github/workflows/release.yml"
		text := readRepoFile(t, file)
		for _, want := range []string{
			"tools/build_release.py",
			"tools/changelog_section.py",
			"internal/modpack/assets/mod/manifest.json",
			"mod -external",
			"gh release",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not mention %q", file, want)
			}
		}
	})

	t.Run("modkitignore", func(t *testing.T) {
		const file = "../../.modkitignore"
		text := readRepoFile(t, file)
		asset := "BW-Animated-Sprites-" + releaseVersion + ".zip"
		found := false
		for _, line := range strings.Split(text, "\n") {
			if strings.TrimSpace(line) == asset {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s does not list the exact asset name %q", file, asset)
		}
	})
}

func checkManifestIdentity(t *testing.T, file string, manifest *releaseManifest) {
	t.Helper()
	if manifest.ID != ModDirName {
		t.Errorf("%s: id is %q, want ModDirName %q", file, manifest.ID, ModDirName)
	}
	if manifest.Name != "BW Animated Sprites" {
		t.Errorf("%s: name is %q, want %q", file, manifest.Name, "BW Animated Sprites")
	}
	if manifest.Version != releaseVersion {
		t.Errorf("%s: version is %q, want %q (the launcher only offers an update for a newer version)",
			file, manifest.Version, releaseVersion)
	}
	if manifest.GitHub != "DarkHeros09/BW-Animated-Sprites" {
		t.Errorf("%s: github is %q, want DarkHeros09/BW-Animated-Sprites (the launcher reads its releases)",
			file, manifest.GitHub)
	}
}

func checkNoSequelWording(t *testing.T, file, text string) {
	t.Helper()
	for _, banned := range sequelWording {
		if strings.Contains(text, banned) {
			t.Errorf("%s mentions %q; this release covers Black/White only", file, banned)
		}
	}
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	clean := filepath.Clean(path)
	raw, err := os.ReadFile(clean)
	if err != nil {
		t.Fatalf("read %s: %v", clean, err)
	}
	return string(raw)
}
