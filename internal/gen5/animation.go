package gen5

import (
	"fmt"

	"bwsprites/internal/nitro"
)

// Element sizes in bytes, indexed by transform format.
var elementSizes = [4]int{2, 16, 8, 4}

// Transform is one animation element.
type Transform struct {
	CellID     int
	Rotation   int
	ScaleX     int
	ScaleY     int
	TranslateX int
	TranslateY int
}

// AnimFrame is one keyframed element.
type AnimFrame struct {
	Transform
	Duration int
}

// Animation is one named animation track.
type Animation struct {
	Frames    []AnimFrame
	LoopStart int
	Playback  int
	Label     string
}

// Bank is a decoded NANR or NMAR animation bank.
type Bank struct{ Animations []Animation }

// MapRecord is one part placement of a multicell (NCMR) map.
type MapRecord struct {
	AnimationIndex int
	X, Y           int
}

// Map is one NCMR multicell map.
type Map struct{ Records []MapRecord }

// Maps is a decoded NCMR bank.
type Maps struct{ Maps []Map }

func sections(r *nitro.Reader, magic string) (map[string]nitro.Section, error) {
	if err := r.Bounds(0, 16); err != nil {
		return nil, fmt.Errorf("truncated Nitro animation header: %w", err)
	}
	head, err := r.Sub(0, 4)
	if err != nil {
		return nil, err
	}
	if string(head) != magic {
		return nil, fmt.Errorf("invalid Nitro animation magic %q, want %q", head, magic)
	}
	out := make(map[string]nitro.Section)
	for pos := 16; pos < r.Size(); {
		if err := r.Bounds(pos, 8); err != nil {
			return nil, fmt.Errorf("truncated Nitro section table: %w", err)
		}
		size, err := r.U32(pos + 4)
		if err != nil {
			return nil, err
		}
		if size < 8 {
			return nil, fmt.Errorf("invalid Nitro section size %d", size)
		}
		if err := r.Bounds(pos, size); err != nil {
			return nil, err
		}
		tag, _ := r.Sub(pos, 4)
		if _, dup := out[string(tag)]; dup {
			return nil, fmt.Errorf("duplicate Nitro section %q", tag)
		}
		out[string(tag)] = nitro.Section{Offset: pos, Size: size, DataOffset: pos + 8}
		pos += size
	}
	return out, nil
}

func readLabels(r *nitro.Reader, section nitro.Section, animations []Animation) error {
	start := section.Offset + 8
	base := start + len(animations)*4
	if err := r.Bounds(start, len(animations)*4); err != nil {
		return err
	}
	for i := range animations {
		rel, err := r.U32(start + i*4)
		if err != nil {
			return err
		}
		pos := base + rel
		if err := r.Bounds(pos, 1); err != nil {
			return err
		}
		label := make([]byte, 0, 32)
		for j := 0; j < 31; j++ {
			if pos+j >= section.Offset+section.Size {
				break
			}
			b, err := r.U8(pos + j)
			if err != nil {
				return err
			}
			if b == 0 {
				break
			}
			label = append(label, b)
		}
		animations[i].Label = string(label)
	}
	return nil
}

func parseElement(r *nitro.Reader, pos, format, limit int) (Transform, error) {
	if format < 0 || format >= len(elementSizes) {
		return Transform{}, fmt.Errorf("unsupported animation element format %d", format)
	}
	if err := r.Bounds(pos, elementSizes[format]); err != nil {
		return Transform{}, err
	}
	if pos+elementSizes[format] > limit {
		return Transform{}, fmt.Errorf("animation element at %d overruns its span", pos)
	}
	cell, err := r.U16(pos)
	if err != nil {
		return Transform{}, err
	}
	t := Transform{CellID: cell, ScaleX: 4096, ScaleY: 4096}
	switch format {
	case 1:
		if t.Rotation, err = r.S16(pos + 2); err != nil {
			return Transform{}, err
		}
		if t.ScaleX, err = r.S32(pos + 4); err != nil {
			return Transform{}, err
		}
		if t.ScaleY, err = r.S32(pos + 8); err != nil {
			return Transform{}, err
		}
		if t.TranslateX, err = r.S16(pos + 12); err != nil {
			return Transform{}, err
		}
		if t.TranslateY, err = r.S16(pos + 14); err != nil {
			return Transform{}, err
		}
		if abs(t.ScaleX) > 65536 || abs(t.ScaleY) > 65536 {
			return Transform{}, fmt.Errorf("animation scale %d/%d exceeds bounds", t.ScaleX, t.ScaleY)
		}
	case 2:
		if t.TranslateX, err = r.S16(pos + 4); err != nil {
			return Transform{}, err
		}
		if t.TranslateY, err = r.S16(pos + 6); err != nil {
			return Transform{}, err
		}
	}
	return t, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ParseBank decodes an NANR (isNmar false) or NMAR (isNmar true) bank.
func ParseBank(data []byte, magic string, isNmar bool) (*Bank, error) {
	if len(data) > MaxEntryBytes {
		return nil, fmt.Errorf("animation bank is %d bytes, over the %d limit", len(data), MaxEntryBytes)
	}
	r := nitro.NewReader(data)
	sec, err := sections(r, magic)
	if err != nil {
		return nil, err
	}
	section, ok := sec["KNBA"]
	if !ok {
		return nil, fmt.Errorf("animation bank is missing its KNBA section")
	}
	s, limit := section.Offset, section.Offset+section.Size
	if err := r.Bounds(s, 24); err != nil {
		return nil, err
	}
	count, err := r.U16(s + 8)
	if err != nil {
		return nil, err
	}
	if count <= 0 || count > 2048 {
		return nil, fmt.Errorf("animation count %d outside 1..2048", count)
	}
	tableRel, err := r.U32(s + 12)
	if err != nil {
		return nil, err
	}
	frameRel, err := r.U32(s + 16)
	if err != nil {
		return nil, err
	}
	elementRel, err := r.U32(s + 20)
	if err != nil {
		return nil, err
	}
	tableBase, frameBase, elementBase := s+8+tableRel, s+8+frameRel, s+8+elementRel
	if err := r.Bounds(tableBase, count*16); err != nil {
		return nil, err
	}
	if frameBase > limit || elementBase > limit {
		return nil, fmt.Errorf("animation bank bases %d/%d overrun the section", frameBase, elementBase)
	}

	offsets := make(map[int]bool)
	forms := make([]int, count)
	bases := make([]int, count)
	frameCounts := make([]int, count)
	total := 0
	for i := 0; i < count; i++ {
		p := tableBase + i*16
		n, err := r.U16(p)
		if err != nil {
			return nil, err
		}
		if n <= 0 || n > 4096 {
			return nil, fmt.Errorf("animation %d has an invalid frame count %d", i, n)
		}
		total += n
		if total > 65536 {
			return nil, fmt.Errorf("animation bank exceeds the 65536 frame limit")
		}
		if forms[i], err = r.U16(p + 4); err != nil {
			return nil, err
		}
		rel, err := r.U32(p + 12)
		if err != nil {
			return nil, err
		}
		base := frameBase + rel
		if err := r.Bounds(base, n*8); err != nil {
			return nil, err
		}
		bases[i], frameCounts[i] = base, n
		for j := 0; j < n; j++ {
			off, err := r.U32(base + j*8)
			if err != nil {
				return nil, err
			}
			offsets[off] = true
		}
	}

	// Element spans are implied by the sorted offsets; the last one runs to the
	// section end. That is how the source decoder recovers per-element length.
	sorted := make([]int, 0, len(offsets)+1)
	for off := range offsets {
		sorted = append(sorted, off)
	}
	sorted = append(sorted, limit-elementBase)
	sortInts(sorted)
	spans := make(map[int]int, len(sorted))
	for i := 0; i+1 < len(sorted); i++ {
		spans[sorted[i]] = sorted[i+1] - sorted[i]
	}

	bank := &Bank{Animations: make([]Animation, count)}
	for i := 0; i < count; i++ {
		p := tableBase + i*16
		loopStart, err := r.U16(p + 2)
		if err != nil {
			return nil, err
		}
		playback, err := r.U32(p + 8)
		if err != nil {
			return nil, err
		}
		a := Animation{Frames: make([]AnimFrame, frameCounts[i]), LoopStart: loopStart, Playback: playback}
		if a.LoopStart >= frameCounts[i] {
			a.LoopStart = 0
		}
		for j := 0; j < frameCounts[i]; j++ {
			f := bases[i] + j*8
			off, err := r.U32(f)
			if err != nil {
				return nil, err
			}
			ep := elementBase + off
			available := min(spans[off], limit-ep)
			if err := r.Bounds(ep, 2); err != nil {
				return nil, fmt.Errorf("animation %d frame %d: %w", i, j, err)
			}
			format := forms[i]
			if !isNmar {
				// NANR elements may be shorter than the declared format claims;
				// a 0xCCCC cell id marks the 4-byte variant.
				if id, _ := r.U16(ep + 2); available == 4 && id == 0xCCCC {
					format = 3
				} else if format > 3 || available < elementSizes[format] {
					switch {
					case available == 2 || available == 4:
						format = 0
					case available == 8:
						format = 2
					case available >= 16:
						format = 1
					default:
						return nil, fmt.Errorf("animation %d frame %d has an invalid element span %d", i, j, available)
					}
				}
			}
			t, err := parseElement(r, ep, format, ep+available)
			if err != nil {
				return nil, fmt.Errorf("animation %d frame %d: %w", i, j, err)
			}
			duration, err := r.U16(f + 4)
			if err != nil {
				return nil, err
			}
			a.Frames[j] = AnimFrame{Transform: t, Duration: duration}
		}
		bank.Animations[i] = a
	}

	if labelSection, ok := sec["LBAL"]; ok {
		if err := readLabels(r, labelSection, bank.Animations); err != nil {
			return nil, err
		}
	}
	return bank, nil
}

// ParseMaps decodes an NCMR multicell map bank.
func ParseMaps(data []byte) (*Maps, error) {
	if len(data) > MaxEntryBytes {
		return nil, fmt.Errorf("NCMR is %d bytes, over the %d limit", len(data), MaxEntryBytes)
	}
	r := nitro.NewReader(data)
	sec, err := sections(r, "RCMN")
	if err != nil {
		return nil, err
	}
	section, ok := sec["KBCM"]
	if !ok {
		return nil, fmt.Errorf("multicell bank is missing its KBCM section")
	}
	s, limit := section.Offset, section.Offset+section.Size
	if err := r.Bounds(s, 24); err != nil {
		return nil, err
	}
	count, err := r.U16(s + 8)
	if err != nil {
		return nil, err
	}
	if count <= 0 || count > 2048 {
		return nil, fmt.Errorf("multicell map count %d outside 1..2048", count)
	}
	tableRel, err := r.U32(s + 12)
	if err != nil {
		return nil, err
	}
	tableBase := s + 8 + tableRel
	if err := r.Bounds(tableBase, count*8); err != nil {
		return nil, err
	}
	recordBase := tableBase + count*8
	if recordBase > limit {
		return nil, fmt.Errorf("multicell record base %d overruns the section", recordBase)
	}

	out := &Maps{Maps: make([]Map, count)}
	total := 0
	for i := 0; i < count; i++ {
		p := tableBase + i*8
		n, err := r.U16(p)
		if err != nil {
			return nil, err
		}
		if n > 4096 {
			return nil, fmt.Errorf("multicell map %d has an invalid record count %d", i, n)
		}
		total += n
		if total > 65536 {
			return nil, fmt.Errorf("multicell bank exceeds the 65536 record limit")
		}
		rel, err := r.U32(p + 4)
		if err != nil {
			return nil, err
		}
		base := recordBase + rel
		if err := r.Bounds(base, n*8); err != nil {
			return nil, err
		}
		m := Map{Records: make([]MapRecord, n)}
		for j := 0; j < n; j++ {
			f := base + j*8
			idx, err := r.U16(f)
			if err != nil {
				return nil, err
			}
			x, err := r.S16(f + 2)
			if err != nil {
				return nil, err
			}
			y, err := r.S16(f + 4)
			if err != nil {
				return nil, err
			}
			m.Records[j] = MapRecord{AnimationIndex: idx, X: x, Y: y}
		}
		out.Maps[i] = m
	}
	return out, nil
}
