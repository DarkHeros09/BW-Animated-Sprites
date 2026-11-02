package pack

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"

	_ "image/png"
)

// Verify re-reads a written pack and checks that every entry is internally
// consistent and that each strip matches its declared geometry. It catches the
// ways a pack goes wrong after export: a missing file, a resized strip, an
// offset outside the frame, durations that do not add up.
func Verify(root string) error {
	m, err := ReadManifest(root)
	if err != nil {
		return err
	}
	if len(m.Entries) == 0 {
		return fmt.Errorf("%s has no entries", root)
	}
	for _, e := range m.Entries {
		if err := verifyEntry(root, m.FrameSize, e); err != nil {
			return fmt.Errorf("entry %s: %w", e.ID, err)
		}
	}
	return nil
}

func verifyEntry(root string, frameSize int, e *Entry) error {
	if e.ID == "" {
		return fmt.Errorf("missing id")
	}
	if e.Dex < 1 || e.Dex > 649 {
		return fmt.Errorf("dex %d outside 1..649", e.Dex)
	}
	if e.Side != "front" && e.Side != "back" {
		return fmt.Errorf("side %q", e.Side)
	}
	if e.Palette != "normal" && e.Palette != "shiny" {
		return fmt.Errorf("palette %q", e.Palette)
	}
	if frameSize != 0 && e.FrameSize != frameSize {
		return fmt.Errorf("frame size %d disagrees with the pack's %d", e.FrameSize, frameSize)
	}
	if e.FrameSize < 1 || e.FrameSize > 256 {
		return fmt.Errorf("frame size %d outside 1..256", e.FrameSize)
	}
	size := e.FrameSize
	if e.Frames <= 0 || e.Frames != len(e.Offsets) || e.Frames != len(e.Durations) {
		return fmt.Errorf("%d frames, %d offsets, %d durations", e.Frames, len(e.Offsets), len(e.Durations))
	}
	if e.LoopStart < 0 || e.LoopStart >= e.Frames {
		return fmt.Errorf("loop start %d outside 0..%d", e.LoopStart, e.Frames-1)
	}
	if e.Ground < 0 || e.Ground >= size {
		return fmt.Errorf("ground row %d outside the %dpx frame", e.Ground, size)
	}
	total := 0
	for i, d := range e.Durations {
		if d <= 0 {
			return fmt.Errorf("frame %d has duration %d", i, d)
		}
		total += d
	}
	if total != e.CycleTicks {
		return fmt.Errorf("durations add up to %d ticks but the cycle is %d", total, e.CycleTicks)
	}
	for i, o := range e.Offsets {
		if o[0] < 0 || o[1] < 0 || o[0] >= size || o[1] >= size {
			return fmt.Errorf("frame %d offset %v outside the %dpx frame", i, o, size)
		}
	}
	if outside(e.Union, size) {
		return fmt.Errorf("union %v outside the %dpx frame", e.Union, size)
	}

	path := filepath.Join(root, filepath.FromSlash(e.Strip))
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	img, format, err := image.DecodeConfig(file)
	if err != nil {
		return fmt.Errorf("%s: %w", e.Strip, err)
	}
	if format != "png" {
		return fmt.Errorf("%s is %s, want png", e.Strip, format)
	}
	want := image.Rect(0, 0, size*e.Frames, size)
	if img.Width != want.Dx() || img.Height != want.Dy() {
		return fmt.Errorf("%s is %dx%d, want %dx%d", e.Strip, img.Width, img.Height, want.Dx(), want.Dy())
	}
	// DecodeConfig consumed the header, so rewind before the full decode.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err := png.Decode(file); err != nil {
		return fmt.Errorf("%s: %w", e.Strip, err)
	}
	return nil
}

func outside(box [4]int, frameSize int) bool {
	return box[2] <= box[0] || box[3] <= box[1] ||
		box[0] < 0 || box[1] < 0 || box[2] > frameSize || box[3] > frameSize
}
