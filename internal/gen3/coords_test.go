package gen3

import (
	"os"
	"path/filepath"
	"testing"
)

// romWithTable builds a ROM-sized buffer holding a table at 0x100.
func romWithTable(records ...byte) []byte {
	rom := make([]byte, 0x200)
	copy(rom[0x100:], records)
	return rom
}

func TestParseSizesAndOffsets(t *testing.T) {
	// slot 0: 64x32, yo 5; slot 1: empty; slot 2: 8x8, yo 63; slot 3: 64x64, yo 0.
	// The size byte packs the width in its upper nibble and the height in its
	// lower one, both in 8px units.
	rom := romWithTable(
		0x84, 5, 0, 0,
		0x00, 0, 0, 0,
		0x11, 63, 0, 0,
		0x88, 0, 0, 0,
	)
	got, err := Parse(rom, 0x100, 4, 4)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Coords{
		{Width: 64, Height: 32, GroundOffset: 5},
		{Empty: true},
		{Width: 8, Height: 8, GroundOffset: 63},
		{Width: 64, Height: 64},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseRejectsBadRanges(t *testing.T) {
	rom := romWithTable(0x44, 5, 0, 0)
	cases := []struct {
		name                  string
		offset, count, stride int
	}{
		{"negative offset", -1, 1, 4},
		{"zero count", 0x100, 0, 4},
		{"stride below two bytes", 0x100, 1, 1},
		{"table past the end", 0x1FC, 4, 4},
	}
	for _, c := range cases {
		if _, err := Parse(rom, c.offset, c.count, c.stride); err == nil {
			t.Errorf("%s: Parse accepted offset %d count %d stride %d", c.name, c.offset, c.count, c.stride)
		}
	}
}

func TestFileForUsesTheRightSide(t *testing.T) {
	f := &File{
		Front: []Coords{{Width: 64, Height: 64, GroundOffset: 4}, {Empty: true}},
		Back:  []Coords{{Width: 32, Height: 32, GroundOffset: 9}},
	}
	front, ok := f.For("front", 0)
	if !ok || front.GroundOffset != 4 {
		t.Errorf("front slot 0 = %+v, %v", front, ok)
	}
	back, ok := f.For("back", 0)
	if !ok || back.GroundOffset != 9 {
		t.Errorf("back slot 0 = %+v, %v", back, ok)
	}
	if _, ok := f.For("front", 1); ok {
		t.Error("empty slot reported as present")
	}
	if _, ok := f.For("front", 2); ok {
		t.Error("slot past the end reported as present")
	}
	if _, ok := f.For("front", -1); ok {
		t.Error("negative slot reported as present")
	}
}

func TestFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "coords.json")
	want := &File{
		Game:   "emerald",
		Source: "synthetic",
		Stride: 4,
		Front:  []Coords{{Width: 64, Height: 64, GroundOffset: 4}, {Empty: true}},
		Back:   []Coords{{Width: 64, Height: 48, GroundOffset: 7}},
	}
	if err := want.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Game != want.Game || len(got.Front) != 2 || got.Back[0].GroundOffset != 7 {
		t.Errorf("round trip = %+v", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("Save left %d files behind, want just coords.json", len(entries))
	}
	if err := (&File{Format: 99}).Save(filepath.Join(dir, "bad.json")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := Load(filepath.Join(dir, "bad.json")); err == nil {
		t.Error("Load accepted a foreign format")
	}
}
