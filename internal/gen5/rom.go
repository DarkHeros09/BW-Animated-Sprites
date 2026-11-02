package gen5

import (
	"fmt"
	"os"
	"path/filepath"

	"bwsprites/internal/nds"
	"bwsprites/internal/nitro"
)

// ImporterID names this importer in pack manifests.
const ImporterID = "gen5_bw"

// ExportVersion is the pack format revision this importer writes.
const ExportVersion = "1.0.0"

// MaxDex is the highest national dex number Black/White contains.
const MaxDex = 649

// MaxTicks bounds one exported animation cycle.
const MaxTicks = 240

// UntrimmedSize is the expected ROM length; a trimmed dump breaks the
// filesystem offsets the importer relies on.
const UntrimmedSize = 268435456

// spriteArchive is the ROM path of the 20-files-per-species sprite NARC.
const spriteArchive = "/a/0/0/4"

// Sprite is one decoded species/gender/side asset.
type Sprite struct {
	Dex            int
	Side           string
	Female         bool
	GenderFallback bool
	Graphics       *Graphics
	Cells          *Cells
	NANR           *Bank
	NCMR           *Maps
	NMAR           *Bank
	Normal         *Palette
	Shiny          *Palette
	MapIndex       int
}

// ROM is an opened Black/White dump with its sprite archive.
type ROM struct {
	inner   *nds.ROM
	archive *nitro.Archive
}

// Identify opens path and checks that it is a supported, untrimmed dump. Size
// and game code are checked before the filesystem is parsed, so a trimmed dump
// or a different game gets an actionable message instead of a complaint about
// a file name table.
func Identify(path string) (*ROM, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	var head [4]byte
	if _, err := f.ReadAt(head[:], 0x0C); err != nil {
		f.Close()
		return nil, fmt.Errorf("%s is too short to be a ROM dump: %w", filepath.Base(path), err)
	}
	f.Close()
	if info.Size() != UntrimmedSize {
		return nil, fmt.Errorf("%s is %d bytes, want an untrimmed %d byte dump (trimmed ROMs move the file offsets)",
			filepath.Base(path), info.Size(), UntrimmedSize)
	}
	if code := string(head[:]); code[:3] != "IRA" && code[:3] != "IRB" {
		return nil, fmt.Errorf("game code %q is not Black/White (IRA*/IRB*); Black 2/White 2 and other revisions are not supported", code)
	}

	r, err := nds.Open(path)
	if err != nil {
		return nil, err
	}

	raw, err := r.File(spriteArchive)
	if err != nil {
		return nil, fmt.Errorf("sprite archive: %w", err)
	}
	// The archive is LZ11-compressed inside the ROM; decompressing is a no-op if
	// a revision ever stores it plain.
	data, err := nitro.Decompress(raw, 8<<20)
	if err != nil {
		return nil, fmt.Errorf("sprite archive %s: %w", spriteArchive, err)
	}
	archive, err := nitro.OpenArchive(data)
	if err != nil {
		return nil, fmt.Errorf("sprite archive %s: %w", spriteArchive, err)
	}
	if archive.Count() < (MaxDex+1)*20 {
		return nil, fmt.Errorf("sprite archive has %d members, want at least %d for a complete Black/White dump",
			archive.Count(), (MaxDex+1)*20)
	}
	return &ROM{inner: r, archive: archive}, nil
}

// Code reports the ROM game code.
func (r *ROM) Code() string { return r.inner.Code() }

// Path reports the source ROM path.
func (r *ROM) Path() string { return r.inner.Path() }

// Size reports the source ROM size in bytes.
func (r *ROM) Size() int { return r.inner.Size() }

// Name reports a human-readable identification.
func (r *ROM) Name() string {
	if r.inner.Code()[:3] == "IRB" {
		return "Pokemon Black (" + r.inner.Code() + ")"
	}
	return "Pokemon White (" + r.inner.Code() + ")"
}

func (r *ROM) member(index int) ([]byte, error) {
	data, err := r.archive.MemberDecompressed(index, MaxEntryBytes)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("member %d is empty", index)
	}
	return data, nil
}

// HasFemale reports whether the species has a distinct female graphic.
func (r *ROM) HasFemale(dex int, side string) (bool, error) {
	if dex < 1 || dex > MaxDex {
		return false, fmt.Errorf("national dex %d outside 1..%d", dex, MaxDex)
	}
	offset, err := sideOffset(side)
	if err != nil {
		return false, err
	}
	data, err := r.archive.Member(dex*20 + offset + 1)
	if err != nil {
		return false, err
	}
	return len(data) > 0, nil
}

func sideOffset(side string) (int, error) {
	switch side {
	case "front":
		return 2, nil
	case "back":
		return 11, nil
	}
	return 0, fmt.Errorf("invalid sprite side %q, want front or back", side)
}

// Read decodes one sprite.
func (r *ROM) Read(dex int, side string, female bool) (*Sprite, error) {
	if dex < 1 || dex > MaxDex {
		return nil, fmt.Errorf("national dex %d outside 1..%d", dex, MaxDex)
	}
	base := dex * 20
	graphicsOffset, err := sideOffset(side)
	if err != nil {
		return nil, err
	}
	first := 4
	if side == "back" {
		first = 13
	}

	s := &Sprite{Dex: dex, Side: side, Female: female}
	if female {
		candidate, err := r.archive.Member(base + graphicsOffset + 1)
		if err != nil {
			return nil, err
		}
		if len(candidate) == 0 {
			s.GenderFallback = true
			female = false
		} else {
			graphicsOffset++
		}
	}

	read := func(index int) ([]byte, error) { return r.member(index) }
	graphics, err := read(base + graphicsOffset)
	if err != nil {
		return nil, fmt.Errorf("dex %d %s graphics: %w", dex, side, err)
	}
	if s.Graphics, err = ParseGraphics(graphics); err != nil {
		return nil, fmt.Errorf("dex %d %s graphics: %w", dex, side, err)
	}
	cells, err := read(base + first)
	if err != nil {
		return nil, fmt.Errorf("dex %d %s cells: %w", dex, side, err)
	}
	if s.Cells, err = ParseCells(cells); err != nil {
		return nil, fmt.Errorf("dex %d %s cells: %w", dex, side, err)
	}
	nanr, err := read(base + first + 1)
	if err != nil {
		return nil, fmt.Errorf("dex %d %s animation: %w", dex, side, err)
	}
	if s.NANR, err = ParseBank(nanr, "RNAN", false); err != nil {
		return nil, fmt.Errorf("dex %d %s animation: %w", dex, side, err)
	}
	nmcr, err := read(base + first + 2)
	if err != nil {
		return nil, fmt.Errorf("dex %d %s multicell: %w", dex, side, err)
	}
	if s.NCMR, err = ParseMaps(nmcr); err != nil {
		return nil, fmt.Errorf("dex %d %s multicell: %w", dex, side, err)
	}
	nmar, err := read(base + first + 3)
	if err != nil {
		return nil, fmt.Errorf("dex %d %s part animation: %w", dex, side, err)
	}
	if s.NMAR, err = ParseBank(nmar, "RAMN", true); err != nil {
		return nil, fmt.Errorf("dex %d %s part animation: %w", dex, side, err)
	}
	normal, err := read(base + 18)
	if err != nil {
		return nil, fmt.Errorf("dex %d normal palette: %w", dex, err)
	}
	if s.Normal, err = ParsePalette(normal); err != nil {
		return nil, fmt.Errorf("dex %d normal palette: %w", dex, err)
	}
	shiny, err := read(base + 19)
	if err != nil {
		return nil, fmt.Errorf("dex %d shiny palette: %w", dex, err)
	}
	if s.Shiny, err = ParsePalette(shiny); err != nil {
		return nil, fmt.Errorf("dex %d shiny palette: %w", dex, err)
	}
	if s.MapIndex, _, err = Idle(s.NMAR); err != nil {
		return nil, fmt.Errorf("dex %d %s idle animation: %w", dex, side, err)
	}
	return s, nil
}
