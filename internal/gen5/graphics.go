// Package gen5 decodes Black/White battle sprites out of an NDS ROM.
// The container, cell, animation and composition algorithms follow AnimaEngine
// v1.0.0 (MIT, Copyright (c) 2026 AnimaEngine contributors) as ported by
// gen1recomp's src/import/gen5; see docs/PROVENANCE.md and
// LICENSES/ANIMAENGINE_LICENSE.
package gen5

import (
	"fmt"
	"image/color"

	"bwsprites/internal/nitro"
)

// MaxEntryBytes bounds one decompressed resource.
const MaxEntryBytes = 8 << 20

// findSection walks a Nitro file's section table from offset 0x10.
func findSection(r *nitro.Reader, magic string) (offset, size int, err error) {
	for pos := 0x10; pos+8 <= r.Size(); {
		tag, err := r.Sub(pos, 4)
		if err != nil {
			return 0, 0, err
		}
		length, err := r.U32(pos + 4)
		if err != nil {
			return 0, 0, err
		}
		if length < 8 {
			return 0, 0, fmt.Errorf("invalid Nitro section length %d", length)
		}
		if err := r.Bounds(pos, length); err != nil {
			return 0, 0, err
		}
		if string(tag) == magic {
			return pos, length, nil
		}
		pos += length
	}
	return 0, 0, fmt.Errorf("missing Nitro section %q", magic)
}

// Graphics is a decoded NCGR character bank.
type Graphics struct {
	bpp        int
	tiles      int
	widthTiles int
	pixels     []byte
}

// ParseGraphics decodes an NCGR resource.
func ParseGraphics(data []byte) (*Graphics, error) {
	if len(data) > MaxEntryBytes {
		return nil, fmt.Errorf("NCGR is %d bytes, over the %d limit", len(data), MaxEntryBytes)
	}
	r := nitro.NewReader(data)
	if err := r.Bounds(0, 0x30); err != nil {
		return nil, fmt.Errorf("truncated NCGR header: %w", err)
	}
	if head, _ := r.Sub(0, 4); string(head) != "RGCN" {
		return nil, fmt.Errorf("invalid NCGR signature %q", head)
	}
	section, size, err := findSection(r, "RAHC")
	if err != nil {
		return nil, err
	}
	if size < 0x20 {
		return nil, fmt.Errorf("NCGR RAHC section is %d bytes, need 32", size)
	}

	width, err := r.U16(section + 8)
	if err != nil {
		return nil, err
	}
	if _, err := r.U16(section + 10); err != nil { // declared height, unused
		return nil, err
	}
	depth, err := r.U32(section + 12)
	if err != nil {
		return nil, err
	}
	kind, err := r.U32(section + 20)
	if err != nil {
		return nil, err
	}
	length, err := r.U32(section + 24)
	if err != nil {
		return nil, err
	}

	bpp := 0
	switch depth {
	case 3:
		bpp = 4
	case 4:
		bpp = 8
	default:
		return nil, fmt.Errorf("unsupported NCGR bit depth %d", depth)
	}
	perTile := bpp * 8
	if length == 0 {
		length = size - 0x20
	}
	if length <= 0 || length > size-0x20 || length%perTile != 0 {
		return nil, fmt.Errorf("invalid NCGR tile data length %d", length)
	}
	pixels, err := r.Sub(section+0x20, length)
	if err != nil {
		return nil, err
	}

	widthTiles := width
	if kind == 1 && bpp == 4 {
		widthTiles = 32 // linear: tile n sits at (n%32, n/32) in a 256-wide raster
	}
	if widthTiles <= 0 {
		widthTiles = 32
	}
	return &Graphics{bpp: bpp, tiles: length / perTile, widthTiles: widthTiles, pixels: pixels}, nil
}

// Bpp reports the bit depth.
func (g *Graphics) Bpp() int { return g.bpp }

// Tiles reports the tile count.
func (g *Graphics) Tiles() int { return g.tiles }

// Pixel reads tile's pixel (x,y), returning 0 outside the tile bank.
func (g *Graphics) Pixel(tile, x, y int) uint8 {
	if tile < 0 || tile >= g.tiles || x < 0 || x > 7 || y < 0 || y > 7 {
		return 0
	}
	at := tile*64 + y*8 + x
	if g.bpp == 4 && g.widthTiles == 32 {
		at = (tile%32*8 + x) + (tile/32*8+y)*256
	}
	if g.bpp == 8 {
		if at >= len(g.pixels) {
			return 0
		}
		return g.pixels[at]
	}
	nibble := at / 2
	if nibble >= len(g.pixels) {
		return 0
	}
	if at%2 == 0 {
		return g.pixels[nibble] & 0x0f
	}
	return g.pixels[nibble] >> 4
}

// Color is one palette entry.
type Color struct{ R, G, B, A uint8 }

// Palette is a decoded NCLR palette, index 0 always transparent.
type Palette struct{ Colors []Color }

// ParsePalette decodes an NCLR resource.
func ParsePalette(data []byte) (*Palette, error) {
	if len(data) > MaxEntryBytes {
		return nil, fmt.Errorf("NCLR is %d bytes, over the %d limit", len(data), MaxEntryBytes)
	}
	r := nitro.NewReader(data)
	if err := r.Bounds(0, 0x28); err != nil {
		return nil, fmt.Errorf("truncated NCLR header: %w", err)
	}
	if head, _ := r.Sub(0, 4); string(head) != "RLCN" {
		return nil, fmt.Errorf("invalid NCLR signature %q", head)
	}
	section, size, err := findSection(r, "TTLP")
	if err != nil {
		return nil, err
	}
	if size < 0x18 {
		return nil, fmt.Errorf("NCLR TTLP section is %d bytes, need 24", size)
	}
	length, err := r.U32(section + 0x10)
	if err != nil {
		return nil, err
	}
	if length == 0 {
		length = size - 0x18
	}
	if length <= 0 || length > size-0x18 || length%2 != 0 {
		return nil, fmt.Errorf("invalid NCLR palette length %d", length)
	}
	count := min(length/2, 256)

	p := &Palette{Colors: make([]Color, count)}
	expand := func(v int) uint8 { return uint8(v*8 + v/4) }
	for i := 0; i < count; i++ {
		v, err := r.U16(section + 0x18 + i*2)
		if err != nil {
			return nil, err
		}
		a := uint8(255)
		if i == 0 {
			a = 0
		}
		p.Colors[i] = Color{R: expand(v % 32), G: expand(v / 32 % 32), B: expand(v / 1024 % 32), A: a}
	}
	return p, nil
}

// Image converts the palette to the indexed colour table an image.Paletted
// needs. Index 0 is transparent, the NNS convention this importer relies on.
func (p *Palette) Image() color.Palette {
	out := make(color.Palette, len(p.Colors))
	for i, c := range p.Colors {
		out[i] = color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A}
	}
	return out
}

// PaletteByName returns the normal or shiny palette of a decoded sprite.
func (s *Sprite) PaletteByName(name string) (*Palette, error) {
	switch name {
	case "normal":
		return s.Normal, nil
	case "shiny":
		return s.Shiny, nil
	}
	return nil, fmt.Errorf("invalid palette %q, want normal or shiny", name)
}
