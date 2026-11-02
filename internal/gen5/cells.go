package gen5

import (
	"fmt"

	"bwsprites/internal/nitro"
)

// OAM sizes indexed by shape then object size.
var (
	oamWidths  = [3][4]int{{8, 16, 32, 64}, {16, 32, 32, 64}, {8, 8, 16, 32}}
	oamHeights = [3][4]int{{8, 16, 32, 64}, {8, 8, 16, 32}, {16, 32, 32, 64}}
)

// OAM is one cell object with its resolved geometry.
type OAM struct {
	X, Y     int
	Width    int
	Height   int
	Tile     int
	Palette  int
	Priority int
	Affine   bool
	Double   bool
	FlipH    bool
	FlipV    bool
	Disabled bool
	DrawX    int
	DrawY    int
}

// Cell is one NCER cell.
type Cell struct{ OAMs []OAM }

// Cells is a decoded NCER cell bank.
type Cells struct{ Cells []Cell }

func bits(n, shift, mod int) int { return n >> uint(shift) % mod }

// ParseCells decodes an NCER resource.
func ParseCells(data []byte) (*Cells, error) {
	if len(data) > MaxEntryBytes {
		return nil, fmt.Errorf("NCER is %d bytes, over the %d limit", len(data), MaxEntryBytes)
	}
	r := nitro.NewReader(data)
	if err := r.Bounds(0, 0x20); err != nil {
		return nil, fmt.Errorf("truncated NCER header: %w", err)
	}
	if head, _ := r.Sub(0, 4); string(head) != "RECN" {
		return nil, fmt.Errorf("invalid NCER signature %q", head)
	}
	section, size, err := findSection(r, "KBEC")
	if err != nil {
		return nil, err
	}
	if size < 0x20 {
		return nil, fmt.Errorf("NCER KBEC section is %d bytes, need 32", size)
	}
	count, err := r.U16(section + 8)
	if err != nil {
		return nil, err
	}
	if count <= 0 || count > 1024 {
		return nil, fmt.Errorf("NCER cell count %d outside 1..1024", count)
	}
	offset, err := r.U32(section + 12)
	if err != nil {
		return nil, err
	}
	tableStart := section + 8 + offset
	oamBase := tableStart + count*8
	if offset < 0x18 || oamBase > section+size {
		return nil, fmt.Errorf("invalid NCER cell table: offset %d, %d cells", offset, count)
	}

	out := &Cells{Cells: make([]Cell, count)}
	for i := 0; i < count; i++ {
		entry := tableStart + i*8
		oamCount, err := r.U16(entry)
		if err != nil {
			return nil, err
		}
		rawOffset, err := r.U32(entry + 4)
		if err != nil {
			return nil, err
		}
		start := oamBase + rawOffset
		if oamCount > 128 || start+oamCount*6 > section+size {
			return nil, fmt.Errorf("cell %d has an invalid OAM range: %d objects at %d", i, oamCount, start)
		}
		cell := Cell{OAMs: make([]OAM, oamCount)}
		for j := 0; j < oamCount; j++ {
			pos := start + j*6
			a, err := r.U16(pos)
			if err != nil {
				return nil, err
			}
			b, err := r.U16(pos + 2)
			if err != nil {
				return nil, err
			}
			c, err := r.U16(pos + 4)
			if err != nil {
				return nil, err
			}
			shape, objSize := bits(a, 14, 4), bits(b, 14, 4)
			if shape >= 3 {
				return nil, fmt.Errorf("cell %d object %d has an unsupported shape %d", i, j, shape)
			}
			x, y := b%512, a%256
			if x >= 256 {
				x -= 512
			}
			if y >= 128 {
				y -= 256
			}
			affine := bits(a, 8, 2) == 1
			double := affine && bits(a, 9, 2) == 1
			w, h := oamWidths[shape][objSize], oamHeights[shape][objSize]
			o := OAM{
				X: x, Y: y, Width: w, Height: h, Tile: c % 1024, Priority: bits(c, 10, 16),
				Palette: bits(c, 12, 16), Affine: affine, Double: double,
				Disabled: !affine && bits(a, 9, 2) == 1,
				FlipH:    !affine && bits(b, 12, 2) == 1, FlipV: !affine && bits(b, 13, 2) == 1,
				DrawX: x, DrawY: y,
			}
			if double {
				o.DrawX += w / 2
				o.DrawY += h / 2
			}
			cell.OAMs[j] = o
		}
		out.Cells[i] = cell
	}
	return out, nil
}
