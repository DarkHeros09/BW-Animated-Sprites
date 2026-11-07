package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bwsprites/internal/modpack"
)

// TestModImagesSurviveCacheEviction pins the ownership rule that caused the
// double-battle crash in anime_vm.lua:722. The engine keeps the Image the mod
// returned in a sprite or particle object for several frames. When the mod's
// LRU cache drops that entry it may forget its Lua reference, but it must not
// call Image:release(): the next engine draw would fail with "Cannot use object
// after it has been released". The Lua harness drives the real external.lua,
// holds the first picture of each cache, fills the cache until that entry is
// evicted and draws the held picture again.
func TestModImagesSurviveCacheEviction(t *testing.T) {
	love := lovec(t)
	modDir := filepath.Join(t.TempDir(), "mod")
	report, err := modpack.AssembleExternal(modDir, true)
	if err != nil {
		t.Fatalf("assemble the external mod: %v", err)
	}
	if report.ModFiles != 4 {
		t.Fatalf("assembled %d mod files, want external.lua, main.lua, pack.lua and manifest.json", report.ModFiles)
	}

	harness := filepath.Join("internal", "modpack", "testdata", "release-after-release")
	cmd := exec.Command(love, harness)
	cmd.Env = envWith(map[string]string{"MOD_DIR": modDir})
	output, err := cmd.CombinedOutput()
	text := string(output)
	if testing.Verbose() {
		t.Log(text)
	}
	if err != nil {
		t.Fatalf("the cache-eviction harness failed: %v; output: %s", err, text)
	}
	// Each held picture must survive the eviction of its own cache entry; the
	// assertions above the draws in the harness also prove the entry really went.
	for _, want := range []string{
		"a battle frame held by the engine survives a cache eviction",
		"a Pokedex picture held by the engine survives a cache eviction",
		"a 64x64 box picture held by the engine survives a cache eviction",
		"a composed part cell held by the engine survives a cache eviction",
		"a composed part image held by the engine survives a cache eviction",
	} {
		if !strings.Contains(text, "PASS  "+want) {
			t.Errorf("the harness did not check %q; output: %s", want, text)
		}
	}
	if !strings.Contains(text, "selftest: OK") {
		t.Fatalf("the cache-eviction harness did not report OK; output: %s", text)
	}
	if strings.Contains(text, "FAIL ") {
		t.Fatalf("the cache-eviction harness reported failures; output: %s", text)
	}
}
