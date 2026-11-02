# Sprite measurement

Every exported frame is placed with the convention from
[facets](https://github.com/ElvieBlooms/facets) (`mods/facets/mons.lua`, MIT), so a
Black/White animation lands where a Gen 3 sprite would: horizontally centred, and
with its lowest drawn row on the game's ground row for that species.

Two deviations are unavoidable, because Black/White artwork is not a single
authored 64×64 picture. Both are listed below with the reason and the exact
arithmetic.

## The convention as facets implements it

```lua
-- mons.lua:33
local SIZE = 64
-- mons.lua:77-101, function mons.layOut(src, bottom, colour, place)
local ox = math.floor((SIZE - sw) / 2)
local oy = bottom - 1 - low
if oy + top < 0 then oy = -top end
if oy + low > SIZE - 1 then oy = SIZE - 1 - low end
if place then place.ox, place.oy = ox, oy end
```

- Coordinates are top-left origin, y down; frames are a fixed `SIZE` square.
- `sw`, `top`, `low` come from an alpha scan of the picture; `high` is unused
  because the vertical rule is anchored to the bottom of the sprite, not its top.
- `bottom` never comes from the art. It is the host engine's per-species ground
  offset: `local yo = tonumber(t[species]) or 0; return SIZE - (yo or 0)`
  (`mons.lua:552-556`), i.e. `bottom` is 1-based and `yo = 0` means "stand on the
  last row".
- The placement is frozen after the first picture of a species
  (`mons.lua:93-100`) so a multi-picture animation cannot jitter.
- A picture that is not exactly 64×64 is rejected (`mons.lua:612-614`).

## What this project keeps

`internal/measure/measure.go` reproduces the arithmetic exactly, in 0-based rows:

| facets | here |
| --- | --- |
| `SIZE` | `Result.FrameSize` |
| `bottom = SIZE - yo` | `Ground = FrameSize - 1 - yo` |
| `ox = floor((SIZE - sw) / 2)` | `ox = (FrameSize - union.Width()) / 2` |
| `oy = bottom - 1 - low` | `oy = Ground - (union.Height() - 1)` |
| `if oy + top < 0 then oy = -top end` | `oy = max(oy, 0)` |
| `if oy + low > SIZE - 1 then oy = SIZE - 1 - low end` | `oy = min(oy, FrameSize - union.Height())` |

`Grounded` records whether the first clamp bit: it is `false` when the sprite was
pinned to the top row because it could not reach `Ground`, and the pack records
that instead of pretending the sprite stands on the ground.

The two clamps are applied in the same order as facets, so for any input that
facets accepts, both produce the same `ox`/`oy`.

## Deviation 1: the frame edge is a parameter (default 64)

Facets rejects non-64×64 art because its host only ever draws 64×64 cells.
Black/White sprites are larger (dex 6 back is 105×88, dex 3 front is 80×80), and
silently scaling them down would throw away the extra detail the mod exists for.

- `frameSize = 0` (the CLI default) fits the cell to the animation's content box:
  `Fit(w, h) = max(64, w, h)` rounded up to a multiple of 8, capped at 256.
- `-frame 64` reproduces facets exactly: content that does not fit is pinned to
  the top of the cell and recorded as `grounded=false` — never cropped, never
  scaled.
- The cap at 256 exists so a corrupt measurement cannot make the exporter
  allocate unbounded memory; exceeding it is an error, not a silent clamp.

### The mod pack's fit: the art's own cells in the host's slots

The gen1recomp mod keeps facets' ground rule but not facets' 64 px ceiling.
Facets can reject or freeze anything above 64; this pack scales nothing up and
nothing down, and ships the art the dump already contains. The earlier
per-species facets target caused exactly the regression the user reported: a
bird whose wings move has a tall union, so fitting the union into a 72 px slot
shrank the body to 58 px and resampled it. Native cells remove both problems.

- **Enemy front: a square cell from 64 px up, as large as the union needs.** Every
  Black/White and Black 2/White 2 front keeps its own pixels — Pidgeotto is
  104 px, Fearow 96, Lugia 156 and Pikachu 64 — because the cell grows with the
  art instead of the art shrinking into a slot. The 240×160 scene has only 72
  rows above the enemy ground row, so a taller front cannot also put its feet on
  that row. The runtime anchors its ink top on scene row 0 and lets the bottom
  run behind the battle UI; the user allowed that as long as the top is never
  clipped. A front wider than the room right of the enemy base (x=176) is slid
  left by the draw wrapper until it fits the 240 px scene.
- **Player back: a square cell from 64 px up.** The player ground row is low, so a
  native back keeps its size; Charizard is 100 px, not 53. A tall back is moved
  down until its first ink row is on scene row 46 or later, below the enemy HP
  box, so the user's top rule also holds for the player's side. It is still the
  same cell size; only its y padding changes, and its feet may sit behind the
  battle menu.
- `scale = 1`: there is no cap to hit. `measure.FitNative` is the fitter the pack
  uses, and it only rounds the union up to a multiple of four; the 256 px bound
  exists so a corrupt measurement is an error, not a memory request.
- `Size = max(64, roundUp4(max(InkW, InkH)))`: the cell is square and never
  smaller than the cartridge's picture. The union is centred horizontally and
  its lowest ink row is the cell's last row, so a wide front has transparent
  rows above its ink.
- Each frame keeps its offset inside the union, so a squash, bob or sway still
  animates instead of being frozen.
- The mod zeroes the served species' `PicCoords` padding and substitutes the
  anchor correction, `64 - Size` (fixed origin) or `floor((64 - Size) / 2)`
  (centre anchor). A front whose ink top would leave the scene gets one more
  downward shift before that. On a fixed origin a picture wider than 64 is also
  shifted left by `(64 - width) / 2`; the engine's `(32,32)` origin would
  otherwise place it right of the battler. The player's back is shifted once
  more by the wide-cell overhang, bounded by `left - 1`, so its right edge stays
  on the cartridge cell's right edge and its left edge stops one pixel from
  screen x=0. A centre-anchored engine does the centring itself.
- A female battler reads the `frontFemale`/`backFemale` view when the dump has
  one. Female cells are fitted independently, and a species without distinct
  female art falls back to the male view.
- The Pokedex path stores the box enclosing every pose (`<side>Dex`) and crops
  that same window from the frame that is currently playing at 1:1, so the dex
  shows the dump's pixels. The draw wrapper centres it in the card's picture area
  (112×68, above the flavour-text divider) and clamps it to the screen: a window
  larger than the area overhangs the card rather than being reduced, and no pose
  is cut off at an edge. Position is fixed, so the idle plays in place.

Consequence: the host sees Pikachu at its native 46 px of ink in a 64 px cell,
Charizard's front at 92 px and back at 100 px, and Pidgeotto at 104 px. Relative
sizes come from the dump itself, and every picture's lowest ink row lands on the
cartridge's ground row unless it had to be top-anchored to avoid clipping.

## Deviation 2: placement comes from the animation's union, not frame 0

Facets has one authored picture per species, so freezing frame 0 is free. A Gen 5
idle animation is a composite assembled part-by-part; nearly every idle is at
most 480 ticks (8 s), and frames arrive already cropped to their opaque content
with a position in the shared sprite space. So:

- The union of all frame boxes is placed as one group by the formulas above.
- Each frame keeps its own offset inside that group:
  `X = ox + frame.MinX - union.MinX`, `Y = oy + frame.MinY - union.MinY`.
- This preserves the animation's internal motion (a bobbing sprite still bobs)
  while keeping the lowest point of the whole cycle on the ground row.

Freezing frame 0 instead would make an animation that starts airborne hang in the
air for the whole loop, which is the jitter facets avoids, in a different form.
Frames are therefore placed per frame, but from one shared anchor — the union's
placement — not from each frame's own alpha scan.

## Where the ground row comes from

Three sources, in order of specificity:

1. `-coords FILE` — a per-species table read from a Gen 3 ROM by
   `bwsprites coords` (see [docs/FORMAT.md](FORMAT.md)). The row is
   `FrameSize - 1 - record.GroundOffset`, so it follows the auto-fitted cell size
   instead of assuming 64. Records are looked up by the numeric value passed to
   `-dex`, which for a Gen 3 table is that game's internal species slot.
2. `-ground N` — one 0-based row for every sprite in the pack.
3. Default — the bottom row (`ground = FrameSize - 1`), the safe choice for a
   previewer.

The importer never invents a per-species offset: if a coords record is missing for
a slot, the export fails with a message naming the slot and suggesting `-ground`.

## Worked examples

All values are copied from real output of this tree.

Synthetic demo, entry `001/front/normal` (`bwsprites demo`):

- union 49×40, auto-fit cell 64, ground 63
- `ox = (64 - 49) / 2 = 7`, `oy = 63 - 39 = 24`
- anchor `(-1, 24)`: the demo's art starts at x = 8 in sprite space, so
  `AnchorX = 7 - 8 = -1`; the origin is outside the cell, which is legitimate
- frames 2 and 3 shift down by 4 and 2 rows inside the cell, keeping the squash

Black dump, dex 1 front normal (`bwsprites export ... -frame 0`):

- union 41×41 inside the 64 cell, ground 63, `anchorX = 25`, `anchorY = 60`,
  `offsets = [[11,23],[11,27],[11,25]]`, `durations = [54,12,6]`, cycle 72 ticks
- the three frames do not share a Y offset, which is the motion the union anchor
  is there to preserve

## What a consumer must do

1. Read `frameSize`, `ground`, `anchorX`, `anchorY` and `union` from the entry.
2. Draw the cell with its bottom edge at the game's ground line; the sprite is
   already placed inside the cell, so no extra offset is needed.
3. If the host has a fixed 64×64 cell and `frameSize > 64`, scale the whole cell
   by `64 / frameSize` (the previewer's `h` overlay shows that ratio) or draw
   overflowing content with the host's own clipping; never re-centre by hand,
   which would discard this measurement.
4. To use `anchorX`/`anchorY` instead of the cell, position by the sprite origin:
   draw at `(originX - anchorX, originY - anchorY)`.
