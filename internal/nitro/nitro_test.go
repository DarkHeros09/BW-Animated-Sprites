package nitro

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// literalStream wraps payload in an LZ11 stream that only uses literal groups.
func literalStream(payload []byte) []byte {
	// LZ11 header: tag byte, then the 24-bit output size.
	out := []byte{0x11, byte(len(payload)), byte(len(payload) >> 8), byte(len(payload) >> 16)}
	for len(payload) > 0 {
		n := min(8, len(payload))
		out = append(out, 0x00) // every bit clear: eight literals
		out = append(out, payload[:n]...)
		payload = payload[n:]
	}
	return out
}

func TestDecompressLZ11Literals(t *testing.T) {
	payload := []byte("a sprite strip that compresses to nothing")
	got, err := Decompress(literalStream(append([]byte(nil), payload...)), 0)
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q, want %q", got, payload)
	}
}

func TestDecompressLZ11BackReference(t *testing.T) {
	// "AB" then a 4-byte copy from 2 bytes back, which repeats "AB".
	stream := []byte{0x11, 6, 0, 0, 0x20, 'A', 'B', 0x30, 0x01}
	got, err := Decompress(stream, 0)
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if string(got) != "ABABAB" {
		t.Fatalf("got %q, want ABABAB", got)
	}
}

func TestDecompressLeavesPlainDataAlone(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x02}
	got, err := Decompress(raw, 0)
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("got %v, want %v", got, raw)
	}
}

func TestDecompressRejectsBadStreams(t *testing.T) {
	cases := []struct {
		name  string
		data  []byte
		limit int
		want  string
	}{
		{"empty", nil, 0, "empty compressed stream"},
		{"truncated header", []byte{0x11, 0x04}, 0, "truncated"},
		{"copy before any output", []byte{0x11, 4, 0, 0, 0x80, 0x20, 0x01}, 0, "back-reference"},
		{"stream ends early", []byte{0x11, 8, 0, 0, 0x00}, 0, "ended after"},
		{"output over the cap", literalStream([]byte("0123456789")), 4, "outside 1..4"},
	}
	for _, c := range cases {
		_, err := Decompress(c.data, c.limit)
		if err == nil {
			t.Errorf("%s: Decompress accepted the stream", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not mention %q", c.name, err, c.want)
		}
	}
}

// section builds one Nitro section: a four-byte tag, its length and a body.
func section(tag string, body []byte) []byte {
	out := append([]byte(tag), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(out[4:], uint32(8+len(body)))
	return append(out, body...)
}

// narc assembles a Nitro archive holding the given members, laid out the way
// Black/White's /a/0/0/4 archive is: BTAF member table, BTNF names, GMIF data.
func narc(members [][]byte) []byte {
	table := make([]byte, 0, len(members)*8)
	payload := make([]byte, 0)
	for _, m := range members {
		table = binary.LittleEndian.AppendUint32(table, uint32(len(payload)))
		payload = append(payload, m...)
		table = binary.LittleEndian.AppendUint32(table, uint32(len(payload)))
	}
	// BTAF body: the member count, then one start/end pair per member.
	btaf := binary.LittleEndian.AppendUint32(nil, uint32(len(members)))
	btaf = append(btaf, table...)

	body := append(section("BTAF", btaf), section("BTNF", make([]byte, 8))...)
	body = append(body, section("GMIF", payload)...)

	head := append([]byte("NARC"), 0xFE, 0xFF, 0x00, 0x01)
	head = binary.LittleEndian.AppendUint32(head, uint32(16+len(body)))
	head = binary.LittleEndian.AppendUint16(head, 0x10)
	head = binary.LittleEndian.AppendUint16(head, 3)
	return append(head, body...)
}

func TestArchiveReadsMembers(t *testing.T) {
	compressed := literalStream([]byte("lazy"))
	data := narc([][]byte{[]byte("hello"), compressed, nil})
	archive, err := OpenArchive(data)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	if archive.Count() != 3 {
		t.Fatalf("count = %d, want 3", archive.Count())
	}
	first, err := archive.Member(0)
	if err != nil {
		t.Fatalf("Member: %v", err)
	}
	if string(first) != "hello" {
		t.Fatalf("member 0 = %q, want hello", first)
	}
	if raw, _ := archive.Member(1); bytes.Equal(raw, []byte("lazy")) {
		t.Fatal("member 1 was not compressed, so the decode proves nothing")
	}
	decoded, err := archive.MemberDecompressed(1, 0)
	if err != nil {
		t.Fatalf("MemberDecompressed: %v", err)
	}
	if string(decoded) != "lazy" {
		t.Fatalf("member 1 decoded to %q, want lazy", decoded)
	}
	empty, err := archive.MemberDecompressed(2, 0)
	if err != nil {
		t.Fatalf("MemberDecompressed on an empty member: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty member decoded to %d bytes", len(empty))
	}
}

func TestArchiveRejectsBrokenLayouts(t *testing.T) {
	if _, err := OpenArchive([]byte("nope")); err == nil {
		t.Error("accepted a file that is not a Nitro archive")
	}
	noNames := narc([][]byte{[]byte("x")})
	copy(noNames[bytes.Index(noNames, []byte("BTNF")):], "JUNK")
	if _, err := OpenArchive(noNames); err == nil || !strings.Contains(err.Error(), "BTNF") {
		t.Errorf("archive without a name table: %v", err)
	}
	noMembers := narc(nil)
	if _, err := OpenArchive(noMembers); err == nil || !strings.Contains(err.Error(), "member table") {
		t.Errorf("archive with no members: %v", err)
	}
}

func TestArchiveRejectsBadMemberRanges(t *testing.T) {
	data := narc([][]byte{[]byte("abc")})
	archive, err := OpenArchive(data)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	if _, err := archive.Member(1); err == nil {
		t.Error("accepted a member index past the end")
	}
	if _, err := archive.Member(-1); err == nil {
		t.Error("accepted a negative member index")
	}

	// Rewrite the single member entry so it ends before it starts.
	entry := bytes.Index(data, []byte("BTAF")) + 8 + 4
	binary.LittleEndian.PutUint32(data[entry:], 2)
	binary.LittleEndian.PutUint32(data[entry+4:], 1)
	broken, err := OpenArchive(data)
	if err != nil {
		t.Fatalf("OpenArchive after patching: %v", err)
	}
	if _, err := broken.Member(0); err == nil || !strings.Contains(err.Error(), "inverted") {
		t.Errorf("inverted member range: %v", err)
	}
}
