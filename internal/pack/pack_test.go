package pack

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func palette() color.Palette {
	return color.Palette{color.NRGBA{}, color.NRGBA{R: 0xff}}
}

func TestWriteEntryWritesStripAndSidecar(t *testing.T) {
	root := t.TempDir()
	size := 8
	frames := []*image.Paletted{
		image.NewPaletted(image.Rect(0, 0, size, size), palette()),
		image.NewPaletted(image.Rect(0, 0, size, size), palette()),
	}
	frames[0].SetColorIndex(3, 7, 1)
	frames[1].SetColorIndex(4, 7, 1)
	e := &Entry{ID: "001/front/normal", FrameSize: size, Ground: 7, Grounded: true,
		Frames: 2, Offsets: [][2]int{{2, 3}, {3, 3}}, Durations: []int{4, 6}, CycleTicks: 10,
		Union: [4]int{2, 3, 5, 8}, Palette: "normal", Side: "front", Dex: 1}
	if err := WriteEntry(root, e, frames); err != nil {
		t.Fatal(err)
	}
	if e.Strip != "001/front/normal/strip.png" {
		t.Fatalf("strip = %q", e.Strip)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(e.ID), "entry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var back Entry
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatalf("entry.json is not valid json: %v", err)
	}
	if back.Ground != 7 || !back.Grounded || back.CycleTicks != 10 {
		t.Fatalf("round trip lost measurement: %+v", back)
	}

	// Nothing may be left behind except the strip and the sidecar.
	names, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(e.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		var list []string
		for _, n := range names {
			list = append(list, n.Name())
		}
		t.Fatalf("entry dir holds %v, want strip.png and entry.json", list)
	}
}

func TestWriteEntryRejectsMismatchedFrames(t *testing.T) {
	root := t.TempDir()
	frames := []*image.Paletted{image.NewPaletted(image.Rect(0, 0, 8, 8), palette())}
	e := &Entry{ID: "001/front/normal", FrameSize: 8, Frames: 1, Offsets: [][2]int{{0, 0}, {0, 0}}, Durations: []int{4}}
	err := WriteEntry(root, e, frames)
	if err == nil || !strings.Contains(err.Error(), "1 frames, 1 durations and 2 offsets") {
		t.Fatalf("WriteEntry: %v", err)
	}
}

func TestWriteEntryRejectsFrameOfTheWrongSize(t *testing.T) {
	root := t.TempDir()
	frames := []*image.Paletted{image.NewPaletted(image.Rect(0, 0, 8, 8), palette())}
	e := &Entry{ID: "001/front/normal", FrameSize: 16, Frames: 1, Offsets: [][2]int{{0, 0}}, Durations: []int{4}}
	err := WriteEntry(root, e, frames)
	if err == nil || !strings.Contains(err.Error(), "want 16x16") {
		t.Fatalf("WriteEntry: %v", err)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		m       Manifest
		wantErr string
	}{
		{"ok", Manifest{Format: 1, FrameSize: 64, TickRate: 60}, ""},
		{"auto-fit", Manifest{Format: 1, FrameSize: 0, TickRate: 60}, ""},
		{"old format", Manifest{Format: 2, FrameSize: 64, TickRate: 60}, "want 1 (rebuild the pack)"},
		{"negative size", Manifest{Format: 1, FrameSize: -8, TickRate: 60}, "outside 0..256"},
		{"huge size", Manifest{Format: 1, FrameSize: 512, TickRate: 60}, "outside 0..256"},
		{"zero tick rate", Manifest{Format: 1, FrameSize: 64, TickRate: 0}, "tick rate 0"},
	}
	for _, c := range cases {
		err := c.m.Validate()
		if c.wantErr == "" {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: %v, want %q", c.name, err, c.wantErr)
		}
	}
}

func TestIDFor(t *testing.T) {
	if got := IDFor(3, "back", "shiny", false); got != "003/back/shiny" {
		t.Fatalf("IDFor = %q", got)
	}
	if got := IDFor(25, "front", "normal", true); got != "025/front/female/normal" {
		t.Fatalf("IDFor = %q", got)
	}
}
