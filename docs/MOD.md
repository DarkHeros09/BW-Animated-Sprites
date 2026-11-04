# The gen1recomp mod

This repository ships an installable mod for the LÖVE-based
[gen1recomp](https://github.com/bryanthaboi/gen1recomp) engine: it replaces the
battle pictures of FireRed, LeafGreen and Emerald with animated Black/White
sprites. It is **not** a patch of the GBA ROM — the ROM is never read by the mod
and the engine is never rebuilt. The mod is plain Lua plus PNG data in its
embedded form, or four Lua/JSON files in its code-only form, loaded by the
engine's own mod loader.

## What it changes

| engine seam | what the mod does |
| --- | --- |
| `Pokemon.frontPic` / `Pokemon.backPic` | returns the pack's native frame for a served species; every other species keeps the cartridge picture |
| `Pokemon.frontSprite` | re-pointed at the wrapper, because the engine aliases it once |
| `MonAnim.framePic` | returns `nil` for served species, so a move does not flash the cartridge's animation sheet over the mod's frame |
| `PicCoords.front` / `PicCoords.back` | zeroes each served species' y padding so every frame's lowest ink row lands on the cartridge's own ground row; restores the cartridge table when a side is switched to DEFAULT |
| `ui.options.rows` (and `OptionRows.build`) | adds **MY POKEMON** and **OPPONENT** switches to the in-game OPTION menu, or one **BW SPRITES** group when the menu supports groups |

### Cell size: the art's own, never resampled

The **ground rule is facets'** (`mons.lua`: lowest ink row on the ground line,
horizontally centred). The **size is the animation's own**: the cell is the
union's larger side rounded up to four, so nothing is enlarged and nothing is
reduced on either side. Facets only accepts 64×64 static art, so it stops at 64;
this pack keeps the pixels the dump contains instead. Pikachu stays 64 px / 46 px
of ink while Pidgeotto is 104 px and Lugia 156 px, on the enemy side and on the
player's.

- **Enemy front: one square cell from 64 px up, as large as the art needs.** A
  front whose ink is taller than the 72 px gap above the enemy ground row is
  anchored with its ink top on scene row 0 and its feet lower, behind the battle
  UI. The user allowed the bottom to be covered but not the top. A front wider
  than the room right of the enemy base is slid left by the draw wrapper until it
  fits the 240 px scene.
- **Player back: one square cell from 64 px up.** The player's ground row is low,
  so a native back fits (Charizard is 100 px, not a squeezed 53 px). A tall back
  whose ink would start above scene row 46 — the bottom of the enemy HP box in a
  single battle — is shifted down until its ink top clears that box. Only the
  position moves, never the cell size; the feet may run behind the battle menu,
  which the user preferred over a clipped top. A 64 px back is untouched.
- **Female views:** when the dump has a distinct female graphic, the pack also
  writes `<side>/female/<dex>/<palette>.png` and the index carries
  `frontFemale*`/`backFemale*` fields. The battle wrappers use `mon.gender`, then
  `mon.personality`, then `Pokemon.gender(species, personality)` to choose; a
  species without distinct female art falls back to its male view. Some engine
  builds wrap `Pokemon.backPic` with a three-argument shim that drops the
  battler, so the mod also remembers the battler's gender across the nested call
  from `Ui.sidePic`, the battler draw seam that still has it.
- **Pokedex:** animated at the art's native size. The pack writes, per view, the
  box enclosing every pose (`<side>Dex = {x,y,w,h}`). The runtime crops that
  window out of whichever frame is playing at 1:1 — no resampling — so Pikachu's
  dex picture keeps its own 51×46 and a Lugia keeps 153×91. The draw wrapper
  centres the picture in the card's picture area (the 112×68 space right of the
  species stats and above the flavour-text divider) and clamps it to the screen,
  so a window larger than the area overhangs the card instead of being reduced
  and no pose is ever cut off. Window and position are fixed, so the idle plays
  without jitter. Battle pictures never pass through this path.

Inside every cell the animation's union is centred with its lowest ink row on the
cell's last row, and each frame keeps its per-frame offset inside the union so
the animation still moves. A wide front therefore has transparent rows above
its ink; the runtime positions the cell bottom, not the ink top. The Pokedex is
the one screen with its own geometry: the runtime crops the pose window, taken
from the current frame, at 1:1, and the draw wrapper centres it in the card's
picture area.

The two engine anchors draw a native cell differently:

- **Fixed origin** (`(32,32)`, the unpatched engine): only a 64 px picture is
  centred. The mod wraps `love.graphics.draw` and shifts any pack picture wider
  than 64 left by `(64 - width) / 2`, then clamps a front inside the 240 px
  scene. The player's back gets a second, bounded shift by the wide-cell
  overhang, so its right edge stays on the cartridge cell's right edge and its
  left edge stops one pixel from the screen edge.
- **Centre-anchored** (`src.core.game3.battle.ui.PIC_CENTRE == true`): the
  engine centres by the picture's own size, so the mod does not wrap the draw.

The mod clears the served species' `PicCoords` y padding and replaces it with
the anchor correction: `64 - size` on the fixed origin, `floor((64 - size) / 2)`
when centred, with an extra shift for a front whose ink top would leave the
scene. With the pack's ink bottom on row `size - 1`, that puts every species'
feet on the cartridge's ground row, independent of the cell size. The Pokedex
path takes its fixed pose window from the current frame at 1:1 and the draw
wrapper centres that box in the card's 112×68 picture area, so it animates
without shrinking with a wing-down frame and without cutting a pose off.

The draw site is `src/core/game3/battle/ui.lua:2299`:
`love.graphics.draw(entry.image, cx, cy, rot, sx, sy, 32, 32)`. See
[MEASUREMENTS.md](MEASUREMENTS.md) for the rules and
[internal/measure/host.go](../internal/measure/host.go) for `FitHost`.

## Build, install, enable

```sh
# 1. Data: every species, both sides, normal and shiny, native cells.
go run ./cmd/bwsprites modpack -rom "Black.nds" -out build/modpack

# 2a. Install straight into a LÖVE save identity
#     (creates <save>/mods/BW-Animated-Sprites):
go run ./cmd/bwsprites install -pack build/modpack -identity pokemon-love2d

# 2b. Or assemble a folder to copy yourself:
go run ./cmd/bwsprites mod -pack build/modpack -out build/bwsprites
#     then copy build/bwsprites to
#     <LOVE save dir>/<identity>/mods/BW-Animated-Sprites

# 2c. Or build the installable .zip the launcher's "Import mod .zip" takes:
go run ./cmd/bwsprites mod -pack build/modpack -out build/bwsprites -zip dist/bwsprites.zip

# 3. Code-only build: no art in the mod. Import the dump once in the launcher's
#    IMPORTERS screen, then install just the reader:
go run ./cmd/bwsprites install -external -identity pokemon-love2d
go run ./cmd/bwsprites mod -external -out build/bwsprites-bw -zip dist/bwsprites-bw.zip
```

The code-only build writes four files (`manifest.json`, `main.lua`, `pack.lua`,
`external.lua`) and declares the imported pack as a required asset. It reads
`asset_packs/gen5_bw/battle_sprites` through `mod.packs`.

The installer accepts one shape only: either `manifest.json` at the archive
root, or exactly one top-level folder holding it
(`LauncherMods.locateRoot`, `src/mods/LauncherMods.lua:436`). `-zip` writes the
second shape — every file under `BW-Animated-Sprites/`, nothing else — so a
README or a screenshot beside the mod folder would not be packaged. Unzipping
that archive into `<save>/<identity>/mods/` installs the same bytes as
`install`.

`-identity` defaults to `pokemon-love2d`; `-mods DIR` overrides the whole path.
The LOVE save root is `%APPDATA%/LOVE` on Windows,
`~/Library/Application Support/LOVE` on macOS and
`$XDG_DATA_HOME/love` (or `~/.local/share/love`) elsewhere. The game's identity
is `POKEPORT_IDENTITY` or the default `pokemon-love2d`.

Then start the game and enable **BW Animated Sprites** in the mod menu (the
engine records this as `modsByVersion.<game>["BW-Animated-Sprites"] = true`
in `options.lua`). The mod logs one line when it loads:

```
[bwsprites] bwsprites: 649 species, 2596 views (1298 female), 53618 frames from Pokemon Black (IRBO) (2/2 sides on BW, native cells, fixed origin)
```

If that line is missing the mod did not load; if it reports 0 species the pack is
missing from the installed folder. `native cells` in the log means "not
compatibility-scaled"; cells are 64..256 px, the art's own size, and
the count in parentheses is the number of extra female views.

### Prebuild and cache: not needed here

Facets has a **PREBUILD** option and an install cache because it *builds* each
picture: a GB Emerald Pokémon, a GBC NPC or a Crystal frame is composed from
other games' art the first time it is needed, which hitches a battle or a map
entry; PREBUILD spreads that work over frames and `diskcache.lua` keeps the
results for the next start (`prebuild.lua:1-12`, `diskcache.lua:1-12`).

Mostly it does not. The launcher's importer, or `modpack`, already wrote one
animation atlas per view; the mod decodes an atlas the first time a species is
seen and cuts each frame once. The embedded runtime keeps those frames per view
(`pack.lua:221-240`); the imported runtime keeps a small LRU of atlases and
frames (`external.lua`). After that, a frame is a table lookup plus
`Pack.frameIndexTicks`, not an image build.

The exception is the imported pack's capped views: the exporter stops their atlas
at 240 ticks but ships part tracks for the rest of the cycle, and the code-only
build composes those poses from the pieces on demand. Composing one 89x91
Charizard pose costs about 0.4 ms; results are cached by the tracks' state
signature (many ticks repeat a pose), so steady-state playback is cache hits. A
full prebuild would decode all 2,596 views up front — hundreds of megabytes of
RGBA for no steady-state gain — so it is deliberately not an option. If a
low-end handheld shows a one-off hitch on the first encounter, pre-decoding that
battler's atlas before the battle transition is the small fix; a full install
cache is not.

## Switching back to cartridge art

The game's own **OPTION** menu gains the two switches. Where they appear depends
on the engine's menu generation:

- **Newer unified OPTION menu** (`src/ui/OptionsMenu.lua`): the two rows sit at
  the end of the top-level list, so scroll down to them. The sandbox only exposes
  a Gen 3 compatibility facade for that screen, so a mod cannot open a dedicated
  sub-page there; two loose rows are the honest seam.
- **Older grouped OPTION menu** (`src/ui/game3/option_menu.lua` with
  `OptionRows.GROUPS`): the rows are folded into one **BW SPRITES** entry
  (shown as `2 OPTIONS`), just below BATTLE OPTIONS.

| row | controls | values |
| --- | --- | --- |
| MY POKEMON | the player's back pictures | `BW` / `DEFAULT` |
| OPPONENT | the foe's front pictures | `BW` / `DEFAULT` |

Both default to `BW`. `DEFAULT` serves the cartridge picture for that side and
restores the `PicCoords` padding the mod cleared, so its feet land exactly where
vanilla puts them; the other side is unaffected. The choice applies on the next
draw, survives a save, and is stored in
`options.modOptions["BW-Animated-Sprites"]` (`playerSprites` / `foeSprites`) —
the same tree the engine's own OPTIONS use.

The mod writes the live choice into both the engine's options tree and its own
table. That matters on engines where `mod.options:get` reads the table loaded at
boot rather than the one the OPTION menu mutates; without the local copy a switch
would only take effect after a restart. Both hosts were verified in-engine.

## What the pack contains

`bwsprites modpack` writes a directory the mod reads through `mod:read` and the
engine's save-relative paths:

```
pack/
  index.lua                  format, source, one entry per national dex
  front/<dex>/normal.png     grid of square cells, frames left-to-right then down
  front/<dex>/shiny.png
  back/<dex>/normal.png
  back/<dex>/shiny.png
```

`bwsprites mod` copies that plus the mod's own three files into
`<out>/{manifest.json,main.lua,pack.lua}` and the pack tree beside them. The mod
sources are embedded in the Go binary (`internal/modpack/assets/mod`), so an
install is one self-contained folder.

The index is a Lua literal rather than JSON because the engine's sandbox has no
JSON reader and `load` is available; `pack.lua` validates every field before use
and refuses the whole pack with one actionable error. The schema and validation
rules are in [FORMAT.md](FORMAT.md#mod-pack-indexlua).

## Verification

Go side:

```sh
gofmt -l .
go vet ./...
go test ./...
BW_ROM=/path/to/black.nds go test ./internal/modpack -run TestRealROMModPack
```

`TestRealROMModPack` builds a pack for dex 1, 3, 6, 25, 150, 249, 382 and 649
and decodes the PNGs back: every sheet's cell side is within the 256 px bound and
is the art's own size, Charizard's front is its native 92 px, its back at
least 96 px, Pikachu's front stays 64 px, Lugia's is 156 px and Rayquaza's 100 px,
every frame has ink inside its cell, the animation union is centred and its lowest
ink row is the cell's last row, female sheets exist where the dump has them, and
the dump's size and mtime are unchanged.

Lua side (no ROM needed): `TestModHarnessPassesTheLuaChecks` in `e2e_test.go`
installs a synthetic pack into a temporary LÖVE save identity and runs
`internal/modpack/testdata/harness` under `lovec` in three modes: fixed origin,
centre-anchored with the grouped OPTION menu, and centre-anchored with the
unified OPTION menu (loose rows). It checks index parsing and its malformed-index
fixtures, frame timing and looping, missing/oversized sheets, the shiny fallback,
both menu paths, a 120 px wide opponent cell, the edge-safe back shift, female
selection and male fallback, and that `main.lua` wraps the engine seams, applies
the footing correction, restores the cartridge padding on DEFAULT, and re-pins
on BW.

In-engine check (needs the user's own imported FireRed and LÖVE):

```sh
POKEPORT_IDENTITY=<identity> \
POKEPORT_DRIVER=tools/drivers/bwsprites_probe.lua \
POKEPORT_SHOT_DIR=build/shots \
"C:/Program Files/LOVE/lovec.exe" .refs/gen1recomp --game=firered
```

On a centre-anchored build point `lovec` at that checkout instead. The driver
boots a new game, starts a wild Pikachu against the player's Charizard, prints
PASS/FAIL for the picture table, cell size and anchor, that the player's
Charizard back keeps a native 100 px cell while the foe's Pikachu stays 64, the
native Pidgeotto (104), Fearow (96), Lugia (156) and Rayquaza (100) fronts, the
female Pikachu picture, the y offset, the OPTION rows, switching to DEFAULT and
back, and a frame change between two captures, and writes the screenshots under
`POKEPORT_SHOT_DIR`.

## How this relates to facets and the engine's own importer

- **facets** is a gen3 mod that swaps NPC, trainer and Pokémon pictures. Its
  Pokémon art sets are GBC-style and a static **DS-style 64×64** resource
  ("DS-style Pokémon Sprite Resource" by Chaos Rush and others) — it ships no NDS
  reader and no Black/White import. Its animation support is the Emerald
  cartridge's own 2-frame front animation and Game Boy frame sets
  (`anim/NNN.png` + `anim/NNN.json`). It cannot show animated Black/White sprites
  from a dump; what it does have is a good static 64×64 convention, which this
  project's measurement follows. Its crisp look also comes from its screen
  shader, which is a facets feature, not a sprite property.
- **The engine's own importer** (launcher → IMPORTERS → Pokémon Black/White)
  reads an untrimmed 256 MiB dump: `src/import/gen5/BwImport.lua`
  (EXPORT_VERSION `1.1.0`) writes `asset_packs/gen5_bw/battle_sprites`, and the
  example provider `mods/examples/gen5_battle_sprites` hands out frames up to
  256×256 with the consumer choosing the display scale. Black is the tested dump;
  White and other regions are unverified, and the importer alone does not draw
  anything in battle — a renderer mod must consume it. This mod is that renderer
  in two forms: the embedded build decodes the same NARC/NNS data in Go offline
  (no import needed, one folder), while the code-only build consumes the
  launcher's `gen5_bw` pack directly through `mod.packs` and ships no art.

## Limits, stated plainly

- **Not a ROM patch.** Nothing writes to a .gba; this only works inside
  gen1recomp. A cartridge or an unmodified emulator still shows the original art.
- **Female art is used, with a male fallback.** The 96 species whose dump has a
  distinct female graphic get an extra pair of views; the rest share the male
  view. Female selection is battle-only (the Pokedex has no gender).
- **Forms and special cases.** Form-specific art (Castform, Deoxys, Unown
  letters) is not exported; those species show the cartridge picture unless their
  base national id is served.
- **Code-only build: capped views are composed from part tracks.** The launcher's
  exporter 1.1.0 caps 360 imported views at 240 ticks (4 s) in their atlas but
  ships part tracks covering the full cycle (Charizard's tracks run 432 ticks).
  The code-only build composes those views from the tracks, matching the
  exporter's atlas pixel-for-pixel inside the prefix (checked with a LÖVE probe),
  so they play the full cycle like the embedded build. Only a pack with a capped
  view and no part track falls back to the 240-tick prefix, and the mod logs how
  many did.
- **One animation per species.** The idle animation only. Move animations stay
  the cartridge's for unserved species and are suppressed for served ones, so a
  served species does not animate during a move; the idle loop keeps playing.
- **Animations play at cartridge speed.** A frame is a distinct
  `(cell, transform)` pose, not only a distinct cell: a sprite that sways,
  squashes or hovers on one cell still animates. That restores 13,777 front /
  13,032 back male poses, plus female views and the full cycles (35,320 frames;
  only dex 273, 299 and 546 genuinely have one frame), so nothing is frozen or
  skipped.
  NMAR durations are hold times in 60 Hz ticks and are used as-is.
- **Native sizes, not facets' 64x64 cap.** A cell is the animation's union, so no
  view is resampled and a bird that moves its wings no longer shrinks to fit a
  union; the 256 px bound only catches a corrupt measurement. A front taller than
  the 72 px slot is anchored at the scene top and extends behind the battle UI.
- **Pokedex.** The Pokedex draws its picture at native size, centred in the
  card's picture area (112×68, larger than the 64×64 slot the cartridge art
  used): the runtime crops the pose window from the current frame at 1:1 and the
  draw wrapper keeps the whole picture on screen, so a window bigger than the
  area overhangs the card instead of being reduced and no pose is cut off. Page
  2's size comparison gets a 64×64 fit instead, so the trainer comparison keeps
  the cartridge's proportions.
- **Sheets are grids.** The writer lays each view out in the fewest columns that
  keep the sheet within 4096 px (the GLES2 texture floor). Most views are a
  single column: Pikachu's 27-frame front is 64x1728, Charizard's 72-frame 92 px
  front is 184x3312 and its 18-frame 100 px back is 100x1800.
- **Cycles cap at 480 ticks (8 s).** Most idles are shorter (median 90 ticks;
  96% at or under Charizard's 432), so they loop whole. A few multi-part idles
  repeat only after a very large least common multiple; those export the first
  8 s (56 views) and the pack build prints the count. No frame is cut or
  resampled.
- **The battle scene stays 240×160.** Gen 3's display size is fixed
  (`src/core/game3/display.lua:9-10`); a mod can change what is drawn in it, not
  its size. The native front/back art runs behind the battle UI when taller than
  its slot; a wider scene is engine work, not mod work.
- **Extension seam.** The mod reaches engine internals through
  `permissions: ["engine_internals"]`; a future engine that renames
  `PicCoords`, `PIC_CENTRE` or `ui.options.rows` can disable parts of it, and
  the mod degrades to cartridge art rather than crashing.