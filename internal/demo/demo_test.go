package demo

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"bwsprites/internal/pack"
)

// writePack builds the demo pack in a fresh directory.
func writePack(t *testing.T) (string, *pack.Manifest) {
	t.Helper()
	dir := t.TempDir()
	manifest, err := Write(dir)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir, manifest
}

// TestWriteVerifies checks that a generated pack passes the same validator an
// imported pack has to pass.
func TestWriteVerifies(t *testing.T) {
	dir, manifest := writePack(t)
	if err := pack.Verify(dir); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if manifest.Importer != ImporterID {
		t.Errorf("importer %q, want %q", manifest.Importer, ImporterID)
	}
	if len(manifest.Entries) != 3 {
		t.Fatalf("%d entries, want 3", len(manifest.Entries))
	}
	if manifest.FrameSize != 0 {
		t.Errorf("manifest frame size %d, want 0 so each entry fits itself", manifest.FrameSize)
	}
}

// TestMeasurementLandsOnTheGround checks the facets placement in the written
// pixels, not just in the metadata: no frame may draw below the ground row and
// the lowest drawn row must reach it.
func TestMeasurementLandsOnTheGround(t *testing.T) {
	dir, manifest := writePack(t)
	for _, e := range manifest.Entries {
		lowest, below := drawnRows(t, dir, e)
		if below != -1 {
			t.Errorf("%s: pixel row %d is below the ground row %d", e.ID, below, e.Ground)
		}
		if e.Grounded && lowest != e.Ground {
			t.Errorf("%s: lowest drawn row %d, ground row %d", e.ID, lowest, e.Ground)
		}
		if e.Grounded && e.Union[3]-1 != e.Ground {
			t.Errorf("%s: union bottom %d, ground row %d", e.ID, e.Union[3]-1, e.Ground)
		}
	}
}

// TestHorizontalCentring checks the facets horizontal rule: the union is
// centred in the frame, within a pixel.
func TestHorizontalCentring(t *testing.T) {
	_, manifest := writePack(t)
	for _, e := range manifest.Entries {
		width := e.Union[2] - e.Union[0]
		slack := e.FrameSize - width - e.Union[0]*2
		if slack < -1 || slack > 1 {
			t.Errorf("%s: union %v in a %dpx frame is off centre by %d", e.ID, e.Union, e.FrameSize, slack)
		}
	}
}

// TestSingleFrameAndAutoFit covers the two shapes the pipeline has to survive:
// a one-frame animation and a sprite wider than the facets 64px cell.
func TestSingleFrameAndAutoFit(t *testing.T) {
	_, manifest := writePack(t)
	for _, e := range manifest.Entries {
		switch e.ID {
		case pack.IDFor(3, "front", "normal", false):
			if e.Frames != 1 {
				t.Errorf("%s: %d frames, want 1", e.ID, e.Frames)
			}
			if e.LoopStart != 0 {
				t.Errorf("%s: loop start %d, want 0", e.ID, e.LoopStart)
			}
			if e.FrameSize != 88 {
				t.Errorf("%s: frame size %d, want the 88px its content needs", e.ID, e.FrameSize)
			}
			if e.Union[2]-e.Union[0] != 88 {
				t.Errorf("%s: union width %d, want 88", e.ID, e.Union[2]-e.Union[0])
			}
		}
		if e.FrameSize < 64 {
			t.Errorf("%s: frame size %d is below the facets 64", e.ID, e.FrameSize)
		}
	}
}

// TestDeterministic checks that the same source draws the same bytes, so a
// regenerated pack can be diffed.
func TestDeterministic(t *testing.T) {
	first, _ := writePack(t)
	second, _ := writePack(t)
	for _, name := range []string{"manifest.json", "001/front/normal/strip.png"} {
		a, err := os.ReadFile(filepath.Join(first, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, err := os.ReadFile(filepath.Join(second, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if sha256.Sum256(a) != sha256.Sum256(b) {
			t.Errorf("%s: %s != %s", name, digest(a), digest(b))
		}
	}
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// drawnRows reports the lowest row any frame draws on and the lowest row drawn
// below the ground row, or -1 when nothing is.
func drawnRows(t *testing.T, root string, e *pack.Entry) (lowest, below int) {
	t.Helper()
	file, err := os.Open(filepath.Join(root, filepath.FromSlash(e.Strip)))
	if err != nil {
		t.Fatalf("%s: %v", e.Strip, err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("%s: %v", e.Strip, err)
	}
	if _, ok := img.(*image.Paletted); !ok {
		t.Fatalf("%s: decoded as %T, want a paletted png", e.Strip, img)
	}
	lowest, below = -1, -1
	for i := 0; i < e.Frames; i++ {
		cell := img.(*image.Paletted).SubImage(image.Rect(i*e.FrameSize, 0, (i+1)*e.FrameSize, e.FrameSize))
		for y := 0; y < e.FrameSize; y++ {
			for x := 0; x < e.FrameSize; x++ {
				_, _, _, alpha := cell.At(x, y).RGBA()
				if alpha == 0 {
					continue
				}
				lowest = max(lowest, y)
				if y > e.Ground {
					below = max(below, y)
				}
			}
		}
	}
	return lowest, below
}
