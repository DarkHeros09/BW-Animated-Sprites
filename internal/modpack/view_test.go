package modpack

import (
	"image"
	"image/color"
	"testing"

	"bwsprites/internal/gen5"
	"bwsprites/internal/measure"
)

// TestPaintUsesTheFittedRow guards the vertical half of FitHost's placement:
// paint must write at at.Y, or every cell draws its art against the top edge and
// the sprite floats above the ground row the engine anchors it on.
func TestPaintUsesTheFittedRow(t *testing.T) {
	pal := color.Palette{
		color.NRGBA{},
		color.NRGBA{R: 255, A: 255},
		color.NRGBA{B: 255, A: 255},
	}
	// A 2x2 canvas with ink in the top-left and bottom-right pixels.
	c := &gen5.Canvas{Width: 2, Height: 2, Pixels: map[int]uint8{0: 1, 3: 2}}
	sheet := image.NewPaletted(image.Rect(0, 0, measure.HostBox, 2*measure.HostBox), pal)
	at := measure.HostFrame{X: 1, Y: 2, W: 2, H: 2}
	paint(c, at, 0, measure.HostBox, sheet)

	if got := sheet.ColorIndexAt(1, measure.HostBox+2); got != 1 {
		t.Errorf("cell 2 row 2 col 1 = %d, want 1", got)
	}
	if got := sheet.ColorIndexAt(2, measure.HostBox+3); got != 2 {
		t.Errorf("cell 2 row 3 col 2 = %d, want 2", got)
	}
	if got := sheet.ColorIndexAt(1, measure.HostBox); got != 0 {
		t.Errorf("cell 2 top row has ink at col 1 (at.Y was ignored)")
	}
}

// TestDexBoxPicksTheLargestPoseWindow keeps the dex picture at the size the
// largest pose earned. The window is fixed while the animation plays inside it,
// so a wing-down frame cannot shrink or swell the picture.
func TestDexBoxPicksTheLargestPoseWindow(t *testing.T) {
	frames := []measure.HostFrame{
		{X: 1, Y: 2, W: 40, H: 40},
		{X: 5, Y: 1, W: 70, H: 30},
		{X: 80, Y: 10, W: 62, H: 64},
		{X: 77, Y: 9, W: 62, H: 64},
	}
	x, y, w, h := dexBox(frames)
	if x != 80 || y != 10 || w != 62 || h != 64 {
		t.Fatalf("dex box = %d,%d %dx%d, want 80,10 62x64 (the first largest pose)", x, y, w, h)
	}
	if x, y, w, h := dexBox(nil); x != 0 || y != 0 || w != 0 || h != 0 {
		t.Fatalf("dex box of nothing = %d,%d %dx%d, want empty", x, y, w, h)
	}
}

// TestInkCanvasTrimsPadding checks the trim that keeps FitHost measuring ink:
// the compositor can pad a canvas with transparent rows, and placing the padded
// box would leave the sprite floating above row 63.
func TestInkCanvasTrimsPadding(t *testing.T) {
	c := &gen5.Canvas{
		Width: 4, Height: 3, MinX: -7, MinY: 10,
		Pixels: map[int]uint8{5: 3, 6: 4}, // (1,1) and (2,1)
	}
	got := inkCanvas(c)
	if got.Width != 2 || got.Height != 1 || got.MinX != -6 || got.MinY != 11 {
		t.Fatalf("trimmed to %dx%d at (%d,%d), want 2x1 at (-6,11)", got.Width, got.Height, got.MinX, got.MinY)
	}
	if got.Pixels[0] != 3 || got.Pixels[1] != 4 {
		t.Fatalf("pixels = %v, want the two opaque indices", got.Pixels)
	}
}
