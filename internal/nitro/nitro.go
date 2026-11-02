// Package nitro decodes Nitro container formats: LZ10/LZ11 streams, NARC archives
// and NNS 2D resource sections. Layouts follow AnimaEngine v1.0.0 (MIT); see
// LICENSES/ANIMAENGINE_LICENSE and docs/PROVENANCE.md.
package nitro

import (
	"encoding/binary"
	"fmt"
)

// DefaultMaxOutput bounds an LZ expansion when the caller sets no limit.
const DefaultMaxOutput = 8 << 20

// Reader is a bounds-checked little-endian view over a byte buffer.
type Reader struct{ data []byte }

// NewReader wraps data.
func NewReader(data []byte) *Reader { return &Reader{data: data} }

// Size reports the buffer length.
func (r *Reader) Size() int { return len(r.data) }

// Bounds reports whether off+n fits inside the buffer.
func (r *Reader) Bounds(off, n int) error {
	if off < 0 || n < 0 || off > len(r.data)-n {
		return fmt.Errorf("range %d+%d outside %d-byte buffer", off, n, len(r.data))
	}
	return nil
}

// Sub returns the sub-slice off..off+n.
func (r *Reader) Sub(off, n int) ([]byte, error) {
	if err := r.Bounds(off, n); err != nil {
		return nil, err
	}
	return r.data[off : off+n], nil
}

// U8 reads one byte at off.
func (r *Reader) U8(off int) (byte, error) {
	if err := r.Bounds(off, 1); err != nil {
		return 0, err
	}
	return r.data[off], nil
}

// U16 reads a little-endian uint16 at off.
func (r *Reader) U16(off int) (int, error) {
	if err := r.Bounds(off, 2); err != nil {
		return 0, err
	}
	return int(binary.LittleEndian.Uint16(r.data[off:])), nil
}

// U32 reads a little-endian uint32 at off.
func (r *Reader) U32(off int) (int, error) {
	if err := r.Bounds(off, 4); err != nil {
		return 0, err
	}
	return int(binary.LittleEndian.Uint32(r.data[off:])), nil
}

// S16 reads a signed 16-bit value at off.
func (r *Reader) S16(off int) (int, error) {
	n, err := r.U16(off)
	if err != nil {
		return 0, err
	}
	if n >= 0x8000 {
		n -= 0x10000
	}
	return n, nil
}

// S32 reads a signed 32-bit value at off.
func (r *Reader) S32(off int) (int, error) {
	n, err := r.U32(off)
	if err != nil {
		return 0, err
	}
	if n >= 0x80000000 {
		n -= 0x100000000
	}
	return n, nil
}

// Section is one block of a Nitro file's section table.
type Section struct {
	Offset     int
	Size       int
	DataOffset int
}

// Sections parses the section table of a Nitro file that starts with magic.
func Sections(data []byte, magic string) (map[string]Section, *Reader, error) {
	r := NewReader(data)
	head, err := r.Sub(0, 4)
	if err != nil {
		return nil, nil, fmt.Errorf("truncated Nitro header: %w", err)
	}
	if string(head) != magic {
		return nil, nil, fmt.Errorf("unexpected Nitro signature %q, want %q", head, magic)
	}
	size, err := r.U32(8)
	if err != nil {
		return nil, nil, fmt.Errorf("truncated Nitro header: %w", err)
	}
	header, err := r.U16(12)
	if err != nil {
		return nil, nil, fmt.Errorf("truncated Nitro header: %w", err)
	}
	count, err := r.U16(14)
	if err != nil {
		return nil, nil, fmt.Errorf("truncated Nitro header: %w", err)
	}
	if size > r.Size() || size < 16 || header < 16 || header > size || count <= 0 || count > 64 {
		return nil, nil, fmt.Errorf("invalid Nitro header: size %d, section table %d, %d sections", size, header, count)
	}

	sections := make(map[string]Section, count)
	pos := header
	for i := 0; i < count; i++ {
		if pos+8 > size {
			return nil, nil, fmt.Errorf("truncated Nitro section table at offset %d", pos)
		}
		tag, err := r.Sub(pos, 4)
		if err != nil {
			return nil, nil, err
		}
		length, err := r.U32(pos + 4)
		if err != nil {
			return nil, nil, err
		}
		if length < 8 || length > size-pos {
			return nil, nil, fmt.Errorf("invalid Nitro section %q: length %d at offset %d", tag, length, pos)
		}
		if _, dup := sections[string(tag)]; dup {
			return nil, nil, fmt.Errorf("duplicate Nitro section %q", tag)
		}
		sections[string(tag)] = Section{Offset: pos, Size: length, DataOffset: pos + 8}
		pos += length
	}
	return sections, r, nil
}

// Decompress expands an LZ10 or LZ11 stream. Uncompressed data passes through.
func Decompress(data []byte, maxOutput int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty compressed stream")
	}
	kind := data[0]
	if kind != 0x10 && kind != 0x11 {
		return data, nil
	}
	if maxOutput <= 0 {
		maxOutput = DefaultMaxOutput
	}

	r := NewReader(data)
	head, err := r.U32(0)
	if err != nil {
		return nil, fmt.Errorf("truncated LZ header: %w", err)
	}
	size, pos := head>>8, 4
	if size == 0 && kind == 0x11 {
		if size, err = r.U32(4); err != nil {
			return nil, fmt.Errorf("truncated LZ11 header: %w", err)
		}
		pos = 8
	}
	if size <= 0 || size > maxOutput {
		return nil, fmt.Errorf("LZ output size %d outside 1..%d", size, maxOutput)
	}

	out := make([]byte, size)
	n := 0
	next := func() (byte, error) {
		b, err := r.U8(pos)
		pos++
		return b, err
	}
	for n < size {
		flags, err := next()
		if err != nil {
			return nil, fmt.Errorf("LZ stream ended after %d of %d bytes: %w", n, size, err)
		}
		for bit := 7; bit >= 0 && n < size; bit-- {
			if flags&(1<<uint(bit)) == 0 {
				b, err := next()
				if err != nil {
					return nil, fmt.Errorf("LZ stream ended after %d of %d bytes: %w", n, size, err)
				}
				out[n] = b
				n++
				continue
			}
			a, err := next()
			if err != nil {
				return nil, fmt.Errorf("LZ stream ended after %d of %d bytes: %w", n, size, err)
			}
			b, err := next()
			if err != nil {
				return nil, fmt.Errorf("LZ stream ended after %d of %d bytes: %w", n, size, err)
			}
			high := int(a) >> 4
			var length, disp int
			switch {
			case kind == 0x10:
				length, disp = high+3, int(a&0x0f)<<8|int(b)
			case high == 0:
				c, err := next()
				if err != nil {
					return nil, fmt.Errorf("LZ stream ended after %d of %d bytes: %w", n, size, err)
				}
				length = (int(a&0x0f) << 4) + (int(b) >> 4) + 17
				disp = int(b&0x0f)<<8 | int(c)
			case high == 1:
				c, err := next()
				if err != nil {
					return nil, fmt.Errorf("LZ stream ended after %d of %d bytes: %w", n, size, err)
				}
				d, err := next()
				if err != nil {
					return nil, fmt.Errorf("LZ stream ended after %d of %d bytes: %w", n, size, err)
				}
				length = (int(a&0x0f) << 12) + (int(b) << 4) + (int(c) >> 4) + 273
				disp = int(c&0x0f)<<8 | int(d)
			default:
				length, disp = high+1, int(a&0x0f)<<8|int(b)
			}
			if disp+1 > n {
				return nil, fmt.Errorf("LZ back-reference %d before %d bytes of output", disp+1, n)
			}
			for i := 0; i < length && n < size; i++ {
				out[n] = out[n-disp-1]
				n++
			}
		}
	}
	return out, nil
}

// Archive is a parsed NARC member table.
type Archive struct {
	payload []byte
	fat     []byte
	count   int
}

// OpenArchive parses a NARC archive.
func OpenArchive(data []byte) (*Archive, error) {
	sections, r, err := Sections(data, "NARC")
	if err != nil {
		return nil, err
	}
	fat, ok := sections["BTAF"]
	if !ok {
		return nil, fmt.Errorf("NARC is missing its BTAF member table")
	}
	body, ok := sections["GMIF"]
	if !ok {
		return nil, fmt.Errorf("NARC is missing its GMIF payload")
	}
	if _, ok := sections["BTNF"]; !ok {
		return nil, fmt.Errorf("NARC is missing its BTNF name table")
	}
	count, err := r.U32(fat.Offset + 8)
	if err != nil {
		return nil, fmt.Errorf("truncated NARC member count: %w", err)
	}
	if count <= 0 || count > 20000 || 12+count*8 > fat.Size {
		return nil, fmt.Errorf("invalid NARC member table: %d entries in a %d-byte table", count, fat.Size)
	}
	table, err := r.Sub(fat.Offset+12, count*8)
	if err != nil {
		return nil, err
	}
	return &Archive{payload: data[body.DataOffset : body.Offset+body.Size], fat: table, count: count}, nil
}

// Count reports the member count.
func (a *Archive) Count() int { return a.count }

// Member returns member index's payload, which may be empty or compressed.
func (a *Archive) Member(index int) ([]byte, error) {
	if index < 0 || index >= a.count {
		return nil, fmt.Errorf("NARC member %d outside 0..%d", index, a.count-1)
	}
	start := int(binary.LittleEndian.Uint32(a.fat[index*8:]))
	end := int(binary.LittleEndian.Uint32(a.fat[index*8+4:]))
	if end < start {
		return nil, fmt.Errorf("NARC member %d has an inverted range %d..%d", index, start, end)
	}
	if end > len(a.payload) {
		return nil, fmt.Errorf("NARC member %d range %d..%d outside %d-byte payload", index, start, end, len(a.payload))
	}
	return a.payload[start:end], nil
}

// MemberDecompressed returns the LZ-decoded member payload.
func (a *Archive) MemberDecompressed(index int, maxOutput int) ([]byte, error) {
	raw, err := a.Member(index)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return raw, nil
	}
	out, err := Decompress(raw, maxOutput)
	if err != nil {
		return nil, fmt.Errorf("NARC member %d: %w", index, err)
	}
	return out, nil
}
