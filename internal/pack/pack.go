// Package pack writes and reads an exported sprite pack.
//
// A pack is a directory of one PNG strip and one JSON sidecar per entry, plus a
// manifest.json. Nothing derived from the ROM is bundled: the pack is produced
// from the user's own dump.
//
//	manifest.json            format, importer, source identity, entries
//	<entry>/strip.png        frames side by side, frameSize square each
//	<entry>/entry.json       measurement, durations, loop metadata
package pack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
)

// Format is the pack layout revision.
const Format = 1

// TickRate is the frame rate the durations are counted in.
const TickRate = 60

// Manifest is manifest.json.
type Manifest struct {
	Format   int    `json:"format"`
	Importer string `json:"importer"`
	TickRate int    `json:"tickRate"`
	// FrameSize is the cell edge the pack was exported with, or 0 when each
	// entry fitted its own (see Entry.FrameSize).
	FrameSize int      `json:"frameSize"`
	Source    Source   `json:"source"`
	Entries   []*Entry `json:"entries"`
}

// Source identifies the dump a pack came from, so a pack built from the wrong
// ROM is caught instead of silently mixing sprites.
type Source struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	GameCode string `json:"gameCode"`
	Digest   string `json:"sha256"`
}

// Entry is one sprite: a strip plus its measurement.
type Entry struct {
	ID string `json:"id"`
	// Label is a human-readable name; the synthetic demo uses it, imported
	// entries leave it empty and are identified by ID alone.
	Label string `json:"label,omitempty"`
	// Dex, Side, Female and Palette identify the slot the sprite came from.
	Dex     int    `json:"dex"`
	Side    string `json:"side"`
	Female  bool   `json:"female"`
	Palette string `json:"palette"`
	// Fallback is set when a female slot was requested but the species has
	// none, so the male graphic was exported instead.
	Fallback string `json:"genderFallbackOf,omitempty"`
	Frames   int    `json:"frames"`
	// FrameSize is the square cell both the strip and the measurement use.
	FrameSize int `json:"frameSize"`
	// Ground is the strip row the animation's lowest content row lands on.
	Ground int `json:"ground"`
	// GroundFrom names the table Ground came from, when it did not come from
	// the command line (see internal/gen3).
	GroundFrom string `json:"groundFrom,omitempty"`
	// Grounded is false when the sprite is too tall to reach Ground without
	// leaving the cell; it is pinned to the top row instead.
	Grounded bool `json:"grounded"`
	// AnchorX, AnchorY place the sprite origin inside a cell.
	AnchorX int `json:"anchorX"`
	AnchorY int `json:"anchorY"`
	// Offsets is where each frame's content box sits inside its cell, so a
	// renderer that ignores the strip's padding still lands on Ground.
	Offsets [][2]int `json:"offsets"`
	// Durations is the tick duration of each frame, index-aligned with Offsets.
	Durations []int `json:"durations"`
	// LoopStart is the frame index the idle loop restarts at.
	LoopStart int `json:"loopStart"`
	// CycleTicks is the animation's full length in ticks before looping.
	CycleTicks int `json:"cycleTicks"`
	// Capped reports that the cycle was cut at the export tick bound.
	Capped bool `json:"cycleCapped"`
	// Union is the cell-space box enclosing every frame.
	Union [4]int `json:"union"`
	Strip string `json:"strip"`
}

// IDFor builds the stable entry id.
func IDFor(dex int, side, palette string, female bool) string {
	gender := ""
	if female {
		gender = "/female"
	}
	return fmt.Sprintf("%03d/%s%s/%s", dex, side, gender, palette)
}

// Digest hashes a file for the source record.
func Digest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// WriteEntry writes entry e's strip and sidecar under root.
func WriteEntry(root string, e *Entry, frames []*image.Paletted) error {
	if len(frames) != len(e.Durations) || len(frames) != len(e.Offsets) {
		return fmt.Errorf("entry %s has %d frames, %d durations and %d offsets",
			e.ID, len(frames), len(e.Durations), len(e.Offsets))
	}
	size := e.FrameSize
	strip := image.NewPaletted(image.Rect(0, 0, size*len(frames), size), frames[0].Palette)
	for i, f := range frames {
		if f.Bounds() != image.Rect(0, 0, size, size) {
			return fmt.Errorf("entry %s frame %d is %v, want %dx%d", e.ID, i, f.Bounds(), size, size)
		}
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				if index := f.ColorIndexAt(x, y); index != 0 {
					strip.SetColorIndex(i*size+x, y, index)
				}
			}
		}
	}

	dir := filepath.Join(root, filepath.FromSlash(e.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(dir, "strip.png"))
	if err != nil {
		return err
	}
	if err := png.Encode(file, strip); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	e.Strip = e.ID + "/strip.png"
	body, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, "entry.json"), append(body, '\n'))
}

// writeFileAtomic writes path through a sibling temp file, so an interrupted
// export never leaves a half-written sidecar behind.
func writeFileAtomic(path string, body []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	if _, err := file.Write(body); err != nil {
		file.Close()
		os.Remove(temp)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}

// WriteManifest writes m as root's manifest.json, entries sorted by id.
func WriteManifest(root string, m *Manifest) error {
	sort.Slice(m.Entries, func(i, j int) bool { return m.Entries[i].ID < m.Entries[j].ID })
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(root, "manifest.json"), append(body, '\n'))
}

// ReadManifest loads and shape-checks a pack manifest.
func ReadManifest(root string) (*Manifest, error) {
	body, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks the manifest's format version and shared frame size.
func (m *Manifest) Validate() error {
	if m.Format != Format {
		return fmt.Errorf("pack format %d, want %d (rebuild the pack)", m.Format, Format)
	}
	if m.FrameSize < 0 || m.FrameSize > 256 {
		return fmt.Errorf("pack frame size %d outside 0..256", m.FrameSize)
	}
	if m.TickRate <= 0 {
		return fmt.Errorf("pack tick rate %d", m.TickRate)
	}
	return nil
}

// PaletteOf converts colours into a drawable palette.
func PaletteOf(colours []color.NRGBA) color.Palette {
	p := make(color.Palette, len(colours))
	for i, c := range colours {
		p[i] = c
	}
	return p
}
