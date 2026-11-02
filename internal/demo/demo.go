// Package demo builds a synthetic, redistributable sprite pack.
//
// The pixels are drawn procedurally: no ROM, no game art. The pack uses the
// same schema as an imported one, so the validator, the LOVE previewer and the
// end-to-end test all run without a copyrighted dump.
package demo

import (
	"fmt"
	"image"
	"image/color"

	"bwsprites/internal/measure"
	"bwsprites/internal/pack"
)

// ImporterID marks a pack that was generated, not imported from a dump.
const ImporterID = "synthetic"

// Palette indices: 0 is transparent, the rest are the only colours used.
const (
	transparent = iota
	dark
	body
	highlight
	accent
)

var palette = color.Palette{
	color.NRGBA{},
	color.NRGBA{R: 24, G: 28, B: 34, A: 255},
	color.NRGBA{R: 86, G: 196, B: 122, A: 255},
	color.NRGBA{R: 178, G: 236, B: 200, A: 255},
	color.NRGBA{R: 236, G: 92, B: 66, A: 255},
}

// frame is one drawn picture: a tight content box plus its indexed pixels.
type frame struct {
	duration int
	box      measure.Rect
	pixels   []uint8
}

// sprite is one synthetic animation.
type sprite struct {
	dex    int
	label  string
	frames []frame
}

// canvas is a scratch index buffer with its content box.
type canvas struct {
	box    measure.Rect
	pixels []uint8
}

func newCanvas(w, h int) *canvas {
	return &canvas{box: measure.Rect{MaxX: w, MaxY: h}, pixels: make([]uint8, w*h)}
}

func (c *canvas) set(x, y int, index uint8) {
	w := c.box.MaxX
	if x < 0 || y < 0 || x >= w || y >= c.box.MaxY {
		return
	}
	c.pixels[y*w+x] = index
}

// ellipse fills the ellipse centred at (cx, cy) with radii (rx, ry).
func (c *canvas) ellipse(cx, cy, rx, ry int) {
	for y := cy - ry; y <= cy+ry; y++ {
		for x := cx - rx; x <= cx+rx; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx*ry*ry+dy*dy*rx*rx <= rx*rx*ry*ry {
				c.set(x, y, body)
			}
		}
	}
}

// trim crops the buffer to its opaque pixels so the frame carries a tight box.
func (c *canvas) trim() frame {
	minX, minY, maxX, maxY := c.box.MaxX, c.box.MaxY, 0, 0
	for y := 0; y < c.box.MaxY; y++ {
		for x := 0; x < c.box.MaxX; x++ {
			if c.pixels[y*c.box.MaxX+x] == transparent {
				continue
			}
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x+1), max(maxY, y+1)
		}
	}
	if maxX == 0 && maxY == 0 {
		panic("demo: drew nothing")
	}
	out := frame{box: measure.Rect{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}}
	w := maxX - minX
	out.pixels = make([]uint8, w*(maxY-minY))
	for y := minY; y < maxY; y++ {
		copy(out.pixels[(y-minY)*w:(y-minY+1)*w], c.pixels[y*c.box.MaxX+minX:y*c.box.MaxX+maxX])
	}
	return out
}

// blob squashes and stretches but keeps its feet on the same row, so every
// frame must stay bottom-aligned when it is measured.
func blob() []frame {
	sizes := [][2]int{{44, 34}, {48, 30}, {46, 36}, {40, 40}, {46, 36}, {48, 30}, {44, 34}, {42, 38}}
	durations := []int{8, 6, 6, 8, 6, 6, 10, 6}
	const boxH = 40
	out := make([]frame, len(sizes))
	for i, size := range sizes {
		c := newCanvas(64, boxH)
		cx, bottom := 32, boxH-1
		c.ellipse(cx, bottom-size[1]/2, size[0]/2, size[1]/2)
		c.ellipse(cx-8, bottom-size[1]+6, 4, 3)
		for y := 0; y < boxH; y++ {
			for x := 0; x < 64; x++ {
				if c.pixels[y*64+x] == body && y < bottom-size[1]+6 {
					c.set(x, y, highlight)
				}
			}
		}
		f := c.trim()
		f.duration = durations[i]
		out[i] = f
	}
	return out
}

// float lifts off the ground, so its content box moves between frames and the
// measured placement must follow it instead of freezing frame 0.
func floatUp() []frame {
	heights := []int{26, 14, 6, 14}
	durations := []int{24, 12, 24, 12}
	out := make([]frame, len(heights))
	for i, top := range heights {
		c := newCanvas(48, 40)
		cx := 24
		c.ellipse(cx, top+8, 10, 8)
		c.set(cx, top+2, accent)
		f := c.trim()
		f.duration = durations[i]
		out[i] = f
	}
	return out
}

// wide is a single frame wider than the facets 64px cell, so the auto-fit edge
// has to grow past 64 instead of cropping it.
func wide() []frame {
	c := newCanvas(96, 24)
	for x := 4; x < 92; x++ {
		c.set(x, 12, body)
		c.set(x, 13, body)
		c.set(x, 14, dark)
	}
	for x := 20; x < 30; x++ {
		c.set(x, 9, accent)
		c.set(x, 10, accent)
		c.set(x, 11, accent)
	}
	f := c.trim()
	f.duration = 60
	return []frame{f}
}

// sprites is the demo set. The dex/side/palette slots are only names that keep
// the synthetic entries flowing through the same validator as imported ones.
func sprites() []sprite {
	return []sprite{
		{dex: 1, label: "squash: feet stay on the ground", frames: blob()},
		{dex: 2, label: "float: content box moves per frame", frames: floatUp()},
		{dex: 3, label: "wide: one frame, auto-fit past 64px", frames: wide()},
	}
}

// Write builds the demo pack under root, replacing any manifest there.
func Write(root string) (*pack.Manifest, error) {
	manifest := &pack.Manifest{
		Format:   pack.Format,
		Importer: ImporterID,
		TickRate: pack.TickRate,
		Source: pack.Source{
			Path:     "synthetic",
			GameCode: "DEMO",
			Digest:   "synthetic",
		},
	}
	for _, s := range sprites() {
		entry, frames, err := build(s)
		if err != nil {
			return nil, fmt.Errorf("demo %s: %w", s.label, err)
		}
		if err := pack.WriteEntry(root, entry, frames); err != nil {
			return nil, err
		}
		manifest.Entries = append(manifest.Entries, entry)
	}
	if err := pack.WriteManifest(root, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// build measures one synthetic animation and lays it into frame cells.
func build(s sprite) (*pack.Entry, []*image.Paletted, error) {
	boxes := make([]measure.Frame, len(s.frames))
	for i, f := range s.frames {
		boxes[i] = measure.Frame{Duration: f.duration, Bounds: f.box}
	}
	result, err := measure.Apply(boxes, 0, -1)
	if err != nil {
		return nil, nil, err
	}
	entry := &pack.Entry{
		ID:        pack.IDFor(s.dex, "front", "normal", false),
		Label:     s.label,
		Dex:       s.dex,
		Side:      "front",
		Palette:   "normal",
		Frames:    len(s.frames),
		FrameSize: result.FrameSize,
		Ground:    result.Ground,
		Grounded:  result.Grounded,
		AnchorX:   result.AnchorX,
		AnchorY:   result.AnchorY,
		LoopStart: 0,
		Union: [4]int{result.Union.MinX, result.Union.MinY,
			result.Union.MaxX, result.Union.MaxY},
		Offsets:   make([][2]int, len(s.frames)),
		Durations: make([]int, len(s.frames)),
	}
	frames := make([]*image.Paletted, len(s.frames))
	for i, f := range s.frames {
		at := result.Frames[i]
		entry.Offsets[i] = [2]int{at.X, at.Y}
		entry.Durations[i] = f.duration
		entry.CycleTicks += f.duration
		cell := image.NewPaletted(image.Rect(0, 0, result.FrameSize, result.FrameSize), palette)
		w := f.box.Width()
		for y := 0; y < f.box.Height(); y++ {
			for x := 0; x < w; x++ {
				cell.SetColorIndex(at.X+x, at.Y+y, f.pixels[y*w+x])
			}
		}
		frames[i] = cell
	}
	return entry, frames, nil
}
