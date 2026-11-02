package measure

import "testing"

func frame(minX, minY, w, h int) Frame {
	return Frame{Duration: 1, Bounds: Rect{MinX: minX, MinY: minY, MaxX: minX + w, MaxY: minY + h}}
}

func TestApplyStandsContentOnGroundAndCentres(t *testing.T) {
	got, err := Apply([]Frame{frame(0, 0, 20, 30)}, 64, 63)
	if err != nil {
		t.Fatal(err)
	}
	if got.AnchorX != 22 || got.AnchorY != 34 {
		t.Fatalf("anchor = %d,%d, want 22,34", got.AnchorX, got.AnchorY)
	}
	if !got.Grounded || got.Frames[0].Y+30-1 != 63 {
		t.Fatalf("lowest row = %d, want 63", got.Frames[0].Y+29)
	}
}

func TestApplyKeepsStoryIncidentOffsetsBetweenFrames(t *testing.T) {
	frames := []Frame{frame(10, 20, 16, 24), frame(14, 18, 16, 26)}
	got, err := Apply(frames, 64, 63)
	if err != nil {
		t.Fatal(err)
	}
	dx, dy := got.Frames[1].X-got.Frames[0].X, got.Frames[1].Y-got.Frames[0].Y
	if dx != 4 || dy != -2 {
		t.Fatalf("frame 1 moved %d,%d, want the crop shift 4,-2", dx, dy)
	}
	if got.Union != (Rect{MinX: 22, MinY: 38, MaxX: 42, MaxY: 64}) {
		t.Fatalf("union = %v", got.Union)
	}
}

func TestApplyPinsContentTooTallForTheGroundRow(t *testing.T) {
	got, err := Apply([]Frame{frame(0, 0, 8, 64)}, 64, 40)
	if err != nil {
		t.Fatal(err)
	}
	if got.Grounded || got.AnchorY != 0 || got.Frames[0].Y != 0 {
		t.Fatalf("anchor y = %d, grounded = %v, want pinned at 0", got.AnchorY, got.Grounded)
	}
}

// A Gen 5 back sprite is bigger than a facets 64x64 cell; frameSize 0 must grow
// the cell instead of refusing the sprite.
func TestApplyFitsFrameToContent(t *testing.T) {
	got, err := Apply([]Frame{frame(0, 0, 105, 88)}, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if got.FrameSize != 112 {
		t.Fatalf("frame size = %d, want 112 (88 rounded up to 8)", got.FrameSize)
	}
	if !got.Grounded || got.Frames[0].Y != 24 {
		t.Fatalf("grounded = %v, y = %d, want the feet on row 111", got.Grounded, got.Frames[0].Y)
	}
	if got.AnchorX != 3 || got.AnchorY != 24 {
		t.Fatalf("anchor = %d,%d, want 3,24", got.AnchorX, got.AnchorY)
	}
}

// The facets 64 is the floor: a small sprite still gets a 64px cell.
func TestApplyFrameNeverBelowFacetsSize(t *testing.T) {
	got, err := Apply([]Frame{frame(0, 0, 8, 8)}, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if got.FrameSize != DefaultFrameSize {
		t.Fatalf("frame size = %d, want %d", got.FrameSize, DefaultFrameSize)
	}
}

func TestApplyRejectsBadInput(t *testing.T) {
	cases := []struct {
		name   string
		frames []Frame
		size   int
		ground int
	}{
		{"no frames", nil, 64, 63},
		{"oversized frame size", []Frame{frame(0, 0, 8, 8)}, 300, 1},
		{"ground past frame", []Frame{frame(0, 0, 8, 8)}, 64, 64},
		{"empty box", []Frame{{Duration: 1}}, 64, 63},
		{"zero width", []Frame{frame(0, 0, 0, 8)}, 64, 63},
		{"content wider than frame", []Frame{frame(0, 0, 65, 8)}, 64, 63},
		{"content taller than frame", []Frame{frame(0, 0, 8, 65)}, 64, 63},
		{"frames together wider than frame", []Frame{frame(0, 0, 40, 8), frame(39, 0, 40, 8)}, 64, 63},
		{"content past the fit ceiling", []Frame{frame(0, 0, 300, 8)}, 0, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Apply(tc.frames, tc.size, tc.ground); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

// Gen 3 stores a ground offset counted up from the bottom edge, so the row has
// to follow the cell size the sprite ended up with.
func TestApplyFromBottomFollowsTheFittedCell(t *testing.T) {
	got, err := ApplyFromBottom([]Frame{frame(0, 0, 105, 88)}, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got.FrameSize != 112 || got.Ground != 107 {
		t.Fatalf("frame size %d ground %d, want 112 and 107", got.FrameSize, got.Ground)
	}
	if got.Frames[0].Y+88-1 != 107 {
		t.Fatalf("lowest row = %d, want 107", got.Frames[0].Y+87)
	}
}

func TestApplyFromBottomKeepsAFixedCellSize(t *testing.T) {
	got, err := ApplyFromBottom([]Frame{frame(0, 0, 24, 24)}, 64, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got.FrameSize != 64 || got.Ground != 60 || !got.Grounded {
		t.Fatalf("frame size %d ground %d grounded %v, want 64, 60, true",
			got.FrameSize, got.Ground, got.Grounded)
	}
}

func TestApplyFromBottomPinsAnOffsetAboveTheCell(t *testing.T) {
	got, err := ApplyFromBottom([]Frame{frame(0, 0, 8, 8)}, 64, 200)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ground != 0 || got.Grounded || got.Frames[0].Y != 0 {
		t.Fatalf("ground %d grounded %v y %d, want 0,false,0", got.Ground, got.Grounded, got.Frames[0].Y)
	}
}

func TestApplyFromBottomRejectsNegativeOffset(t *testing.T) {
	if _, err := ApplyFromBottom([]Frame{frame(0, 0, 8, 8)}, 64, -1); err == nil {
		t.Fatal("accepted a negative ground offset")
	}
}
