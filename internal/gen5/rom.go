package gen5

import (
	"fmt"
	"os"
	"path/filepath"

	"bwsprites/internal/nds"
	"bwsprites/internal/nitro"
)

// ImporterID names the Black/White importer in pack manifests.
const ImporterID = "gen5_bw"

// ImporterIDBW2 names the Black 2/White 2 importer in pack manifests. The two
// games share the archive layout this package decodes, but a pack records which
// dump it came from.
const ImporterIDBW2 = "gen5_bw2"

// ExportVersion is the pack format revision this importer writes.
const ExportVersion = "1.0.0"

// MaxDex is the highest national dex number Black/White contains.
const MaxDex = 649

// MaxTicks bounds one exported animation cycle.
const MaxTicks = 240

// UntrimmedSize is the expected Black/White ROM length; a trimmed dump breaks
// the filesystem offsets the importer relies on.
const UntrimmedSize = 268435456

// UntrimmedSizeBW2 is the expected Black 2/White 2 ROM length. Black 2 and
// White 2 are 512 MiB dumps: a larger sound archive and the extra form sprite
// entries than Black/White's 256 MiB dump.
const UntrimmedSizeBW2 = 536870912

// game is one supported generation 5 pair: the untrimmed dump length, the game
// codes it accepts and the importer id a pack records.
type game struct {
	family    string
	untrimmed int
	importer  string
	names     map[string]string
}

// supported lists the dumps this importer reads. Black 2/White 2 keep the same
// sprite archive (/a/0/0/4), the same 20 members per species and the same
// NCGR/NCER/NANR/NCMR/NMAR/palette order, with their extra form graphics after
// national dex 649, so one decoder serves both; every member is still validated
// while it is read, so a dump that differs fails with the member at fault.
var supported = []game{
	{
		family: "bw", untrimmed: UntrimmedSize, importer: ImporterID,
		names: map[string]string{"IRA": "Pokemon White", "IRB": "Pokemon Black"},
	},
	{
		family: "bw2", untrimmed: UntrimmedSizeBW2, importer: ImporterIDBW2,
		names: map[string]string{"IRD": "Pokemon White 2", "IRE": "Pokemon Black 2"},
	},
}

// gameFor returns the game a 4-byte ROM code belongs to.
func gameFor(code string) (game, string, bool) {
	if len(code) < 3 {
		return game{}, "", false
	}
	for _, g := range supported {
		if name, ok := g.names[code[:3]]; ok {
			return g, name, true
		}
	}
	return game{}, "", false
}

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

// ROM is an opened Black/White or Black 2/White 2 dump with its sprite archive.
type ROM struct {
	inner   *nds.ROM
	archive *nitro.Archive
	game    game
	title   string
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
	if info.Size() != UntrimmedSize && info.Size() != UntrimmedSizeBW2 {
		return nil, fmt.Errorf("%s is %d bytes, want an untrimmed %d byte Black/White or %d byte Black 2/White 2 dump (trimmed ROMs move the file offsets)",
			filepath.Base(path), info.Size(), UntrimmedSize, UntrimmedSizeBW2)
	}
	code := string(head[:])
	g, title, ok := gameFor(code)
	if !ok {
		return nil, fmt.Errorf("game code %q is not a supported game: this tool reads Black/White (IRA*/IRB*) and Black 2/White 2 (IRD*/IRE*) dumps", code)
	}
	if info.Size() != int64(g.untrimmed) {
		return nil, fmt.Errorf("%s (%s) is %d bytes, want an untrimmed %d byte dump (trimmed ROMs move the file offsets)",
			filepath.Base(path), title, info.Size(), g.untrimmed)
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
		return nil, fmt.Errorf("sprite archive has %d members, want at least %d for a complete %s dump",
			archive.Count(), (MaxDex+1)*20, title)
	}
	return &ROM{inner: r, archive: archive, game: g, title: title}, nil
}

// Code reports the ROM game code.
func (r *ROM) Code() string { return r.inner.Code() }

// Path reports the source ROM path.
func (r *ROM) Path() string { return r.inner.Path() }

// Size reports the source ROM size in bytes.
func (r *ROM) Size() int { return r.inner.Size() }

// Name reports a human-readable identification.
func (r *ROM) Name() string { return r.title + " (" + r.inner.Code() + ")" }

// ImporterID reports the importer id a pack built from this dump records.
func (r *ROM) ImporterID() string { return r.game.importer }

// Family reports "bw" or "bw2", the pair this dump belongs to.
func (r *ROM) Family() string { return r.game.family }

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
