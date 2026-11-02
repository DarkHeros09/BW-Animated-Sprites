// Package measure applies the Facets sprite-measurement convention to a
// decoded frame sequence.
//
// Facets (mons.lua in ElvieBlooms/facets, MIT) lays every picture into a fixed
// SIZE x SIZE frame so the lowest drawn row sits on the game's ground row for
// that species and the picture is centred horizontally:
//
//	ox = floor((SIZE - sw) / 2)
//	oy = bottom - 1 - low
//	if oy + top < 0 then oy = -top end
//	if oy + low > SIZE - 1 then oy = SIZE - 1 - low end
//
// with bottom = SIZE - yo, yo coming from the host engine's per-species offset
// table (0 when absent), and SIZE hard 64 (mons.lua:33, :79). Facets freezes the
// placement one picture produced and reuses it for every later picture, so an
// animation does not jitter (mons.lua:93-100).
//
// Two things differ here, both forced by Black/White data being a decoded
// multi-part composite rather than one authored 64x64 picture. They are
// recorded in docs/MEASUREMENTS.md:
//
//   - Frames arrive cropped to their opaque content and carry a position in one
//     shared sprite space. The crop boxes are placed as one group so the
//     animation keeps its internal offsets; the group's lowest row lands on the
//     ground row and the group is centred horizontally. A per-frame placement
//     derived from that group replaces the frozen first picture, which would be
//     wrong for a sprite that happens to be airborne when its animation starts.
//   - The frame edge is a parameter whose default is the Facets 64, because
//     Gen 5 sprites exceed 64x64 and must not be silently scaled down. Fit
//     reports the edge a box needs; a caller that must stay inside a host's
//     64px cell passes 64 and gets a recorded Grounded=false when content is
//     too tall rather than a cropped picture.
package measure

import (
	"fmt"
	"math"
)

// DefaultFrameSize is the Facets frame edge, kept as the default canvas.
const DefaultFrameSize = 64

// MaxFrameSize bounds a frame edge so a bad measurement cannot allocate wildly.
const MaxFrameSize = 256

// Fit reports the frame edge a box needs: never below DefaultFrameSize, never
// above MaxFrameSize, grown to a multiple of 8 so a strip of frames stays
// tile-aligned.
func Fit(w, h int) int {
	size := max(DefaultFrameSize, w, h)
	return min((size+7)/8*8, MaxFrameSize)
}

// Rect is an opaque-content box in sprite space, exclusive on max.
type Rect struct{ MinX, MinY, MaxX, MaxY int }

// Width reports the box width.
func (r Rect) Width() int { return r.MaxX - r.MinX }

// Height reports the box height.
func (r Rect) Height() int { return r.MaxY - r.MinY }

// Frame is one cropped frame with its duration in ticks.
type Frame struct {
	Duration int
	Bounds   Rect
}

// Placed is one frame's crop position inside the measurement frame.
type Placed struct {
	X, Y int
}

// Result is the measured animation.
type Result struct {
	// FrameSize is the square measurement frame both components agree on.
	FrameSize int
	// Ground is the frame row the lowest content row was asked to land on.
	Ground int
	// Grounded reports whether it does. A sprite too tall to reach the ground
	// row without leaving the frame is pinned to the top instead, as the
	// facets clamp does, and Grounded is false.
	Grounded bool
	// AnchorX, AnchorY is the frame-space position of the sprite-space origin,
	// so a consumer places the sprite by a known point instead of guessing.
	AnchorX, AnchorY int
	// Frames holds one placement per input frame, in input order.
	Frames []Placed
	// Union is the bounding box of all placed content, in frame space.
	Union Rect
}

// Union reports the sprite-space box enclosing every frame.
func Union(frames []Frame) Rect {
	u := Rect{MinX: math.MaxInt, MinY: math.MaxInt, MaxX: math.MinInt, MaxY: math.MinInt}
	for _, f := range frames {
		u.MinX, u.MinY = min(u.MinX, f.Bounds.MinX), min(u.MinY, f.Bounds.MinY)
		u.MaxX, u.MaxY = max(u.MaxX, f.Bounds.MaxX), max(u.MaxY, f.Bounds.MaxY)
	}
	return u
}

// Apply measures frames with the Facets convention.
//
// frameSize 0 fits the frame to the content (see Fit). ground is the 0-based
// frame row the animation's lowest content row must land on, so the Facets
// expression bottom = Size - yo becomes ground = Size - yo - 1; ground < 0
// means the bottom row.
func Apply(frames []Frame, frameSize, ground int) (*Result, error) {
	return apply(frames, frameSize, func(size int) (int, error) {
		if ground < 0 {
			return size - 1, nil
		}
		if ground >= size {
			return 0, fmt.Errorf("ground row %d outside the %dpx frame", ground, size)
		}
		return ground, nil
	})
}

// ApplyFromBottom is Apply for a ground offset counted up from the bottom edge,
// the Facets yo convention (mons.lua:552-556). Gen 3 stores those offsets and
// they only turn into a row once the cell size is known, so this resolves the
// row after an auto-fit instead of pinning the sprite to a fixed row.
func ApplyFromBottom(frames []Frame, frameSize, fromBottom int) (*Result, error) {
	if fromBottom < 0 {
		return nil, fmt.Errorf("ground offset %d must not be negative", fromBottom)
	}
	return apply(frames, frameSize, func(size int) (int, error) {
		if fromBottom >= size {
			// The Facets top clamp would pin the content to the top row; report
			// that row so Grounded stays meaningful.
			return 0, nil
		}
		return size - 1 - fromBottom, nil
	})
}

// apply is Apply with the ground row resolved once the frame size is known.
func apply(frames []Frame, frameSize int, groundFor func(size int) (int, error)) (*Result, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("animation has no frames")
	}
	for i, f := range frames {
		if f.Bounds.MaxX <= f.Bounds.MinX || f.Bounds.MaxY <= f.Bounds.MinY {
			return nil, fmt.Errorf("frame %d has an empty content box %v", i, f.Bounds)
		}
	}
	u := Union(frames)
	if frameSize == 0 {
		frameSize = Fit(u.Width(), u.Height())
	}
	if frameSize < 1 || frameSize > MaxFrameSize {
		return nil, fmt.Errorf("frame size %d outside 1..%d", frameSize, MaxFrameSize)
	}
	if u.Width() > frameSize || u.Height() > frameSize {
		return nil, fmt.Errorf("animation content %dx%d exceeds the %dpx frame", u.Width(), u.Height(), frameSize)
	}
	ground, err := groundFor(frameSize)
	if err != nil {
		return nil, err
	}
	if ground >= frameSize {
		return nil, fmt.Errorf("ground row %d outside the %dpx frame", ground, frameSize)
	}

	ox := (frameSize - u.Width()) / 2
	oy := min(max(ground-(u.Height()-1), 0), frameSize-u.Height())
	res := &Result{
		FrameSize: frameSize, Ground: ground, Grounded: oy+u.Height()-1 == ground,
		AnchorX: ox - u.MinX, AnchorY: oy - u.MinY,
		Union:  Rect{MinX: ox, MinY: oy, MaxX: ox + u.Width(), MaxY: oy + u.Height()},
		Frames: make([]Placed, len(frames)),
	}
	for i, f := range frames {
		res.Frames[i] = Placed{X: ox + f.Bounds.MinX - u.MinX, Y: oy + f.Bounds.MinY - u.MinY}
	}
	return res, nil
}
