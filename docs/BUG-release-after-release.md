# Bug: engine-held picture released by the mod cache

## Symptom

A double battle crashed while drawing particles:

```
game3/battle/anime_vm.lua:722: Cannot use object after it has been released
  anim_vm.lua:722 draw_sprite
  anim_vm.lua:863 draw
  anim.lua:1141 drawParticles
  ui.lua:2868 particles
  ui.lua:2882 draw_double_mons
```

`anime_vm.lua:722` draws `pimg or s.image`. The engine had kept that `Image` in
a sprite/particle object, but the mod's external build had already called
`Image:release()` on it.

## Reproduction

`internal/modpack/testdata/release-after-release/` is a standalone LOVE app that
loads the real `internal/modpack/assets/mod/{pack,external}.lua` and stubs only
the launcher pack (`External.new` + `packs.info/entries/read/metadata`). For each
affected cache it:

1. asks the mod for the first picture and keeps it, as a particle/sprite does;
2. fills the cache until that entry is evicted (the harness asserts the cache
   key is gone);
3. draws the held picture again.

Run it directly:

```powershell
$env:MOD_DIR = (Resolve-Path 'internal\modpack\assets\mod').Path
& 'C:\Program Files\LOVE\lovec.exe' 'internal\modpack\testdata\release-after-release'
```

Before the fix, all five draw checks failed with the game's exact error:

```
FAIL  a battle frame held by the engine survives a cache eviction  (Cannot use object after it has been released.)
FAIL  a Pokedex picture held by the engine survives a cache eviction  (Cannot use object after it has been released.)
FAIL  a 64x64 box picture held by the engine survives a cache eviction  (Cannot use object after it has been released.)
FAIL  a composed part cell held by the engine survives a cache eviction  (Cannot use object after it has been released.)
FAIL  a composed part image held by the engine survives a cache eviction  (Cannot use object after it has been released.)
selftest: 5 failure(s)   (exit 1)
```

The eviction checks themselves passed, so the pictures really were still
reachable from the engine when the mod released them.

## Root cause

`external.lua` evicts one entry when a cache is full. The old `newest()` called a
releaser on the evicted item, and `releaseImage()` called `item.image:release()`
for the frame, dex, box, part-cell and part-image caches. Those images are
returned to the engine by `External:frame`, `External:dexFrame` and
`External:boxFrame`; `main.lua` passes them to `Pokemon.frontPic/backPic/
dexFrontPic`. `picMark` (`main.lua:129`) is a weak-key table, so it never kept
the image alive.

A double battle's particle/sprite keeps drawing the same image over several
frames. When the 16/32/64/192-entry LRU reached its limit, the next lookup
evicted and released a picture the particle still held. The next
`love.graphics.draw` raised the crash.

The embedded (non-external) path already had the correct rule: `pack.lua:234-247`
caches its images in `view.images` and never releases them, only the temporary
`ImageData`. The external build's `releaseImage()` was the outlier.

## Fix

`internal/modpack/assets/mod/external.lua:212-236` now states and follows one
ownership rule: **an `Image` handed to the engine is owned by whoever holds it.
Cache eviction only drops the mod's Lua reference; LOVE's garbage collector frees
the texture when the last holder lets go.** Only `ImageData` that never leaves
the module is released explicitly.

- `external.lua:338` frames — no releaser.
- `external.lua:355` dex — no releaser.
- `external.lua:568` part cells — no releaser.
- `external.lua:637` boxes — no releaser.
- `external.lua:552` part images — releases `data` only (the composed source is
  internal to the mod and is not returned to the engine).
- `external.lua:320` atlases — still releases `data`; it is internal only.
- `external.lua:473,498` plans/pixels — unchanged, never released.

No double release remains:

- `Pack.fitRect` (`pack.lua:278-321`) always builds a new Canvas/Image; boxes
  never share an `Image` with the dex or part caches.
- part cells are fresh `newImage` copies of the composed data, not the
  `partImages` image.
- frames, dex, boxes and part cells each own a distinct `Image`.
- the two released `ImageData` owners (atlases and part-image sources) are
  single-owner caches and are never handed to the engine.

## Regression test

`release_after_release_test.go` is a root-package Go test. It assembles the real
external mod with `modpack.AssembleExternal`, locates `lovec` exactly like
`e2e_test.go` (skips cleanly when LOVE is absent), runs the harness with
`MOD_DIR`, and requires:

- `selftest: OK`;
- no `FAIL ` line;
- a `PASS` for each of the five "held by the engine survives a cache eviction"
  checks.

It therefore fails against the old `releaseImage()` code and passes now.

```powershell
go test -run TestModImagesSurviveCacheEviction -v .
```

Post-fix result: all 15 checks `PASS`, `selftest: OK`, `PASS
TestModImagesSurviveCacheEviction (1.01s)`.

## Release 1.0.1

- `internal/modpack/assets/mod/manifest.json:4` — version `1.0.1`.
- `CHANGELOG.md:3-12` — new `## 1.0.1` section.
- `internal/modpack/release_test.go:13` — `releaseVersion = "1.0.1"`.
- `internal/modpack/modpack_test.go:207-209` — compares against
  `releaseVersion` instead of a second hard-coded version.
- `.modkitignore:15-16` — guards for `BW-Animated-Sprites-1.0.1.zip` /
  `.modpkg`.
- `README.md:30` — install step names the 1.0.1 asset.

## Verification

- `go vet ./...` — exit 0.
- `go test ./...` — all packages ok; the LOVE test ran (not skipped).
- `go run ./cmd/bwsprites mod -external -out <tmp>/BW-Animated-Sprites -force`
  — `4 mod files`, exact set `external.lua main.lua manifest.json pack.lua`.
- `python tools/build_release.py <tmp>/BW-Animated-Sprites -o <tmp>/out.zip` —
  members `BW-Animated-Sprites/{external.lua,main.lua,manifest.json,pack.lua}`,
  manifest `version` 1.0.1.
- `python tools/changelog_section.py 1.0.1 CHANGELOG.md` — exit 0.

`gofmt` caveat: this checkout has `core.autocrlf=true`; the working tree is
CRLF while the index is LF. `gofmt -l cmd internal` therefore lists every Go
file, including untouched ones, because `gofmt` would rewrite CRLF to LF. The
changed Go files are gofmt-clean when checked from their LF index content (and
the new test is clean as LF); CI checks the LF tree.
