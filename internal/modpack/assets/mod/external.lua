-- Reads the animated Black/White battle sprites the launcher's IMPORTERS
-- screen extracts into asset_packs/gen5_bw/battle_sprites. This is the
-- asset-free BW build: no PNG ships in the mod, and the user's own dump is
-- decoded by the engine. The pack's frames are already the animation's union;
-- cut a frame, put its bottom row on the cell's last row and let main.lua do the
-- same cell correction it uses for an embedded pack.
local External = {}
local PackFrameIndexTicks, PackFitRect

local IMPORTER, PACK = "gen5_bw", "battle_sprites"
local TICK_RATE = 60
local BOX = 64
local MAX_SIZE = 256
local MAX_FRAMES = 1024
local MAX_COLUMNS = 512
local MAX_ATLAS = 8 * 1024 * 1024
local ATLAS_CACHE = 8
local FRAME_CACHE = 192
local DEX_CACHE = 64
local BOX_CACHE = 32
local PARTS_PLAN_CACHE = 8
local PARTS_PIXEL_CACHE = 6
local PARTS_IMAGE_CACHE = 16
local TRANSPARENT = "\0\0\0\0"

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

-- The exporter caps a view at 240 ticks and publishes the rest of the loop as a
-- parts atlas: a small set of piece grids and per-track state runs. A capped
-- view that names one of those atlases is composed at draw time, which is how
-- the code-only build gets the same full idle cycle as the embedded build.
local function validateParts(raw)
  local s = raw.sprite
  if type(s) ~= "table" or s.kind ~= "parts" or s.format ~= "gen5-parts" then return nil end
  integer(s.width, raw.id .. " part width", 1, MAX_SIZE)
  integer(s.height, raw.id .. " part height", 1, MAX_SIZE)
  integer(s.frames, raw.id .. " part frames", 1, MAX_FRAMES)
  integer(s.columns, raw.id .. " part columns", 1, MAX_COLUMNS)
  integer(raw.width, raw.id .. " part atlas width", 1, MAX_ATLAS)
  integer(raw.height, raw.id .. " part atlas height", 1, MAX_ATLAS)
  integer(raw.size, raw.id .. " part atlas size", 1, MAX_ATLAS)
  if raw.width ~= s.width * s.columns
    or raw.height ~= s.height * math.ceil(s.frames / s.columns) then
    error("bwsprites imported pack: " .. raw.id .. " part atlas does not match its grid", 0)
  end
  if type(raw.metadata) ~= "table" or type(raw.metadata.file) ~= "string" then return nil end
  return { size = raw.size, width = raw.width, height = raw.height }
end

-- packEntries turns the launcher's flat entry list into per-species views.
-- The importer writes normal/shiny, front/back and /female aliases; the parts
-- atlases are collected too and linked to the capped view that needs them.
local function packEntries(packs)
  local list, err = packs:entries(IMPORTER, PACK)
  if type(list) ~= "table" then error(err or "the launcher's Black/White pack has no entries", 0) end
  local views, parts, all, capped = {}, {}, {}, 0
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
      local view = {
        id = raw.id, file = raw.file, size = raw.size,
        width = s.width, height = s.height, columns = s.columns,
        frames = s.frames, durations = s.durations, loopStart = s.loopStartFrame or 0,
        cell = cell, atlasW = raw.width, atlasH = raw.height,
        capped = s.cycleCapped == true,
      }
      gender[palette] = view
      all[#all + 1] = view
      if view.capped then
        if type(s.partsEntry) == "string" and (s.partsPalette == "normal" or s.partsPalette == "shiny") then
          view.parts = s.partsEntry
          view.partsPalette = s.partsPalette
        else
          capped = capped + 1
        end
      end
    else
      local part = validateParts(raw)
      if part then parts[raw.id] = part end
    end
  end
  for _, view in ipairs(all) do
    if view.parts then
      local part = parts[view.parts]
      if part then
        view.partsAtlas = part
      else
        view.parts = nil
        capped = capped + 1
      end
    end
  end
  return views, capped, parts
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

-- newest evicts one entry once the cache is full. The evicted texture or image
-- data has to be released: dropping the Lua reference alone left the GPU copy
-- alive, so a long session collected every frame it had ever drawn. The cache
-- is sized by its entry count, not by how many calls have happened: testing the
-- ever-growing serial against the limit evicted an entry on every insert after
-- the first few, so a two-atlas battle decoded both PNGs again every frame.
local function releaseImage(item)
  if item and item.image and item.image.release then item.image:release() end
end
local function releaseData(item)
  if item and item.data and item.data.release then item.data:release() end
end
local function newest(cache, limit, used, release)
  local count = 0
  for _ in pairs(cache) do count = count + 1 end
  if count < limit then return end
  local oldest, stamp
  for key, item in pairs(cache) do
    if not stamp or item.used < stamp then oldest, stamp = key, item.used end
  end
  if oldest then
    local item = cache[oldest]
    cache[oldest] = nil
    if release then release(item) end
  end
end

local function newRuntime(packs, views)
  local self = { packs = packs, views = views, atlases = {}, frames = {}, dex = {}, boxes = {},
    plans = {}, pixels = {}, partImages = {}, partCells = {}, serial = 0,
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
  return PackFrameIndexTicks(view, math.floor(t * TICK_RATE))
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
  newest(self.atlases, ATLAS_CACHE, self.serial, releaseData)
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
  newest(self.frames, FRAME_CACHE, self.serial, releaseImage)
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
  newest(self.dex, DEX_CACHE, self.serial, releaseImage)
  self.dex[key] = { image = image, used = self.serial }
  return image
end

-- partsPlan validates the piece grids and state runs the exporter publishes
-- for a capped view. Layout: a flat six-number piece list (source x,y,w,h and
-- atlas x,y), then one track per moving layer with intro/period and run-length
-- state indexes. Everything is checked here so a corrupt pack fails loudly.
function External:partsPlan(view)
  local hit = self.plans[view.parts]
  self.serial = self.serial + 1
  if hit then hit.used = self.serial; return hit.plan end
  local meta, err = self.packs:metadata(IMPORTER, PACK, view.parts)
  if type(meta) ~= "table" then error(err or "missing imported part tracks", 0) end
  local part = view.partsAtlas
  if meta.format ~= "gen5-parts" or meta.version ~= 1 or meta.tickRate ~= TICK_RATE
    or meta.width ~= view.width or meta.height ~= view.height
    or meta.atlasWidth ~= part.width or meta.atlasHeight ~= part.height then
    error("bwsprites imported pack: bad part metadata for " .. view.parts, 0)
  end
  local rawPieces = meta.pieces
  if type(rawPieces) ~= "table" or #rawPieces < 6 or #rawPieces % 6 ~= 0
    or #rawPieces > 6 * 4096 then
    error("bwsprites imported pack: bad piece table for " .. view.parts, 0)
  end
  local pieces, area = {}, 0
  for i = 1, #rawPieces / 6 do
    local o = i * 6 - 6
    local x, y, w, h, ax, ay = rawPieces[o + 1], rawPieces[o + 2],
      rawPieces[o + 3], rawPieces[o + 4], rawPieces[o + 5], rawPieces[o + 6]
    integer(x, "piece x", 0, 4096); integer(y, "piece y", 0, 4096)
    integer(w, "piece width", 1, 4096); integer(h, "piece height", 1, 4096)
    integer(ax, "piece atlas x", 0, part.width); integer(ay, "piece atlas y", 0, part.height)
    if x + w > view.width or y + h > view.height or ax + w > part.width or ay + h > part.height then
      error("bwsprites imported pack: piece outside its atlas", 0)
    end
    area = area + w * h
    if area > part.width * part.height then
      error("bwsprites imported pack: pieces exceed the part atlas", 0)
    end
    pieces[i] = { index = i, x = x, y = y, w = w, h = h, ax = ax, ay = ay }
  end
  local tracks = {}
  for r = 1, #meta.tracks do
    local t = meta.tracks[r]
    if type(t) ~= "table" or not integer(t.intro, "track intro", 0, 65535)
      or not integer(t.period, "track period", 1, 65536)
      or t.intro + t.period > 65536 then
      error("bwsprites imported pack: bad part track", 0)
    end
    local span = t.intro + t.period
    local rawStates = t.states
    if type(rawStates) ~= "table" or #rawStates < 9 or #rawStates % 9 ~= 0
      or #rawStates > 9 * math.min(span, 4096) then
      error("bwsprites imported pack: bad part states", 0)
    end
    local states = {}
    for s = 1, #rawStates / 9 do
      local o = s * 9 - 9
      local has = rawStates[o + 1]
      if has ~= 0 and has ~= 1 then error("bwsprites imported pack: bad part state", 0) end
      local state = { pieces = {}, key = s }
      if has == 1 then
        state.bounds = { rawStates[o + 2], rawStates[o + 3], rawStates[o + 4], rawStates[o + 5] }
      end
      for q = 1, 4 do
        local index = rawStates[o + 5 + q]
        if not integer(index, "part state piece", 0, #pieces) then
          error("bwsprites imported pack: bad part state piece", 0)
        end
        if index > 0 then state.pieces[q] = pieces[index] end
      end
      states[s] = state
    end
    local rawRuns = t.runs
    if type(rawRuns) ~= "table" or #rawRuns < 2 or #rawRuns % 2 ~= 0
      or #rawRuns > 2 * span then
      error("bwsprites imported pack: bad part runs", 0)
    end
    local at, n = {}, 0
    for i = 1, #rawRuns, 2 do
      local length, state = rawRuns[i], rawRuns[i + 1]
      if not integer(length, "part run length", 1, span)
        or not integer(state, "part run state", 1, #states) or n + length > span then
        error("bwsprites imported pack: bad part run", 0)
      end
      for k = n + 1, n + length do at[k] = states[state] end
      n = n + length
    end
    if n ~= span then error("bwsprites imported pack: part run length mismatch", 0) end
    tracks[r] = { intro = t.intro, period = t.period, at = at }
  end
  local function palette(values)
    if type(values) ~= "table" or #values < 4 or #values % 4 ~= 0 or #values > 1024 then
      error("bwsprites imported pack: bad part palette", 0)
    end
    local colors = { [0] = TRANSPARENT }
    for i = 1, #values / 4 do
      local o = i * 4 - 4
      local r, g, b, a = values[o + 1], values[o + 2], values[o + 3], values[o + 4]
      if not (integer(r, "palette red", 0, 255) and integer(g, "palette green", 0, 255)
        and integer(b, "palette blue", 0, 255) and integer(a, "palette alpha", 0, 255)) then
        error("bwsprites imported pack: bad part palette colour", 0)
      end
      colors[i] = (i == 1) and TRANSPARENT or string.char(r, g, b, a)
    end
    return colors
  end
  local plan = { width = view.width, height = view.height, tracks = tracks, pieces = pieces,
    palettes = { normal = palette(meta.palettes and meta.palettes.normal),
      shiny = palette(meta.palettes and meta.palettes.shiny) } }
  newest(self.plans, PARTS_PLAN_CACHE, self.serial, nil)
  self.plans[view.parts] = { plan = plan, used = self.serial }
  return plan
end

function External:partsPixels(view)
  local hit = self.pixels[view.parts]
  self.serial = self.serial + 1
  if hit then hit.used = self.serial; return hit end
  local bytes, err = self.packs:read(IMPORTER, PACK, view.parts)
  if type(bytes) ~= "string" or #bytes ~= view.partsAtlas.size then
    error(err or "the imported part atlas is missing or has the wrong size", 0)
  end
  local file = love.filesystem.newFileData(bytes, "bw-parts.png")
  if not file then error("could not open the imported part atlas", 0) end
  local data = love.image.newImageData(file)
  file:release()
  if not data then error("could not decode the imported part atlas", 0) end
  local w, h = data:getDimensions()
  if w ~= view.partsAtlas.width or h ~= view.partsAtlas.height then
    data:release()
    error("the imported part atlas does not match its metadata", 0)
  end
  local pixels = { raw = data:getString(), atlasW = w, used = self.serial }
  data:release()
  newest(self.pixels, PARTS_PIXEL_CACHE, self.serial, nil)
  self.pixels[view.parts] = pixels
  return pixels
end

local function partsStates(plan, tick)
  local states, key = {}, {}
  for r, track in ipairs(plan.tracks) do
    local t = tick
    if t >= track.intro then t = track.intro + (t - track.intro) % track.period end
    local state = track.at[t + 1]
    states[r], key[r] = state, state.key
  end
  return states, table.concat(key, ",")
end

-- composeParts paints the selected piece of every track into one native-size
-- index buffer, then maps the indexes through the palette. Results are cached
-- by state signature: the exporter's tracks repeat many ticks unchanged.
function External:composeParts(view, tick)
  local plan = self:partsPlan(view)
  local states, signature = partsStates(plan, tick)
  local key = view.parts .. "|" .. view.partsPalette .. "@" .. signature
  self.serial = self.serial + 1
  local hit = self.partImages[key]
  if hit then hit.used = self.serial; return hit.image, signature, hit.data end
  local pixels = self:partsPixels(view)
  local W, H, raw, atlasW = plan.width, plan.height, pixels.raw, pixels.atlasW
  local out = {}
  for i = 1, W * H do out[i] = 0 end
  for layer = 4, 1, -1 do
    for r = #plan.tracks, 1, -1 do
      local piece = states[r].pieces[layer]
      if piece then
        local pw = piece.w
        for y = 0, piece.h - 1 do
          local src = (piece.ay + y) * atlasW + piece.ax
          local dst = (piece.y + y) * W + piece.x
          for x = 0, pw - 1 do
            local o = (src + x) * 4
            if raw:byte(o + 4) ~= 0 then out[dst + x + 1] = raw:byte(o + 1) end
          end
        end
      end
    end
  end
  local colors = plan.palettes[view.partsPalette] or plan.palettes.normal
  local strings = {}
  -- Piece pixels store a zero-based palette index; palette entry 1 is the
  -- transparent slot, so colour n lives at colors[n + 1].
  for i = 1, W * H do strings[i] = colors[out[i] + 1] or TRANSPARENT end
  local data = love.image.newImageData(W, H, "rgba8", table.concat(strings))
  local image = love.graphics.newImage(data)
  image:setFilter("nearest", "nearest")
  newest(self.partImages, PARTS_IMAGE_CACHE, self.serial, function(item)
    releaseImage(item)
    releaseData(item)
  end)
  self.partImages[key] = { image = image, data = data, used = self.serial }
  return image, signature, data
end

function External:partsCell(view, tick)
  local native, signature, data = self:composeParts(view, tick)
  local key = view.id .. "@" .. signature
  self.serial = self.serial + 1
  local hit = self.partCells[key]
  if hit then hit.used = self.serial; return hit.image end
  local out = love.image.newImageData(view.cell, view.cell)
  out:paste(data, math.floor((view.cell - view.width) / 2), view.cell - view.height)
  local image = love.graphics.newImage(out)
  out:release()
  image:setFilter("nearest", "nearest")
  newest(self.partCells, PARTS_IMAGE_CACHE, self.serial, releaseImage)
  self.partCells[key] = { image = image, used = self.serial }
  return image
end

function External:frame(nat, side, shiny, now, female)
  local view = self:viewFor(nat, side, shiny, female)
  if not view then return nil end
  local t = now
  if type(t) ~= "number" then t = self.now() end
  local image
  if view.parts then
    image = self:partsCell(view, math.floor(t * TICK_RATE))
  else
    local index = self:frameIndex(view, t)
    if not index then return nil, "no animation frame" end
    image = self:cellImage(view, index)
  end
  if not image then return nil, "could not cut an imported frame" end
  return { image = image, w = view.cell, h = view.cell, animated = view.frames > 1 }
end

-- The Pokedex gets the same picture the embedded pack serves: the current pose
-- at its native size, which main.lua centres in the card. Fitting it into 64x64
-- here shrank wide species and re-rendered a canvas on every draw.
function External:dexFrame(nat, side, shiny, now)
  local view = self:viewFor(nat, side, shiny, false)
  if not view then return nil end
  local t = now
  if type(t) ~= "number" then t = self.now() end
  local image, w, h
  if view.parts then
    image = self:composeParts(view, math.floor(t * TICK_RATE))
    w, h = view.width, view.height
  else
    local index = self:frameIndex(view, t)
    if not index then return nil, "no animation frame" end
    image = self:dexImage(view, index)
    w, h = view.width, view.height
  end
  if not image then return nil, "could not cut an imported dex picture" end
  return { image = image, w = w, h = h, animated = view.frames > 1 }
end

-- boxFrame is the 64x64 fit the Pokedex's size comparison and the credits roll
-- ask for. It is cached per pose and box size, so those screens do not build a
-- canvas every frame.
function External:boxFrame(nat, side, shiny, now, boxW, boxH)
  local view = self:viewFor(nat, side, shiny, false)
  if not view then return nil end
  local t = now
  if type(t) ~= "number" then t = self.now() end
  local image, cacheKey
  if view.parts then
    local native, signature = self:composeParts(view, math.floor(t * TICK_RATE))
    image, cacheKey = native, view.id .. "@" .. signature
  else
    local index = self:frameIndex(view, t)
    if not index then return nil, "no animation frame" end
    image, cacheKey = self:dexImage(view, index), view.id .. ":" .. index
  end
  boxW, boxH = boxW or BOX, boxH or BOX
  local key = cacheKey .. ":" .. boxW .. "x" .. boxH
  self.serial = self.serial + 1
  local hit = self.boxes[key]
  if hit then hit.used = self.serial; return { image = hit.image, w = boxW, h = boxH, animated = view.frames > 1 } end
  if not image then return nil, "could not cut an imported dex picture" end
  local boxed, err = PackFitRect(image, { x = 0, y = 0, w = view.width, h = view.height }, boxW, boxH)
  if not boxed then return nil, err end
  newest(self.boxes, BOX_CACHE, self.serial, releaseImage)
  self.boxes[key] = { image = boxed, used = self.serial }
  return { image = boxed, w = boxW, h = boxH, animated = view.frames > 1 }
end

-- The pack's frame order is the importer's: play the intro once, then loop from
-- loopStart+1. Pack.frameIndex already implements that rule and is passed in so
-- this module shares the timing code with the embedded pack.
function External.new(packModule, engineMod)
  PackFrameIndexTicks = assert(packModule.frameIndexTicks, "pack module has no frameIndexTicks")
  PackFitRect = assert(packModule.fitRect, "pack module has no fitRect")
  local packs = engineMod and engineMod.packs
  if type(packs) ~= "table" then
    return nil, "this engine exposes no mod.packs; the asset-free build needs the launcher's Black/White import"
  end
  local info, infoErr = packs:info(IMPORTER, PACK)
  if not info then
    return nil, ("no imported Black/White pack (%s); import your own unzipped 256 MiB Black or White dump in the launcher's IMPORTERS tab, then restart"):format(tostring(infoErr))
  end
  local ok, views, capped, parts = pcall(packEntries, packs)
  if not ok then return nil, tostring(views) end
  local runtime = newRuntime(packs, views)
  runtime.partsEntries = parts
  return {
    index = buildIndex(views, info),
    runtime = runtime,
    capped = capped,
  }
end

return External
