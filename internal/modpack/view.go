package modpack

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"bwsprites/internal/gen5"
	"bwsprites/internal/measure"
)

// maxSheet is the largest sheet side asked of a GLES2 device. Columns are
// chosen per view so neither sheet dimension goes past it.
const maxSheet = 4096

// maxColumns bounds the grid width. The tallest animation in the ROM is 240
// ticks; even a 128 px cell needs only a few columns, so this is a sanity cap.
const maxColumns = 64

// rendered is one palette's sheet and the timing shared by every palette.
type rendered struct {
	view  *View
	sheet *image.Paletted
}

// sheetColumns picks the grid width for a view: enough columns that the sheet
// height stays within maxSheet, and no more columns than frames.
func sheetColumns(frames, size int) (int, error) {
	maxRows := maxSheet / size
	cols := (frames + maxRows - 1) / maxRows
	if cols > maxColumns {
		return 0, fmt.Errorf("%d frames in %d px cells need %d columns, more than %d",
			frames, size, cols, maxColumns)
	}
	return min(cols, frames), nil
}

// dexBox is the Pokedex's crop window: the box that encloses every painted pose
// of the animation, in cell coordinates. One window for the whole cycle means
// the dex picture keeps one scale and one position while the idle plays, and a
// wing-down or swaying pose stays inside it instead of being cut off.
func dexBox(frames []measure.HostFrame) (x, y, w, h int) {
	if len(frames) == 0 {
		return 0, 0, 0, 0
	}
	minX, minY := frames[0].X, frames[0].Y
	maxX, maxY := frames[0].X+frames[0].W, frames[0].Y+frames[0].H
	for _, f := range frames[1:] {
		minX, minY = min(minX, f.X), min(minY, f.Y)
		maxX, maxY = max(maxX, f.X+f.W), max(maxY, f.Y+f.H)
	}
	return minX, minY, maxX - minX, maxY - minY
}

// render composes every frame of a sprite's idle animation, places it in one
// native cell and paints the frames into a grid sheet. The cell is the
// animation's own union: nothing is scaled up and nothing is scaled down, so a
// view keeps every pixel the dump contains.
func render(s *gen5.Sprite, palette string) (*rendered, error) {
	pal, err := s.PaletteByName(palette)
	if err != nil {
		return nil, err
	}
	timeline, err := s.Timeline(pal)
	if err != nil {
		return nil, fmt.Errorf("idle animation: %w", err)
	}
	if len(timeline.Poses) == 0 {
		return nil, fmt.Errorf("idle animation has no frames")
	}
	boxes := make([]measure.Frame, len(timeline.Poses))
	durations := make([]int, len(timeline.Poses))
	crops := make([]*gen5.Canvas, len(timeline.Poses))
	for i, pose := range timeline.Poses {
		c := inkCanvas(pose.Canvas)
		crops[i] = c
		boxes[i] = measure.Frame{
			Duration: pose.Duration,
			Bounds:   measure.Rect{MinX: c.MinX, MinY: c.MinY, MaxX: c.MinX + c.Width, MaxY: c.MinY + c.Height},
		}
		durations[i] = pose.Duration
	}
	fit, err := measure.FitNative(boxes)
	if err != nil {
		return nil, err
	}
	cols, err := sheetColumns(len(timeline.Poses), fit.Size)
	if err != nil {
		return nil, err
	}
	rows := (len(timeline.Poses) + cols - 1) / cols
	sheet := image.NewPaletted(image.Rect(0, 0, cols*fit.Size, rows*fit.Size), pal.Image())
	for i := range timeline.Poses {
		col, row := i%cols, i/cols
		paint(crops[i], fit.Frames[i], col*fit.Size, row*fit.Size, sheet)
	}
	dexX, dexY, dexW, dexH := dexBox(fit.Frames)
	return &rendered{
		view: &View{
			Frames:    len(timeline.Poses),
			Size:      fit.Size,
			Columns:   cols,
			InkH:      fit.InkH,
			InkW:      fit.InkW,
			DexX:      dexX,
			DexY:      dexY,
			DexW:      dexW,
			DexH:      dexH,
			Durations: durations,
			LoopStart: timeline.LoopStart,
		},
		sheet: sheet,
	}, nil
}

// inkCanvas trims a composed canvas to its opaque pixels. The compositor sizes
// canvases from transformed OAM bounds, so they can carry transparent padding;
// the mod places *ink* (lowest row HostPad above the cell bottom, centre
// column), so padding must go before measuring.
func inkCanvas(c *gen5.Canvas) *gen5.Canvas {
	minX, minY := c.Width, c.Height
	maxX, maxY := -1, -1
	for key := range c.Pixels {
		x, y := key%c.Width, key/c.Width
		if x < minX {
			minX = x
		}
		if y < minY {
			minY = y
		}
		if x > maxX {
			maxX = x
		}
		if y > maxY {
			maxY = y
		}
	}
	if maxX < 0 {
		return c
	}
	out := &gen5.Canvas{
		Width:  maxX - minX + 1,
		Height: maxY - minY + 1,
		MinX:   c.MinX + minX,
		MinY:   c.MinY + minY,
		Pixels: make(map[int]uint8, len(c.Pixels)),
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if index, ok := c.Pixels[y*c.Width+x]; ok {
				out.Pixels[(y-minY)*out.Width+(x-minX)] = index
			}
		}
	}
	return out
}

// paint copies one frame's canvas into its cell, sampling only when the fit
// had to scale the art down.
func paint(c *gen5.Canvas, at measure.HostFrame, left, top int, sheet *image.Paletted) {
	for dy := 0; dy < at.H; dy++ {
		sy := sampleIndex(dy, at.H, c.Height)
		for dx := 0; dx < at.W; dx++ {
			sx := sampleIndex(dx, at.W, c.Width)
			index, ok := c.Pixels[sy*c.Width+sx]
			if !ok {
				continue
			}
			sheet.SetColorIndex(left+at.X+dx, top+at.Y+dy, index)
		}
	}
}

// sampleIndex maps a destination pixel to its nearest source pixel. It is the
// identity when the fit did not scale, so untouched art keeps its pixels.
func sampleIndex(dst, dstSize, srcSize int) int {
	if dstSize == srcSize {
		return dst
	}
	i := (2*dst + 1) * srcSize / (2 * dstSize)
	if i >= srcSize {
		i = srcSize - 1
	}
	return i
}

// writePNG writes an indexed sheet and returns its size in bytes.
func writePNG(path string, img *image.Paletted) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	if err := writeFileAtomic(path, buf.Bytes()); err != nil {
		return 0, err
	}
	return int64(buf.Len()), nil
}
