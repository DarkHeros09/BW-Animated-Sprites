// Command bwsprites exports Black/White animated sprites from a user-supplied
// ROM into a pack, or lists what it found.
//
// The ROM is only ever read. Nothing Nintendo-made is bundled with this tool.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"bwsprites/internal/demo"
	"bwsprites/internal/export"
	"bwsprites/internal/gen3"
	"bwsprites/internal/gen5"
	"bwsprites/internal/modpack"
	"bwsprites/internal/pack"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bwsprites:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		usage()
		return errors.New("missing subcommand")
	}
	switch os.Args[1] {
	case "list":
		return list(os.Args[2:])
	case "export":
		return exportCommand(os.Args[2:])
	case "verify":
		return verify(os.Args[2:])
	case "coords":
		return coordsCommand(os.Args[2:])
	case "demo":
		return demoCommand(os.Args[2:])
	case "modpack":
		return modpackCommand(os.Args[2:])
	case "mod":
		return modCommand(os.Args[2:])
	case "install":
		return installCommand(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return nil
	}
	usage()
	return fmt.Errorf("unknown subcommand %q", os.Args[1])
}

func usage() {
	fmt.Fprint(os.Stderr, `bwsprites exports animated Pokemon Black/White sprites from a ROM dump.

  bwsprites list   -rom ROM.nds [-dex N] [-side front|back]
  bwsprites export -rom ROM.nds -out DIR [-dex LIST] [-side both] [-palette both]
                   [-frame 0] [-ground -1] [-coords FILE] [-force]
  bwsprites coords -rom ROM.gba -game GAME -out FILE [table flags]
  bwsprites modpack -rom ROM.nds -out DIR [-dex LIST] [-side both] [-palette both] [-force]
  bwsprites mod    -pack DIR -out DIR [-force]
  bwsprites install -pack DIR [-identity NAME] [-mods DIR] [-force]
  bwsprites verify -pack DIR
  bwsprites demo   -out DIR [-force]

The ROM must be an untrimmed 256 MiB Black or White (IRB*/IRA*) or 512 MiB
Black 2 or White 2 (IRE*/IRD*) dump.
A range like 1-20 or a list 1,4,25 works; "both" expands to every value.

-frame 0 fits each sprite's cell to its own content, never below the facets
64. Pass 64 to stay inside a host's fixed 64x64 cell: a taller sprite is then
pinned to the top of the cell and recorded with grounded=false rather than
cropped. -ground is the cell row sprites stand on; -1 is the bottom row.

coords reads a Generation 3 game's per-species picture table out of your own
dump and writes it as JSON. -coords then lands each sprite on the row that game
uses instead of one row for the whole pack. Those tables are indexed by the
game's internal species slot, not national dex.

modpack builds the sprite data the gen1recomp mod reads: one native-size cell
per frame (nothing is resampled), ink centred with its lowest row on the ground
row, both sides and both palettes. mod wraps that data and the mod's
own Lua into a folder you can drop into <LOVE save dir>/<identity>/mods/, and
install does that drop for you. The mod itself replaces the battle pictures of
FireRed, LeafGreen and Emerald; it is not an in-game ROM patch.

demo writes a procedurally drawn pack, so the pipeline and the LOVE previewer
run without a ROM.
`)
}

func list(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	romPath := fs.String("rom", "", "path to the ROM dump")
	dex := fs.Int("dex", 0, "national dex to describe, 1..649")
	side := fs.String("side", "front", "front or back")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rom, err := openROM(*romPath)
	if err != nil {
		return err
	}
	if *dex <= 0 {
		return describeROM(rom)
	}
	has, err := rom.HasFemale(*dex, *side)
	if err != nil {
		return err
	}
	s, err := rom.Read(*dex, *side, false)
	if err != nil {
		return err
	}
	female := "no"
	if has {
		female = "yes"
	}
	fmt.Printf("dex %d %s: female variant %s, %d cells, %d part animations, map %d\n",
		*dex, *side, female, len(s.Cells.Cells), len(s.NMAR.Animations), s.MapIndex)
	return nil
}

func describeROM(rom *gen5.ROM) error {
	fmt.Printf("%s  code %s\n", rom.Name(), rom.Code())
	n := 0
	for dex := 1; dex <= gen5.MaxDex; dex++ {
		has, err := rom.HasFemale(dex, "front")
		if err != nil {
			return err
		}
		if has {
			n++
		}
	}
	fmt.Printf("%d of %d species have a female front sprite\n", n, gen5.MaxDex)
	return nil
}

func exportCommand(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	romPath := fs.String("rom", "", "path to the ROM dump")
	out := fs.String("out", "", "pack directory to write")
	dexSpec := fs.String("dex", "1-649", "national dex list or range")
	side := fs.String("side", "both", "front, back or both")
	palettes := fs.String("palette", "both", "normal, shiny or both")
	frame := fs.Int("frame", 0, "square frame edge in pixels; 0 fits each sprite")
	ground := fs.Int("ground", -1, "frame row the sprites stand on, 0-based; default is the bottom row")
	coordsPath := fs.String("coords", "", "Gen 3 coords JSON to take each species' ground offset from")
	female := fs.Bool("female", true, "also export female variants where the species has one")
	force := fs.Bool("force", false, "overwrite an existing pack directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("missing -out")
	}
	rom, err := openROM(*romPath)
	if err != nil {
		return err
	}
	list, err := parseDex(*dexSpec)
	if err != nil {
		return err
	}
	sides, err := parseChoice(*side, "side", "front", "back")
	if err != nil {
		return err
	}
	pals, err := parseChoice(*palettes, "palette", "normal", "shiny")
	if err != nil {
		return err
	}
	if err := prepareOut(*out, *force); err != nil {
		return err
	}

	var coords *gen3.File
	if *coordsPath != "" {
		if coords, err = gen3.Load(*coordsPath); err != nil {
			return fmt.Errorf("read coords: %w", err)
		}
	}

	digest, err := pack.Digest(rom.Path())
	if err != nil {
		return fmt.Errorf("hash ROM: %w", err)
	}
	manifest := &pack.Manifest{
		Format:    pack.Format,
		Importer:  gen5.ImporterID,
		TickRate:  pack.TickRate,
		FrameSize: *frame,
		Source:    pack.Source{Path: filepath.Base(rom.Path()), Size: int64(rom.Size()), GameCode: rom.Code(), Digest: digest},
	}

	total, written := 0, 0
	for _, dex := range list {
		for _, s := range sides {
			req := export.Request{
				Dex: dex, Side: s, Palette: "normal", FrameSize: *frame,
				Ground: *ground, GroundFromBottom: -1,
			}
			if coords != nil {
				c, ok := coords.For(s, dex)
				if !ok {
					return fmt.Errorf("dex %d %s: %s has no %s record for slot %d; pass -ground to use one row instead",
						dex, s, filepath.Base(*coordsPath), s, dex)
				}
				req.GroundFromBottom = c.GroundOffset
				req.GroundFrom = fmt.Sprintf("%s %s[%d]", filepath.Base(*coordsPath), s, dex)
			}
			hasFemale, err := rom.HasFemale(dex, s)
			if err != nil {
				return describeFailure(dex, s, err)
			}
			if hasFemale && *female {
				req.Female = true
			}
			for _, pal := range pals {
				req.Palette = pal
				total++
				entry, err := export.One(rom, req)
				if err != nil {
					return describeFailure(dex, s, err)
				}
				if err := pack.WriteEntry(*out, entry.Entry, entry.Frames); err != nil {
					return fmt.Errorf("write dex %d %s: %w", dex, s, err)
				}
				manifest.Entries = append(manifest.Entries, entry.Entry)
				written++
			}
		}
	}
	if err := pack.WriteManifest(*out, manifest); err != nil {
		return err
	}
	fmt.Printf("%d entries written to %s\n", written, *out)
	return nil
}

func describeFailure(dex int, side string, err error) error {
	return fmt.Errorf("dex %d %s: %w", dex, side, err)
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	dir := fs.String("pack", "", "pack directory to check")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return errors.New("missing -pack")
	}
	if err := pack.Verify(*dir); err != nil {
		return err
	}
	manifest, err := pack.ReadManifest(*dir)
	if err != nil {
		return err
	}
	fmt.Printf("%d entries in %s verified\n", len(manifest.Entries), *dir)
	return nil
}

func demoCommand(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	out := fs.String("out", "", "pack directory to write")
	force := fs.Bool("force", false, "overwrite an existing pack directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("missing -out")
	}
	if err := prepareOut(*out, *force); err != nil {
		return err
	}
	manifest, err := demo.Write(*out)
	if err != nil {
		return err
	}
	fmt.Printf("%d synthetic entries written to %s\n", len(manifest.Entries), *out)
	return nil
}

func openROM(path string) (*gen5.ROM, error) {
	if path == "" {
		return nil, errors.New("missing -rom")
	}
	return gen5.Identify(path)
}

// parseDex expands "1-20,25,30" into the dex numbers to export.
func parseDex(spec string) ([]int, error) {
	var out []int
	seen := map[int]bool{}
	add := func(n int) error {
		if n < 1 || n > gen5.MaxDex {
			return fmt.Errorf("dex %d outside 1..%d", n, gen5.MaxDex)
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
		return nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		low, err := strconv.Atoi(strings.TrimSpace(lo))
		if err != nil {
			return nil, fmt.Errorf("invalid dex %q", part)
		}
		if !isRange {
			if err := add(low); err != nil {
				return nil, err
			}
			continue
		}
		high, err := strconv.Atoi(strings.TrimSpace(hi))
		if err != nil {
			return nil, fmt.Errorf("invalid dex range %q", part)
		}
		if high < low {
			low, high = high, low
		}
		for n := low; n <= high; n++ {
			if err := add(n); err != nil {
				return nil, err
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("dex list %q is empty", spec)
	}
	return out, nil
}

// parseChoice expands "both" into all values of one flag.
func parseChoice(value, name string, all ...string) ([]string, error) {
	if value == "both" {
		return all, nil
	}
	for _, v := range all {
		if value == v {
			return []string{value}, nil
		}
	}
	return nil, fmt.Errorf("invalid -%s %q, want %s or both", name, value, strings.Join(all, ", "))
}

// prepareOut guarantees the pack directory is empty or safe to fill.
func prepareOut(dir string, force bool) error {
	info, err := os.Stat(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return os.MkdirAll(dir, 0o755)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		if !force {
			return fmt.Errorf("%s is not empty, pass -force to overwrite", dir)
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		return os.MkdirAll(dir, 0o755)
	}
	return nil
}

// gen3Tables are the per-species picture table addresses of the Generation 3
// games, in file offsets. Emerald's two addresses come from the pret symbol
// table gen1recomp bundles (src/import/gba/syms/emerald.lua: 'gMonFrontPicCoords
// 300d38 6e0' and 'gMonBackPicCoords 3021d8 6e0'), 0x6e0 bytes over 440 species
// giving a 4-byte stride. No FireRed/LeafGreen symbol table is bundled in
// anything inspected here - gen1recomp bakes that game's table into generated
// data - so those two games need the addresses supplied from the user's own
// pret symbols.
type gen3Tables struct {
	front, back, count, stride int
	headerCode                 string
}

var gen3Games = map[string]gen3Tables{
	"emerald":   {front: 0x300d38, back: 0x3021d8, count: 440, stride: 4, headerCode: "BPEE"},
	"firered":   {headerCode: "BPRE"},
	"leafgreen": {headerCode: "BPGE"},
}

func coordsCommand(args []string) error {
	fs := flag.NewFlagSet("coords", flag.ExitOnError)
	romPath := fs.String("rom", "", "path to a Generation 3 ROM dump")
	game := fs.String("game", "", "emerald, firered or leafgreen")
	out := fs.String("out", "", "coords JSON to write")
	frontBase := fs.Int("front-base", 0, "file offset of gMonFrontPicCoords; 0 uses the game default")
	backBase := fs.Int("back-base", 0, "file offset of gMonBackPicCoords; 0 uses the game default")
	count := fs.Int("count", 0, "species records per table; 0 uses the game default")
	stride := fs.Int("stride", 0, "bytes per species record; 0 uses the game default")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *romPath == "" {
		return errors.New("missing -rom")
	}
	if *out == "" {
		return errors.New("missing -out")
	}
	name := strings.ToLower(*game)
	tables, ok := gen3Games[name]
	if !ok {
		return fmt.Errorf("missing or unknown -game %q, want emerald, firered or leafgreen", *game)
	}
	if *frontBase == 0 {
		*frontBase = tables.front
	}
	if *backBase == 0 {
		*backBase = tables.back
	}
	if *count == 0 {
		*count = tables.count
	}
	if *stride == 0 {
		*stride = tables.stride
	}
	if *frontBase == 0 || *backBase == 0 || *count == 0 || *stride == 0 {
		return fmt.Errorf("%s: no bundled table addresses (gen1recomp ships no %s symbol table), "+
			"so pass -front-base, -back-base, -count and -stride from your own pret symbols, "+
			"or export with a single -ground row", name, name)
	}

	data, err := os.ReadFile(*romPath)
	if err != nil {
		return err
	}
	if len(data) < 0xC0 {
		return fmt.Errorf("%s is %d bytes, too small to be a Generation 3 ROM", *romPath, len(data))
	}
	if code := string(data[0xAC:0xB0]); code != tables.headerCode {
		return fmt.Errorf("%s has header code %q, want %q for %s: pass the ROM of the game the table addresses belong to",
			filepath.Base(*romPath), code, tables.headerCode, name)
	}
	front, err := gen3.Parse(data, fileOffset(*frontBase), *count, *stride)
	if err != nil {
		return fmt.Errorf("%s front table: %w", name, err)
	}
	back, err := gen3.Parse(data, fileOffset(*backBase), *count, *stride)
	if err != nil {
		return fmt.Errorf("%s back table: %w", name, err)
	}
	file := &gen3.File{
		Game:   name,
		Source: filepath.Base(*romPath),
		Stride: *stride,
		Front:  front,
		Back:   back,
	}
	if err := file.Save(*out); err != nil {
		return err
	}
	fmt.Printf("%s: %d of %d front and %d of %d back slots have a picture, written to %s\n",
		name, countPictures(front), *count, countPictures(back), *count, *out)
	fmt.Println("slots are the game's internal species ids, not national dex numbers")
	return nil
}

// fileOffset accepts a pret symbol address (0x08xxxxxx) as well as a raw file
// offset, since that is what a symbol file prints.
func fileOffset(base int) int {
	if base >= 0x08000000 && base < 0x0A000000 {
		return base - 0x08000000
	}
	return base
}

func countPictures(table []gen3.Coords) int {
	n := 0
	for _, c := range table {
		if !c.Empty {
			n++
		}
	}
	return n
}

func modpackCommand(args []string) error {
	fs := flag.NewFlagSet("modpack", flag.ExitOnError)
	romPath := fs.String("rom", "", "path to the ROM dump")
	out := fs.String("out", "", "mod pack directory to write")
	dexSpec := fs.String("dex", "1-649", "national dex list or range")
	side := fs.String("side", "both", "front, back or both")
	palettes := fs.String("palette", "both", "normal, shiny or both")
	force := fs.Bool("force", false, "overwrite an existing pack directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("missing -out")
	}
	rom, err := openROM(*romPath)
	if err != nil {
		return err
	}
	list, err := parseDex(*dexSpec)
	if err != nil {
		return err
	}
	sides, err := parseChoice(*side, "side", "front", "back")
	if err != nil {
		return err
	}
	pals, err := parseChoice(*palettes, "palette", "normal", "shiny")
	if err != nil {
		return err
	}
	report, err := modpack.Build(rom, *out, modpack.Options{Dex: list, Sides: sides, Palettes: pals}, *force)
	if err != nil {
		return err
	}
	fmt.Printf("%d species, %d strips (%s, every view at native size) written to %s\n",
		report.Entries, report.Sheets, humanBytes(report.Bytes), *out)
	if report.Capped > 0 {
		fmt.Printf("%d views exceed the %d tick bound and export the start of their cycle\n",
			report.Capped, gen5.MaxTicks)
	}
	fmt.Printf("next: bwsprites install -pack %s\n", *out)
	return nil
}

func modCommand(args []string) error {
	fs := flag.NewFlagSet("mod", flag.ExitOnError)
	packDir := fs.String("pack", "", "mod pack directory to wrap")
	external := fs.Bool("external", false, "ship code only and read the launcher's imported Black/White pack")
	out := fs.String("out", "", "directory to write the installable mod into")
	zipPath := fs.String("zip", "", "also write an installable .zip of the mod to this file")
	force := fs.Bool("force", false, "replace a non-empty destination directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("missing -out")
	}
	if *external && *packDir != "" {
		return errors.New("-external reads the launcher's imported pack and takes no -pack")
	}
	if !*external && *packDir == "" {
		return errors.New("missing -pack; pass -pack DIR for an embedded pack or -external for the launcher's imported Black/White pack")
	}
	var report *modpack.AssembleReport
	var err error
	if *external {
		report, err = modpack.AssembleExternal(*out, *force)
	} else {
		report, err = modpack.Assemble(*packDir, *out, *force)
	}
	if err != nil {
		return err
	}
	if *external {
		fmt.Printf("%d mod files (%s, code only; reads the launcher's imported Black/White pack) written to %s\n",
			report.ModFiles, humanBytes(report.Bytes), *out)
	} else {
		fmt.Printf("%d mod files and %d pack files (%s) written to %s\n",
			report.ModFiles, report.PackFiles, humanBytes(report.Bytes), *out)
	}
	if *zipPath != "" {
		files, bytes, err := modpack.Zip(*out, *zipPath)
		if err != nil {
			return err
		}
		fmt.Printf("%d files (%s) zipped to %s; install it with the launcher's \"Import mod .zip\"\n",
			files, humanBytes(bytes), *zipPath)
		return nil
	}
	if *external {
		fmt.Printf("next: import your own Black/White dump in the launcher's IMPORTERS screen, then copy this folder to <LOVE save dir>/<identity>/mods/%s\n",
			modpack.ModDirName)
		return nil
	}
	fmt.Printf("next: copy it to <LOVE save dir>/<identity>/mods/%s, or run bwsprites install -pack %s\n",
		modpack.ModDirName, *packDir)
	return nil
}

func installCommand(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	packDir := fs.String("pack", "", "mod pack directory to install")
	external := fs.Bool("external", false, "install code only; read the launcher's imported Black/White pack")
	identity := fs.String("identity", "pokemon-love2d", "LOVE save identity the game runs under")
	modsDir := fs.String("mods", "", "mods directory to install into; overrides -identity")
	force := fs.Bool("force", false, "replace an existing bwsprites installation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *external && *packDir != "" {
		return errors.New("-external reads the launcher's imported pack and takes no -pack")
	}
	if !*external && *packDir == "" {
		return errors.New("missing -pack; pass -pack DIR for an embedded pack or -external for the launcher's imported Black/White pack")
	}
	mods, err := loveModsDir(*identity, *modsDir)
	if err != nil {
		return err
	}
	out := filepath.Join(mods, modpack.ModDirName)
	var report *modpack.AssembleReport
	if *external {
		report, err = modpack.AssembleExternal(out, *force)
	} else {
		report, err = modpack.Assemble(*packDir, out, *force)
	}
	if err != nil {
		return err
	}
	if *external {
		fmt.Printf("%d mod files (%s, code only) installed to %s\n",
			report.ModFiles, humanBytes(report.Bytes), out)
		fmt.Println("import your own Black/White dump in the launcher's IMPORTERS screen, then start the game")
	} else {
		fmt.Printf("%d mod files and %d pack files (%s) installed to %s\n",
			report.ModFiles, report.PackFiles, humanBytes(report.Bytes), out)
		fmt.Println("start the game; the mod menu lists it as \"BW Animated Sprites\"")
	}
	return nil
}

// loveModsDir resolves where the engine keeps mods for one save identity.
func loveModsDir(identity, modsDir string) (string, error) {
	if modsDir != "" {
		return modsDir, nil
	}
	if identity == "" {
		identity = "pokemon-love2d"
	}
	root, err := loveSaveRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, identity, "mods"), nil
}

// loveSaveRoot returns the directory LOVE.filesystem calls the save root: the
// "identity" directories live directly under it.
func loveSaveRoot() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if dir := os.Getenv("APPDATA"); dir != "" {
			return filepath.Join(dir, "LOVE"), nil
		}
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "LOVE"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "LOVE"), nil
	default:
		if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
			return filepath.Join(dir, "love"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", "love"), nil
	}
}

// humanBytes formats a byte count for the one-line summaries.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
