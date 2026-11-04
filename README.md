# BW Animated Sprites

Animated Black/White battle and Pokedex sprites for FireRed, LeafGreen and
Emerald on the gen1recomp launcher. Not a ROM patch — the mod swaps pictures at
runtime, and it ships no game art.

## ✨ What you get

- Animated Black/White battle sprites for both sides — the enemy's front and
  your own back — at the art's native size. Nothing is resampled.
- Pokedex entries play the same idle animation, at native size, in the card's
  picture area.
- Distinct female views, front and back.
- Normal and shiny palettes for every view.
- Two in-game OPTION switches, **MY POKEMON** and **OPPONENT**, each `BW` or
  `DEFAULT`. The choice is saved with the other game options.

## 📦 Requirements

- The gen1recomp launcher, with FireRed, LeafGreen or Emerald.
- Your own, legally obtained Japanese or US **Black** or **White** ROM — an
  untrimmed 256 MiB `.nds` — imported once through the launcher's **IMPORTERS**
  screen.
- The mod zip from this repository's Releases.

## 🚀 Install

1. Open the launcher, go to **IMPORTERS → Pokémon Black/White**, pick your
   `.nds`, and wait for `gen5_bw` to finish.
2. Download `BW-Animated-Sprites-1.0.0.zip` from Releases and install it with
   the launcher's **Import mod .zip** — or unzip it so that
   `mods/BW-Animated-Sprites/manifest.json` exists.
3. Enable **BW Animated Sprites** in the mod menu and restart the game.
4. Optional: switch a side back to cartridge art in
   **OPTION → MY POKEMON / OPPONENT**.

## 🔄 Updates

The launcher checks this repository's Releases and shows an **Update** button
when a newer version is published. Every release asset is named
`BW-Animated-Sprites-<version>.zip`.

## 🛠 Building from source

```sh
go run ./cmd/bwsprites modpack -rom "Black.nds" -out build/modpack
go run ./cmd/bwsprites mod -pack build/modpack -out build/bwsprites -zip dist/bwsprites.zip
```

Engine details are in [docs/MOD.md](docs/MOD.md); provenance and licences are in
[docs/PROVENANCE.md](docs/PROVENANCE.md).

The source tree is not the payload: the Release zip is built in CI and contains
only `manifest.json`, `main.lua`, `pack.lua` and `external.lua`.

## ❤️ Credits

The sprite pipeline follows facets' ground rule: measure every frame by its own
pixels, then stand it on the species' ground row. Thanks to the gen1recomp
project for the engine and its mod system.

## ⚠️ Not a ROM patch

Nothing writes to a `.gba`; a cartridge or a plain emulator still shows the
original art.
