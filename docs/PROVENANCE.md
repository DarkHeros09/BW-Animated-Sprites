# Provenance and licensing

This project ships no ROM data, no extracted sprites and no game code. Everything
it produces comes from a dump the user supplies, and every operation on that dump
is read-only.

## AnimaEngine (MIT) via gen1recomp

The container, decode and rasterisation algorithms behind the Black/White import
path were re-implemented in Go from gen1recomp's Lua importer:

- `.refs/gen1recomp/src/import/gen5/Nds`, `Narc`, `Lz`, `Graphics`, `Cells`,
  `Animation`, `Composer`
- `.refs/gen1recomp/src/import/gen5/PROVENANCE.md`, which states those files
  "adapt AnimaEngine v1.0.0 container, decoding and rasterization algorithms,
  copyright (c) 2026 AnimaEngine contributors, under MIT", referencing
  <https://github.com/KillDaWill/AnimaEngine/tree/v1.0.0> (70547df).

The MIT notice is preserved at [LICENSES/ANIMAENGINE_LICENSE](../LICENSES/ANIMAENGINE_LICENSE),
satisfying the licence's condition that the notice travel with the derived work.
Local Go packages affected: `internal/nds`, `internal/nitro`,
`internal/gen5/{graphics,cells,animation,playback,composer,timeline}`.

The port is a rewrite, not a copy: it drops the Lua-style sparse tables for
bounds-checked slices, returns errors instead of nil-vs-false sentinels, and keeps
the same arithmetic (tile addressing, 9/8-bit signed OAM coordinates, the 4bpp
`palette*16+colour` rule, descending priority compositing, symmetric
nearest-neighbour rounding).

## gen1recomp (GPL-3.0-or-later)

gen1recomp itself is GPL-3.0-or-later with Section 7 additional terms
(BOIS CLUB GAMES, LLC), and it is **not** vendored or copied here: `.refs/` is
git-ignored and only read for evidence.

Two specific things were deliberately **not** taken:

- `.refs/gen1recomp/src/core/game3/battle/pic_coords.lua` hardcodes the FireRed/
  LeafGreen per-species ground offsets ("Auto-derived from pret
  gMonFrontPicCoords + gMonBackPicCoords + gEnemyMonElevation (FRLG)"). Copying
  that table would import GPL-derived pret data into this tree, so instead
  `bwsprites coords` reads those bytes out of the user's own ROM using addresses
  from the user's own pret symbol file. Only the *layout* of the record is
  reproduced: `size = byte0`, `width = (size >> 4) * 8`, `height = (size & 0xF) * 8`,
  `ground = byte1` (see `.refs/gen1recomp/src/import/gba/pic_coords_extract.lua`).
- Its asset packs and mod scaffolding. This project defines its own pack format
  instead ([docs/FORMAT.md](FORMAT.md)).

## facets (MIT)

`.refs/facets` is MIT, copyright (c) 2026 Elvie, kept at
[LICENSES/FACETS_LICENSE](../LICENSES/FACETS_LICENSE). Only the measurement
convention is reproduced (`mods/facets/mons.lua:77-101`, `:552-556`), as three
lines of arithmetic in `internal/measure`; no facets Lua is copied. The
adaptations are documented in [docs/MEASUREMENTS.md](MEASUREMENTS.md).

The mod uses only facets' ground rule, not its picture sizes: opponents keep
Black/White's native art, which the user chose after the facets-sized version
resampled moving wing frames. No facets sprite, palette or measurement table is
copied into this tree.

## LÖVE

The previewer in `love/` targets LÖVE 11.x (zlib licence). It has no external
Lua dependencies: `love/json.lua` is a small decoder written for this project,
and `love/pack.lua` validates packs with the same rules as `internal/pack`.

## Game data

`*.nds`, `*.gba`, `*.srl`, `.refs/` and every generated pack or screenshot are
git-ignored. `bwsprites demo` draws its own shapes, so the repository and its
tests need no copyrighted material at all.
