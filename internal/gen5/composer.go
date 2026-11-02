package gen5

import (
	"fmt"
	"math"
	"sort"
)

// MaxComposedOAMs bounds the objects a single tick may compose.
const MaxComposedOAMs = 2048

// Canvas is a composed indexed frame: opaque pixels keyed by y*Width+x.
type Canvas struct {
	Width, Height int
	MinX, MinY    int
	Pixels        map[int]uint8
}

type oamItem struct {
	oam         OAM
	a, b, c, d  float64
	tx, ty      float64
	determinant float64
	index       int
	minX, minY  float64
	maxX, maxY  float64
}

func scaleOf(v int) float64 {
	if v == 0 {
		return 1
	}
	return float64(v) / 4096
}

func transform(t Transform, x, y float64) (float64, float64) {
	angle := float64(t.Rotation) * math.Pi * 2 / 65536
	sx, sy := x*scaleOf(t.ScaleX), y*scaleOf(t.ScaleY)
	return float64(t.TranslateX) + sx*math.Cos(angle) - sy*math.Sin(angle),
		float64(t.TranslateY) + sx*math.Sin(angle) + sy*math.Cos(angle)
}

func matrix(r MapRecord, t Transform) (a, b, c, d, tx, ty float64) {
	point := func(x, y float64) (float64, float64) {
		x, y = transform(t, x, y)
		return x + float64(r.X), y + float64(r.Y)
	}
	tx, ty = point(0, 0)
	ax, ay := point(1, 0)
	bx, by := point(0, 1)
	return ax - tx, ay - ty, bx - tx, by - ty, tx, ty
}

func compose(cells *Cells, nanr *Bank, nmcr *Maps, mapIndex, tick int) ([]oamItem, error) {
	if mapIndex < 0 || mapIndex >= len(nmcr.Maps) {
		return nil, fmt.Errorf("multicell map %d outside 0..%d", mapIndex, len(nmcr.Maps)-1)
	}
	m := nmcr.Maps[mapIndex]
	items := make([]oamItem, 0, 8)
	for _, record := range m.Records {
		t, err := FrameAt(nanr, record.AnimationIndex, tick)
		if err != nil {
			return nil, err
		}
		if t.CellID < 0 || t.CellID >= len(cells.Cells) {
			return nil, fmt.Errorf("animation refers to missing cell %d", t.CellID)
		}
		cell := cells.Cells[t.CellID]
		a, b, c, d, tx, ty := matrix(record, t)
		det := a*d - b*c
		if math.Abs(det) < 1e-12 {
			return nil, fmt.Errorf("singular sprite transform in map %d", mapIndex)
		}
		for _, oam := range cell.OAMs {
			if oam.Disabled {
				continue
			}
			if len(items) >= MaxComposedOAMs {
				return nil, fmt.Errorf("composed object limit %d exceeded", MaxComposedOAMs)
			}
			it := oamItem{
				oam: oam, a: a, b: b, c: c, d: d, tx: tx, ty: ty, determinant: det,
				index: len(items), minX: math.Inf(1), minY: math.Inf(1),
				maxX: math.Inf(-1), maxY: math.Inf(-1),
			}
			for _, corner := range [4][2]float64{
				{float64(oam.DrawX), float64(oam.DrawY)},
				{float64(oam.DrawX + oam.Width), float64(oam.DrawY)},
				{float64(oam.DrawX), float64(oam.DrawY + oam.Height)},
				{float64(oam.DrawX + oam.Width), float64(oam.DrawY + oam.Height)},
			} {
				wx := a*corner[0] + c*corner[1] + tx
				wy := b*corner[0] + d*corner[1] + ty
				it.minX, it.minY = math.Min(it.minX, wx), math.Min(it.minY, wy)
				it.maxX, it.maxY = math.Max(it.maxX, wx), math.Max(it.maxY, wy)
			}
			items = append(items, it)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].oam.Priority != items[j].oam.Priority {
			return items[i].oam.Priority > items[j].oam.Priority
		}
		return items[i].index > items[j].index
	})
	return items, nil
}

func round(v float64) int {
	if v >= 0 {
		return int(math.Floor(v + 0.5))
	}
	return int(math.Ceil(v - 0.5))
}

// Render composes one tick of mapIndex into an indexed canvas.
func Render(g *Graphics, pal *Palette, cells *Cells, nanr *Bank, nmcr *Maps, mapIndex, tick int) (*Canvas, error) {
	items, err := compose(cells, nanr, nmcr, mapIndex, tick)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("map %d composes no visible objects", mapIndex)
	}

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, it := range items {
		minX, minY = math.Min(minX, math.Floor(it.minX)), math.Min(minY, math.Floor(it.minY))
		maxX, maxY = math.Max(maxX, math.Ceil(it.maxX)), math.Max(maxY, math.Ceil(it.maxY))
	}
	width, height := int(maxX)-int(minX), int(maxY)-int(minY)
	if width <= 0 || height <= 0 || width > 1024 || height > 1024 {
		return nil, fmt.Errorf("composed canvas %dx%d outside bounds", width, height)
	}

	canvas := &Canvas{Width: width, Height: height, MinX: int(minX), MinY: int(minY), Pixels: map[int]uint8{}}
	for _, it := range items {
		oam := it.oam
		minXp := max(0, int(math.Floor(it.minX))-canvas.MinX-1)
		minYp := max(0, int(math.Floor(it.minY))-canvas.MinY-1)
		maxXp := min(canvas.Width-1, int(math.Ceil(it.maxX))-canvas.MinX+1)
		maxYp := min(canvas.Height-1, int(math.Ceil(it.maxY))-canvas.MinY+1)
		for y := minYp; y <= maxYp; y++ {
			for x := minXp; x <= maxXp; x++ {
				dx := float64(x + canvas.MinX - int(it.tx))
				dy := float64(y + canvas.MinY - int(it.ty))
				sx := round((dx*it.d-dy*it.c)/it.determinant) - oam.DrawX
				sy := round((-dx*it.b+dy*it.a)/it.determinant) - oam.DrawY
				if sx < 0 || sy < 0 || sx >= oam.Width || sy >= oam.Height {
					continue
				}
				if oam.FlipH {
					sx = oam.Width - 1 - sx
				}
				if oam.FlipV {
					sy = oam.Height - 1 - sy
				}
				tile := oam.Tile + sy/8*32 + sx/8
				color := g.Pixel(tile, sx%8, sy%8)
				if color == 0 {
					continue
				}
				index := int(color)
				if g.Bpp() == 4 && len(pal.Colors) > 16 {
					index = oam.Palette*16 + int(color)
				}
				if index >= len(pal.Colors) {
					index = int(color)
				}
				if index < len(pal.Colors) {
					canvas.Pixels[y*canvas.Width+x] = uint8(index)
				}
			}
		}
	}
	if len(canvas.Pixels) == 0 {
		return nil, fmt.Errorf("map %d tick %d renders no opaque pixels", mapIndex, tick)
	}
	return canvas, nil
}
