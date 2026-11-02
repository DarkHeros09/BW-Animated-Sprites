-- Reads the animated Black/White battle sprites the launcher's IMPORTERS
-- screen extracts into asset_packs/gen5_bw/battle_sprites. This is the
-- asset-free BW build: no PNG ships in the mod, and the user's own dump is
-- decoded by the engine. The pack's frames are already the animation's union;
-- cut a frame, put its bottom row on the cell's last row and let main.lua do the
-- same cell correction it uses for an embedded pack.
local External = {}
local PackFrameIndex, PackFitRect

local IMPORTER, PACK = "gen5_bw", "battle_sprites"
local TICK_RATE = 60
local BOX = 64
local MAX_SIZE = 256
local MAX_FRAMES = 1024
local MAX_COLUMNS = 512
local MAX_ATLAS = 8 * 1024 * 1024
local ATLAS_CACHE = 8
local FRAME_CACHE = 96
local DEX_CACHE = 64

local function integer(value, what, lo, hi)
  if type(value) ~= "number" or value ~= math.floor(value)
    or (lo and value < lo) or (hi and value > hi) then
    error(("bwsprites imported pack: %s is invalid (%s)"):format(what, tostring(value)), 0)
  end
  return value
end

local function roundUp4(value)
  return math.ceil(value / 4) * 4
end

local function validate(raw)
  local s = raw.sprite
  if type(s) ~= "table" then error("bwsprites imported pack: " .. tostring(raw.id) .. " has no sprite metadata", 0) end
  integer(s.width, raw.id .. " width", 1, MAX_SIZE)
  integer(s.height, raw.id .. " height", 1, MAX_SIZE)
  integer(s.frames, raw.id .. " frames", 1, MAX_FRAMES)
  integer(s.columns, raw.id .. " columns", 1, MAX_COLUMNS)
  if s.tickRate ~= TICK_RATE then error("bwsprites imported pack: " .. raw.id .. " is not 60 Hz", 0) end
  if type(s.durations) ~= "table" or #s.durations ~= s.frames then
    error("bwsprites imported pack: " .. raw.id .. " duration table does not match its frames", 0)
  end
  local total = 0
  for i, duration in ipairs(s.durations) do
    integer(duration, raw.id .. " frame " .. i .. " duration", 1, 3600000)
    total = total + duration
  end
  local loop = s.loopStartFrame or 0
  integer(loop, raw.id .. " loop start", 0, s.frames - 1)
  if s.cycleTicks ~= nil and s.cycleTicks ~= total then
    error("bwsprites imported pack: " .. raw.id .. " cycle length mismatch", 0)
  end
  integer(raw.width, raw.id .. " atlas width", 1, MAX_ATLAS)
  integer(raw.height, raw.id .. " atlas height", 1, MAX_ATLAS)
  integer(raw.size, raw.id .. " size", 1, MAX_ATLAS)
  local rows = math.ceil(s.frames / s.columns)
  if raw.width ~= s.width * s.columns or raw.height ~= s.height * rows then
    error("bwsprites imported pack: " .. raw.id .. " atlas does not match its grid", 0)
  end
  local cell = math.max(BOX, roundUp4(math.max(s.width, s.height)))
  if cell > MAX_SIZE then error("bwsprites imported pack: " .. raw.id .. " cell is too large", 0) end
  return s, cell
end

-- parseID splits normal/025/front and normal/025/front/female. Lua patterns
-- have no alternation, so the segments are checked one by one.
local function parseID(id)
  local parts = {}
  for part in id:gmatch("[^/]+") do parts[#parts + 1] = part end
  local palette, side = parts[1], parts[3]
  if palette ~= "normal" and palette ~= "shiny" then return nil end
  if side ~= "front" and side ~= "back" then return nil end
  local nat = tonumber(parts[2])
  if not nat then return nil end
  local female = false
  if #parts == 4 and parts[4] == "female" then
    female = true
  elseif #parts ~= 3 then
    return nil
  end
  return palette, nat, side, female
end

-- packEntries turns the launcher's flat entry list into per-species views.
-- The importer writes normal/shiny, front/back and /female aliases; only those
-- ids are used here. The separately published parts atlases are ignored by this
-- reader: a capped view plays the exported prefix the pack contains.
local function packEntries(packs)
  local list, err = packs:entries(IMPORTER, PACK)
  if type(list) ~= "table" then error(err or "the launcher's Black/White pack has no entries", 0) end
  local views, capped = {}, 0
  for _, raw in ipairs(list) do
    local palette, nat, side, female = parseID(raw.id or "")
    if palette and nat >= 1 and nat <= 649 then
      local s, cell = validate(raw)
      local bySide = views[nat] or {}
      views[nat] = bySide
      local group = bySide[side] or {}
      bySide[side] = group
      local gender = group[female and "female" or "male"] or {}
      group[female and "female" or "male"] = gender
      gender[palette] = {
        id = raw.id, file = raw.file, size = raw.size,
        width = s.width, height = s.height, columns = s.columns,
        frames = s.frames, durations = s.durations, loopStart = s.loopStartFrame or 0,
        cell = cell, atlasW = raw.width, atlasH = raw.height,
        capped = s.cycleCapped == true,
      }
      if s.cycleCapped == true then capped = capped + 1 end
    end
  end
  return views, capped
end

local function buildIndex(views, info)
  local entries = {}
  for nat, bySide in pairs(views) do
    local entry = {}
    for _, side in ipairs({ "front", "back" }) do
      local group = bySide[side]
      if group then
        local male = group.male and (group.male.normal or group.male.shiny)
        local female = group.female and (group.female.normal or group.female.shiny)
        if male then
          entry[side .. "Frames"] = male.frames
          entry[side .. "Durations"] = male.durations
          entry[side .. "LoopStart"] = male.loopStart
          entry[side .. "Size"] = male.cell
          entry[side .. "InkH"] = male.height
          entry[side .. "Columns"] = male.columns
          entry[side .. "Dex"] = { x = 0, y = 0, w = male.width, h = male.height }
        end
        if female then
          entry[side .. "FemaleFrames"] = female.frames
          entry[side .. "FemaleDurations"] = female.durations
          entry[side .. "FemaleLoopStart"] = female.loopStart
          entry[side .. "FemaleSize"] = female.cell
          entry[side .. "FemaleInkH"] = female.height
          entry[side .. "FemaleColumns"] = female.columns
          entry[side .. "FemaleDex"] = { x = 0, y = 0, w = female.width, h = female.height }
        end
      end
    end
    if entry.frontFrames or entry.backFrames then entries[nat] = entry end
  end
  return {
    format = 2, box = BOX, pad = 0, tickRate = TICK_RATE,
    source = { name = (info and info.source) or "launcher Black/White import" },
    entries = entries,
  }
end

local function newest(cache, limit, used)
  if used >= limit then
    local oldest, stamp
    for key, item in pairs(cache) do
      if not stamp or item.used < stamp then oldest, stamp = key, item.used end
    end
    if oldest then cache[oldest] = nil end
  end
end

local function newRuntime(packs, views)
  local self = { packs = packs, views = views, atlases = {}, frames = {}, dex = {}, serial = 0,
    now = function()
      return love.timer and love.timer.getTime and love.timer.getTime() or os.clock()
    end }
  return setmetatable(self, { __index = External })
end

function External:viewFor(nat, side, shiny, female)
  local group = self.views[nat] and self.views[nat][side]
  if not group then return nil end
  local gender = (female and group.female) or group.male
  if not gender then return nil end
  return (shiny and (gender.shiny or gender.normal)) or gender.normal
end

function External:serves(nat, side, female)
  return self:viewFor(nat, side, false, female) ~= nil
end

function External:frameIndex(view, now)
  local t = now
  if type(t) ~= "number" then t = self.now() end
  -- Pack.frameIndex is a colon method: self supplies the tick rate and clock,
  -- the view supplies durations/frames/loopStart.
  return PackFrameIndex({ index = { tickRate = TICK_RATE }, now = function() return t end }, view, t)
end

function External:atlas(view)
  local hit = self.atlases[view.id]
  self.serial = self.serial + 1
  if hit then hit.used = self.serial; return hit.data end
  local bytes, err = self.packs:read(IMPORTER, PACK, view.id)
  if type(bytes) ~= "string" or #bytes ~= view.size then
    error(err or "the imported sprite atlas is missing or has the wrong size", 0)
  end
  local file, fileErr = love.filesystem.newFileData(bytes, "bw-import.png")
  if not file then error(fileErr or "could not open the imported atlas", 0) end
  local data, dataErr = love.image.newImageData(file)
  file:release()
  if not data then error(dataErr or "could not decode the imported atlas", 0) end
  local w, h = data:getDimensions()
  if w ~= view.atlasW or h ~= view.atlasH then
    data:release()
    error("the imported atlas does not match its metadata", 0)
  end
  newest(self.atlases, ATLAS_CACHE, self.serial)
  self.atlases[view.id] = { data = data, used = self.serial }
  return data
end

function External:cellImage(view, index)
  local key = view.id .. ":" .. index
  self.serial = self.serial + 1
  local hit = self.frames[key]
  if hit then hit.used = self.serial; return hit.image end
  local data = self:atlas(view)
  local out = love.image.newImageData(view.cell, view.cell)
  local col, row = (index - 1) % view.columns, math.floor((index - 1) / view.columns)
  out:paste(data, math.floor((view.cell - view.width) / 2), view.cell - view.height,
    col * view.width, row * view.height, view.width, view.height)
  local image = love.graphics.newImage(out)
  out:release()
  image:setFilter("nearest", "nearest")
  newest(self.frames, FRAME_CACHE, self.serial)
  self.frames[key] = { image = image, used = self.serial }
  return image
end

function External:dexImage(view, index)
  local key = view.id .. ":" .. index
  self.serial = self.serial + 1
  local hit = self.dex[key]
  if hit then hit.used = self.serial; return hit.image end
  local data = self:atlas(view)
  local out = love.image.newImageData(view.width, view.height)
  local col, row = (index - 1) % view.columns, math.floor((index - 1) / view.columns)
  out:paste(data, 0, 0, col * view.width, row * view.height, view.width, view.height)
  local image = love.graphics.newImage(out)
  out:release()
  image:setFilter("nearest", "nearest")
  newest(self.dex, DEX_CACHE, self.serial)
  self.dex[key] = { image = image, used = self.serial }
  return image
end

function External:frame(nat, side, shiny, now, female)
  local view = self:viewFor(nat, side, shiny, female)
  if not view then return nil end
  local index = self:frameIndex(view, now)
  if not index then return nil, "no animation frame" end
  local image = self:cellImage(view, index)
  if not image then return nil, "could not cut an imported frame" end
  return { image = image, w = view.cell, h = view.cell, animated = view.frames > 1 }
end

-- The Pokedex card is a fixed 64x64 slot, exactly like the embedded pack's
-- dex window, so the imported pose is fitted into that box instead of being
-- handed over at its native size. It stays animated: each call cuts a new pose.
function External:dexFrame(nat, side, shiny, now)
  return self:boxFrame(nat, side, shiny, now, BOX, BOX)
end

function External:boxFrame(nat, side, shiny, now, boxW, boxH)
  local view = self:viewFor(nat, side, shiny, false)
  if not view then return nil end
  local index = self:frameIndex(view, now)
  if not index then return nil, "no animation frame" end
  local image = self:dexImage(view, index)
  if not image then return nil, "could not cut an imported dex picture" end
  local boxed, err = PackFitRect(image, { x = 0, y = 0, w = view.width, h = view.height },
    boxW or BOX, boxH or BOX)
  if not boxed then return nil, err end
  return { image = boxed, w = boxW or BOX, h = boxH or BOX, animated = view.frames > 1 }
end

-- The pack's frame order is the importer's: play the intro once, then loop from
-- loopStart+1. Pack.frameIndex already implements that rule and is passed in so
-- this module shares the timing code with the embedded pack.
function External.new(packModule, engineMod)
  PackFrameIndex = assert(packModule.frameIndex, "pack module has no frameIndex")
  PackFitRect = assert(packModule.fitRect, "pack module has no fitRect")
  local packs = engineMod and engineMod.packs
  if type(packs) ~= "table" then
    return nil, "this engine exposes no mod.packs; the asset-free build needs the launcher's Black/White import"
  end
  local info, infoErr = packs:info(IMPORTER, PACK)
  if not info then
    return nil, ("no imported Black/White pack (%s); import your own unzipped 256 MiB Black or White dump in the launcher's IMPORTERS tab, then restart"):format(tostring(infoErr))
  end
  local ok, views, capped = pcall(packEntries, packs)
  if not ok then return nil, tostring(views) end
  return {
    index = buildIndex(views, info),
    runtime = newRuntime(packs, views),
    capped = capped,
  }
end

return External
