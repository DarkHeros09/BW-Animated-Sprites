// Package gen3 reads the per-species picture table a Generation 3 game uses to
// place a battler on the ground.
//
// The layout is the one gen1recomp's src/import/gba/pic_coords_extract.lua reads
// (a pret gMonFrontPicCoords / gMonBackPicCoords table): one record per species
// slot, byte 0 packs the picture size in 8px units as floor(size/16) units wide
// and size%16 units tall, byte 1 is the ground offset counted up from the bottom
// edge of a 64px cell. A size byte of 0 means the slot has no picture.
//
// Facets (ElvieBlooms/facets, mons.lua:552-556) turns that offset into a row:
// bottom = SIZE - yo, and the lowest content row must land on bottom-1. The
// offset is therefore reported here unchanged; internal/measure resolves it
// against whatever cell size a sprite ends up with.
package gen3

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileFormat is the coords file revision.
const FileFormat = 1

// Coords is one species slot's picture measurement.
type Coords struct {
	// Width, Height are the picture size in pixels, 0 when Empty.
	Width, Height int
	// GroundOffset is the Facets yo: rows between the picture's lowest row and
	// the bottom edge of the cell.
	GroundOffset int
	// Empty marks a slot the game leaves blank.
	Empty bool
}

// Parse reads count records of stride bytes each, starting at offset.
func Parse(rom []byte, offset, count, stride int) ([]Coords, error) {
	if count <= 0 {
		return nil, fmt.Errorf("species count %d must be positive", count)
	}
	if stride < 2 {
		return nil, fmt.Errorf("record stride %d must be at least 2 (size and ground offset)", stride)
	}
	if offset < 0 {
		return nil, fmt.Errorf("table offset %d must not be negative", offset)
	}
	end := offset + count*stride
	if end < offset || end > len(rom) {
		return nil, fmt.Errorf("table at 0x%x needs %d bytes but the ROM holds %d", offset, count*stride, len(rom))
	}
	out := make([]Coords, count)
	for i := range out {
		record := rom[offset+i*stride:]
		size, yo := record[0], record[1]
		if size == 0 {
			out[i] = Coords{Empty: true}
			continue
		}
		out[i] = Coords{
			Width:        int(size>>4) * 8,
			Height:       int(size&0xF) * 8,
			GroundOffset: int(yo),
		}
	}
	return out, nil
}

// File is a coords file: both sides of one game's tables.
type File struct {
	Format int    `json:"format"`
	Game   string `json:"game,omitempty"`
	Source string `json:"source,omitempty"`
	Stride int    `json:"stride,omitempty"`
	// Front and Back are indexed by species slot. The slot is the game's own
	// internal species id, which is not national dex order.
	Front []Coords `json:"front"`
	Back  []Coords `json:"back"`
}

// For returns the measurement for a side and species slot. ok is false when the
// side has no record for that slot.
func (f *File) For(side string, slot int) (Coords, bool) {
	table := f.Front
	if side == "back" {
		table = f.Back
	}
	if slot < 0 || slot >= len(table) {
		return Coords{}, false
	}
	c := table[slot]
	if c.Empty {
		return Coords{}, false
	}
	return c, true
}

// Load reads and validates a coords file.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Format != FileFormat {
		return nil, fmt.Errorf("%s: format %d, want %d", path, f.Format, FileFormat)
	}
	if len(f.Front) == 0 && len(f.Back) == 0 {
		return nil, fmt.Errorf("%s: no front or back records", path)
	}
	return &f, nil
}

// Save writes the file atomically, so an interrupted run cannot leave a
// half-written table behind.
func (f *File) Save(path string) error {
	f.Format = FileFormat
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".coords-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
