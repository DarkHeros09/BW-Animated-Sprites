package modpack_test

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bwsprites/internal/gen5"
	"bwsprites/internal/measure"
	"bwsprites/internal/modpack"
)

// TestRealROMModPack checks the mod pack the installer ships against a dump the
// user supplies. It cannot be run here without the ROM, so it is gated:
//
//	BW_ROM=/path/to/black.nds go test ./internal/modpack -run TestRealROMModPack
func TestRealROMModPack(t *testing.T) {
	path := os.Getenv("BW_ROM")
	if path == "" {
		t.Skip("set BW_ROM to an untrimmed Black/White dump to check the mod pack against real data")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	rom, err := gen5.Identify(path)
	if err != nil {
		t.Fatalf("Identify: %v", err)
	}

	out := filepath.Join(t.TempDir(), "pack")
	dex := []int{1, 3, 6, 25, 150, 249, 382, 649}
	report, err := modpack.Build(rom, out, modpack.Options{
		Dex:      dex,
		Sides:    []string{"front", "back"},
		Palettes: []string{"normal", "shiny"},
	}, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	wantSheets := 0
	for _, d := range dex {
		for _, side := range []string{"front", "back"} {
			wantSheets += 2 // normal and shiny
			has, err := rom.HasFemale(d, side)
			if err != nil {
				t.Fatal(err)
			}
			if has {
				wantSheets += 2
			}
		}
	}
	if report.Entries != len(dex) || report.Sheets != wantSheets {
		t.Fatalf("report = %d entries, %d sheets; want %d and %d", report.Entries, report.Sheets, len(dex), wantSheets)
	}

	// Reading the dump must not modify it.
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("the ROM was modified by a build")
	}

	index, err := os.ReadFile(filepath.Join(out, "index.lua"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"  format = 2,",
		"  importer = \"gen5_bw\",",
		"  tickRate = 60,",
		"  box = 64,",
		"  pad = 0,",
		"frontSize = ",
		"frontColumns = ",
		"frontInkH = ",
		"sha256:",
		"\"IRB",
		"[649] = {",
	} {
		if !strings.Contains(string(index), want) {
			t.Errorf("index.lua does not contain %q", want)
		}
	}

	for _, d := range dex {
		for _, side := range []string{"front", "back"} {
			sprite, err := rom.Read(d, side, false)
			if err != nil {
				t.Fatalf("dex %d %s: %v", d, side, err)
			}
			pal, err := sprite.PaletteByName("normal")
			if err != nil {
				t.Fatalf("dex %d %s: %v", d, side, err)
			}
			timeline, err := sprite.Timeline(pal)
			if err != nil {
				t.Fatalf("dex %d %s: %v", d, side, err)
			}
			for _, palette := range []string{"normal", "shiny"} {
				sheet := filepath.Join(out, side, itoa(d), palette+".png")
				maxCell := measure.HostFrontWidth
				if side == "back" {
					maxCell = measure.HostBackMax
				}
				checkSheet(t, sheet, len(timeline.Poses), maxCell)
				// Charizard's back art is the reason the back cell may exceed 64:
				// at a facets 64x64 cell its waist-wide silhouette is only 53 px
				// tall, which reads as tiny next to a 64 px front. The back cell
				// must therefore keep the art's native size.
				if d == 6 && side == "back" {
					if size := sheetCellOf(t, sheet, len(timeline.Poses)); size < 96 {
						t.Errorf("Charizard back is a %d px cell, want its native size (at least 96)", size)
					}
				}
				// The opponent's front keeps its native Black/White pixels, so
				// Pidgeotto's moving wings are not resampled: Charizard's 89x91
				// art is a 92 px cell, Pikachu keeps its 51x46 in a 64 px cell,
				// Lugia's 153x92 is a 156 px cell, and Rayquaza's 98x82 a 100 px
				// cell. The 112/160 caps never bind.
				if side == "front" {
					size := sheetCellOf(t, sheet, len(timeline.Poses))
					for _, want := range []struct {
						dex, cell int
						why       string
					}{
						{6, 92, "its native 89x91 cell"},
						{25, measure.HostBox, "its native 64 px (no upscaling)"},
						{249, 156, "its native 153x92 cell"},
						{382, 100, "its native 98x82 cell"},
					} {
						if d == want.dex && size != want.cell {
							t.Errorf("dex %d front is a %d px cell, want %d for %s", d, size, want.cell, want.why)
						}
					}
				}
			}
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// sheetCellOf derives the cell side of one sheet.
func sheetCellOf(t *testing.T, path string, frames int) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return sheetCell(t, path, img, frames)
}

// checkSheet checks the geometry the mod relies on: the cell side is derived
// from the sheet itself (64..160, step 4), is never past the side's cap, every
// frame has ink, the lowest ink row across the animation is the cell's last row,
// and the ink centre sits on the centre column.
func checkSheet(t *testing.T, path string, frames, maxCell int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	size := sheetCell(t, path, img, frames)
	if size > maxCell {
		t.Errorf("%s uses %d px cells, past the %d px cap", path, size, maxCell)
	}
	cols := img.Bounds().Dx() / size
	union := image.Rectangle{}
	for i := 0; i < frames; i++ {
		col, row := i%cols, i/cols
		box := image.Rect(col*size, row*size, (col+1)*size, (row+1)*size)
		bounds, ok := inkBounds(img, box)
		if !ok {
			t.Fatalf("%s frame %d has no ink", path, i)
		}
		// inkBounds works in sheet coordinates; the geometry is per cell.
		bounds = bounds.Sub(image.Pt(col*size, row*size))
		union = union.Union(bounds)
	}
	if want := size - 1; union.Max.Y-1 != want {
		t.Errorf("%s (%d px cells) lowest ink row across the animation = %d, want %d",
			path, size, union.Max.Y-1, want)
	}
	centre, want := float64(union.Min.X+union.Max.X)/2, float64(size)/2
	if centre < want-1 || centre > want+1 {
		t.Errorf("%s ink centre = %.1f, want %.0f±1 (union %v)", path, centre, want, union)
	}
	if union.Min.X < 0 || union.Min.Y < 0 || union.Max.X > size || union.Max.Y > size {
		t.Errorf("%s ink %v leaves its %d px cell", path, union, size)
	}
}

// sheetCell finds the one cell side in 64..160 (step 4) that turns the sheet
// dimensions into the columns/rows the builder could have used for frames.
func sheetCell(t *testing.T, path string, img image.Image, frames int) int {
	t.Helper()
	if frames < 1 {
		t.Fatalf("%s: %d frames", path, frames)
	}
	dx, dy := img.Bounds().Dx(), img.Bounds().Dy()
	found := 0
	count := 0
	for size := 64; size <= 160; size += 4 {
		if dx%size != 0 || dy%size != 0 {
			continue
		}
		cols, rows := dx/size, dy/size
		if cols < 1 || cols > frames {
			continue
		}
		if rows != (frames+cols-1)/cols {
			continue
		}
		found = size
		count++
	}
	if count != 1 {
		t.Fatalf("%s is %dx%d for %d frames: %d possible cell sides, want exactly 1",
			path, dx, dy, frames, count)
	}
	return found
}

// inkBounds returns the box of non-transparent pixels inside cell, in sheet rows.
func inkBounds(img image.Image, cell image.Rectangle) (image.Rectangle, bool) {
	b := image.Rectangle{}
	for y := cell.Min.Y; y < cell.Max.Y; y++ {
		for x := cell.Min.X; x < cell.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			if b.Empty() {
				b = image.Rect(x, y, x+1, y+1)
				continue
			}
			if x < b.Min.X {
				b.Min.X = x
			}
			if y < b.Min.Y {
				b.Min.Y = y
			}
			if x+1 > b.Max.X {
				b.Max.X = x + 1
			}
			if y+1 > b.Max.Y {
				b.Max.Y = y + 1
			}
		}
	}
	return b, !b.Empty()
}
