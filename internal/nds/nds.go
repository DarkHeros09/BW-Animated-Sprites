// Package nds reads a Nintendo DS ROM image's header and filesystem tables.
// The FNT/FAT layout follows AnimaEngine v1.0.0 (MIT, see LICENSES/ANIMAENGINE_LICENSE).
package nds

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// HeaderSize is the fixed NDS header length; nothing before it is FS data.
const HeaderSize = 512

// ROM is an opened, fully buffered NDS image. The file on disk is never written.
type ROM struct {
	path     string
	data     []byte
	fnt      []byte
	fat      []byte
	code     string
	dirCount int
}

// Open reads path into memory and validates the filesystem tables it contains.
func Open(path string) (*ROM, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ROM: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat ROM: %w", err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%s is a directory, expected an .nds file", path)
	}
	if st.Size() < HeaderSize {
		return nil, fmt.Errorf("truncated ROM header: %d bytes, need at least %d", st.Size(), HeaderSize)
	}
	data := make([]byte, st.Size())
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, fmt.Errorf("read ROM: %w", err)
	}

	r := &ROM{path: path, data: data, code: string(data[12:16])}
	if err := r.loadTables(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *ROM) loadTables() error {
	fo := int(binary.LittleEndian.Uint32(r.data[0x40:]))
	fl := int(binary.LittleEndian.Uint32(r.data[0x44:]))
	ao := int(binary.LittleEndian.Uint32(r.data[0x48:]))
	al := int(binary.LittleEndian.Uint32(r.data[0x4c:]))
	if err := r.check(fo, fl); err != nil {
		return fmt.Errorf("file name table: %w", err)
	}
	if err := r.check(ao, al); err != nil {
		return fmt.Errorf("file allocation table: %w", err)
	}
	if fl < 8 {
		return fmt.Errorf("invalid file name table length %d", fl)
	}
	if al == 0 || al%8 != 0 {
		return fmt.Errorf("invalid file allocation table length %d, want a non-zero multiple of 8", al)
	}

	r.fnt, r.fat = r.data[fo:fo+fl], r.data[ao:ao+al]
	r.dirCount = int(binary.LittleEndian.Uint16(r.fnt[6:]))
	if r.dirCount <= 0 || r.dirCount > 4096 || r.dirCount*8 > fl {
		return fmt.Errorf("invalid root directory count %d", r.dirCount)
	}
	return nil
}

func (r *ROM) check(off, length int) error {
	if off < 0 || length < 0 || off > len(r.data)-length {
		return fmt.Errorf("range %d+%d outside %d-byte image", off, length, len(r.data))
	}
	return nil
}

// List names a directory's entries, for diagnostics.
func (r *ROM) List(path string) ([]string, error) {
	dir := 0
	parts := splitPath(path)
	for _, part := range parts {
		pos := r.u32at(r.fnt, dir*8)
		next := -1
		for pos < len(r.fnt) {
			control := r.u8(pos)
			pos++
			if control == 0 {
				break
			}
			length := int(control & 0x7f)
			if length == 0 || pos+length > len(r.fnt) {
				return nil, fmt.Errorf("bad name")
			}
			name := string(r.fnt[pos : pos+length])
			pos += length
			if control >= 0x80 {
				child := int(binary.LittleEndian.Uint16(r.fnt[pos:]))
				pos += 2
				if name == part {
					next = child - 0xf000
				}
			}
		}
		if next < 0 {
			return nil, fmt.Errorf("no dir %q", part)
		}
		dir = next
	}
	pos := r.u32at(r.fnt, dir*8)
	var out []string
	for pos < len(r.fnt) {
		control := r.u8(pos)
		pos++
		if control == 0 {
			break
		}
		length := int(control & 0x7f)
		if length == 0 || pos+length > len(r.fnt) {
			break
		}
		out = append(out, string(r.fnt[pos:pos+length]))
		pos += length
		if control >= 0x80 {
			pos += 2
		}
	}
	return out, nil
}

// RootNames lists the root directory's entries, for diagnostics.
func (r *ROM) RootNames() []string {
	var out []string
	for pos := r.u32at(r.fnt, 0); pos < len(r.fnt); {
		control := r.u8(pos)
		pos++
		if control == 0 {
			break
		}
		length := int(control & 0x7f)
		if length == 0 || pos+length > len(r.fnt) {
			break
		}
		out = append(out, string(r.fnt[pos:pos+length]))
		pos += length
		if control >= 0x80 {
			pos += 2
		}
	}
	return out
}

// Image returns the whole image, for diagnostics.
func (r *ROM) Image() []byte { return r.data }

// Code reports the 4-byte game code from the header (for example "IRBO").
func (r *ROM) Code() string { return r.code }

// Size reports the image size in bytes.
func (r *ROM) Size() int { return len(r.data) }

// Path reports the source image path.
func (r *ROM) Path() string { return r.path }

func (r *ROM) u8(pos int) byte { return r.fnt[pos] }

func (r *ROM) u16(pos int) int { return int(binary.LittleEndian.Uint16(r.fnt[pos:])) }

func (r *ROM) u32at(b []byte, pos int) int { return int(binary.LittleEndian.Uint32(b[pos:])) }

// File returns the contents of the ROM file at slash-separated path.
func (r *ROM) File(path string) ([]byte, error) {
	parts := splitPath(path)
	if len(parts) == 0 || len(parts) > 32 {
		return nil, fmt.Errorf("invalid ROM path %q", path)
	}

	dir := 0
	for i, part := range parts {
		if dir < 0 || dir >= r.dirCount {
			return nil, fmt.Errorf("ROM path %q escapes the directory table", path)
		}
		// Each directory entry names its own name-table offset and its first
		// file id; ids of the following files count up from there.
		pos, id := r.u32at(r.fnt, dir*8), r.u16(dir*8+4)
		if pos < r.dirCount*8 || pos >= len(r.fnt) {
			return nil, fmt.Errorf("ROM path %q has a corrupt name table offset", path)
		}

		var child int
		var isDir bool
		var found bool
		for pos < len(r.fnt) {
			control := r.u8(pos)
			pos++
			if control == 0 {
				break
			}
			length, isDirectory := int(control&0x7f), control >= 0x80
			if length == 0 || pos+length > len(r.fnt) {
				return nil, fmt.Errorf("ROM path %q has an empty or truncated name", path)
			}
			name := string(r.fnt[pos : pos+length])
			pos += length
			if isDirectory {
				if pos+2 > len(r.fnt) {
					return nil, fmt.Errorf("ROM path %q has a truncated directory id", path)
				}
				child = r.u16(pos)
				pos += 2
			}
			if name != part {
				if !isDirectory {
					id++
				}
				continue
			}
			found, isDir = true, isDirectory
			break
		}
		if !found {
			return nil, fmt.Errorf("ROM file not found: %q", path)
		}

		if i == len(parts)-1 {
			if isDir {
				return nil, fmt.Errorf("ROM path %q is a directory", path)
			}
			if id >= len(r.fat)/8 {
				return nil, fmt.Errorf("ROM file %q has an out-of-range id %d", path, id)
			}
			first := r.u32at(r.fat, id*8)
			last := r.u32at(r.fat, id*8+4)
			if last < first {
				return nil, fmt.Errorf("ROM file %q has an inverted range %d..%d", path, first, last)
			}
			if err := r.check(first, last-first); err != nil {
				return nil, fmt.Errorf("ROM file %q: %w", path, err)
			}
			return r.data[first:last], nil
		}
		if !isDir || child < 0xf000 {
			return nil, fmt.Errorf("ROM path %q: %q is not a directory", path, part)
		}
		dir = child - 0xf000
	}
	return nil, errors.New("empty ROM path")
}

func splitPath(path string) []string {
	var parts []string
	start := -1
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if start >= 0 {
				parts = append(parts, path[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return parts
}
