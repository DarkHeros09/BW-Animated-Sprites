// Package modpack builds the LÖVE engine mod: one cell of animated Black/White
// art per species and view at the art's own native size (nothing is resampled,
// so a cell can be larger than the battle slot), plus the Lua index the mod
// reads at runtime.
package modpack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"bwsprites/internal/gen5"
)

// Format is the mod pack format revision. Format 2 uses native square cells.
const Format = 2

// TickRate is the animation clock: one tick is one sixtieth of a second, the
// rate the ROM's frame durations are authored at.
const TickRate = 60

// IndexFile is the entry point the mod loads.
const IndexFile = "index.lua"

// Source identifies the dump a pack was built from.
type Source struct {
	Path string
	Code string
	Name string
	// Importer is the id of the importer that read the dump: gen5_bw for
	// Black/White, gen5_bw2 for Black 2/White 2.
	Importer string
	// Digest is the SHA-256 of the dump, so two builds can be compared
	// without shipping the ROM.
	Digest string
}

// View is one side's animation, as the mod needs it at runtime. Both palettes
// share the timing and geometry; they only differ in the sheet drawn.
type View struct {
	// Frames is the frame count.
	Frames int
	// Size is the square cell side the frames were fitted into.
	Size int
	// Columns is the number of cells per sheet row, chosen per view so the
	// sheet stays within a 4096 px texture.
	Columns int
	// InkH is the scaled union ink height, used to keep a tall enemy sprite
	// on screen once the engine's offset is corrected.
	InkH int
	// InkW is the scaled union ink width, kept for reports only.
	InkW int
	// DexX/DexY/DexW/DexH is the box enclosing every frame's painted ink in cell
	// coordinates. The Pokedex crops that fixed window from the current frame at
	// 1:1, so the animation keeps its own pixels and position and no pose swings
	// out of the window; the mod centres the picture in the card and keeps it on
	// screen rather than reducing it.
	DexX, DexY int
	DexW, DexH int
	// Durations is the tick count of each frame.
	Durations []int
	// LoopStart is the frame the animation jumps back to after its last
	// frame; 0 plays the whole strip in a loop.
	LoopStart int
	// Normal, Shiny are the sheet paths relative to the pack root. Shiny is
	// empty when only normal sheets were built.
	Normal, Shiny string
}

// Entry is one species' animation, with either view optional. The Female views
// exist only when the dump has distinct female graphics for that side; the mod
// falls back to the male view otherwise.
type Entry struct {
	Dex         int
	Front       *View
	Back        *View
	FrontFemale *View
	BackFemale  *View
}

// Report summarizes a build.
type Report struct {
	// Entries is the number of species with at least one view.
	Entries int
	// Sheets is the number of strips written.
	Sheets int
	// Bytes is the total size of the files written.
	Bytes int64
	// Capped counts views whose idle cycle was longer than gen5.MaxTicks
	// and therefore exported its first MaxTicks only.
	Capped int
}

// Options limits what a build renders; the zero value builds every species,
// both sides and both palettes.
type Options struct {
	// Dex limits the species built; empty means every species.
	Dex []int
	// Sides limits the views built; empty means front and back.
	Sides []string
	// Palettes limits the palettes built; empty means normal and shiny.
	Palettes []string
}

func (o Options) dexes() []int {
	if len(o.Dex) == 0 {
		out := make([]int, gen5.MaxDex)
		for i := range out {
			out[i] = i + 1
		}
		return out
	}
	return o.Dex
}

func (o Options) sides() []string {
	if len(o.Sides) == 0 {
		return []string{"front", "back"}
	}
	return o.Sides
}

func (o Options) palettes() []string {
	if len(o.Palettes) == 0 {
		return []string{"normal", "shiny"}
	}
	return o.Palettes
}

func (o Options) hasPalette(name string) bool {
	for _, p := range o.palettes() {
		if p == name {
			return true
		}
	}
	return false
}

// Build renders a pack into out. out must be empty or absent; the caller
// decides whether to clear an old pack. Every file is written through a
// temporary name and renamed into place, and the index is written last, so an
// interrupted build leaves no pack the mod would load.
func Build(rom *gen5.ROM, out string, opts Options, force bool) (*Report, error) {
	for _, side := range opts.sides() {
		if side != "front" && side != "back" {
			return nil, fmt.Errorf("invalid side %q, want front or back", side)
		}
	}
	for _, palette := range opts.palettes() {
		if palette != "normal" && palette != "shiny" {
			return nil, fmt.Errorf("invalid palette %q, want normal or shiny", palette)
		}
	}
	if err := prepare(out, force); err != nil {
		return nil, err
	}
	digest, err := hashFile(rom.Path())
	if err != nil {
		return nil, err
	}

	report := &Report{}
	entries := make(map[int]*Entry)
	for _, dex := range opts.dexes() {
		entry := &Entry{Dex: dex}
		for _, side := range opts.sides() {
			view, err := buildView(rom, out, dex, side, false, opts, report)
			if err != nil {
				return nil, err
			}
			if side == "front" {
				entry.Front = view
			} else {
				entry.Back = view
			}
			hasFemale, err := rom.HasFemale(dex, side)
			if err != nil {
				return nil, fmt.Errorf("dex %d %s female check: %w", dex, side, err)
			}
			if !hasFemale {
				continue
			}
			female, err := buildView(rom, out, dex, side, true, opts, report)
			if err != nil {
				return nil, err
			}
			if side == "front" {
				entry.FrontFemale = female
			} else {
				entry.BackFemale = female
			}
		}
		entries[dex] = entry
		report.Entries++
	}

	index := &indexFile{
		Source: Source{
			Path:     rom.Path(),
			Code:     rom.Code(),
			Name:     rom.Name(),
			Importer: rom.ImporterID(),
			Digest:   digest,
		},
		Entries: entries,
	}
	if err := writeFileAtomic(filepath.Join(out, IndexFile), []byte(index.lua())); err != nil {
		return nil, err
	}
	return report, nil
}

// buildView renders every palette of one side and returns the shared timing.
// A female view reads the distinct female graphic; callers check HasFemale
// first, so Read never silently falls back to the male art here.
func buildView(rom *gen5.ROM, out string, dex int, side string, female bool, opts Options, report *Report) (*View, error) {
	sprite, err := rom.Read(dex, side, female)
	if err != nil {
		return nil, fmt.Errorf("dex %d %s: %w", dex, side, err)
	}
	if female && sprite.GenderFallback {
		return nil, fmt.Errorf("dex %d %s: the dump has no female graphic", dex, side)
	}
	var view *View
	for _, palette := range opts.palettes() {
		rendered, err := render(sprite, palette)
		if err != nil {
			return nil, fmt.Errorf("dex %d %s %s: %w", dex, side, palette, err)
		}
		parts := []string{side}
		if female {
			parts = append(parts, "female")
		}
		parts = append(parts, strconv.Itoa(dex), palette+".png")
		rel := filepath.ToSlash(filepath.Join(parts...))
		size, err := writePNG(filepath.Join(out, filepath.FromSlash(rel)), rendered.sheet)
		if err != nil {
			return nil, err
		}
		report.Sheets++
		report.Bytes += size
		if view == nil {
			view = rendered.view
			if rendered.capped {
				report.Capped++
			}
		} else if view.Frames != rendered.view.Frames || !sameDurations(view.Durations, rendered.view.Durations) {
			return nil, fmt.Errorf("dex %d %s: %s and %s disagree on the animation (%d vs %d frames)",
				dex, side, opts.palettes()[0], palette, view.Frames, rendered.view.Frames)
		}
		if palette == "normal" {
			view.Normal = rel
		} else {
			view.Shiny = rel
		}
	}
	return view, nil
}

func sameDurations(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// prepare makes out, refusing to fill a directory that already holds files
// unless replacing it is explicitly allowed.
func prepare(out string, force bool) error {
	info, err := os.Stat(out)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(out, 0o755)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is a file, want a directory", out)
	}
	names, err := os.ReadDir(out)
	if err != nil {
		return err
	}
	if len(names) > 0 {
		if !force {
			return fmt.Errorf("%s is not empty; remove it first or pass -force", out)
		}
		if err := os.RemoveAll(out); err != nil {
			return err
		}
		return os.MkdirAll(out, 0o755)
	}
	return nil
}

// hashFile returns "sha256:<hex>" for path.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
