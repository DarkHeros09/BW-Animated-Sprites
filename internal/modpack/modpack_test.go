package modpack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModFiles(t *testing.T) {
	files, err := ModFiles()
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, f := range files {
		have[f] = true
	}
	for _, want := range []string{"manifest.json", "main.lua", "pack.lua", "external.lua"} {
		if !have[want] {
			t.Errorf("embedded mod is missing %s (have %v)", want, files)
		}
	}
}

func TestLuaQuote(t *testing.T) {
	quote := string(rune(34))
	slash := string(rune(92))
	toLuaQuote := func(s string) string {
		var b strings.Builder
		b.WriteString(quote)
		for _, r := range s {
			switch r {
			case 34, 92:
				b.WriteString(slash)
				b.WriteRune(r)
			case 10:
				b.WriteString(slash + "n")
			case 13:
				b.WriteString(slash + "r")
			case 9:
				b.WriteString(slash + "t")
			default:
				if r < 0x20 || r == 0x7f {
					b.WriteString(fmt.Sprintf("%s%03d", slash, r))
				} else {
					b.WriteRune(r)
				}
			}
		}
		b.WriteString(quote)
		return b.String()
	}
	cases := []string{
		"plain",
		"a" + quote + "b",
		"c:" + slash + "path",
		"line" + string(rune(10)) + "feed",
		"tab" + string(rune(9)) + "here",
		"bell" + string(rune(7)),
	}
	for _, in := range cases {
		want := toLuaQuote(in)
		if got := luaQuote(in); got != want {
			t.Errorf("luaQuote(%q) = %s, want %s", in, got, want)
		}
	}
	if got, want := luaQuote(string(rune(7))), quote+slash+"007"+quote; got != want {
		t.Errorf("a control character: %s, want %s", got, want)
	}
}

func TestIndexLuaShape(t *testing.T) {
	quote := string(rune(34))
	entry := &Entry{Dex: 25, Front: &View{
		Frames: 3, Size: 108, Columns: 2, InkW: 105, InkH: 88,
		Durations: []int{54, 12, 6}, LoopStart: 1,
		Normal: "front/25/normal.png", Shiny: "front/25/shiny.png",
	}, FrontFemale: &View{
		Frames: 2, Size: 64, Columns: 1, InkW: 40, InkH: 32,
		Durations: []int{5, 15}, LoopStart: 0,
		Normal: "front/female/25/normal.png",
	}}
	index := &indexFile{
		Source:  Source{Path: "Pokemon - Black.nds", Code: "IRBO", Name: "Pokemon Black", Importer: "gen5_bw", Digest: "sha256:00"},
		Entries: map[int]*Entry{25: entry},
	}
	text := index.lua()
	quoted := func(s string) string { return quote + s + quote }
	for _, want := range []string{
		"  format = 2,",
		"  importer = " + quoted("gen5_bw") + ",",
		"  tickRate = 60,",
		"  box = 64,",
		"  pad = 0,",
		"    [25] = {",
		"      front = " + quoted("front/25/normal.png") + ",",
		"      frontShiny = " + quoted("front/25/shiny.png") + ",",
		"      frontFrames = 3,",
		"      frontSize = 108,",
		"      frontColumns = 2,",
		"      frontInkH = 88,",
		"      frontDurations = {54, 12, 6},",
		"      frontLoopStart = 1,",
		"      frontFemale = " + quoted("front/female/25/normal.png") + ",",
		"      frontFemaleFrames = 2,",
		"      frontFemaleSize = 64,",
		"      frontFemaleDurations = {5, 15},",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("index.lua is missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "back = ") {
		t.Errorf("index.lua wrote a back view the entry does not have: %s", text)
	}
	if strings.Contains(text, "frontScaled") || strings.Contains(text, "tileColumns") {
		t.Errorf("index.lua wrote format-1 geometry: %s", text)
	}
	if again := index.lua(); again != text {
		t.Error("index.lua is not deterministic for the same entries")
	}
}

func writeTestPack(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "front", "1"), 0o755); err != nil {
		t.Fatal(err)
	}
	index := "return { format = 2 }" + string(rune(10))
	if err := os.WriteFile(filepath.Join(dir, "index.lua"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "front", "1", "normal.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAssembleWritesModAndPack(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "pack")
	writeTestPack(t, pack)
	out := filepath.Join(root, "mod")
	report, err := Assemble(pack, out, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.ModFiles != 4 || report.PackFiles != 2 {
		t.Fatalf("report %+v, want 4 mod files and 2 pack files", report)
	}
	for _, rel := range []string{"manifest.json", "main.lua", "pack.lua", "index.lua", "front/1/normal.png"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s was not written: %v", rel, err)
		}
	}
	got, err := os.ReadFile(filepath.Join(out, "index.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "return { format = 2 }"+string(rune(10)) {
		t.Errorf("the pack index was altered: %q", got)
	}
	if _, err := Assemble(pack, out, false); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("assembling over an existing directory: %v", err)
	}
	if _, err := Assemble(pack, out, true); err != nil {
		t.Fatalf("assembling with force: %v", err)
	}
}

func TestAssembleExternalWritesCodeOnlyMod(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "mod")
	report, err := AssembleExternal(out, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.ModFiles != 4 || report.PackFiles != 0 {
		t.Fatalf("report %+v, want 4 mod files and no pack files", report)
	}
	for _, rel := range []string{"manifest.json", "main.lua", "pack.lua", "external.lua"} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("%s was not written: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "index.lua")); !os.IsNotExist(err) {
		t.Errorf("the code-only mod embeds a pack index: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version        string `json:"version"`
		Description    string `json:"description"`
		RequiredAssets []struct {
			Importer string `json:"importer"`
			Pack     string `json:"pack"`
			Version  string `json:"version"`
		} `json:"required_assets"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("the generated manifest is not JSON: %v", err)
	}
	if manifest.Version != "1.15.3" {
		t.Errorf("version %q, want 1.15.3", manifest.Version)
	}
	if !strings.Contains(manifest.Description, "IMPORTERS") || !strings.Contains(manifest.Description, "ships no game art") {
		t.Errorf("the description does not explain the importer route: %q", manifest.Description)
	}
	if len(manifest.RequiredAssets) != 1 {
		t.Fatalf("required_assets %+v, want one entry", manifest.RequiredAssets)
	}
	asset := manifest.RequiredAssets[0]
	if asset.Importer != "gen5_bw" || asset.Pack != "battle_sprites" || asset.Version != ">=1.0.0" {
		t.Errorf("required_assets entry %+v, want gen5_bw/battle_sprites >=1.0.0", asset)
	}
	if _, err := AssembleExternal(out, false); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("assembling over an existing directory: %v", err)
	}
	if _, err := AssembleExternal(out, true); err != nil {
		t.Fatalf("assembling with force: %v", err)
	}
}

func TestAssembleRejectsBadInputs(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "nope")
	if _, err := Assemble(missing, filepath.Join(root, "mod"), false); err == nil ||
		!strings.Contains(err.Error(), "bwsprites modpack -rom") {
		t.Fatalf("a missing pack: %v", err)
	}
	empty := filepath.Join(root, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(empty, "index.lua"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Assemble(empty, filepath.Join(root, "mod"), false); err == nil ||
		!strings.Contains(err.Error(), "is empty") {
		t.Fatalf("an empty index: %v", err)
	}
	pack := filepath.Join(root, "pack")
	writeTestPack(t, pack)
	if _, err := Assemble(pack, filepath.Join(pack, "mod"), false); err == nil ||
		!strings.Contains(err.Error(), "inside the pack") {
		t.Fatalf("assembling into the pack: %v", err)
	}
}

func TestBuildRejectsInvalidOptions(t *testing.T) {
	out := filepath.Join(t.TempDir(), "pack")
	if _, err := Build(nil, out, Options{Sides: []string{"side"}}, true); err == nil ||
		!strings.Contains(err.Error(), "invalid side") {
		t.Fatalf("a bad side: %v", err)
	}
	if _, err := Build(nil, out, Options{Palettes: []string{"sepia"}}, true); err == nil ||
		!strings.Contains(err.Error(), "invalid palette") {
		t.Fatalf("a bad palette: %v", err)
	}
}
