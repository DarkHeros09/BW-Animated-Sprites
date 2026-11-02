# Pack format, version 1

A pack is a directory. The Go tool writes it; the LÖVE previewer reads it. Both
validate it, against the same rules, so a pack one accepts cannot be silently
misread by the other.

```
pack/
  manifest.json            format, source, entry list
  001/front/normal/
    strip.png              frames left to right, frameSize x frameSize each
    entry.json             measurement and timing for this entry
```

Generated packs are build outputs: `demo/pack/` and every other pack is
git-ignored, because the frames are derived from a ROM.

## manifest.json

```json
{
  "format": 1,
  "importer": "gen5_bw",
  "tickRate": 60,
  "frameSize": 0,
  "source": {"path": "...", "size": 268435456, "gameCode": "IRBO", "sha256": "..."},
  "entries": [ /* Entry objects, sorted by id */ ]
}
```

| field | meaning |
| --- | --- |
| `format` | pack layout revision; a mismatch is an error, never a guess |
| `importer` | `gen5_bw` (from a ROM) or `synthetic` (the demo) |
| `tickRate` | ticks per second; fixed at 60, the rate every duration is expressed in |
| `frameSize` | cell edge shared by the whole pack, or `0` when each entry fitted its own |
| `source` | identity of the dump, so a pack built from the wrong ROM is caught |
| `entries` | the sprites, sorted by `id` for reproducible output |

`source.digest`/`sha256` is the SHA-256 of the whole ROM. It is not a security
boundary; it is there to make "which dump produced this?" answerable.

## entry.json

```json
{
  "id": "001/front/normal",
  "dex": 1, "side": "front", "female": false, "palette": "normal",
  "frames": 3,
  "frameSize": 64,
  "ground": 63,
  "grounded": true,
  "anchorX": 25, "anchorY": 60,
  "offsets": [[11, 23], [11, 27], [11, 25]],
  "durations": [54, 12, 6],
  "loopStart": 0,
  "cycleTicks": 72,
  "union": [11, 23, 52, 64],
  "strip": "001/front/normal/strip.png"
}
```

| field | meaning |
| --- | --- |
| `id` | `NNN/side/palette`, or `NNN/side/female/palette`; also the directory name |
| `label` | optional human-readable name; the synthetic demo sets it, ROM imports do not |
| `dex`, `side`, `female`, `palette` | which slot the sprite came from |
| `genderFallbackOf` | set when a female slot was requested but the species has none; names the male entry used |
| `frames` | frame count; equals `len(offsets)` and `len(durations)` |
| `frameSize` | square cell edge, 1..256; with `manifest.frameSize = 0` this is per entry |
| `ground` | 0-based cell row the lowest content row lands on |
| `groundFrom` | name of the table `ground` came from, when it came from a coords file |
| `grounded` | false when the sprite was pinned to the top row instead of reaching `ground` |
| `anchorX`, `anchorY` | cell-space position of the sprite-space origin (0,0) |
| `offsets` | per frame, where its content box starts inside the cell |
| `durations` | per frame, in ticks; index-aligned with `offsets` |
| `loopStart` | frame index the loop returns to |
| `cycleTicks` | sum of `durations` |
| `cycleCapped` | true when the source cycle was cut at the export bound (240 ticks) |
| `union` | `[minX, minY, maxX, maxY]` in cell space, max exclusive, enclosing every frame |
| `strip` | strip path relative to the pack root, forward slashes |

### Coordinates

Cell space is top-left origin, y down, in pixels: the same convention facets uses
(see [docs/MEASUREMENTS.md](MEASUREMENTS.md)). `union` is exclusive on `max`, so
a union of `[11,23,52,64]` covers columns 11..51 and rows 23..63.

### Time

Time is counted in ticks at `tickRate` (60/s), not in milliseconds, because the
source animations are tick-based. A frame is current while the play head is
inside its `duration`; after the last frame the loop restarts at `loopStart`,
which `gen5_bw` sets to the point its loop-prefix analysis found. `cycleTicks` is
the sum, so a consumer never has to add it up.

Identical consecutive frames are already merged into one frame with a longer
duration by the importer, so nothing in a pack relies on frame duplication.

## strip.png

- Palette PNG, width `frameSize * frames`, height `frameSize`.
- Frame `i` occupies columns `[i*frameSize, (i+1)*frameSize)`.
- Palette index 0 is transparent, as in the source NCGR/NCLR pair.
- Nearest-neighbour sampling; the previewer sets the filter explicitly.

## coords.json (input, not part of a pack)

Written by `bwsprites coords`, read by `bwsprites export -coords`.

```json
{
  "format": 1,
  "game": "emerald",
  "source": "Pokemon_Emerald.gba",
  "stride": 4,
  "front": [{"Width": 64, "Height": 64, "GroundOffset": 0, "Empty": false}],
  "back":  [{"Width": 64, "Height": 64, "GroundOffset": 0, "Empty": true}]
}
```

- `front`/`back` are indexed by the game's **internal species slot**, not by
  national dex. Slots are produced densely from index 0, so the array order is
  the slot; `Empty` marks a slot with no picture.
- `Width`/`Height` come from the record's packed size byte
  (`width = (size >> 4) * 8`, `height = (size & 0xF) * 8`); `GroundOffset` is the
  record's `yo` byte, counted up from the bottom edge.
- `stride` is the record stride the table was read with, kept for provenance.
- Validation: `format` must be 1, at least one side must be present. A missing
  record for a requested slot is an export error, not a silent zero.

## mod pack (index.lua)

`bwsprites modpack` writes the same measurements for the gen1recomp mod, keeping
facets' ground rule but using the art's own size: **one square cell per frame,
as large as the animation's union needs, never below 64 and never resampled**.
Nothing is enlarged and nothing is reduced on either side, so a front or a back
keeps every pixel the dump contains and a cell may be wider or taller than the
battle's 64 px slot. The union's lowest ink row lands on the cell's last row,
horizontally centred, and every frame keeps its own offset inside the union so
intra-animation motion survives. A front taller than the 72 rows above the enemy
ground row is anchored with its ink top on scene row 0 and runs behind the
battle UI. A front wider than the room right of the enemy base is slid left by
the draw wrapper to fit the 240 px scene, and a tall player back is moved down
until its ink top clears the enemy HP box (row 46) without changing its size.

The mod zeroes each served species' `PicCoords` y padding and substitutes the
cell correction, `64 - size` (fixed origin) or `floor((64 - size) / 2)` (centre
anchor), so the last ink row lands on the same screen row as the cartridge's
feet. On a fixed-origin engine a picture wider than 64 also shifts its draw left
by `(64 - width) / 2`; the player's back is shifted once more by its wide-cell
overhang so it stops one pixel from the screen edge, and a tall back is shifted
down until its ink starts on scene row 46 or later (the bottom of the enemy HP
box in singles). Centre-anchored engines need no centring shift.
`Pack:dexFrame` takes the entry's `<side>Dex` window -- the box enclosing every
pose -- from the current animation frame and crops it at 1:1: no resampling, so
the Pokedex shows the dump's pixels. The mod centres that picture in the card's
picture area (112×68 at 124,20, above the flavour-text divider) and clamps it to
the screen, so a window larger than the area overhangs the card instead of being
reduced and no pose is cut off. `Pack:boxFrame` is the 64×64 fit that page 2's
size comparison and the credits roll still use. A pack may declare cells up to
256 px; the reader accepts that range and applies the same correction.

```
pack/
  index.lua                  one Lua table literal, read with load()
  front/<dex>/normal.png     grid of square cells, side = that view's Size
  front/<dex>/shiny.png
  back/<dex>/normal.png
  back/<dex>/shiny.png
  front/female/<dex>/normal.png   only when the dump has a distinct female view
  front/female/<dex>/shiny.png
  back/female/<dex>/normal.png
  back/female/<dex>/shiny.png
```

`index.lua`:

```lua
return {
  format = 2,
  importer = "gen5_bw",       -- which importer produced the art
  tickRate = 60,              -- ticks per second, as in manifest.json
  box = 64,                   -- cartridge cell side the footing correction uses
  pad = 0,                    -- no transparent rows below the union ink
  source = { path, code, name, digest },  -- the dump this pack came from
  entries = {
    [1] = {
      front = "front/1/normal.png",
      frontShiny = "front/1/shiny.png",
      frontFrames = 3,
      frontSize = 64,         -- 64..256, multiple of 4, the art's own size
      frontColumns = 1,       -- cells per sheet row, 1..64
      frontInkH = 46,         -- union ink height inside the cell (1..Size)
      frontDex = { x = 8, y = 21, w = 49, h = 43 },  -- Pokedex window: every pose's box
      frontDurations = { 54, 12, 6 },
      frontLoopStart = 0,
      back = "back/1/normal.png",
      -- backShiny, backFrames, backSize, backColumns, backInkH, backDex,
      -- backDurations, backLoopStart
      -- frontFemale, frontFemaleShiny, frontFemaleFrames, ... when present
    },
  },
}
```

- Entry keys are **national dex numbers**; the mod maps the game's internal
  species id through `Pokemon.national`. A side may be absent from an entry but
  an entry must have at least one side.
- Sheets use the same palette rules as `strip.png` above, laid out as a grid:
  frame `i` (0-based) sits at column `i % Columns` and row `floor(i / Columns)`,
  each cell `Size`×`Size`, column 0 of the cell transparent. `Columns` and
  `InkH` are per view and shared by every cell in that sheet. The writer picks
  the fewest columns that keep the sheet within the 4096 px GLES2 texture floor,
  so most views are a single column (Pikachu's 27-frame front is 64x1728;
  Charizard's 40-frame 92 px front is 92x3680, its 18-frame back is 100x1800;
  Pidgeotto's 18-frame 104 px front is 104x1872) and only a view taller than
  4096 px is split.
- The time model is identical to the previewer's: `tick = floor(seconds * 60)`,
  walk the durations, and when the walk passes the last frame continue at
  `loopStart + 1` (Lua 1-based), so the intro plays once per animation.
- Validation is done by `Pack.parseIndex` in `internal/modpack/assets/mod/pack.lua`
  and covered by the harness: `format` must be 2, `box` 64, `pad` 0, `tickRate`
  positive, keys positive integers, 1..1024 frames per side, one positive
  duration per frame, `loopStart` in range, non-empty sheet paths, `Size` a
  multiple of four from 64..160 (a new pack writes 64..160 front / 64..112 back),
  `Columns` 1..64, `InkH` 1..Size, and an optional `Dex` table whose `frame` is
  1..frames and whose `x, y, w, h` box lies inside the `Size` cell (`x+w`,
  `y+h` at most `Size`), and every declared sheet exactly `Columns*Size` wide and
  `ceil(frames/Columns)*Size` tall. A sheet that is missing or the wrong size
  makes that one side fall back to the cartridge picture rather than failing
  the pack.

## Validation rules

Applied by `bwsprites verify`, by `internal/pack`, and by `love/pack.lua`:

- `format` = 1; `tickRate` > 0; pack `frameSize` 0..256.
- entry `frameSize` 1..256, and equal to the pack's when the pack sets one.
- `frames` > 0 and equal to the lengths of `offsets` and `durations`.
- every duration > 0; their sum equals `cycleTicks`.
- `ground` inside `0..frameSize-1`; `loopStart` inside `0..frames-1`.
- every offset inside the cell; `union` non-empty and inside the cell.
- the strip exists, is PNG, and is exactly `frameSize*frames` by `frameSize`.

Writes are atomic: sidecars and the manifest go through a sibling temp file and a
rename, so an interrupted export leaves the previous pack intact rather than a
truncated JSON file.
