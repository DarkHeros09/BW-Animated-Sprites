package measure

import "testing"

func hostFrames(boxes ...Rect) []Frame {
	frames := make([]Frame, len(boxes))
	for i, r := range boxes {
		frames[i] = Frame{Duration: 1, Bounds: r}
	}
	return frames
}

// TestFitHostKeepsSmallArtAtNativeSize checks the Facets rule: art smaller than
// 64x64 is neither enlarged nor padded at the bottom; it is centred on the
// column and its lowest ink row lands on the cell's last row. The front and the
// back side agree for small art because both floor the cell at 64.
func TestFitHostKeepsSmallArtAtNativeSize(t *testing.T) {
	for _, caps := range [][2]int{{HostFrontWidth, HostFrontMax}, {HostBackMax, HostBackMax}} {
		fit, err := FitHost(hostFrames(Rect{MinX: 10, MinY: 5, MaxX: 50, MaxY: 43}), caps[0], caps[1], 0)
		if err != nil {
			t.Fatal(err)
		}
		if fit.Scaled || fit.Size != HostBox || fit.InkW != 40 || fit.InkH != 38 {
			t.Fatalf("caps %v: cell %d ink %dx%d scaled %v, want 64 with 40x38 ink",
				caps, fit.Size, fit.InkW, fit.InkH, fit.Scaled)
		}
		got := fit.Frames[0]
		if got.X != 12 || got.Y != 26 || got.W != 40 || got.H != 38 {
			t.Fatalf("caps %v: placed %+v, want 12,26,40,38", caps, got)
		}
		if last := got.Y + got.H - 1; last != HostBox-1 {
			t.Fatalf("caps %v: lowest ink row %d, want the cell's last row %d", caps, last, HostBox-1)
		}
		if cx := got.X + got.W/2; cx != HostBox/2 {
			t.Fatalf("caps %v: ink centre at %d, want the host centre %d", caps, cx, HostBox/2)
		}
	}
}

// TestFitHostKeepsIntraAnimationMotion checks that frames keep their
// sprite-space offset inside the union cell instead of each being centred:
// a 20 px lift in the source must stay a 20 px lift on screen.
func TestFitHostKeepsIntraAnimationMotion(t *testing.T) {
	fit, err := FitHost(hostFrames(
		Rect{MinX: 0, MinY: 0, MaxX: 20, MaxY: 40},
		Rect{MinX: 0, MinY: 20, MaxX: 20, MaxY: 40},
	), HostFrontWidth, HostFrontMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := fit.Frames[1].Y - fit.Frames[0].Y; d != 20 {
		t.Fatalf("frame offsets %d and %d do not preserve the 20px lift",
			fit.Frames[0].Y, fit.Frames[1].Y)
	}
	if fit.Frames[0].Y != 24 || fit.Frames[1].Y != 44 {
		t.Fatalf("placements %+v // %+v, want tops 24 and 44", fit.Frames[0], fit.Frames[1])
	}
	if fit.Size != HostBox {
		t.Fatalf("cell side %d, want %d", fit.Size, HostBox)
	}
}

// TestFitHostKeepsCharizardBackNative is the user-facing case: a 105x88 back
// union must NOT be squeezed into 64x64 on the player's side. It keeps its
// native pixels in a 108 px cell, centred, feet on the last row.
func TestFitHostKeepsCharizardBackNative(t *testing.T) {
	fit, err := FitHost(hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 105, MaxY: 88}), HostBackMax, HostBackMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fit.Scaled || fit.Size != 108 || fit.InkW != 105 || fit.InkH != 88 {
		t.Fatalf("cell %d ink %dx%d scaled %v, want a native 108 px cell with 105x88 ink",
			fit.Size, fit.InkW, fit.InkH, fit.Scaled)
	}
	got := fit.Frames[0]
	if got.X != 1 || got.Y != 20 || got.W != 105 || got.H != 88 {
		t.Fatalf("placed %+v, want 1,20,105,88", got)
	}
	if last := got.Y + got.H - 1; last != fit.Size-1 {
		t.Fatalf("lowest ink row %d, want %d", last, fit.Size-1)
	}
	if d := abs(got.X + got.W/2 - fit.Size/2); d > 1 {
		t.Fatalf("ink centre %d is %d px from the cell centre %d", got.X+got.W/2, d, fit.Size/2)
	}
}

// TestFitHostKeepsLargeFrontNative is the enemy-side requirement: a 105x88
// union keeps every Black/White pixel in a 108 px cell instead of being
// resampled to the 72 px gap above the enemy ground row. At runtime a cell
// taller than that gap is top-anchored on row 0 and runs behind the menu.
func TestFitHostKeepsLargeFrontNative(t *testing.T) {
	fit, err := FitHost(hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 105, MaxY: 88}), HostFrontWidth, HostFrontMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fit.Scaled || fit.Size != 108 || fit.InkW != 105 || fit.InkH != 88 {
		t.Fatalf("cell %d ink %dx%d scaled %v, want a native 108 px cell with 105x88 ink",
			fit.Size, fit.InkW, fit.InkH, fit.Scaled)
	}
	got := fit.Frames[0]
	if got.X != 1 || got.Y != 20 || got.W != 105 || got.H != 88 {
		t.Fatalf("placed %+v, want 1,20,105,88", got)
	}
	if last := got.Y + got.H - 1; last != fit.Size-1 {
		t.Fatalf("lowest ink row %d, want the cell's last row %d", last, fit.Size-1)
	}
}

// TestFitHostKeepsPidgeottoNative is the reported species: Pidgeotto's wing
// motion makes an 82x101 union, which the old 72 px cap shrank to a 58 px body.
// It now keeps native pixels in a 104 px cell and shifts only by its frame
// offset.
func TestFitHostKeepsPidgeottoNative(t *testing.T) {
	fit, err := FitHost(hostFrames(
		Rect{MinX: 0, MinY: 20, MaxX: 82, MaxY: 101},
		Rect{MinX: 4, MinY: 0, MaxX: 82, MaxY: 58},
	), HostFrontWidth, HostFrontMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fit.Scaled || fit.Size != 104 || fit.InkW != 82 || fit.InkH != 101 {
		t.Fatalf("cell %d ink %dx%d scaled %v, want a native 104 px cell with 82x101 ink",
			fit.Size, fit.InkW, fit.InkH, fit.Scaled)
	}
	if got := fit.Frames[1]; got.X != 15 || got.Y != 3 || got.W != 78 || got.H != 58 {
		t.Fatalf("the raised frame is %+v, want an unscaled 78x58 at 15,3", got)
	}
}

// TestFitHostKeepsWideFrontWidth checks the other native dimension: a 153x92
// Lugia-like union keeps all 153 px instead of losing a third to a width cap.
func TestFitHostKeepsWideFrontWidth(t *testing.T) {
	fit, err := FitHost(hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 153, MaxY: 92}), HostFrontWidth, HostFrontMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fit.Scaled || fit.Size != 156 || fit.InkW != 153 || fit.InkH != 92 {
		t.Fatalf("cell %d ink %dx%d scaled %v, want a native 156 px cell with 153x92 ink",
			fit.Size, fit.InkW, fit.InkH, fit.Scaled)
	}
	if got := fit.Frames[0]; got.X != 1 || got.Y != 64 || got.W != 153 || got.H != 92 {
		t.Fatalf("placed %+v, want 1,64,153,92", got)
	}
}

// TestFitHostUpscalesToTheFacetsTarget is the size complaint: Black/White
// Lugia's tallest frame is 40 px tall where facets' DS Lugia is 64 px tall, so
// a 64 px target grows it by 1.6 without touching the cell height cap.
func TestFitHostUpscalesToTheFacetsTarget(t *testing.T) {
	fit, err := FitHost(hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 45, MaxY: 40}), HostFrontWidth, HostFrontMax, 64)
	if err != nil {
		t.Fatal(err)
	}
	if fit.Scaled || fit.Size != 72 || fit.InkW != 72 || fit.InkH != 64 {
		t.Fatalf("cell %d ink %dx%d scaled %v, want a 72 px cell with 72x64 ink",
			fit.Size, fit.InkW, fit.InkH, fit.Scaled)
	}
	if got := fit.Frames[0]; got.X != 0 || got.Y != 8 || got.W != 72 || got.H != 64 {
		t.Fatalf("placed %+v, want 0,8,72,64", got)
	}
}

// TestFitHostTargetStopsAtTheUnionCap checks that the target cannot push the
// animation's vertical motion out of the enemy slot: Moltres' 58 px union caps
// the 62 px target at 72/58, so the frame still has its feet on row 71.
func TestFitHostTargetStopsAtTheUnionCap(t *testing.T) {
	fit, err := FitHost(hostFrames(
		Rect{MinX: 0, MinY: 0, MaxX: 51, MaxY: 43},
		Rect{MinX: 0, MinY: 15, MaxX: 51, MaxY: 58},
	), 120, 72, 62)
	if err != nil {
		t.Fatal(err)
	}
	if fit.Scaled || fit.InkH != 72 || fit.InkW != 63 {
		t.Fatalf("cell %d ink %dx%d scaled %v, want 63x72 ink",
			fit.Size, fit.InkW, fit.InkH, fit.Scaled)
	}
	if last := fit.Frames[1].Y + fit.Frames[1].H - 1; last != fit.Size-1 {
		t.Fatalf("lowest ink row %d, want %d", last, fit.Size-1)
	}
}

// TestFitHostKeepsSmallFrontInTheHostCell checks the other half of the rule:
// a 51x46 Pikachu is not blown up to fill the 72 px slot.
func TestFitHostKeepsSmallFrontInTheHostCell(t *testing.T) {
	fit, err := FitHost(hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 51, MaxY: 46}), HostFrontWidth, HostFrontMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fit.Scaled || fit.Size != HostBox || fit.InkW != 51 || fit.InkH != 46 {
		t.Fatalf("cell %d ink %dx%d scaled %v, want a native 64 px cell with 51x46 ink",
			fit.Size, fit.InkW, fit.InkH, fit.Scaled)
	}
	if got := fit.Frames[0]; got.X != 6 || got.Y != 18 {
		t.Fatalf("placed %+v, want 6,18", got)
	}
}

// TestFitHostKeepsTallBackNative checks a tall narrow back union keeps its
// native size and stays centred instead of being scaled to 64.
func TestFitHostKeepsTallBackNative(t *testing.T) {
	fit, err := FitHost(hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 40, MaxY: 100}), HostBackMax, HostBackMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fit.Scaled || fit.Size != 100 || fit.InkW != 40 || fit.InkH != 100 {
		t.Fatalf("cell %d ink %dx%d scaled %v, want a native 100 px cell with 40x100 ink",
			fit.Size, fit.InkW, fit.InkH, fit.Scaled)
	}
	if got := fit.Frames[0]; got.X != 30 || got.Y != 0 {
		t.Fatalf("placed %+v, want 30,0", got)
	}
}

func TestFitHostScalesWideArtToTheBackCap(t *testing.T) {
	fit, err := FitHost(hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 300, MaxY: 40}), HostBackMax, HostBackMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !fit.Scaled || fit.Size != HostBackMax || fit.InkW != HostBackMax || fit.InkH != 15 {
		t.Fatalf("cell %d ink %dx%d scaled %v, want a scaled %d px cell with %dx15 ink",
			fit.Size, fit.InkW, fit.InkH, fit.Scaled, HostBackMax, HostBackMax)
	}
	if got := fit.Frames[0]; got.X != 0 || got.Y != HostBackMax-15 {
		t.Fatalf("placed %+v, want 0,%d", got, HostBackMax-15)
	}
}

func TestFitHostScalesTallArtToTheBackCap(t *testing.T) {
	fit, err := FitHost(hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 40, MaxY: 300}), HostBackMax, HostBackMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !fit.Scaled || fit.Size != HostBackMax || fit.InkH != HostBackMax {
		t.Fatalf("cell %d inkH %d scaled %v, want a scaled %d px cell", fit.Size, fit.InkH, fit.Scaled, HostBackMax)
	}
	if got := fit.Frames[0]; got.Y != fit.Size-HostPad-got.H {
		t.Fatalf("placed %+v, want its feet on row %d", got, fit.Size-1-HostPad)
	}
}

func TestFitHostNeverLeavesTheCell(t *testing.T) {
	frames := hostFrames(
		Rect{MinX: -20, MinY: 0, MaxX: 44, MaxY: 64},
		Rect{MinX: 40, MinY: 8, MaxX: 100, MaxY: 56},
	)
	fit, err := FitHost(frames, HostBackMax, HostBackMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range fit.Frames {
		if f.X < 0 || f.Y < 0 || f.X+f.W > fit.Size || f.Y+f.H > fit.Size {
			t.Fatalf("frame %d %+v leaves the %dx%d cell", i, f, fit.Size, fit.Size)
		}
		if f.W < 1 || f.H < 1 {
			t.Fatalf("frame %d has an empty scaled box %+v", i, f)
		}
	}
}

func TestFitHostRejectsEmptyAnimations(t *testing.T) {
	if _, err := FitHost(nil, HostFrontWidth, HostFrontMax, 0); err == nil {
		t.Fatal("an empty animation was accepted")
	}
	if _, err := FitHost(hostFrames(Rect{MinX: 4, MinY: 4, MaxX: 4, MaxY: 9}), HostFrontWidth, HostFrontMax, 0); err == nil {
		t.Fatal("an empty content box was accepted")
	}
}

func TestFitHostRejectsBadCellCaps(t *testing.T) {
	frames := hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 8, MaxY: 8})
	for _, cap := range []int{0, 60, 200} {
		if _, err := FitHost(frames, cap, cap, 0); err == nil {
			t.Fatalf("cell cap %d was accepted", cap)
		}
	}
	for _, target := range []int{-1, 129} {
		if _, err := FitHost(frames, HostFrontWidth, HostFrontMax, target); err == nil {
			t.Fatalf("target height %d was accepted", target)
		}
	}
}

func TestFitHostOnePixelBox(t *testing.T) {
	fit, err := FitHost(hostFrames(Rect{MinX: 7, MinY: 7, MaxX: 8, MaxY: 8}), HostFrontWidth, HostFrontMax, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := fit.Frames[0]
	if got.W != 1 || got.H != 1 || got.X != 31 || got.Y != 63 {
		t.Fatalf("placed %+v, want a single pixel at 31,63", got)
	}
	if fit.Size != HostBox {
		t.Fatalf("cell side %d, want %d", fit.Size, HostBox)
	}
}

// TestFitNativeKeepsEveryViewAtItsOwnSize is the no-downscaling rule: a view
// whose union is larger than any battle slot still gets a cell of its own size,
// so nothing is resampled. The union is centred with its feet on the last row.
func TestFitNativeKeepsEveryViewAtItsOwnSize(t *testing.T) {
	for _, box := range []Rect{
		{MinX: 0, MinY: 0, MaxX: 153, MaxY: 104},
		{MinX: 0, MinY: 0, MaxX: 120, MaxY: 120},
		{MinX: 0, MinY: 0, MaxX: 40, MaxY: 128},
	} {
		fit, err := FitNative(hostFrames(box))
		if err != nil {
			t.Fatal(err)
		}
		want := roundUp4(max(HostBox, box.Width(), box.Height()))
		if fit.Scaled || fit.Size != want || fit.InkW != box.Width() || fit.InkH != box.Height() {
			t.Fatalf("%v: cell %d ink %dx%d scaled %v, want %d with %dx%d ink and no scaling",
				box, fit.Size, fit.InkW, fit.InkH, fit.Scaled, want, box.Width(), box.Height())
		}
		got := fit.Frames[0]
		if got.X+got.W > fit.Size || got.Y+got.H != fit.Size {
			t.Fatalf("%v: placed %+v in a %d px cell", box, got, fit.Size)
		}
	}
}

// TestFitNativeKeepsTheAnimationMotion checks that a native cell still carries
// the per-frame offset, so a bob or a sway keeps moving.
func TestFitNativeKeepsTheAnimationMotion(t *testing.T) {
	fit, err := FitNative(hostFrames(
		Rect{MinX: 0, MinY: 0, MaxX: 60, MaxY: 90},
		Rect{MinX: 0, MinY: 30, MaxX: 60, MaxY: 90},
	))
	if err != nil {
		t.Fatal(err)
	}
	if d := fit.Frames[1].Y - fit.Frames[0].Y; d != 30 {
		t.Fatalf("frame tops %d and %d do not keep the 30 px lift",
			fit.Frames[0].Y, fit.Frames[1].Y)
	}
	// The 90 px union rounds up to a 92 px cell, so the pair sits two rows down.
	if fit.Size != 92 || fit.Frames[0].Y != 2 || fit.Frames[1].Y != 32 {
		t.Fatalf("placements %+v // %+v in a %d px cell, want tops 2 and 32",
			fit.Frames[0], fit.Frames[1], fit.Size)
	}
}

// TestFitNativeAcceptsSmallArtAndRejectsOverlargeArt keeps the two ends of the
// scale honest: small art stays in a 64 px cell, and art past MaxCell is an
// error rather than a silent shrink.
func TestFitNativeAcceptsSmallArtAndRejectsOverlargeArt(t *testing.T) {
	fit, err := FitNative(hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 51, MaxY: 46}))
	if err != nil {
		t.Fatal(err)
	}
	if fit.Size != HostBox || fit.InkW != 51 || fit.InkH != 46 {
		t.Fatalf("cell %d ink %dx%d, want 64 with 51x46 ink", fit.Size, fit.InkW, fit.InkH)
	}
	if _, err := FitNative(nil); err == nil {
		t.Fatal("an empty animation was accepted")
	}
	if _, err := FitNative(hostFrames(Rect{MinX: 0, MinY: 0, MaxX: 300, MaxY: 40})); err == nil {
		t.Fatal("art past MaxCell was accepted instead of reported")
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
