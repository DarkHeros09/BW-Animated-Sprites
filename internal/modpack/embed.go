package modpack

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The mod ships as source: the engine loads Lua, not bytecode.
//
//go:embed assets/mod
var embedded embed.FS

// modRoot is where the embedded mod lives inside this package.
const modRoot = "assets/mod"

// ModDirName is the directory name the mod must be installed under.
const ModDirName = "bwsprites"

// AssembleReport summarizes what Assemble wrote.
type AssembleReport struct {
	// ModFiles is the number of mod resources (manifest and Lua) written.
	ModFiles int
	// PackFiles is the number of sprite pack files copied.
	PackFiles int
	// Bytes is the total size written.
	Bytes int64
}

// ModFiles lists the embedded mod resources, relative to the mod directory.
func ModFiles() ([]string, error) {
	var files []string
	err := fs.WalkDir(embedded, modRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(filepath.FromSlash(modRoot), filepath.FromSlash(p))
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

// Assemble writes an installable mod into out: the embedded Lua and manifest,
// plus the sprite pack from packDir when it is set. out is the directory that
// belongs under <LOVE save dir>/<identity>/mods/, so its name should be
// ModDirName.
func Assemble(packDir, out string, force bool) (*AssembleReport, error) {
	if out == "" {
		return nil, fmt.Errorf("missing destination directory")
	}
	if packDir != "" {
		if err := checkPack(packDir); err != nil {
			return nil, err
		}
		if sameOrInside(out, packDir) || sameOrInside(packDir, out) {
			return nil, fmt.Errorf("refusing to build %s over or inside the pack it copies (%s); choose another -out", out, packDir)
		}
	}
	if err := prepare(out, force); err != nil {
		return nil, err
	}
	report := &AssembleReport{}
	if err := copyEmbedded(out, report, false); err != nil {
		return nil, err
	}
	if packDir != "" {
		if err := copyTree(packDir, out, report); err != nil {
			return nil, err
		}
	}
	return report, nil
}

// AssembleExternal writes the code-only build: the same Lua, a generated
// manifest that requires the launcher's imported gen5_bw pack instead of a local
// sprite tree, and no pack files. Use it for a Black/White dump extracted by the
// launcher's IMPORTERS screen; the ROM and its art never enter the mod.
func AssembleExternal(out string, force bool) (*AssembleReport, error) {
	if out == "" {
		return nil, fmt.Errorf("missing destination directory")
	}
	if err := prepare(out, force); err != nil {
		return nil, err
	}
	report := &AssembleReport{}
	if err := copyEmbedded(out, report, true); err != nil {
		return nil, err
	}
	data, err := externalManifest()
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(filepath.Join(out, "manifest.json"), data); err != nil {
		return nil, err
	}
	report.ModFiles++
	report.Bytes += int64(len(data))
	return report, nil
}

// externalDescription is the manifest text for the code-only build. It must not
// promise a bundled pack or a BW2 route: the launcher importer handles Black and
// White only.
const externalDescription = "Replaces the Gen 3 battle pictures and Pokedex entry with the animated Black/White battle sprites that the launcher's IMPORTERS screen extracted from your own unzipped 256 MiB Black or White dump. This build ships no game art and no sprite pack: the mod reads the imported gen5_bw battle_sprites pack, so import Black/White in IMPORTERS first and restart. Every view plays at the art's native size; a picture taller than the enemy slot is anchored at the scene top and runs behind the battle menu, wide fronts are kept inside the 240 px scene, and the player's back starts below the enemy healthbox. The Pokedex plays the idle animation at native size, centred in the card's picture area. Distinct female graphics are used front and back. The OPTION menu gains MY POKEMON and OPPONENT switches (BW or DEFAULT) and the choice is saved. Black 2/White 2 dumps are not supported by the launcher importer; use the BW2 build for those."

// externalManifest derives the code-only manifest from the embedded one, so
// identity, entry point, profile and permissions stay in one place.
func externalManifest() ([]byte, error) {
	raw, err := embedded.ReadFile(modRoot + "/manifest.json")
	if err != nil {
		return nil, err
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("embedded manifest: %w", err)
	}
	manifest["version"] = "1.14.0"
	manifest["description"] = externalDescription
	manifest["required_assets"] = []any{
		map[string]any{"importer": "gen5_bw", "pack": "battle_sprites", "version": ">=1.0.0"},
	}
	return json.MarshalIndent(manifest, "", "  ")
}

// checkPack fails early, with the command that builds a pack, when packDir is
// not a pack this mod can load.
func checkPack(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("sprite pack %s: %w; build one with 'bwsprites modpack -rom ROM.nds -out %s'", dir, err, dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("sprite pack %s is not a directory", dir)
	}
	index := filepath.Join(dir, IndexFile)
	info, err = os.Stat(index)
	if err != nil {
		return fmt.Errorf("sprite pack %s has no %s; build it with 'bwsprites modpack -rom ROM.nds -out %s'",
			dir, IndexFile, dir)
	}
	if info.Size() == 0 {
		return fmt.Errorf("sprite pack %s: %s is empty; rebuild the pack", dir, IndexFile)
	}
	return nil
}

// sameOrInside reports whether a and b are the same directory or one holds the
// other, which would make the copy recurse into itself.
func sameOrInside(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// copyEmbedded writes the mod resources into out. skipManifest lets
// AssembleExternal replace the embedded manifest with its generated one.
func copyEmbedded(out string, report *AssembleReport, skipManifest bool) error {
	return fs.WalkDir(embedded, modRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if skipManifest && d.Name() == "manifest.json" {
			return nil
		}
		data, err := embedded.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.FromSlash(modRoot), filepath.FromSlash(p))
		if err != nil {
			return err
		}
		target := filepath.Join(out, rel)
		if err := writeFileAtomic(target, data); err != nil {
			return err
		}
		report.ModFiles++
		report.Bytes += int64(len(data))
		return nil
	})
}

// copyTree copies every regular file under src into dst, keeping the layout.
func copyTree(src, dst string, report *AssembleReport) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("sprite pack file %s is not a regular file", p)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if err := writeFileAtomic(filepath.Join(dst, rel), data); err != nil {
			return err
		}
		report.PackFiles++
		report.Bytes += int64(len(data))
		return nil
	})
}
