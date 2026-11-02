package nds

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// image writes a synthetic ROM image: a header pointing at the given tables,
// then the tables themselves. Nothing here is game data; the tests only need
// the table layout.
func image(t *testing.T, fntOff int, fnt []byte, fatOff int, fat []byte) string {
	t.Helper()
	data := make([]byte, 0x400)
	copy(data[12:16], "TEST")
	binary.LittleEndian.PutUint32(data[0x40:], uint32(fntOff))
	binary.LittleEndian.PutUint32(data[0x44:], uint32(len(fnt)))
	binary.LittleEndian.PutUint32(data[0x48:], uint32(fatOff))
	binary.LittleEndian.PutUint32(data[0x4c:], uint32(len(fat)))
	if fntOff > 0 && fntOff <= len(data)-len(fnt) {
		copy(data[fntOff:], fnt)
	}
	if fatOff > 0 && fatOff <= len(data)-len(fat) {
		copy(data[fatOff:], fat)
	}
	path := filepath.Join(t.TempDir(), "test.nds")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// fnt is a root directory with one file named test.bin.
func fntOneFile() []byte {
	b := make([]byte, 8, 32)
	binary.LittleEndian.PutUint32(b[0:], 8) // root subtable starts after the record
	binary.LittleEndian.PutUint16(b[4:], 0) // root owns file id 0
	binary.LittleEndian.PutUint16(b[6:], 1) // one directory
	b = append(b, 8)                        // a file entry: length only, no directory flag
	b = append(b, "test.bin"...)
	b = append(b, 0)
	return b
}

// fatOneFile maps file id 0 onto [dataStart, dataEnd).
func fatOneFile(dataStart, dataEnd int) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b[0:], uint32(dataStart))
	binary.LittleEndian.PutUint32(b[4:], uint32(dataEnd))
	return b
}

func TestOpenReadsAFile(t *testing.T) {
	const start, end = 0x300 + 32, 0x300 + 32 + 5
	path := image(t, 0x300, fntOneFile(), 0x350, fatOneFile(start, end))
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Code() != "TEST" || r.Size() != 0x400 {
		t.Fatalf("code %q size %d", r.Code(), r.Size())
	}
	body, err := r.File("/test.bin")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(make([]byte, 5)) {
		t.Fatalf("File = %q, want five zero bytes", body)
	}
}

func TestOpenRejectsBrokenImages(t *testing.T) {
	goodFNT, goodFAT := fntOneFile(), fatOneFile(0x300, 0x305)
	tooSmall := filepath.Join(t.TempDir(), "small.nds")
	if err := os.WriteFile(tooSmall, make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"truncated header", tooSmall, "truncated ROM header"},
		{"fnt outside image", image(t, 0x3f0, goodFNT, 0x350, goodFAT), "file name table"},
		{"fat outside image", image(t, 0x300, goodFNT, 0x3fc, goodFAT), "file allocation table"},
		{"short fnt", image(t, 0x300, []byte{1, 2, 3, 4}, 0x350, goodFAT), "invalid file name table length 4"},
		{"fat not a multiple of 8", image(t, 0x300, goodFNT, 0x350, make([]byte, 12)), "non-zero multiple of 8"},
		{"no directories", image(t, 0x300, []byte{8, 0, 0, 0, 0, 0, 0, 0}, 0x350, goodFAT), "invalid root directory count"},
	}
	for _, c := range cases {
		_, err := Open(c.path)
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: %v, want %q", c.name, err, c.wantErr)
		}
	}
}

func TestFileReportsMissingPathsAndBadRanges(t *testing.T) {
	path := image(t, 0x300, fntOneFile(), 0x350, fatOneFile(0x300, 0x305))
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.File("/missing.bin"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := r.File(""); err == nil {
		t.Fatal("empty path was accepted")
	}

	// An inverted FAT range must be an error, not an empty slice.
	r, err = Open(image(t, 0x300, fntOneFile(), 0x350, fatOneFile(0x305, 0x300)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.File("/test.bin"); err == nil || !strings.Contains(err.Error(), "inverted range") {
		t.Fatalf("inverted range: %v", err)
	}

	// A range that runs past the end of the image must be an error too.
	r, err = Open(image(t, 0x300, fntOneFile(), 0x350, fatOneFile(0x300, 0x1000)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.File("/test.bin"); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("past the end: %v", err)
	}
}

func TestRootNames(t *testing.T) {
	r, err := Open(image(t, 0x300, fntOneFile(), 0x350, fatOneFile(0x300, 0x305)))
	if err != nil {
		t.Fatal(err)
	}
	names := r.RootNames()
	if len(names) != 1 || names[0] != "test.bin" {
		t.Fatalf("RootNames = %v", names)
	}
}
