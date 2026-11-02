-- bwsprites: swaps the Gen 3 battle pictures for animated Black/White sprites.
--
-- Geometry. The pack keeps facets' ground rule: the ink is centred on the column
-- and its lowest row is the cell's last row. The cell is the animation's own
-- size: nothing is enlarged and nothing is scaled down, so an enemy front and
-- the player's back both keep the pixels Black/White (or Black 2/White 2)
-- authored. The mod zeroes the species' y padding and substitutes the anchor
-- correction, and on a fixed-origin engine shifts a wide cell left by half its
-- extra width. Nothing is scaled at load. See docs/MEASUREMENTS.md and
-- docs/FORMAT.md.
local mod = ...

local function loadPart(relative)
  local source = mod:read(relative)
  if type(source) ~= "string" or source == "" then
    return nil, relative .. " is missing from the mod"
  end
  local chunk, err = load(source, "@" .. mod.path .. "/" .. relative)
  if not chunk then return nil, err end
  local ok, part = pcall(chunk)
  if not ok then return nil, part end
  if type(part) ~= "table" then return nil, relative .. " did not return a table" end
  return part
end

local Pack, packErr = loadPart("pack.lua")
if not Pack then
  mod.log:error("bwsprites: %s", tostring(packErr))
  return
end

-- Two builds share this file. The BW2 build ships an embedded index.lua and
-- PNGs; the asset-free BW build ships neither and reads the launcher's own
-- gen5_bw/battle_sprites pack instead. A missing index is therefore not an
-- error until the imported pack is also unavailable.
local indexPath = "index.lua"
local indexSource = mod:read(indexPath)
local runtime, index, backend
if type(indexSource) == "string" and indexSource ~= "" then
  local okIndex, parsed = pcall(Pack.parseIndex, indexSource, "@" .. mod.path .. "/" .. indexPath)
  if not okIndex then
    mod.log:error("bwsprites: %s", tostring(parsed))
    return
  end
  index = parsed
  runtime = Pack.newRuntime(index, {
    read = function(relative) return mod:read(relative) end,
    path = function(relative) return mod.assets:path(relative) end,
  })
  backend = "embedded"
else
  local External, externalErr = loadPart("external.lua")
  if not External then
    mod.log:error("bwsprites: the asset-free reader is missing: %s", tostring(externalErr))
    return
  end
  local okExternal, external, externalNewErr = pcall(External.new, Pack, mod)
  if not okExternal then
    mod.log:error("bwsprites: the imported Black/White pack could not be read: %s", tostring(external))
    return
  end
  if not external then
    mod.log:error("bwsprites: this build has no sprite pack and no launcher import: %s", tostring(externalNewErr))
    return
  end
  index, runtime = external.index, external.runtime
  backend = "imported"
  if external.capped and external.capped > 0 then
    mod.log:warn("bwsprites: %d imported views have no part tracks and play the 240 tick prefix", external.capped)
  end
end

-- PIC_CENTRE is the patched engine's promise that every picture is drawn
-- around its centre. Without it the battle uses a constant (32,32) origin, so a
-- back cell wider than 64 needs the draw wrapper below to land centred.
local function engineCentres()
  local Ui = package.loaded["src.core.game3.battle.ui"]
  if type(Ui) ~= "table" then
    local ok, loaded = pcall(require, "src.core.game3.battle.ui")
    Ui = ok and loaded or nil
  end
  return type(Ui) == "table" and Ui.PIC_CENTRE == true
end
local CENTRED = engineCentres()

-- The Pokedex card's picture area: the space right of the species stats and
-- above the flavour-text divider (the card's upper section is rows 19..87).
-- That screen draws its picture with a bare love.graphics.draw(pic.image, 152,
-- 24) at the top-left of the card's 64x64 slot; the pack serves the animation's
-- whole pose window at 1:1 instead, and the draw wrapper below centres it here.
-- The picture is never scaled and never cut: a window larger than the area
-- overhangs it, and the placement is clamped to the screen so no pixel is lost.
local DEX = { x = 124, y = 20, w = 112, h = 68 }
local SCREEN_W, SCREEN_H = 240, 160

-- placeDex centres a w x h picture in the card's picture area and keeps it
-- inside the screen: horizontally centred in the area, vertically centred while
-- it fits and otherwise standing on the divider row, then clamped so the whole
-- picture stays on screen.
local function placeDex(w, h)
  local x = DEX.x + (DEX.w - w) / 2
  local y
  if h <= DEX.h then
    y = DEX.y + (DEX.h - h) / 2
  else
    y = DEX.y + DEX.h - h
  end
  x = math.min(math.max(x, 0), math.max(0, SCREEN_W - w))
  y = math.min(math.max(y, 0), math.max(0, SCREEN_H - h))
  return math.floor(x + 0.5), math.floor(y + 0.5)
end

-- A fixed-origin engine starts a picture at (x-32, y-32), so a native cell
-- wider than 64 would sit right of the battler and PicCoords has no x to correct.
-- Wrap the one draw call the battle uses: when one of our pictures is drawn at
-- that origin, move it left by (size-64)/2 so the cell centre lands on the
-- battler. The player's back is shifted once more by its overhang, so its right
-- edge stays on the cartridge cell's right edge; both sides are then kept inside
-- the 240 px scene (a native Lugia front is 156 px wide and would otherwise lose
-- 15 px off the right edge). Images are marked in serve(); 64 px cells and every
-- other picture pass through untouched. A centre-anchored engine already places
-- native cells by their centre. Backs are up to 112 px, fronts up to 160.
--
-- The scene/overhang corrections only describe a picture at rest. During the
-- battle intro (and a move or faint) the engine slides the battler by
-- Anim.present(...).ox; clamping then would pin the picture on screen and the
-- slide would start fully visible. When the engine is moving the battler, only
-- the centring shift applies and the engine keeps control of the position.
local picMark = setmetatable({}, { __mode = "k" })
local SCENE_W = SCREEN_W
local battleAnim
local function presentOffset(side)
  if battleAnim == nil then
    local ok, loaded = pcall(require, "src.core.game3.battle.anim")
    battleAnim = (ok and type(loaded) == "table" and loaded) or false
  end
  if not battleAnim then return 0 end
  local ok, p = pcall(battleAnim.present, side == "back" and "player" or "enemy")
  if not ok or type(p) ~= "table" then return 0 end
  return tonumber(p.ox) or 0
end
local engineDraw = love.graphics.draw
love.graphics.draw = function(drawable, x, y, r, sx, sy, ox, oy, kx, ky)
  if type(drawable) == "userdata" then
    local mark = picMark[drawable]
    if mark and mark.dex then
      -- The dex card's picture area is the pack's own placement. The size
      -- comparison on the dex's second page draws the same picture around its
      -- 64x64 centre (origin 32,32) beside the trainer silhouette, so that one
      -- call gets a 64x64 fit instead and keeps the cartridge's proportions.
      if ox == 32 and oy == 32 then
        local alt = mark.alt and mark.alt()
        if alt and alt.image then
          return engineDraw(alt.image, x, y, r, sx, sy, ox, oy, kx, ky)
        end
      elseif ox == nil and oy == nil then
        local px, py = placeDex(mark.w or Pack.BOX, mark.h or Pack.BOX)
        return engineDraw(drawable, px, py, r, sx, sy, ox, oy, kx, ky)
      end
    elseif mark and not CENTRED and ox == 32 and oy == 32 and (mark.w or 0) > Pack.BOX then
      local width = mark.w
      local scale = sx or 1
      local shift = (Pack.BOX - width) * scale / 2
      local moving = presentOffset(mark.side) ~= 0
      if mark.side == "back" then
        if not moving then
          -- Cell left with only the centring shift; leave the last pixel free.
          local left = x - (width * scale) / 2
          shift = shift - math.min((width - Pack.BOX) * scale / 2,
            math.max(0, left - 1))
        end
      elseif not moving then
        -- A native front wider than the enemy base's room is slid left until
        -- its right edge is the scene's last column; never let it leave the
        -- left edge either.
        local left = x - 32 * scale + shift
        local right = left + width * scale
        if right > SCENE_W - 1 then
          shift = shift - (right - (SCENE_W - 1))
          left = x - 32 * scale + shift
        end
        if left < 1 then shift = shift + (1 - left) end
      end
      return engineDraw(drawable, x + shift, y, r, sx, sy, ox, oy, kx, ky)
    end
  end
  return engineDraw(drawable, x, y, r, sx, sy, ox, oy, kx, ky)
end

local okP, Pokemon = pcall(require, "src.core.game3.pokemon")
if not okP or type(Pokemon) ~= "table" then
  mod.log:error("bwsprites: the Gen 3 Pokemon module is unavailable; %s", tostring(Pokemon))
  return
end
local okC, PicCoords = pcall(require, "src.core.game3.battle.pic_coords")
if not okC or type(PicCoords) ~= "table" then
  PicCoords = nil
end

local function national(species)
  local sp = tonumber(species)
  if not sp or sp < 1 then return nil end
  local ok, nat = pcall(Pokemon.national, sp)
  if ok and type(nat) == "number" and nat >= 1 then return nat end
  return nil
end

-- ---- settings: BW art or the cartridge's own, per side --------------------
--
-- frontPic is the OPPONENT's picture and backPic is YOUR OWN, so the two rows
-- are per side. Both default to BW so an install with no saved choice swaps
-- everything, which is what the mod is for.
local SWITCH_VALUES = { "bw", "default" }
local SWITCH_LABEL = { bw = "BW", default = "DEFAULT" }
local SETTINGS = {
  { key = "playerSprites", id = "bwPlayer", label = "MY POKEMON", side = "back" },
  { key = "foeSprites", id = "bwFoe", label = "OPPONENT", side = "front" },
}

local settingsOk, settingsErr = pcall(function()
  local rows = {}
  for _, setting in ipairs(SETTINGS) do
    local choices = {}
    for i, value in ipairs(SWITCH_VALUES) do
      choices[i] = { SWITCH_LABEL[value], value }
    end
    rows[#rows + 1] = {
      key = setting.key,
      label = setting.label,
      type = "choice",
      default = "bw",
      choices = choices,
    }
  end
  mod.options:define(rows)
end)
if not settingsOk then
  mod.log:warn("bwsprites: settings are unavailable (%s); serving BW sprites on both sides",
    tostring(settingsErr))
end

-- The loader's mod.options:get reads the options table it loaded at boot; a
-- menu step writes the live game options table (the one SaveData persists), so
-- keep the chosen values here too and consult them first. Without this the new
-- row would only take effect after a restart on engines where the two tables
-- are not the same object.
local current = {}

local function valid(value)
  return value == "bw" or value == "default"
end

local function mode(key)
  local chosen = current[key]
  if valid(chosen) then return chosen end
  local ok, value = pcall(function() return mod.options:get(key) end)
  if ok and valid(value) then return value end
  return "bw"
end

-- side is "front" (opponent, frontPic) or "back" (own, backPic).
local function sideEnabled(side)
  return mode(side == "back" and "playerSprites" or "foeSprites") ~= "default"
end

-- The engine positions a picture by adding a per-species y padding
-- (PicCoords.front/back) to the battler's base point, authored for the
-- cartridge's 64x64 art. This pack keeps native cells but targets that same
-- ground row: the mod zeroes the padding and substitutes the cell correction,
-- (64-Size)/2 on a centring engine or (64-Size) on the fixed origin. A tall
-- enemy cell is slid down until its ink top stays on screen; a tall own back is
-- moved below the enemy healthbox (OWN_TOP_CLEAR).
-- The single battle's enemy base row (ui.lua ENEMY_MON). A cell no taller than
-- the 72 px slot between row 0 and the enemy ground row (71) needs no clamp;
-- this value only decides where a legacy taller cell stops at the scene top.
local ENEMY_TOP = 40
local PLAYER_TOP = 80
if PicCoords and type(PicCoords.battlerCoords) == "function" then
  local ok, base = pcall(PicCoords.battlerCoords, false, 1)
  if ok and type(base) == "table" and tonumber(base.y) then ENEMY_TOP = base.y end
  ok, base = pcall(PicCoords.battlerCoords, false, 0)
  if ok and type(base) == "table" and tonumber(base.y) then PLAYER_TOP = base.y end
end
-- The enemy healthbox is drawn on top of the player's own picture. In singles
-- its 128x32 sheet is placed at (44-32, 30-16), i.e. scene rows 14..45
-- (healthbox.lua ENEMY_CENTER + enemy_top_left, battle_chrome.lua drawEnemyBox),
-- so row 46 is the first clear row. A tall native back has to start below it;
-- the user allows the feet to run behind the battle menu instead.
local OWN_TOP_CLEAR = 46

local offsets, savedOffsets = {}, {}
local corrections = { front = {}, back = {} }
if PicCoords then
  for _, field in ipairs({ "front", "back" }) do
    if type(PicCoords[field]) == "table" then
      offsets[field] = PicCoords[field]
      local copy = {}
      for sp, yo in pairs(PicCoords[field]) do copy[sp] = yo end
      savedOffsets[field] = copy
    end
  end
end

-- prefix names the index view: "front"/"back" for the male art, and
-- "frontFemale"/"backFemale" when the dump has a distinct female graphic.
local function viewPrefix(side, female)
  return female and (side .. "Female") or side
end

local function offsetFor(side, prefix, entry, size, original, sp)
  -- The pack's cell is the whole picture: its ink bottom row is size-1, so the
  -- species' cartridge padding (original) must not be added again. Zero it at
  -- 64 px, which puts feet on the same shared row facets uses; a wider legacy
  -- cell needs the anchor correction only.
  local total
  if CENTRED then
    total = math.floor((Pack.BOX - size) / 2)
  else
    total = Pack.BOX - size
  end
  local inkH = tonumber(entry[prefix .. "InkH"]) or 0
  local half = CENTRED and size / 2 or Pack.BOX / 2
  if side ~= "front" then
    -- Keep the top of the own picture below the enemy healthbox. Only moves the
    -- cell; size and pixels are untouched, and the feet just go behind the menu.
    local top = PLAYER_TOP + total + 4 - half + (size - Pack.PAD - inkH)
    if top < OWN_TOP_CLEAR then total = total + (OWN_TOP_CLEAR - top) end
    return math.floor(total)
  end
  local elev = 0
  if PicCoords and type(PicCoords.elev) == "table" then
    elev = tonumber(PicCoords.elev[sp]) or 0
  end
  local top = ENEMY_TOP + total - elev - half + (size - Pack.PAD - inkH)
  if top < 0 then total = total - top end
  local bottom = ENEMY_TOP + total - elev - half + (size - Pack.PAD) - 1
  if bottom > SCREEN_H - 1 then total = total - (bottom - (SCREEN_H - 1)) end
  return math.floor(total)
end

local served = {}
for nat, entry in pairs(index.entries) do
  local ok, sp = pcall(Pokemon.speciesFromNational, nat)
  if ok and type(sp) == "number" and sp >= 1 then
    served[#served + 1] = { sp = sp, nat = nat }
    for _, side in ipairs({ "front", "back" }) do
      local size = entry[side .. "Size"]
      if entry[side .. "Frames"] and size then
        local original = 0
        if PicCoords and type(PicCoords[side]) == "table" then
          original = tonumber(PicCoords[side][sp]) or 0
        end
        corrections[side][sp] = offsetFor(side, side, entry, size, original, sp)
      end
    end
  end
end

local function applyOffset(side, pinned)
  local table_, original = offsets[side], savedOffsets[side]
  if not table_ or not original then return end
  if pinned then
    for sp, total in pairs(corrections[side]) do table_[sp] = total end
  else
    for sp in pairs(table_) do table_[sp] = nil end
    for sp, yo in pairs(original) do table_[sp] = yo end
  end
end

local function applyModes()
  for _, setting in ipairs(SETTINGS) do
    applyOffset(setting.side, sideEnabled(setting.side))
  end
end

-- The battle passes the battler as the last argument (frontPic: personality,
-- nil, mon; backPic: nil, mon). mon.gender is "M", "F" or "U"; when it is
-- missing, the personality decides through Pokemon.gender, the engine's own
-- rule. The Pokedex has no gender and keeps the male view.
-- Some engine builds wrap Pokemon.backPic with a three-argument shim that
-- drops the battler, so a back picture can never ask for its own gender. The
-- battler draw still passes the mon to Ui.sidePic; remembering it there and
-- reading it back here keeps female backs selectable without depending on that
-- shim's argument list.
local pendingFemale = nil

local function monIsFemale(mon)
  if type(mon) ~= "table" then return nil end
  local gender = mon.gender
  if gender == "F" or gender == "female" then return true end
  if gender == "M" or gender == "male" then return false end
  return nil
end

local function isFemale(species, ...)
  local args = { ... }
  local mon = type(args[3]) == "table" and args[3] or (type(args[2]) == "table" and args[2] or nil)
  local personality = args[1]
  if mon then
    local female = monIsFemale(mon)
    if female ~= nil then return female end
    if type(mon.personality) == "number" then personality = mon.personality end
  end
  if type(personality) == "number" and type(Pokemon.gender) == "function" then
    local ok, gender = pcall(Pokemon.gender, species, personality)
    return ok and (gender == "F" or gender == "female")
  end
  if pendingFemale ~= nil then return pendingFemale end
  return false
end

local function serve(side, species, shiny, female)
  if not sideEnabled(side) then return nil end
  local nat = national(species)
  if not nat or not runtime:serves(nat, side, female) then return nil end
  -- A later table rebuild would forget the whole-table swap above; pinning the
  -- one species being drawn keeps its feet on the ground either way. A female
  -- cell can be a different size, so its padding is computed for that view.
  local sp, table_ = tonumber(species), offsets[side]
  local entryIndex = index.entries[nat]
  local prefix = viewPrefix(side, female)
  local size = entryIndex and entryIndex[prefix .. "Size"]
  local total = sp and (female and size and offsetFor(side, prefix, entryIndex, size,
    (savedOffsets[side] and savedOffsets[side][sp]) or 0, sp) or corrections[side][sp]) or nil
  if table_ and total ~= nil and table_[sp] ~= total then table_[sp] = total end
  local now = love.timer and love.timer.getTime and love.timer.getTime() or os.clock()
  local entry, err = runtime:frame(nat, side, shiny, now, female)
  if not entry and err then
    mod.log:warn("bwsprites: %s", tostring(err))
  end
  if entry and entry.w > Pack.BOX then
    picMark[entry.image] = { w = entry.w, side = side }
  end
  return entry
end

local function wrapPic(name, side)
  local original = Pokemon[name]
  if type(original) ~= "function" then return end
  Pokemon[name] = function(species, form, shiny, ...)
    local entry = serve(side, species, shiny, isFemale(species, ...))
    if entry then return entry end
    return original(species, form, shiny, ...)
  end
end

wrapPic("frontPic", "front")
wrapPic("backPic", "back")
-- pokemon.lua points frontSprite at frontPic when it loads; re-point it so
-- every caller of the alias sees the wrapper too.
if type(Pokemon.frontPic) == "function" then
  Pokemon.frontSprite = Pokemon.frontPic
end

-- Battler draws go through Ui.sidePic, which still has the mon. Stash its
-- gender across the nested Pokemon.backPic call; a build without this module
-- simply keeps the three-argument path.
do
  local ok, Ui = pcall(require, "src.core.game3.battle.ui")
  if ok and type(Ui) == "table" and type(Ui.sidePic) == "function" then
    local original = Ui.sidePic
    Ui.sidePic = function(back, ignoreDeoxys, species, form, shiny, personality, mon)
      local previous = pendingFemale
      pendingFemale = monIsFemale(mon)
      if pendingFemale == nil and type(personality) == "number" then
        pendingFemale = isFemale(species, personality)
      end
      local called, entry, extra = pcall(original, back, ignoreDeoxys, species, form, shiny, personality, mon)
      pendingFemale = previous
      if not called then error(entry, 0) end
      return entry, extra
    end
  end
end

-- During a move the engine replaces the front picture with one frame of the
-- cart's own animation sheet (mon_anim.framePic, ui.lua:2246). Returning nil
-- for served species makes it fall back to our animated entry instead of
-- flashing the cartridge's static art.
local okM, MonAnim = pcall(require, "src.core.game3.mon_anim")
if okM and type(MonAnim) == "table" and type(MonAnim.framePic) == "function" then
  local original = MonAnim.framePic
  MonAnim.framePic = function(species, frame, shiny)
    local nat = national(species)
    if sideEnabled("front") and nat and runtime:serves(nat, "front") then return nil end
    return original(species, frame, shiny)
  end
end

-- The Pokedex asks for its own front picture (dexFrontPic) and lays it out at
-- (152,24) in the card's 64x64 slot. Hand it the animation's pose window at its
-- own size and let the draw wrapper centre it in the card's picture area; battle
-- pictures keep their native cell. A caller that names another kind (the credits
-- roll passes "credits") gets a 64x64 fit for its own grid.
if type(Pokemon.dexFrontPic) == "function" then
  local original = Pokemon.dexFrontPic
  local function dexShiny(personality)
    local p = (tonumber(personality) or 0) % 4294967296
    local ok, shiny = pcall(Pokemon.isShiny, { personality = p, otId = 8, otSecretId = 0 })
    return ok and shiny == true
  end
  Pokemon.dexFrontPic = function(species, personality, kind)
    if sideEnabled("front") then
      local sp = species
      if type(Pokemon.picSpecies) == "function" then
        local ok, picked = pcall(Pokemon.picSpecies, species, personality)
        if ok and tonumber(picked) then sp = picked end
      end
      local nat = national(sp)
      if nat and runtime:serves(nat, "front") then
        local now = love.timer and love.timer.getTime and love.timer.getTime() or os.clock()
        local shiny = dexShiny(personality)
        if kind ~= nil then
          -- The credits roll draws its own 64x64 slots, so it keeps that box.
          local boxed = runtime:boxFrame(nat, "front", shiny, now, Pack.BOX, Pack.BOX)
          if boxed then return boxed end
        else
          local entry = runtime:dexFrame(nat, "front", shiny, now)
          if entry then
            -- The draw wrapper centres this picture in the card's picture area;
            -- page 2's size comparison can still ask for a 64x64 fit of the
            -- same pose, so the mark carries a way to make one.
            picMark[entry.image] = { dex = true, w = entry.w, h = entry.h,
              alt = function()
                return runtime:boxFrame(nat, "front", shiny, now, Pack.BOX, Pack.BOX)
              end }
            return entry
          end
        end
      end
    end
    return original(species, personality, kind)
  end
end

-- ---- the way in: a BW SPRITES group in the game's own OPTION menu ----------
--
-- The rows travel through the engine's ui.options.rows hook, which every Gen 3
-- option menu (FireRed/LeafGreen's and Emerald's) call, so one wrap reaches all
-- of them. src/ui/game3/option_rows.lua then folds any id listed in Rows.GROUPS
-- into a group row whose activate opens a sub-page of its members, in the order
-- Rows.ORDER names them; registering one group is what turns the two switches
-- into "BW SPRITES 2 OPTIONS".
local OptionRows
do
  local ok, rows = pcall(require, "src.ui.game3.option_rows")
  if ok and type(rows) == "table" and type(rows.build) == "function" then
    OptionRows = rows
  end
end

-- Newer engine builds replace that module with a single src/ui/OptionsMenu,
-- where a mod's rows stay at the top level (its groups are hard-coded). The
-- sandbox only ever sees a Gen 3 compatibility facade for that name, so the
-- mod cannot open a real sub-page there; two loose rows are the honest seam.

local GROUP_ID = "group.bwSprites"

local function modId()
  return mod.id or (mod.manifest and mod.manifest.id) or "bwsprites"
end

-- The persisted values live in the engine's own options tree under
-- options.modOptions[<mod id>] -- where the loader reads them back from and
-- SaveData.saveOptions writes them (the option menu calls g:writeOptions()
-- after a step, so persisting is its own job). Mirroring the loader's live
-- copy is what makes mod.options:get -- and therefore sideEnabled -- see the
-- change at once instead of after the next boot.
local function optionsBucket(game)
  local o = game and game.options
  if type(o) ~= "table" then return nil end
  if type(o.modOptions) ~= "table" then o.modOptions = {} end
  local id = modId()
  local bucket = o.modOptions[id]
  if type(bucket) ~= "table" then
    bucket = {}
    o.modOptions[id] = bucket
  end
  return bucket
end

local function readChoice(game, key)
  if valid(current[key]) then return current[key] end
  local bucket = optionsBucket(game)
  local value = bucket and bucket[key]
  if valid(value) then return value end
  return mode(key)
end

local function writeChoice(game, key, value)
  local bucket = optionsBucket(game)
  if bucket then bucket[key] = value end
  local loader = game and game.mods
  if type(loader) == "table" then
    loader.modOptions = loader.modOptions or {}
    local id = modId()
    loader.modOptions[id] = loader.modOptions[id] or {}
    loader.modOptions[id][key] = value
  end
end

-- The hook hands a row its game, so both callbacks take one argument.
local function switchRow(setting)
  return {
    id = setting.id,
    label = setting.label,
    value = function(game) return SWITCH_LABEL[readChoice(game, setting.key)] end,
    step = function(game, dir)
      local at = readChoice(game, setting.key)
      at = (at == "bw") and "default" or "bw"
      current[setting.key] = at
      writeChoice(game, setting.key, at)
      applyModes()
      mod.log:info("bwsprites: %s sprites now %s", setting.label, SWITCH_LABEL[at])
      return true
    end,
  }
end

local function registerGroup()
  local found = false
  for _, group in ipairs(OptionRows.GROUPS or {}) do
    if group.id == GROUP_ID then found = true break end
  end
  if not found then
    OptionRows.GROUPS = OptionRows.GROUPS or {}
    OptionRows.GROUPS[#OptionRows.GROUPS + 1] = {
      id = GROUP_ID,
      label = "BW SPRITES",
      members = { "bwPlayer", "bwFoe" },
    }
  end
  local inOrder = false
  for _, id in ipairs(OptionRows.ORDER or {}) do
    if id == GROUP_ID then inOrder = true break end
  end
  if not inOrder then
    OptionRows.ORDER = OptionRows.ORDER or {}
    OptionRows.ORDER[#OptionRows.ORDER + 1] = GROUP_ID
  end
end

if OptionRows then registerGroup() end

-- Some Gen 3 engine builds (including the patched desktop one this mod is
-- tested on) never call the ui.options.rows hook from src/ui/game3/option_rows:
-- they build the flat list with Rows.build and group it. Wrapping that build
-- (once) adds the same rows there, and the dedupe keeps a future engine that
-- does call the hook from appending them twice.
if OptionRows and type(OptionRows.build) == "function" then
  local build = OptionRows.build
  local wrapped = function(...)
    local rows = build(...)
    if type(rows) ~= "table" then return rows end
    local have = {}
    for _, row in ipairs(rows) do
      if type(row) == "table" then have[row.id] = true end
    end
    for _, setting in ipairs(SETTINGS) do
      if not have[setting.id] then rows[#rows + 1] = switchRow(setting) end
    end
    return rows
  end
  OptionRows.build = wrapped
end

local hooks = mod.hooks
if type(hooks) == "table" and type(hooks.wrap) == "function" then
  hooks:wrap("ui.options.rows", function(next, game, rows)
    local out = next(game, rows)
    if type(out) ~= "table" then return out end
    local function already(id)
      for _, row in ipairs(out) do
        if type(row) == "table" and row.id == id then return true end
      end
      return false
    end
    for _, setting in ipairs(SETTINGS) do
      if not already(setting.id) then out[#out + 1] = switchRow(setting) end
    end
    return out
  end)
else
  mod.log:warn("bwsprites: the engine exposes no ui.options.rows hook; the OPTION menu rows stay out (the launcher's mod options still work)")
end

-- Honour a saved choice before the first picture is asked for.
applyModes()

local views, frames, female = 0, 0, 0
for _, entry in pairs(index.entries) do
  if entry.frontFrames then views = views + 1; frames = frames + entry.frontFrames end
  if entry.backFrames then views = views + 1; frames = frames + entry.backFrames end
  if entry.frontFemaleFrames then views = views + 1; frames = frames + entry.frontFemaleFrames; female = female + 1 end
  if entry.backFemaleFrames then views = views + 1; frames = frames + entry.backFemaleFrames; female = female + 1 end
end
local species, pinned = 0, 0
for _ in pairs(index.entries) do species = species + 1 end
for _, setting in ipairs(SETTINGS) do
  if sideEnabled(setting.side) then pinned = pinned + 1 end
end
mod.log:info("bwsprites: %d species, %d views (%d female), %d frames from %s (%d/%d sides on BW, native cells, %s)",
  species, views, female, frames, tostring(index.source and index.source.name or "pack"),
  pinned, #SETTINGS, CENTRED and "centre anchor" or "fixed origin")