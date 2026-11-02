# bwsprites

An animated-sprite pipeline for Black/White sprites, aimed at Generation 3 hosts
(FireRed/LeafGreen, Emerald), with a LÖVE previewer that draws each animation
already measured the way [facets](https://github.com/ElvieBlooms/facets) measures
sprites.

It has three parts and two shared formats:

- `cmd/bwsprites` (Go) — reads the sprite archive out of a local Black/White or
  Black 2/White 2 dump,
  plays back each idle animation, measures every frame with the facets convention,
  and writes a **pack** directory (a PNG strip plus a JSON sidecar per sprite).
- `love/` (Lua, LÖVE 11) — loads a pack, plays the animations, and draws the
  measurement it was given: cell, union box, ground row, anchor, and an optional
  64×64 host-cell overlay.
- `internal/modpack` + `install` — builds the same animations as an installable
  **gen1recomp mod** that swaps FireRed/LeafGreen and Emerald battle pictures for
  the Black/White sprites. The mod's data layout and its exact engine seams are in
  [docs/MOD.md](docs/MOD.md).

The previewer's pack format is [docs/FORMAT.md](docs/FORMAT.md); the mod pack
keeps the same ground rule and centres every frame in a cell the art's own size,
so nothing is resampled on either side: every front and back keeps the pixels
Black/White or Black 2/White 2 authored, and a cell can be larger than the
battle's 64 px slot. A female battler gets the distinct female graphic, front
and back, when the dump has one. The Pokedex plays the animation in the card's
picture area at the art's native size, centred there and kept wholly on screen,
so no pose is ever cut off; a window larger than the area overhangs the card
instead of being reduced. The measurement
rules, and the places the
pipeline differs from a single static picture, are in
[docs/MEASUREMENTS.md](docs/MEASUREMENTS.md). Provenance and licences are in
[docs/PROVENANCE.md](docs/PROVENANCE.md).

**This is not a ROM patch.** The mod only works inside the LÖVE-based gen1recomp
engine; nothing writes to a .gba and an unmodified emulator or cartridge still
shows the original art. See [Limitations](#limitations).

## Prerequisites

- Go 1.21 or newer.
- LÖVE 11.x (`lovec`/`love` on `PATH` for the Go end-to-end test).
- Your own, legally obtained ROMs. Nothing in this repository needs them to build
  or to run its tests; only the import and coords commands do.

No Python, no third-party Go modules, no external Lua libraries.

## Quick start, with no ROM

```sh
go run ./cmd/bwsprites demo -out demo/pack -force
love demo/pack
```

The demo pack contains three procedurally drawn sprites, labelled as synthetic in
`manifest.json` (`"importer": "synthetic"`): one that squashes with its feet on the
ground, one whose content box moves per frame, and one 88 px wide sprite that does
not fit a 64 px cell.

The LÖVE app also checks itself without a window:

```sh
lovec love demo/pack --selftest   # prints PASS/FAIL lines, exit code = failures
```

## Previewer controls

| key | action |
| --- | --- |
| `up`/`down` | previous/next entry |
| `space` | play/pause |
| `left`/`right` | step one frame |
| `r` | restart the animation |
| `d` | toggle the debug view (8 px grid, union box, ground row, anchor) |
| `h` | toggle the 64×64 host-cell overlay, including the scale warning |
| `=`/`-` | zoom in/out (`0` resets to fit) |
| `esc` | quit |

```sh
love demo/pack                       # open a pack
lovec love demo/pack --shot out.png --entry 2 --frame 3 --no-debug
```

`--shot` writes one frame to a PNG and exits, which is handy for a screenshot
regression without a display. `--host` starts with the host-cell overlay on.

## Importing a Black/White dump

```sh
go run ./cmd/bwsprites list -rom "Pokemon - Black Version (USA, Europe) (NDSi Enhanced).nds" -dex 1-9
go run ./cmd/bwsprites export -rom "Pokemon - Black Version (USA, Europe) (NDSi Enhanced).nds" \
    -out build/pack -dex 1,3,6 -side both -palette both -force
go run ./cmd/bwsprites verify -pack build/pack
love build/pack
```

- Supported dumps: unsupported game codes and trimmed dumps are rejected by name.
  Black/White (`IRA*`/`IRB*`, an untrimmed 256 MiB dump) and Black 2/White 2
  (`IRD*`/`IRE*`, 512 MiB) are read through one decoder: each pair keeps the same
  sprite archive path, the same twenty members per species and the same resource
  order, with Black 2/White 2's extra form graphics after national dex 649.
  Every member is still validated as it is read, so a dump that differs fails
  with the member at fault instead of producing wrong art.
- The dump is opened read-only. The tests assert the file's size and mtime are
  unchanged after an import.
- `-frame 0` (default) sizes each cell to its own content, never below the facets
  64. `-frame 64` keeps every sprite inside a host's fixed cell; a taller sprite is
  pinned to the top and recorded as `grounded: false` instead of being cropped.
- `-ground N` puts every sprite on one 0-based cell row; `-ground -1` (default) uses
  the bottom row.

## Installing the mod

There are two builds. The **embedded** one ships the extracted art inside the
mod and needs no launcher import; the **code-only** one ships four Lua/JSON files
and reads the launcher's own Black/White import.

Embedded (Black/White or Black 2/White 2):

```sh
go run ./cmd/bwsprites modpack -rom "BLACK.nds" -out build/modpack
go run ./cmd/bwsprites install -pack build/modpack -identity pokemon-love2d
```

The same commands read a Black 2/White 2 dump (`IRD*`/`IRE*`, an untrimmed
512 MiB image); the pack then records `importer = "gen5_bw2"` and the mod reads
it unchanged. One mod is installed at a time, so a Black 2 build replaces a
Black build's art in the same folder.

Code-only (Black/White only; the launcher's importer rejects Black 2/White 2):

```sh
# Once in the launcher: IMPORTERS -> Pokemon Black / White, pick your own
# untrimmed 256 MiB dump. Then:
go run ./cmd/bwsprites install -external -identity pokemon-love2d
```

The code-only build ships no game art. It reads
`asset_packs/gen5_bw/battle_sprites` from the save identity through
`mod.packs`; if the import is missing it refuses to load and says so.

`-identity` is the LÖVE save folder (`POKEPORT_IDENTITY`, default
`pokemon-love2d`); `-mods DIR` overrides the whole path and `-force` replaces an
existing install. Then enable **BW Animated Sprites** in the game's mod menu.

To hand the mod to someone (or to install it from the launcher's **Import mod
.zip**), build the installable archive instead:

```sh
go run ./cmd/bwsprites mod -pack build/modpack -out build/bwsprites -zip dist/bwsprites.zip
# Code-only instead (four files, no PNGs):
go run ./cmd/bwsprites mod -external -out build/bwsprites-bw -zip dist/bwsprites-bw.zip
```

The archive holds exactly one top-level folder, `bwsprites/`, with
`manifest.json` at its root; that is the only shape the engine's installer
accepts. Extract it into `<LOVE save dir>/<identity>/mods/` if you prefer a
folder install. The mod's Lua is embedded in the Go binary, so an install is one
self-contained folder with no dependency on this checkout.

Inside the game, the OPTION menu gains a **BW SPRITES** entry with two rows:
**MY POKEMON** (your back sprites) and **OPPONENT** (the foe's front sprites),
each switchable between `BW` and `DEFAULT`. Both default to `BW`; setting one
side to `DEFAULT` serves the cartridge picture for that side only and restores
its original y padding, live, without a reload. The choice is saved with the
other game options. Verified on FireRed and Emerald.

What the mod swaps, how its cells are fitted, and exactly what it does not cover:
[docs/MOD.md](docs/MOD.md).

## Per-species ground rows from a Gen 3 ROM

A single ground row is fine for a preview, wrong for a mod: FireRed/LeafGreen and
Emerald stand each species on its own row. Those rows live in the target game's
picture-coordinate table, so read them from your own ROM:

```sh
# Emerald: addresses come from the pret symbol file that shipped with the ROM build.
go run ./cmd/bwsprites coords -rom Pokemon_Emerald.gba -game emerald -out build/emerald-coords.json

# FireRed/LeafGreen: this project ships no symbol table for them, so pass your own
# pret addresses (0x08xxxxxx or a raw file offset) plus count and stride.
go run ./cmd/bwsprites coords -rom Pokemon_FireRed-.gba -game firered \
    -front-base 0x08XXXXXX -back-base 0x08YYYYYY -count 412 -stride 4 -out build/frlg-coords.json

go run ./cmd/bwsprites export -rom BLACK.nds -out build/pack -dex 1,3,6 \
    -coords build/frlg-coords.json -ground -1 -force
```

Emerald's `gMonFrontPicCoords`/`gMonBackPicCoords` addresses (0x300d38 / 0x3021d8,
440 records, stride 4) are the ones pret publishes in the symbol table that
gen1recomp ships; there is no Emerald dump in this workspace, so that default was
read from the symbols, not exercised against a ROM — pass your own flags if your
build differs. FireRed/LeafGreen have no bundled addresses at all: the
command fails with the exact flags to supply rather than inventing a table. The
coords table is indexed by the game's **internal species slot**, not national dex —
`-dex` is read as that slot when `-coords` is used, and a missing record is an
error, not a silent zero.

## Command summary

```
bwsprites list   -rom ROM.nds [-dex N] [-side front|back]
bwsprites export -rom ROM.nds -out DIR [-dex LIST] [-side both] [-palette both]
                 [-frame 0] [-ground -1] [-coords FILE] [-force]
bwsprites coords -rom ROM.gba -game GAME -out FILE [table flags]
bwsprites modpack -rom ROM.nds -out DIR [-dex LIST] [-side both] [-palette both] [-force]
bwsprites mod     -pack DIR | -external -out DIR [-zip FILE] [-force]
bwsprites install -pack DIR | -external [-identity NAME] [-mods DIR] [-force]
bwsprites verify -pack DIR
bwsprites demo   -out DIR [-force]
```

`-dex` accepts a list (`1,4,25`) or a range (`1-151`); `-side`, `-palette` and
`-dex` accept the keywords shown by `bwsprites` with no arguments.

## Measurement, in one paragraph

Frames are placed with the facets ground rule: horizontally centred, lowest
opaque row on the species' ground row, top-left origin, y down. The previewer
pack can auto-fit a cell past 64 px (the synthetic wide demo entry is 88 px).
The mod uses the same rule with the host's slots: a view keeps its own native
pixels in a square cell sized by the animation's union, so Pidgeotto stays sharp
at 104 px and Lugia keeps its 153 px width; nothing is enlarged and nothing is
reduced, and Pikachu stays 64 px.
A front taller than the 72 px gap above the enemy ground row is anchored at the
scene top and runs behind the battle menu; a wide front is slid left to stay
inside the 240 px scene. The player's back keeps its native size too, so
Charizard is not squeezed to 53 px; a tall back is moved down until its ink
starts below the enemy HP box, so its top is never clipped even though its feet
may run behind the battle menu. Because
a Gen 5 idle animation is a composite whose frames already carry positions in one
shared space, the whole animation's union box is placed once and each frame keeps
its offset inside it, which preserves intra-animation motion without jittering
the sprite. Every entry records `frameSize`, `ground`, `grounded`, `anchorX`,
`anchorY`, `union` and per-frame `offsets`, so a consumer never has to re-derive
placement. Full reasoning and worked numbers: [docs/MEASUREMENTS.md](docs/MEASUREMENTS.md).

## Outputs

Everything generated is a build artifact:

| path | content |
| --- | --- |
| `build/pack/` / `demo/pack/` | the previewer pack: `manifest.json`, one directory per entry with `strip.png` + `entry.json` |
| `build/modpack/` / `dist/bwsprites/` | the mod pack (`index.lua` + `front|back/<dex>/*.png`) and the assembled mod folder |
| `build/*-coords.json` | Gen 3 picture-coordinate tables read from a ROM |
| `demo/*.png`, `build/shots/` | previewer and in-engine screenshots |

`*.nds`, `*.gba`, `*.srl`, `.refs/`, `.cache/`, `demo/pack/` and `demo/*.png` are
git-ignored, so no ROM, extract or preview can be committed by accident.

## Tests

```sh
go vet ./...
go test ./...                     # unit tests; the LÖVE end-to-end tests skip without lovec
BW_ROM=/path/to/black.nds go test ./internal/gen5 -run TestRealROM   # needs your dump
BW_ROM=/path/to/black.nds go test ./internal/modpack -run TestRealROMModPack
```

`BW_ROM` may point at either pair: the tests read the game code and check the
size and art for that dump.

What they cover: NDS/NARC/LZ11 container decoding and its rejection paths; NCGR/
NCLR/NCER/NANR/NCMR parsing; playback timing and run merging; the measurement
rules (including auto-fit, the two clamps, and the reject cases); pack writing,
validation and round-tripping; Gen 3 coordinate parsing and its reject cases; the
synthetic demo's determinism and its measured alignment; the mod pack's index,
atomic assembly and rejection paths; and two end-to-end tests that run under
`lovec`: one writes a demo pack, runs `lovec love <pack> --selftest` and requires
a PASS line per entry (the Go-side/Lua-side previewer format agreement), the other
installs a synthetic mod pack into a temporary LÖVE save identity and runs the
mod harness (index parsing, sixteen malformed-index fixtures, frame timing, sheet
fallbacks, the animated Pokedex fit and top-clearance checks, female-back
selection through the engine's
`Ui.sidePic` seam, and the wrappers `main.lua` installs). `TestRealROMModPack` decodes the
mod pack's PNGs back and checks every animation's ink centre and ground row.

## ROM handling

- Never downloaded, never bundled, never redistributed.
- Opened read-only; the source ROM is never overwritten. Packs are written to
  paths you name, with `-force` required to overwrite an existing directory.
- Extracted art is copyrighted game data: keep packs out of source control (the
  `.gitignore` does that) and out of anything you redistribute.
- The synthetic demo pack is the redistributable example; every sprite in it is
  drawn by `internal/demo`, not extracted from a game.

## Adding a sprite

1. Export it: `bwsprites export -rom BLACK.nds -out build/pack -dex 25 -side front`
   (or start from `bwsprites demo` and copy an entry directory).
2. Edit `entry.json`: `frames`, `offsets`, `durations`, `loopStart` and
   `cycleTicks` must agree; `frameSize` must match the strip and every offset and
   the union must stay inside the cell.
3. Run `bwsprites verify -pack build/pack`, then `love build/pack` and press `d` to
   check the union box and the ground row sit where you expect.

## Troubleshooting

| symptom | cause / fix |
| --- | --- |
| `is N bytes, want an untrimmed 268435456 byte dump` | trimmed ROM; use an untrimmed 256 MiB dump |
| `game code "..." is not a supported game` | wrong game; this reads Black/White and Black 2/White 2 |
| `sprite archive /a/0/0/4: ...` | the dump is damaged, or its archive layout differs from the supported revision |
| `has no front record for slot N` | the coords file has no picture for that slot, or `-dex` is a national dex number where an internal slot is expected; pass `-ground` to use one row |
| `no bundled table addresses` | FireRed/LeafGreen: supply `-front-base -back-base -count -stride` |
| LÖVE shows a red panel | it prints the pack error, every directory it tried, and the commands that build a pack |
| nothing renders but validation passes | press `d`: the union box shows where content is, and `grounded: false` means the sprite was pinned to the top |
| the mod is not listed in the game | it was installed into a different save identity; pass `-identity <name>` (or `-mods <dir>`) |
| battle sprites do not change | the mod is installed but not enabled in the mod menu; the load log line names the species it serves |
| the mod loads with 0 species | embedded build: the installed folder has no `index.lua`; reinstall with `bwsprites install -force`. Code-only build: the identity has no `asset_packs/gen5_bw/battle_sprites`, so import the dump in the launcher first |

## Limitations

- **No ROM patch.** There is a working mod, but only for the LÖVE-based
  gen1recomp engine: it swaps pictures at runtime. Patching a .gba would mean
  replacing the cartridge's compressed graphics and animation tables
  (no verified tooling for that exists in this workspace) and respecting ROM
  checksums and free-space layout. None of that is implemented, and no patch
  format is invented here.
- **The mod is battle pictures plus the Pokedex.** Front and back, normal and
  shiny, one idle animation each; the Pokedex plays that same animation at its
  native size in the card's picture area. Distinct female art is used for a
  female battler, front
  and back; there is no form-specific art (see
  [docs/MOD.md](docs/MOD.md#limits-stated-plainly)).
- **Black/White and Black 2/White 2.** Both pairs are read (`IRA*`/`IRB*` and
  `IRD*`/`IRE*`); other games and trimmed dumps are refused by name.
- **Idle animations only.** Each sprite exports its idle/loop animation; battles
  and other animation banks are not exported.
- **The battle scene stays 240×160.** Gen 3's display size is fixed
  (`src/core/game3/display.lua:9-10`) and the wide/fill battle layout is a Gen 1/2
  feature, so no mod can enlarge the scene for portrait screens. Enemy fronts
  keep their native size (tall ones run behind the battle UI, never clipped at
  the top; wide ones are slid left) and the player's back keeps its own size,
  re-centred on engines with the fixed (32,32) draw origin and
  moved below the enemy HP box when it would otherwise be clipped; a wider scene
  is engine work, not mod work.
- **The previewer is a viewer, not a game.** It draws one entry at a time at an
  integer zoom, with no battle, map or UI context.
- **Animation cycles cap at 480 ticks (8 s).** Most idles are shorter (median 90,
  96% at or under Charizard's 432), so they loop whole. A few multi-part idles
  repeat only after a very large least common multiple; those export their first
  480 ticks and are counted in the `modpack` report (`56 views exceed the 480
  tick bound` for the BW2 dump). No frame is cut or resampled.
