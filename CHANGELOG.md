# Changelog

## 1.0.1 — Keep engine-held pictures alive across cache eviction

- Fixes the double-battle crash `Cannot use object after it has been released`
  at `anime_vm.lua:722`: a battle, Pokedex, box or part picture could be evicted
  from the mod's cache and released while a particle or sprite still held it.
- The mod now only drops its Lua reference when it evicts a picture. The image
  stays valid for as long as the engine holds it, and LÖVE's garbage collector
  frees it afterwards. Internal image data is still released explicitly.
- Adds a LOVE regression test for every affected cache: the engine holds the
  first picture, the cache evicts it, and the held picture must still draw.

## 1.0.0 — First release

- Animated Black/White battle sprites for enemy and player sides, drawn at the
  art's native size with no resampling.
- Pokedex entries animate too, at native size in the card's picture area.
- Distinct female views, front and back, with normal and shiny palettes.
- OPTION gains MY POKEMON and OPPONENT switches (`BW` or `DEFAULT`), saved with
  the other game options.
- Installed from the launcher after a one-time `gen5_bw` import; the mod ships
  no game art and no sprite pack.
- Updates come from this repository's GitHub Releases, with the launcher's
  Update button.
