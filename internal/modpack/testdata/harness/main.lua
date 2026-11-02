-- Harness for the bwsprites mod: loads the real pack.lua and the real
-- main.lua against a synthetic pack and stubbed engine modules, so the mod's
-- parsing, frame timing, cell geometry, fallbacks and engine wrapping are
-- checked without a game or a ROM. MOD_DIR points at a built mod directory and
-- BAD_DIR at the malformed indexes; every check prints PASS or FAIL and the
-- exit code is the number of failures.
--
-- CENTRED=1 stubs the patched engine (ui.PIC_CENTRE = true), which centres
-- native cells; without it the stubbed engine has the fixed (32,32) origin. Both
-- get the pack's native cells now: only the y correction differs. Files are read through io.open with absolute paths on
-- purpose: the mod runs inside the engine sandbox and reads through mod:read,
-- so the harness cannot use love.filesystem here without also testing itself.
local MOD_DIR = assert(os.getenv("MOD_DIR"), "MOD_DIR is not set")
local SAVE_MOD = os.getenv("SAVE_MOD") or "mods/bwsprites"
local BAD_DIR = os.getenv("BAD_DIR")
local CENTRED = os.getenv("CENTRED") == "1"
local NEWMENU = os.getenv("NEWMENU") == "1"
local MODE = (CENTRED and "centre-anchored engine (native cells)"
  or "fixed-origin engine (native cells)")
  .. (NEWMENU and ", new options menu" or ", legacy option group")

local failures = 0
local function check(ok, label, detail)
  if ok then
    print("PASS  " .. label)
  else
    failures = failures + 1
    print("FAIL  " .. label .. (detail and ("  (" .. tostring(detail) .. ")") or ""))
  end
end

local function join(root, rel)
  return (root:gsub("[/\\]+$", "") .. "/" .. rel)
end

local function readIn(root, rel)
  local fh, err = io.open(join(root, rel), "rb")
  if not fh then return nil, err or rel end
  local data = fh:read("*a")
  fh:close()
  return data
end

local function expectError(index, source, want, label)
  local ok, err = pcall(index.parseIndex, source, "@" .. label)
  check(not ok and tostring(err):find(want, 1, true) ~= nil, label,
    ok and "accepted" or tostring(err))
end

local function main()
  print("mode: " .. MODE)
  local indexSource, indexErr = readIn(MOD_DIR, "index.lua")
  check(indexSource ~= nil, "the mod directory has an index", indexErr)
  if not indexSource then return failures end

  local function read(rel) return readIn(MOD_DIR, rel) end
  -- LOVE resolves image paths against the game and save directories only, and
  -- the engine installs mods under the save directory, so the pack is addressed
  -- from the save root exactly as mod.assets:path does in the game.
  local function path(rel) return SAVE_MOD .. "/" .. rel end

  local source = read("pack.lua")
  local chunk, chunkErr = load(source, "@pack.lua")
  check(chunk ~= nil, "pack.lua parses", chunkErr)
  if not chunk then return failures end
  local Pack = chunk()
  check(type(Pack) == "table" and type(Pack.parseIndex) == "function", "pack.lua loads")

  local ok, index = pcall(Pack.parseIndex, indexSource, "@index.lua")
  check(ok, "the generated index parses", not ok and tostring(index))
  if not ok then return failures end
  check(index.pad == 0 and index.box == 64 and index.format == 2,
    "the index declares the format-2 native geometry",
    tostring(index.format) .. "/" .. tostring(index.box) .. "/" .. tostring(index.pad))
  check(index.entries[25] ~= nil and index.entries[25].frontFrames == 3,
    "entry 25 carries three front frames")
  check(index.entries[25].backFrames == 2, "entry 25 carries two back frames")

  local now = 0
  local runtime = Pack.newRuntime(index, {
    read = read,
    path = path,
    now = function() return now end,
  })
  check(runtime:serves(25, "front"), "runtime serves species 25 front")
  check(not runtime:serves(1, "front"), "runtime does not serve an absent species")

  local view, viewErr = runtime:view(25, "front", false)
  check(view ~= nil, "the front sheet opens", viewErr)
  if view then
    local w, h = view.data:getDimensions()
    local cell = 68
    check(view.size == 68, "the native cell side is read from the index", view.size)
    check(view.cell == cell, "this engine is served " .. cell .. " px cells", view.cell)
    check(w == cell and h == cell * 3, "the sheet holds three " .. cell .. " px cells", w .. "x" .. h)
    local at = function(ticks) return runtime:frameIndex(view, ticks / 60) end
    check(at(0) == 1 and at(9) == 1, "the first frame holds its duration")
    check(at(10) == 2 and at(19) == 2, "the second frame follows")
    check(at(20) == 3 and at(29) == 3, "the third frame follows")
    check(at(30) == 2 and at(45) == 3 and at(55) == 2,
      "the loop resumes at loopStart+1 instead of replaying the intro",
      ("%d/%d/%d"):format(at(30), at(45), at(55)))

    local first = runtime:frameImage(view, 1)
    local second = runtime:frameImage(view, 2)
    check(first ~= nil and second ~= nil and first ~= second, "each frame is its own image")
    if first then first:release() end
    if second then second:release() end

    -- The fixture ink ends on size-5, the pad the cartridge's own art keeps.
    local x, y = 34, 67
    local _, _, _, alpha = view.data:getPixel(x, y)
    check(alpha > 0.03, "the ink still ends on the cell's last row",
      ("%d,%d=%s"):format(x, y, tostring(alpha)))
  end

  local shiny = runtime:frame(25, "front", true, 0)
  check(shiny ~= nil and shiny.image ~= nil, "a missing shiny sheet falls back to the normal one")
  check(runtime:frame(26, "front", false, 0) == nil, "a missing sheet reports no frame")
  check(runtime:frame(27, "front", false, 0) == nil, "a wrongly sized sheet reports no frame")

  local wide = runtime:frame(28, "front", false, 0)
  local wideCell = 80
  check(wide ~= nil and wide.image:getWidth() == wideCell and wide.image:getHeight() == wideCell,
    "a wide cell is cut at its native size on a centring engine",
    wide and wide.image:getWidth() .. "x" .. wide.image:getHeight())
  check(runtime:frame(29, "front", false, 0) == nil,
    "a sheet narrower than its declared cell reports no frame")

  if BAD_DIR then
    local function bad(name)
      local data = readIn(BAD_DIR, name)
      return data or ""
    end
    expectError(Pack, bad("format1.lua"), "format 1 is not supported", "format1.lua")
    expectError(Pack, bad("box32.lua"), "cell box is 32", "box32.lua")
    expectError(Pack, bad("pad2.lua"), "cell pad is 2", "pad2.lua")
    expectError(Pack, bad("zeroframes.lua"), "has 0 frames", "zeroframes.lua")
    expectError(Pack, bad("shortdurations.lua"), "does not hold 3 durations", "shortdurations.lua")
    expectError(Pack, bad("loopout.lua"), "loopStart is 5", "loopout.lua")
    expectError(Pack, bad("zeroduration.lua"), "lasts 0 ticks", "zeroduration.lua")
    expectError(Pack, bad("nosheet.lua"), "has no sheet path", "nosheet.lua")
    expectError(Pack, bad("emptyentry.lua"), "neither a front nor a back view", "emptyentry.lua")
    expectError(Pack, bad("badkey.lua"), "is not a national dex number", "badkey.lua")
    expectError(Pack, bad("wide200.lua"), "cell size is 200", "wide200.lua")
    expectError(Pack, bad("size66.lua"), "cell size is 66", "size66.lua")
    expectError(Pack, bad("inkbig.lua"), "ink height is 65", "inkbig.lua")
    expectError(Pack, bad("dexout.lua"), "is outside the 64 px cell", "dexout.lua")
    expectError(Pack, bad("zerocolumns.lua"), "columns is 0", "zerocolumns.lua")
    expectError(Pack, "", "is empty", "empty.lua")
  end

  -- Engine integration: run the real main.lua against stub modules.
  local Pokemon = {
    national = function(sp) if sp == 1 or sp == 25 or sp == 28 or sp == 30 or sp == 31 or sp == 32 or sp == 33 then return sp end return nil end,
    speciesFromNational = function(nat) return nat end,
  }
  Pokemon.frontPic = function(sp, form, shiny) return { vanilla = "front", sp = sp } end
  Pokemon.backPic = function(sp, form, shiny) return { vanilla = "back", sp = sp } end
  Pokemon.frontSprite = Pokemon.frontPic
  Pokemon.picSpecies = function(sp) return sp end
  Pokemon.isShiny = function(m) return m.personality == 99 end
  Pokemon.gender = function(sp, personality) return personality == 77 and "F" or "M" end
  Pokemon.dexFrontPic = function(sp) return { vanilla = "dex", sp = sp } end
  local PicCoords = {
    front = { [25] = 8 },
    back = { [25] = 3 },
    elev = {},
    BATTLER_COORDS = {
      [false] = { [0] = { x = 72, y = 80 }, [1] = { x = 176, y = 40 } },
      [true] = { [0] = { x = 32, y = 80 }, [1] = { x = 200, y = 40 }, [3] = { x = 152, y = 32 } },
    },
  }
  local MonAnim = { framePic = function(sp, frame, shiny) return { vanilla = "frame", sp = sp } end }
  -- The battle intro slides the enemy by Anim.present('enemy').ox. The draw
  -- wrapper must stop clamping while that offset is non-zero.
  local BattleAnim = { ox = 0 }
  BattleAnim.present = function(_) return { ox = BattleAnim.ox } end

  package.preload["src.core.game3.pokemon"] = function() return Pokemon end
  package.preload["src.core.game3.battle.pic_coords"] = function() return PicCoords end
  package.preload["src.core.game3.mon_anim"] = function() return MonAnim end
  package.preload["src.core.game3.battle.anim"] = function() return BattleAnim end
  -- Some engine builds put a three-argument shim around Pokemon.backPic, which
  -- drops the battler the UI passes. Ui.sidePic is the live seam that still has
  -- it, so the stub must reproduce that shim to prove female backs survive it.
  local BattleUi = { PIC_CENTRE = CENTRED }
  BattleUi.sidePic = function(back, ignoreDeoxys, species, form, shiny, personality, mon)
    if back then return Pokemon.backPic(species, form, shiny) end
    return Pokemon.frontPic(species, form, shiny, personality, nil, mon)
  end
  package.preload["src.core.game3.battle.ui"] = function() return BattleUi end

  -- The option-menu seam: the engine's Rows table plus the loader's live copy
  -- of the saved mod options, which is what mod.options:get reads. The newer
  -- engine has no Rows module; a mod's rows stay loose in its OptionsMenu.
  local OptionRows = { build = function() return { { id = "textSpeed" } } end,
    GROUPS = {}, ORDER = {} }
  if not NEWMENU then
    package.preload["src.ui.game3.option_rows"] = function() return OptionRows end
  end

  local liveOptions = {}
  local game = { options = {}, mods = { modOptions = { bwsprites = liveOptions } } }
  local defined
  local options = {
    define = function(_, rows) defined = rows end,
    get = function(_, key) return liveOptions[key] or "bw" end,
  }
  local hookWrap
  local hooks = {
    wrap = function(_, event, fn)
      if event == "ui.options.rows" then hookWrap = fn end
      return true
    end,
  }

  local originalFront = Pokemon.frontPic
  local originalFramePic = MonAnim.framePic
  local originalGetTime = love.timer.getTime
  now = 0
  love.timer.getTime = function() return now end

  local logged = {}
  local function record(format, ...)
    local ok, line = pcall(string.format, format, ...)
    logged[#logged + 1] = ok and line or tostring(format)
  end
  local api = {
    id = "bwsprites",
    path = MOD_DIR,
    options = options,
    hooks = hooks,
    -- assets.path is what the game hands a mod: a save-relative pack path.
    read = function(_, rel) return read(rel) end,
    assets = { path = function(_, rel) return path(rel) end },
    log = {
      info = function(_, format, ...) record(format, ...) end,
      warn = function(_, format, ...) record(format, ...) end,
      error = function(_, format, ...) record(format, ...) end,
    },
  }
  -- Spy on the engine draw before the mod loads: on a fixed-origin engine the
  -- mod has to shift a wide picture left by half its extra width itself, so the
  -- spy is the only way to see whether the user's Charizard lands centred.
  local drawCalls = {}
  local engineDraw = love.graphics.draw
  love.graphics.draw = function(...)
    drawCalls[#drawCalls + 1] = { ... }
    return engineDraw(...)
  end

  local mainChunk = assert(load(read("main.lua"), "@main.lua"))
  local mainOK, mainErr = pcall(mainChunk, api)
  check(mainOK, "main.lua initializes against the stubs", mainErr)

  check(Pokemon.frontPic ~= originalFront, "frontPic is wrapped")
  check(Pokemon.frontSprite == Pokemon.frontPic, "the frontSprite alias is re-pointed")

  local frontCell = 68
  local entry = Pokemon.frontPic(25, 0, false)
  check(type(entry) == "table" and entry.image ~= nil
    and entry.w == frontCell and entry.h == frontCell and entry.vanilla == nil,
    "a served species gets its own cell side", type(entry))
  local fallback = Pokemon.frontPic(1, 0, false)
  check(type(fallback) == "table" and fallback.vanilla == "front",
    "an unserved species keeps the cartridge picture")
  local unknown = Pokemon.frontPic(7, 0, false)
  check(type(unknown) == "table" and unknown.vanilla == "front",
    "an unknown species falls back cleanly")

  -- Native cells are taller than the cartridge's 64px art, so the per-species
  -- y offset grows by (64-Size)/2. Entry 28 is an 80px cell whose 72px ink is
  -- already at the scene top, so a centring engine clamps it by another 8px.
  local wantFront = CENTRED and -2 or -4
  local wantWide = CENTRED and -8 or -16
  check(PicCoords.front[25] == wantFront, "the front offset carries the cell correction",
    PicCoords.front[25])
  check(PicCoords.back[25] == 0, "the 64px back cell zeroes the cartridge offset", PicCoords.back[25])
  check(PicCoords.front[28] == wantWide,
    "a tall enemy cell is corrected and clamped on screen", PicCoords.front[28])
  -- A 96px own back starts at scene row 20 with the plain anchor, under the
  -- enemy healthbox (rows 14..45). The mod slides it down to row 46; the cell
  -- size does not change, so the feet run behind the battle menu.
  local wantBack = CENTRED and 10 or -6
  check(PicCoords.back[33] == wantBack,
    "a tall own back starts below the enemy healthbox", PicCoords.back[33])

  -- Option menu: two independent rows in one group. Each step must reach the
  -- picture wrapper at once (not only after a boot) and put the cartridge's
  -- per-species y padding back when a side returns to DEFAULT.
  -- The engine hands rows to the ui.options.rows hook; here the vanilla list
  -- is empty, so whatever comes back is exactly what this mod contributes.
  local function hookedRows()
    local rows = {}
    if hookWrap then
      local out = hookWrap(function(_, r) return r end, game, rows)
      if type(out) == "table" then rows = out end
    end
    return rows
  end
  local function rowById(id)
    for _, row in ipairs(hookedRows()) do
      if row.id == id then return row end
    end
  end
  local playerRow, foeRow
  if NEWMENU then
    playerRow, foeRow = rowById("bwPlayer"), rowById("bwFoe")
  else
    local group
    for _, g in ipairs(OptionRows.GROUPS) do
      if g.id == "group.bwSprites" then group = g end
    end
    check(group ~= nil and #group.members == 2, "the BW SPRITES group is registered")
    -- Engine builds without the ui.options.rows hook still get the rows: the
    -- mod wraps Rows.build as well.
    local built = OptionRows.build({})
    local builtIds = {}
    for _, row in ipairs(built or {}) do builtIds[row.id] = true end
    check(builtIds.bwPlayer and builtIds.bwFoe,
      "the old OPTION builder sees both switches without the rows hook")
    playerRow, foeRow = rowById("bwPlayer"), rowById("bwFoe")
  end
  check(playerRow ~= nil and playerRow.label == "MY POKEMON" and playerRow.value(game) == "BW",
    "MY POKEMON reads BW by default")
  check(foeRow ~= nil and foeRow.label == "OPPONENT" and foeRow.value(game) == "BW",
    "OPPONENT reads BW by default")

  playerRow.step(game, 1)
  check(liveOptions.playerSprites == "default"
    and game.options.modOptions.bwsprites.playerSprites == "default",
    "a step persists into the engine options tree and the live copy",
    tostring(liveOptions.playerSprites))
  check(PicCoords.back[25] == 3 and PicCoords.front[25] == wantFront,
    "MY POKEMON at DEFAULT restores only the back y offset",
    PicCoords.front[25] .. "/" .. PicCoords.back[25])
  check(Pokemon.backPic(25, 0, false).vanilla == "back",
    "MY POKEMON at DEFAULT serves the cartridge back picture")
  check(Pokemon.frontPic(25, 0, false).vanilla == nil,
    "switching MY POKEMON leaves the opponent side on BW")
  playerRow.step(game, 1)
  check(PicCoords.back[25] == 0 and Pokemon.backPic(25, 0, false).vanilla == nil,
    "switching MY POKEMON back to BW serves the pack again")

  foeRow.step(game, 1)
  check(PicCoords.front[25] == 8 and Pokemon.frontPic(25, 0, false).vanilla == "front",
    "OPPONENT at DEFAULT restores the cartridge picture and its offset")
  check(MonAnim.framePic(25, 1, false) ~= nil,
    "mon_anim.framePic answers again once the opponent side is on DEFAULT")
  foeRow.step(game, 1)
  check(PicCoords.front[25] == wantFront and MonAnim.framePic(25, 1, false) == nil,
    "switching OPPONENT back to BW re-pins it")
  foeRow.step(game, 1)
  check(Pokemon.dexFrontPic(25, 0).vanilla == "dex",
    "OPPONENT at DEFAULT restores the cartridge Pokedex picture")
  foeRow.step(game, 1)

  local animated = entry.image
  now = 10 / 60
  local nextFrame = Pokemon.frontPic(25, 0, false)
  check(nextFrame ~= nil and nextFrame.image ~= nil and nextFrame.image ~= animated,
    "the returned picture changes with the animation clock")

  local back = Pokemon.backPic(25, 0, false)
  check(back ~= nil and back.image ~= nil and back.w == 64 and back.h == 64,
    "the 64px back picture is served from the pack")

  -- A native back cell wider than the 64px host picture has to be drawn left
  -- of the engine's (32,32) origin by (w-64)/2, or the player's Pokemon sits
  -- right of its base. PIC_CENTRE engines already do this in the engine.
  local wideBack = Pokemon.backPic(30, 0, false)
  check(wideBack ~= nil and wideBack.image ~= nil and wideBack.w == 80 and wideBack.h == 80,
    "a native back cell wider than 64 is served at its own width",
    wideBack and (wideBack.w .. "x" .. wideBack.h))
  drawCalls = {}
  love.graphics.draw(wideBack.image, 100, 100, 0, 1, 1, 32, 32)
  check(#drawCalls == 1, "the engine draw is reached once", #drawCalls)
  local wantX = CENTRED and 100 or 84
  check(drawCalls[1] ~= nil and drawCalls[1][2] == wantX,
    "a wide back picture is centred on a " .. (CENTRED and "centring" or "fixed-origin") .. " engine",
    drawCalls[1] and drawCalls[1][2])
  -- Near the left edge the extra back shift has to stop with a one-pixel margin
  -- instead of pushing the cell off screen: base shift -8, room for 4 more.
  drawCalls = {}
  love.graphics.draw(wideBack.image, 45, 100, 0, 1, 1, 32, 32)
  local wantEdge = CENTRED and 45 or 33
  check(drawCalls[1] ~= nil and drawCalls[1][2] == wantEdge,
    "the extra back shift stops one pixel from the screen edge",
    drawCalls[1] and drawCalls[1][2])
  -- The opponent's native 72px front is wider than the 64px host picture, so
  -- the same fixed-origin correction has to reach the enemy draw call.
  local wideFront = Pokemon.frontPic(31, 0, false)
  check(wideFront ~= nil and wideFront.image ~= nil and wideFront.w == 72 and wideFront.h == 72,
    "a native opponent front is served at the full 72px slot",
    wideFront and (wideFront.w .. "x" .. wideFront.h))
  drawCalls = {}
  love.graphics.draw(wideFront.image, 100, 100, 0, 1, 1, 32, 32)
  check(#drawCalls == 1, "the engine draw is reached once for a wide front", #drawCalls)
  local wantFrontX = CENTRED and 100 or 96
  check(drawCalls[1] ~= nil and drawCalls[1][2] == wantFrontX,
    "a wide opponent front is centred on a " .. (CENTRED and "centring" or "fixed-origin") .. " engine",
    drawCalls[1] and drawCalls[1][2])
  -- A 120px front is the widest the enemy slot offers; it still has to centre.
  local widestFront = Pokemon.frontPic(32, 0, false)
  check(widestFront ~= nil and widestFront.image ~= nil
    and widestFront.w == 120 and widestFront.h == 120,
    "the widest opponent front keeps its 120px cell",
    widestFront and (widestFront.w .. "x" .. widestFront.h))
  drawCalls = {}
  love.graphics.draw(widestFront.image, 100, 100, 0, 1, 1, 32, 32)
  local wantWidestX = CENTRED and 100 or 72
  check(drawCalls[1] ~= nil and drawCalls[1][2] == wantWidestX,
    "the widest opponent front is centred on a " .. (CENTRED and "centring" or "fixed-origin") .. " engine",
    drawCalls[1] and drawCalls[1][2])
  -- The battle intro moves the battler to x=-240 before sliding it in. The mod
  -- must not pull that picture back on screen, or the opponent pops in whole.
  if not CENTRED then
    BattleAnim.ox = -240
    drawCalls = {}
    love.graphics.draw(widestFront.image, -64, 100, 0, 1, 1, 32, 32)
    check(drawCalls[1] ~= nil and drawCalls[1][2] == -92,
      "an intro-sliding front keeps the engine's off-screen position",
      drawCalls[1] and drawCalls[1][2])
    BattleAnim.ox = 0
    drawCalls = {}
    love.graphics.draw(widestFront.image, 176, 100, 0, 1, 1, 32, 32)
    check(drawCalls[1] ~= nil and drawCalls[1][2] == 148,
      "a resting front is centred on the battler",
      drawCalls[1] and drawCalls[1][2])
    drawCalls = {}
    love.graphics.draw(widestFront.image, 230, 100, 0, 1, 1, 32, 32)
    check(drawCalls[1] ~= nil and drawCalls[1][2] == 151,
      "a resting front still stops at the scene's right edge",
      drawCalls[1] and drawCalls[1][2])
  end
  now = 0
  local dex = Pokemon.dexFrontPic(25, 0)
  check(dex ~= nil and dex.w == 64 and dex.h == 64 and dex.vanilla == nil
    and dex.image ~= nil and dex.image:getWidth() == 64 and dex.image:getHeight() == 64,
    "the Pokedex gets the pack frame fitted into its 64x64 box", dex)
  if dex and dex.image then
    -- LÖVE 11 images cannot be read back directly; a canvas can.
    local canvas = love.graphics.newCanvas(64, 64, { dpiscale = 1 })
    canvas:renderTo(function()
      love.graphics.clear(0, 0, 0, 0)
      love.graphics.setBlendMode("replace")
      love.graphics.setColor(1, 1, 1, 1)
      love.graphics.draw(dex.image, 0, 0)
    end)
    local data = canvas:newImageData()
    canvas:release()
    local minX, minY, maxX, maxY = 64, 64, -1, -1
    for y = 0, 63 do
      for x = 0, 63 do
        local _, _, _, a = data:getPixel(x, y)
        if a > 0 then
          if x < minX then minX = x end
          if y < minY then minY = y end
          if x > maxX then maxX = x end
          if y > maxY then maxY = y end
        end
      end
    end
    data:release()
    local w, h = maxX - minX + 1, maxY - minY + 1
    check(minX == 20 and minY == 22 and w == 24 and h == 20,
      "the dex picture fits the largest-pose window, not a shrunken whole cell",
      ("%d,%d %dx%d"):format(minX, minY, w, h))
  end
  now = 10 / 60
  local dexNext = Pokemon.dexFrontPic(25, 0)
  check(dexNext ~= nil and dexNext.image ~= nil and dex.image ~= nil
    and dexNext.image ~= dex.image,
    "the Pokedex keeps animating inside the fixed largest-pose window")
  check(Pokemon.dexFrontPic(1, 0).vanilla == "dex",
    "the Pokedex keeps the cartridge picture for an absent species")

  -- A distinct female graphic wins when the battler says so; a male battler or
  -- a species without female art keeps the male view.
  local maleFront = Pokemon.frontPic(25, 0, false)
  local femaleFront = Pokemon.frontPic(25, 0, false, 42, nil, { gender = "F" })
  check(maleFront ~= nil and femaleFront ~= nil and femaleFront.w == 64
    and femaleFront.image ~= maleFront.image,
    "a female battler gets the female front at its own cell size",
    femaleFront and (femaleFront.w .. "x" .. femaleFront.h))
  local maleAgain = Pokemon.frontPic(25, 0, false, 42, nil, { gender = "M" })
  check(maleAgain ~= nil and maleAgain.w == 68,
    "a male battler keeps the male front cell", maleAgain and (maleAgain.w .. "x" .. maleAgain.h))
  local byPersonality = Pokemon.frontPic(25, 0, false, 77, nil, nil)
  check(byPersonality ~= nil and byPersonality.w == 64 and byPersonality.image ~= maleAgain.image,
    "the personality decides gender when the battler has none",
    byPersonality and (byPersonality.w .. "x" .. byPersonality.h))
  local femaleBack = Pokemon.backPic(25, 0, false, nil, { gender = "F" })
  local maleBack = Pokemon.backPic(25, 0, false, nil, { gender = "M" })
  check(femaleBack ~= nil and maleBack ~= nil and femaleBack.w == 64
    and femaleBack.image ~= maleBack.image,
    "a female battler gets the female back picture")
  -- Through the engine's battler draw the backPic shim drops the mon; the
  -- mod remembers it from Ui.sidePic so the female view still wins.
  local uiFemaleBack = BattleUi.sidePic(true, false, 25, 0, false, nil, { gender = "F" })
  local uiMaleBack = BattleUi.sidePic(true, false, 25, 0, false, nil, { gender = "M" })
  check(uiFemaleBack ~= nil and uiMaleBack ~= nil and uiFemaleBack.image ~= uiMaleBack.image,
    "a female back survives the engine's three-argument backPic shim")
  local noFemale = Pokemon.frontPic(31, 0, false, 42, nil, { gender = "F" })
  check(noFemale ~= nil and noFemale.w == 72,
    "a species without female art falls back to its male view",
    noFemale and (noFemale.w .. "x" .. noFemale.h))

  check(MonAnim.framePic(25, 1, false) == nil, "mon_anim.framePic stays out of the way for served species")
  check(MonAnim.framePic(1, 1, false) ~= nil, "mon_anim.framePic still answers for unserved species")

  local loggedMode = false
  for _, line in ipairs(logged) do
    if line:find("native cells", 1, true) then loggedMode = true end
  end
  check(loggedMode, "the startup log names the cell mode the engine gets")

  love.timer.getTime = originalGetTime
  love.graphics.draw = engineDraw
  Pokemon.frontPic = originalFront
  MonAnim.framePic = originalFramePic

  if #logged > 0 and os.getenv("VERBOSE_LOG") then
    print("logged:")
    for _, line in ipairs(logged) do print("  " .. line) end
  end
  return failures
end

function love.load()
  local ok, err = pcall(main)
  if not ok then
    print("FAIL  harness raised: " .. tostring(err))
    failures = failures + 1
  end
  print(failures == 0 and "selftest: OK" or ("selftest: " .. failures .. " failure(s)"))
  love.event.quit(failures == 0 and 0 or 1)
end
