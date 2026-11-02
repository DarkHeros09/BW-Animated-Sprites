package measure

import (
	"fmt"
	"math"
)

// HostBox is the side of the engine's own mon picture cell (64x64). It is also
// the floor for a replacement cell: a smaller picture would have to be padded
// back up before it could be drawn with the engine's origin.
const HostBox = 64

// HostFrontMax is the enemy picture's cell height cap. No Black/White front is
// taller than 104 px of ink, so this cap never shrinks one: the enemy picture
// keeps its native pixels. A front taller than the 72 px gap between the scene
// top and the enemy ground row is placed with its top on row 0 and its feet
// lower, behind the battle menu (the user allowed the bottom to be covered but
// not the top).
const HostFrontMax = 112

// HostFrontWidth is the enemy picture's cell width cap. The widest Black/White
// front is 153 px, so this cap also leaves every front at native size. The
// runtime slides a wide cell left until it fits the 240 px scene, because the
// enemy base at x=176 would otherwise put more than a third of a Lugia off the
// right edge.
const HostFrontWidth = 160

// HostBackMax is the player picture's cell cap. The player's ground row sits
// low in the scene, so a native Black/White back sprite (Charizard is 105x88)
// fits without being squeezed into 64x64; 112 leaves a 4 px top margin.
const HostBackMax = 112

// HostPad is the number of transparent rows below the ink. Facets puts the
// lowest ink row on row 63 of its 64x64 cell; this mod zeroes the engine's
// per-species y offset, so the feet land on the cartridge's shared ground row.
const HostPad = 0

// MaxCell is the largest square cell the mod pack writes. FitNative never
// reduces art, so the cell is the animation's own size; this bound only stops a
// corrupt measurement from asking the engine for an unbounded texture. A
// Black/White or Black 2/White 2 front is at most 160 px and a back at most
// 128, so the bound is headroom, not a cap.
const MaxCell = 256

// HostFrame is one frame's scaled content box inside a cell.
type HostFrame struct{ X, Y, W, H int }

// HostFit is an animation placed in one square native cell.
type HostFit struct {
	// Size is the square cell side: HostBox (64) when the art fits, otherwise
	// its native size rounded up to four. FitNative never caps it, so a cell can
	// be larger than the battle slot; FitHost caps it per side.
	Size int
	// Frames holds one placement per input frame, in cell coordinates.
	Frames []HostFrame
	// InkW, InkH is the scaled size of the box enclosing every frame.
	InkW, InkH int
	// Scaled reports whether the art had to be reduced to fit a cap. FitNative
	// never reduces anything, so a mod pack always reports false.
	Scaled bool
}

// scaledSize rounds a source dimension to the scaled cell, never below one.
func scaledSize(n int, scale float64) int {
	return max(1, int(math.Round(float64(n)*scale)))
}

// roundUp4 rounds n up to the next multiple of four. Cells are a multiple of
// four so the engine's centre anchor lands on a whole pixel and the (64-Size)/2
// footing correction is an integer.
func roundUp4(n int) int {
	return (n + 3) / 4 * 4
}

// FitNative places an animation in a square cell at its own scale. Nothing is
// enlarged and nothing is reduced: every Black/White (and Black 2/White 2)
// pixel the dump contains is kept, so a moving wing is never resampled into a
// smaller cell. The cell side is the union's larger side rounded up to four,
// never below HostBox.
//
// The union is centred horizontally with its lowest ink row on the cell's last
// row, and each frame keeps its sprite-space offset inside it, exactly as
// FitHost places a fitted animation. The runtime then zeroes the per-species y
// padding and substitutes the cell correction, so the feet land on the
// cartridge's ground row whatever the cell size.
func FitNative(frames []Frame) (*HostFit, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("animation has no frames")
	}
	for i, f := range frames {
		if f.Bounds.MaxX <= f.Bounds.MinX || f.Bounds.MaxY <= f.Bounds.MinY {
			return nil, fmt.Errorf("frame %d has an empty content box %v", i, f.Bounds)
		}
	}
	u := Union(frames)
	if u.Width() < 1 || u.Height() < 1 {
		return nil, fmt.Errorf("animation union %dx%d is empty", u.Width(), u.Height())
	}
	fit := &HostFit{InkW: u.Width(), InkH: u.Height()}
	fit.Size = max(HostBox, roundUp4(max(fit.InkW, fit.InkH)))
	if fit.Size > MaxCell {
		return nil, fmt.Errorf("animation needs a %d px cell, over the %d px limit", fit.Size, MaxCell)
	}
	left := (fit.Size - fit.InkW) / 2
	fit.Frames = make([]HostFrame, len(frames))
	for i, f := range frames {
		w, h := f.Bounds.Width(), f.Bounds.Height()
		x := left + f.Bounds.MinX - u.MinX
		// The union's lowest ink row lands on the cell's last row; a frame sits
		// however far its own bottom is above the union's.
		y := fit.Size - HostPad - h - (u.MaxY - f.Bounds.MaxY)
		x = min(max(x, 0), fit.Size-w)
		y = min(max(y, 0), fit.Size-h)
		fit.Frames[i] = HostFrame{X: x, Y: y, W: w, H: h}
	}
	return fit, nil
}

// FitHost places an animation in a square cell. maxW and maxH are the side's
// caps (a front: HostFrontWidth x HostFrontMax; a back: HostBackMax for both).
// targetH is the smallest on-screen height the tallest frame should reach, or 0
// for native size. The pack passes 0 for both sides: an enemy front keeps
// Black/White's native pixels so a moving animation is not resampled, and a
// small species (Pikachu) is not enlarged past its own art. The parameter stays
// for callers that need a floor (tests); the caps still win.
//
// Art that fits 64 stays in a 64 px cell (facets' rule); art that needs more
// keeps its native size up to the caps, so a wide Gyarados front is not squeezed
// by a 72 px width limit and Charizard's back is not squeezed to 53 px. The
// union is centred horizontally and its lowest ink row sits HostPad rows above
// the cell bottom; frames keep their sprite-space offsets, so intra-animation
// motion survives. When the cell is wider than it is tall (a wide front), the
// transparent margin is above the ink, so the runtime's cell-bottom positioning
// still lands the feet on the cartridge ground row.
//
// The runtime shifts a wider-than-64 picture left on a fixed-origin engine so
// its centre lands on the battler's x, and its y correction puts the cell
// bottom on the cartridge ground row. FitNative is the mod pack's fitter: it
// never reduces art, so no view loses pixels to a cap.
func FitHost(frames []Frame, maxW, maxH, targetH int) (*HostFit, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("animation has no frames")
	}
	if maxW < HostBox || maxW > HostFrontWidth || maxH < HostBox || maxH > HostFrontWidth {
		return nil, fmt.Errorf("cell caps %dx%d are outside %d..%d", maxW, maxH, HostBox, HostFrontWidth)
	}
	if targetH < 0 || targetH > 128 {
		return nil, fmt.Errorf("target height %d is outside 0..128", targetH)
	}
	for i, f := range frames {
		if f.Bounds.MaxX <= f.Bounds.MinX || f.Bounds.MaxY <= f.Bounds.MinY {
			return nil, fmt.Errorf("frame %d has an empty content box %v", i, f.Bounds)
		}
	}
	u := Union(frames)
	// One uniform scale keeps the art proportional. A target may enlarge the art
	// until its tallest frame reaches the facets height; the cell caps then have
	// the final say, and art larger than a cap is reduced.
	scale := 1.0
	if targetH > 0 {
		tallest := 0
		for _, f := range frames {
			if h := f.Bounds.Height(); h > tallest {
				tallest = h
			}
		}
		if float64(targetH) > float64(tallest) {
			scale = float64(targetH) / float64(tallest)
		}
	}
	scale = math.Min(scale, math.Min(
		float64(maxW)/float64(u.Width()),
		float64(maxH-HostPad)/float64(u.Height()),
	))
	fit := &HostFit{Scaled: scale < 1}
	fit.InkW, fit.InkH = scaledSize(u.Width(), scale), scaledSize(u.Height(), scale)
	fit.Size = max(HostBox, roundUp4(max(fit.InkW, fit.InkH)))
	left := (fit.Size - fit.InkW) / 2
	fit.Frames = make([]HostFrame, len(frames))
	for i, f := range frames {
		w, h := scaledSize(f.Bounds.Width(), scale), scaledSize(f.Bounds.Height(), scale)
		x := left + int(math.Round(float64(f.Bounds.MinX-u.MinX)*scale))
		// The frame's last row is Size-1-HostPad minus however far its bottom
		// sits above the union's bottom, so the union's bottom row lands on
		// row Size-1-HostPad.
		y := fit.Size - HostPad - h - int(math.Round(float64(u.MaxY-f.Bounds.MaxY)*scale))
		x = min(max(x, 0), fit.Size-w)
		y = min(max(y, 0), fit.Size-h)
		fit.Frames[i] = HostFrame{X: x, Y: y, W: w, H: h}
	}
	return fit, nil
}
