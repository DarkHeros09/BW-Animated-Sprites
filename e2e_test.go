// Package e2e checks the two halves of the project against each other: the Go
// tool writes a pack, the LOVE previewer reads the same pack and re-checks every
// measurement it finds.
package e2e

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bwsprites/internal/demo"
	"bwsprites/internal/modpack"
)

// lovec finds the LOVE console runner, or reports that this host has none.
func lovec(t *testing.T) string {
	t.Helper()
	if from, ok := os.LookupEnv("LOVE"); ok && from != "" {
		return from
	}
	if from, err := exec.LookPath("lovec"); err == nil {
		return from
	}
	for _, guess := range []string{
		`C:\Program Files\LOVE\lovec.exe`,
		`C:\Program Files (x86)\LOVE\lovec.exe`,
	} {
		if _, err := os.Stat(guess); err == nil {
			return guess
		}
	}
	t.Skip("LOVE is not installed here; run 'bwsprites demo -out demo/pack' then 'lovec love demo/pack --selftest' to check it by hand")
	return ""
}

// TestDemoPackPassesTheLOVESelfTest is the format agreement test: the demo pack
// the Go side writes must load, validate and cycle in the Lua side.
func TestDemoPackPassesTheLOVESelfTest(t *testing.T) {
	out := filepath.Join(t.TempDir(), "pack")
	manifest, err := demo.Write(out)
	if err != nil {
		t.Fatalf("demo.Write: %v", err)
	}

	cmd := exec.Command(lovec(t), "love", out, "--selftest")
	output, err := cmd.CombinedOutput()
	text := string(output)
	if err != nil {
		t.Fatalf("love --selftest: %v\n%s", err, text)
	}
	if !strings.Contains(text, "selftest: OK") {
		t.Fatalf("love --selftest did not report OK:\n%s", text)
	}
	for _, entry := range manifest.Entries {
		if !strings.Contains(text, "PASS  "+entry.ID) {
			t.Errorf("the LOVE selftest never checked %s:\n%s", entry.ID, text)
		}
	}
}

// TestMissingPackFailsLoudly checks the previewer's error path, so a user who
// launches it before building a pack is told what to run.
func TestMissingPackFailsLoudly(t *testing.T) {
	empty := t.TempDir()
	cmd := exec.Command(lovec(t), "love", filepath.Join(empty, "nope"), "--selftest")
	output, _ := cmd.CombinedOutput()
	text := string(output)
	if !strings.Contains(text, "cannot open") && !strings.Contains(text, "no pack") {
		t.Fatalf("no actionable message for a missing pack:\n%s", text)
	}
}

// modPackIndex is the synthetic pack the mod harness reads: species 25 has both
// views with a native 68px front cell, 26 points at a sheet that does not
// exist, 27 points at a sheet of the wrong size, 28 is a tall 80px cell that
// needs the on-screen clamp, 29 declares an 80px cell for a 64px sheet, 30 is
// the wide player back cell a fixed-origin engine has to centre by hand, 31 is
// the native 72px opponent front that fills the whole enemy slot, 32 is the
// 120px wide opponent front that only the rectangular slot can serve, 33 is a
// native 96px player back whose top must clear the enemy healthbox, and 25
// also carries distinct 64px female front and back views with their own image.
// In each cell the ink ends on the last row, as the builder places it, so the
// harness can check the cell geometry too.
const modPackIndex = `return {
  format = 2,
  importer = "gen5_bw",
  tickRate = 60,
  box = 64,
  pad = 0,
  source = { path = "synthetic", code = "DEMO", name = "synthetic harness pack", digest = "synthetic" },
  entries = {
    [25] = {
      front = "front/25/normal.png", frontFrames = 3, frontSize = 68, frontColumns = 1,
      frontInkH = 16, frontDex = { x = 22, y = 48, w = 24, h = 20 },
      frontDurations = { 10, 10, 10 }, frontLoopStart = 1,
      frontFemale = "front/female/25/normal.png", frontFemaleFrames = 2,
      frontFemaleSize = 64, frontFemaleColumns = 1, frontFemaleInkH = 16,
      frontFemaleDurations = { 5, 5 }, frontFemaleLoopStart = 0,
      back = "back/25/normal.png", backFrames = 2, backSize = 64, backColumns = 1,
      backInkH = 16, backDurations = { 15, 15 }, backLoopStart = 0,
      backFemale = "back/female/25/normal.png", backFemaleFrames = 2,
      backFemaleSize = 64, backFemaleColumns = 1, backFemaleInkH = 16,
      backFemaleDurations = { 5, 5 }, backFemaleLoopStart = 0,
    },
    [26] = { front = "front/26/missing.png", frontFrames = 2, frontSize = 64,
      frontColumns = 1, frontInkH = 16, frontDurations = { 10, 10 }, frontLoopStart = 0 },
    [27] = { front = "front/27/normal.png", frontFrames = 1, frontSize = 80,
      frontColumns = 1, frontInkH = 16, frontDurations = { 10 }, frontLoopStart = 0 },
    [28] = { front = "front/28/normal.png", frontFrames = 1, frontSize = 80,
      frontColumns = 1, frontInkH = 72, frontDurations = { 10 }, frontLoopStart = 0 },
    [29] = { front = "front/27/normal.png", frontFrames = 1, frontSize = 80,
      frontColumns = 1, frontInkH = 16, frontDurations = { 10 }, frontLoopStart = 0 },
    [30] = { back = "back/30/normal.png", backFrames = 1, backSize = 80, backColumns = 1,
      backInkH = 72, backDurations = { 10 }, backLoopStart = 0 },
    [31] = { front = "front/31/normal.png", frontFrames = 1, frontSize = 72,
      frontColumns = 1, frontInkH = 72, frontDurations = { 10 }, frontLoopStart = 0 },
    [32] = { front = "front/32/normal.png", frontFrames = 1, frontSize = 120,
      frontColumns = 1, frontInkH = 72, frontDurations = { 10 }, frontLoopStart = 0 },
    [33] = { back = "back/33/normal.png", backFrames = 1, backSize = 96, backColumns = 1,
      backInkH = 96, backDurations = { 10 }, backLoopStart = 0 },
  },
}
`

// writeModPackFixture writes a hand-built pack, independent of the ROM importer,
// so the mod's loader is tested on its own.
func writeModPackFixture(t *testing.T, dir string) {
	t.Helper()
	// strip writes one view as size x size*frames square cells, columns=1. The
	// ink starts at inkTop and ends on the cell's last row, where the builder
	// places the union's lowest row.
	strip := func(rel string, size, frames, inkTop int) {
		img := image.NewRGBA(image.Rect(0, 0, size, size*frames))
		for f := 0; f < frames; f++ {
			for y := inkTop; y < size; y++ {
				for x := size/2 - 12; x < size/2+12; x++ {
					img.SetRGBA(x, f*size+y, color.RGBA{R: uint8(40 * (f + 1)), G: 200, B: 60, A: 255})
				}
			}
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("encode %s: %v", rel, err)
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	strip("front/25/normal.png", 68, 3, 48)
	strip("front/female/25/normal.png", 64, 2, 40)
	strip("back/25/normal.png", 64, 2, 44)
	strip("back/female/25/normal.png", 64, 2, 36)
	strip("front/27/normal.png", 64, 1, 44)
	strip("front/28/normal.png", 80, 1, 4)
	strip("back/30/normal.png", 80, 1, 8)
	strip("front/31/normal.png", 72, 1, 0)
	strip("front/32/normal.png", 120, 1, 48)
	strip("back/33/normal.png", 96, 1, 0)
	if err := os.WriteFile(filepath.Join(dir, "index.lua"), []byte(modPackIndex), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestModHarnessPassesTheLuaChecks runs the mod's real pack.lua and main.lua
// against the synthetic pack and stubbed engine modules. It is the end-to-end
// agreement test for the mod: parsing, frame timing, fallbacks for missing and
// malformed sheets, the optional picture wrap, the cell-size offset
// correction, the OPTION rows under both menu generations, and the mon_anim
// bypass are all checked from Lua.
func TestModHarnessPassesTheLuaChecks(t *testing.T) {
	love := lovec(t)
	root := t.TempDir()
	packDir := filepath.Join(root, "pack")
	writeModPackFixture(t, packDir)

	modDir := filepath.Join(loveIdentityDir(t, "bwsprites-harness"), "mods", "bwsprites")
	t.Cleanup(func() { os.RemoveAll(modDir) })
	if err := os.MkdirAll(filepath.Dir(modDir), 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := modpack.Assemble(packDir, modDir, true)
	if err != nil {
		t.Fatalf("assemble the mod: %v", err)
	}
	if report.ModFiles == 0 || report.PackFiles != 11 {
		t.Fatalf("unexpected assemble report: %+v", report)
	}

	harness := filepath.Join(root, "harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"conf.lua", "main.lua"} {
		source, err := os.ReadFile(filepath.Join("internal", "modpack", "testdata", "harness", name))
		if err != nil {
			t.Fatalf("read harness %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(harness, name), source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bad, err := filepath.Abs(filepath.Join("internal", "modpack", "testdata", "bad"))
	if err != nil {
		t.Fatal(err)
	}

	// Both engine anchors are checked: a native cell wider than 64 lands off
	// centre under the fixed (32,32) origin unless the mod shifts the draw, while
	// PIC_CENTRE does that itself. The newer engine replaced the option_rows group
	// registry with one OptionsMenu, so the centre-anchored engine is checked
	// under both menu generations.
	for _, mode := range []struct{ name, centred, newmenu string }{
		{"fixed-origin", "0", "0"},
		{"centre-anchored", "1", "0"},
		{"centre-anchored-new-options", "1", "1"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			cmd := exec.Command(love, harness)
			cmd.Env = envWith(map[string]string{
				"MOD_DIR":  modDir,
				"SAVE_MOD": "mods/bwsprites",
				"BAD_DIR":  bad,
				"CENTRED":  mode.centred,
				"NEWMENU":  mode.newmenu,
			})
			output, runErr := cmd.CombinedOutput()
			text := string(output)
			if testing.Verbose() {
				t.Log(text)
			}
			if runErr != nil {
				t.Fatalf("the mod harness failed: %v; output: %s", runErr, text)
			}
			if !strings.Contains(text, "selftest: OK") {
				t.Fatalf("the mod harness did not report OK; output: %s", text)
			}
			if strings.Contains(text, "FAIL ") {
				t.Fatalf("the mod harness reported failures; output: %s", text)
			}
		})
	}
}

// envWith returns the current environment with the given variables replaced.
// Duplicate keys are avoided because the child's lookup order is platform
// dependent, and the harness must see exactly one value per switch.
func envWith(overrides map[string]string) []string {
	out := make([]string, 0, len(os.Environ())+len(overrides))
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if _, ok := overrides[key]; ok {
			continue
		}
		out = append(out, kv)
	}
	for key, value := range overrides {
		out = append(out, key+"="+value)
	}
	return out
}

// loveIdentityDir mirrors where the engine keeps a save identity. The mod
// harness reads its sheets through LÖVE's filesystem, which only resolves paths
// inside the game and save directories, so the assembled mod is installed under
// the harness identity exactly where a real install would put it.
func loveIdentityDir(t *testing.T, identity string) string {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			t.Skip("APPDATA is not set, cannot locate the LÖVE save directory")
		}
		return filepath.Join(base, "LOVE", identity)
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("cannot locate the home directory: %v", err)
		}
		return filepath.Join(home, "Library", "Application Support", "LOVE", identity)
	default:
		if base := os.Getenv("XDG_DATA_HOME"); base != "" {
			return filepath.Join(base, "love", identity)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("cannot locate the home directory: %v", err)
		}
		return filepath.Join(home, ".local", "share", "love", identity)
	}
}
