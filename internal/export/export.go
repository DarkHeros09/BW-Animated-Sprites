// Package export turns a decoded sprite into a measured pack entry: the
// cropped, measured frames that get written to a strip, and the metadata that
// describes how to play them.
package export

import (
	"fmt"
	"image"
	"image/color"

	"bwsprites/internal/gen5"
	"bwsprites/internal/measure"
	"bwsprites/internal/pack"
)

// Request is one sprite to export. FrameSize 0 fits the frame to the content
// and Ground < 0 uses the bottom row.
type Request struct {
	Dex       int
	Side      string
	Female    bool
	Palette   string // "normal" or "shiny"
	FrameSize int
	Ground    int
	// GroundFromBottom, when non-negative, is a ground offset counted up from
	// the cell's bottom edge (the Gen 3 tables' yo) and replaces Ground.
	GroundFromBottom int
	// GroundFrom names the table GroundFromBottom came from, for the pack
	// record. Empty when the ground row was given on the command line.
	GroundFrom string
}

// Outcome is one exported sprite.
type Outcome struct {
	Entry  *pack.Entry
	Frames []*image.Paletted
}

// One renders, measures and lays out one sprite.
func One(rom *gen5.ROM, req Request) (*Outcome, error) {
	s, err := rom.Read(req.Dex, req.Side, req.Female)
	if err != nil {
		return nil, err
	}
	pal, err := s.PaletteByName(req.Palette)
	if err != nil {
		return nil, err
	}
	timeline, err := s.Timeline(pal)
	if err != nil {
		return nil, fmt.Errorf("dex %d %s idle animation: %w", req.Dex, req.Side, err)
	}

	boxes := make([]measure.Frame, len(timeline.Poses))
	for i, pose := range timeline.Poses {
		c := pose.Canvas
		boxes[i] = measure.Frame{
			Duration: pose.Duration,
			Bounds:   measure.Rect{MinX: c.MinX, MinY: c.MinY, MaxX: c.MinX + c.Width, MaxY: c.MinY + c.Height},
		}
	}
	var result *measure.Result
	if req.GroundFromBottom >= 0 {
		result, err = measure.ApplyFromBottom(boxes, req.FrameSize, req.GroundFromBottom)
	} else {
		result, err = measure.Apply(boxes, req.FrameSize, req.Ground)
	}
	if err != nil {
		return nil, fmt.Errorf("dex %d %s: %w", req.Dex, req.Side, err)
	}

	sheet := pal.Image()
	out := &Outcome{Entry: &pack.Entry{
		ID:         pack.IDFor(req.Dex, req.Side, req.Palette, req.Female),
		Dex:        req.Dex,
		Side:       req.Side,
		Female:     req.Female,
		Palette:    req.Palette,
		Frames:     len(timeline.Poses),
		FrameSize:  result.FrameSize,
		Ground:     result.Ground,
		Grounded:   result.Grounded,
		AnchorX:    result.AnchorX,
		AnchorY:    result.AnchorY,
		LoopStart:  timeline.LoopStart,
		CycleTicks: timeline.Ticks,
		Capped:     timeline.Capped,
		Union:      [4]int{result.Union.MinX, result.Union.MinY, result.Union.MaxX, result.Union.MaxY},
		GroundFrom: req.GroundFrom,
		Offsets:    make([][2]int, len(result.Frames)),
		Durations:  make([]int, len(result.Frames)),
	}}
	if s.GenderFallback {
		out.Entry.Fallback = pack.IDFor(req.Dex, req.Side, req.Palette, false)
	}

	out.Frames = make([]*image.Paletted, len(timeline.Poses))
	for i, pose := range timeline.Poses {
		placed := result.Frames[i]
		out.Entry.Offsets[i] = [2]int{placed.X, placed.Y}
		out.Entry.Durations[i] = pose.Duration
		out.Frames[i] = place(pose.Canvas, result.FrameSize, placed, sheet)
	}
	return out, nil
}

// place copies a cropped canvas into an empty measurement cell.
func place(c *gen5.Canvas, size int, at measure.Placed, pal color.Palette) *image.Paletted {
	frame := image.NewPaletted(image.Rect(0, 0, size, size), pal)
	for y := 0; y < c.Height; y++ {
		for x := 0; x < c.Width; x++ {
			index, ok := c.Pixels[y*c.Width+x]
			if !ok {
				continue
			}
			frame.SetColorIndex(at.X+x, at.Y+y, index)
		}
	}
	return frame
}
