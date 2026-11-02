-- Loads a bwsprites pack and turns its sheets into frame images on demand.
-- Format 2 packs store square cells in a grid: native 64..160 px fronts (no
-- Black/White front needs more) and up to 112 px backs. The reader accepts the
-- same range for older packs. Every entry point takes the reader it needs, so
-- this file can be exercised without the engine.
local Pack = {}

Pack.BOX = 64
Pack.MIN_SIZE = 64
Pack.MAX_SIZE = 160
Pack.PAD = 0
Pack.MAX_FRAMES = 1024
Pack.MAX_COLUMNS = 64

local function fail(format, ...)
  error(("bwsprites pack: " .. format):format(...), 0)
end

local function integer(value, what)
  if type(value) ~= "number" or value ~= math.floor(value) then
    fail("%s is not an integer (got %s)", what, tostring(value))
  end
  return value
end

-- Validates and returns the index a build wrote. Any defect fails the whole
-- load with the path and field at fault, because a half-understood pack would
-- put the wrong art on a species.
function Pack.parseIndex(source, chunkName)
  if type(source) ~= "string" or source == "" then
    fail("%s is empty", tostring(chunkName))
  end
  local chunk, err = load(source, chunkName or "@index.lua")
  if not chunk then fail("%s", tostring(err)) end
  local ok, index = pcall(chunk)
  if not ok then fail("%s", tostring(index)) end
  if type(index) ~= "table" then fail("index.lua did not return a table") end
  if index.format ~= 2 then
    fail("pack format %s is not supported; this mod reads format 2", tostring(index.format))
  end
  if index.box ~= Pack.BOX then
    fail("pack cell box is %s, want %s", tostring(index.box), tostring(Pack.BOX))
  end
  if index.pad ~= Pack.PAD then
    fail("pack cell pad is %s, want %s", tostring(index.pad), tostring(Pack.PAD))
  end
  if type(index.tickRate) ~= "number" or index.tickRate <= 0 then
    fail("tickRate is %s, want a positive number", tostring(index.tickRate))
  end
  if type(index.entries) ~= "table" then fail("entries is not a table") end
  for nat, entry in pairs(index.entries) do
    if type(nat) ~= "number" or nat ~= math.floor(nat) or nat < 1 then
      fail("entry key %s is not a national dex number", tostring(nat))
    end
    if type(entry) ~= "table" then fail("entry %d is not a table", nat) end
    if entry.frontFrames == nil and entry.backFrames == nil then
      fail("entry %d has neither a front nor a back view", nat)
    end
    for _, side in ipairs({ "front", "back", "frontFemale", "backFemale" }) do
      local frames = entry[side .. "Frames"]
      if frames ~= nil then
        frames = integer(frames, ("entry %d %sFrames"):format(nat, side))
        if frames < 1 or frames > Pack.MAX_FRAMES then
          fail("entry %d %s has %d frames, want 1..%d", nat, side, frames, Pack.MAX_FRAMES)
        end
        local durations = entry[side .. "Durations"]
        if type(durations) ~= "table" or #durations ~= frames then
          fail("entry %d %sDurations does not hold %d durations", nat, side, frames)
        end
        for i = 1, frames do
          local d = integer(durations[i], ("entry %d %s frame %d duration"):format(nat, side, i))
          if d < 1 then
            fail("entry %d %s frame %d lasts %d ticks; durations are positive", nat, side, i, d)
          end
        end
        local loopStart = integer(entry[side .. "LoopStart"] or 0, ("entry %d %sLoopStart"):format(nat, side))
        if loopStart < 0 or loopStart >= frames then
          fail("entry %d %s loopStart is %d, want 0..%d", nat, side, loopStart, frames - 1)
        end
        local size = integer(entry[side .. "Size"], ("entry %d %sSize"):format(nat, side))
        if size < Pack.MIN_SIZE or size > Pack.MAX_SIZE or size % 4 ~= 0 then
          fail("entry %d %s cell size is %d, want %d..%d in steps of 4",
            nat, side, size, Pack.MIN_SIZE, Pack.MAX_SIZE)
        end
        local columns = integer(entry[side .. "Columns"], ("entry %d %sColumns"):format(nat, side))
        if columns < 1 or columns > Pack.MAX_COLUMNS then
          fail("entry %d %s columns is %d, want 1..%d", nat, side, columns, Pack.MAX_COLUMNS)
        end
        local inkH = integer(entry[side .. "InkH"], ("entry %d %sInkH"):format(nat, side))
        if inkH < 1 or inkH > size then
          fail("entry %d %s ink height is %d, want 1..%d", nat, side, inkH, size)
        end
        local dex = entry[side .. "Dex"]
        if dex ~= nil then
          if type(dex) ~= "table" then
            fail("entry %d %sDex is not a table", nat, side)
          end
          local x = integer(dex.x, ("entry %d %sDex x"):format(nat, side))
          local y = integer(dex.y, ("entry %d %sDex y"):format(nat, side))
          local w = integer(dex.w, ("entry %d %sDex w"):format(nat, side))
          local h = integer(dex.h, ("entry %d %sDex h"):format(nat, side))
          if x < 0 or y < 0 or w < 1 or h < 1 or x + w > size or y + h > size then
            fail("entry %d %sDex box %d,%d %dx%d is outside the %d px cell",
              nat, side, x, y, w, h, size)
          end
        end
        local path = entry[side]
        if type(path) ~= "string" or path == "" then
          fail("entry %d %s has no sheet path", nat, side)
        end
        if entry[side .. "Shiny"] ~= nil and (type(entry[side .. "Shiny"]) ~= "string" or entry[side .. "Shiny"] == "") then
          fail("entry %d %sShiny is not a sheet path", nat, side)
        end
      end
    end
  end
  return index
end

-- Runtime resolves frames for one loaded pack. opts.read(relative) returns
-- the stripped bytes and opts.path(relative) returns the love filesystem path
-- of the same file; opts.now() is the animation clock.
function Pack.newRuntime(index, opts)
  local self = {
    index = index,
    read = assert(opts.read, "runtime needs read(relative)"),
    path = assert(opts.path, "runtime needs path(relative)"),
    now = opts.now or function() return love.timer.getTime() end,
    views = {},
  }
  return setmetatable(self, { __index = Pack })
end

-- prefix is the index field prefix for a view; a female request falls back to
-- the male view when the dump had no distinct female graphic for that side.
local function prefixFor(entry, side, female)
  if female and entry and entry[side .. "FemaleFrames"] ~= nil then
    return side .. "Female"
  end
  return side
end

-- serves reports whether nat has a view for side, without touching the disk.
function Pack:serves(nat, side, female)
  local entry = self.index.entries[nat]
  if not entry then return false end
  return entry[prefixFor(entry, side, female) .. "Frames"] ~= nil
end

function Pack:view(nat, side, shiny, female)
  local entry = self.index.entries[nat]
  local prefix = prefixFor(entry, side, female)
  local key = ("%d/%s/%s"):format(nat, prefix, shiny and "shiny" or "normal")
  local hit = self.views[key]
  if hit ~= nil then return hit or nil end
  if not entry then
    self.views[key] = false
    return nil
  end
  local frames = entry[prefix .. "Frames"]
  local durations = entry[prefix .. "Durations"]
  if not frames or not durations then
    self.views[key] = false
    return nil
  end
  local sheet = shiny and entry[prefix .. "Shiny"] or nil
  sheet = sheet or entry[prefix]
  local ok, data = pcall(love.image.newImageData, self.path(sheet))
  if not ok or not data then
    self.views[key] = false
    return nil
  end
  local size = entry[prefix .. "Size"]
  local columns = entry[prefix .. "Columns"]
  local cols = math.min(frames, columns)
  local rows = math.ceil(frames / columns)
  local sheetW, sheetH = data:getDimensions()
  if sheetW ~= cols * size or sheetH ~= rows * size then
    local want = ("%dx%d"):format(cols * size, rows * size)
    data:release()
    self.views[key] = false
    return nil, ("%s is %dx%d, want %s"):format(sheet, sheetW, sheetH, want)
  end
  local view = {
    sheet = sheet,
    data = data,
    size = size,
    cell = size,
    columns = columns,
    frames = frames,
    durations = durations,
    loopStart = entry[prefix .. "LoopStart"] or 0,
    dexRect = entry[prefix .. "Dex"],
    images = {},
    dex = {},
  }
  self.views[key] = view
  return view
end

-- frameIndex maps a wall-clock instant onto the view's frames, playing the
-- intro once and then looping from loopStart+1, the order the pack records.
function Pack:frameIndex(view, now)
  local rate = self.index.tickRate or 60
  local t = math.floor((now or self.now()) * rate)
  local i = 1
  -- Play the intro once, then keep looping from loopStart+1. Subtracting the
  -- intro from the clock instead of taking it modulo the whole cycle is what
  -- keeps frame 1 from reappearing on every lap.
  while true do
    local duration = view.durations[i]
    if t < duration then return i end
    t = t - duration
    i = i + 1
    if i > view.frames then i = view.loopStart + 1 end
  end
end

-- frameImage cuts one frame out of the sheet's grid, once per view and index.
function Pack:frameImage(view, index)
  local cached = view.images[index]
  if cached ~= nil then return cached or nil end
  local ok, data = pcall(love.image.newImageData, view.cell, view.cell)
  if not ok or not data then
    view.images[index] = false
    return nil
  end
  local col, row = (index - 1) % view.columns, math.floor((index - 1) / view.columns)
  data:paste(view.data, 0, 0, col * view.cell, row * view.cell, view.cell, view.cell)
  local made, image = pcall(love.graphics.newImage, data)
  data:release()
  if not made or not image then
    view.images[index] = false
    return nil
  end
  image:setFilter("nearest", "nearest")
  view.images[index] = image
  return image
end

-- fitRect fits the Pokedex's fixed crop window (the largest pose's content box,
-- not a whole square cell) into the 64x64 picture the dex reserves. It never
-- enlarges, so a species that facets draws small stays small, and a wide
-- animation is not squeezed by the transparent margins around its wings. The
-- window is fixed, so the current frame animates without changing scale.
function Pack.fitRect(image, rect)
  local okCanvas, canvas = pcall(love.graphics.newCanvas, Pack.BOX, Pack.BOX, { dpiscale = 1 })
  if not okCanvas or not canvas then
    okCanvas, canvas = pcall(love.graphics.newCanvas, Pack.BOX, Pack.BOX)
  end
  if not okCanvas or not canvas then return nil end
  local scale = math.min(Pack.BOX / rect.w, Pack.BOX / rect.h, 1)
  local ok, err = pcall(function()
    image:setFilter("nearest", "nearest")
    local quad = love.graphics.newQuad(rect.x, rect.y, rect.w, rect.h, image:getDimensions())
    canvas:renderTo(function()
      love.graphics.push("all")
      local innerOk, innerErr = pcall(function()
        love.graphics.origin()
        love.graphics.setScissor()
        love.graphics.setStencilTest()
        love.graphics.setShader()
        love.graphics.clear(0, 0, 0, 0)
        love.graphics.setBlendMode("replace")
        love.graphics.setColor(1, 1, 1, 1)
        love.graphics.draw(image, quad,
          (Pack.BOX - rect.w * scale) / 2, (Pack.BOX - rect.h * scale) / 2, 0, scale, scale)
      end)
      love.graphics.pop()
      if not innerOk then error(innerErr, 0) end
    end)
  end)
  if not ok then
    canvas:release()
    return nil, tostring(err)
  end
  local okData, out = pcall(canvas.newImageData, canvas)
  canvas:release()
  if not okData or not out then return nil end
  local made, scaled = pcall(love.graphics.newImage, out)
  out:release()
  if not made or not scaled then return nil end
  scaled:setFilter("nearest", "nearest")
  return scaled
end

-- scaleCell draws a non-64 cell from an older pack, which carries no fitted
-- pose box, into the 64x64 box the Pokedex reserves.
function Pack.scaleCell(image, from, to)
  local okCanvas, canvas = pcall(love.graphics.newCanvas, to, to, { dpiscale = 1 })
  if not okCanvas or not canvas then
    okCanvas, canvas = pcall(love.graphics.newCanvas, to, to)
  end
  if not okCanvas or not canvas then return nil end
  local ok, err = pcall(function()
    image:setFilter("nearest", "nearest")
    canvas:renderTo(function()
      love.graphics.push("all")
      local innerOk, innerErr = pcall(function()
        love.graphics.origin()
        love.graphics.setScissor()
        love.graphics.setStencilTest()
        love.graphics.setShader()
        love.graphics.clear(0, 0, 0, 0)
        love.graphics.setBlendMode("replace")
        love.graphics.setColor(1, 1, 1, 1)
        love.graphics.draw(image, 0, 0, 0, to / from, to / from)
      end)
      love.graphics.pop()
      if not innerOk then error(innerErr, 0) end
    end)
  end)
  if not ok then
    canvas:release()
    return nil, tostring(err)
  end
  local okData, out = pcall(canvas.newImageData, canvas)
  canvas:release()
  if not okData or not out then return nil end
  local made, scaled = pcall(love.graphics.newImage, out)
  out:release()
  if not made or not scaled then return nil end
  scaled:setFilter("nearest", "nearest")
  return scaled
end

-- dexFrame is the Pokedex's view: the current animation frame cropped to the
-- fixed window of the largest pose and fitted into the 64x64 box the dex lays
-- out. A pack built before that metadata existed falls back to the current
-- frame scaled as a whole cell.
function Pack:dexFrame(nat, side, shiny, now)
  local view, err = self:view(nat, side, shiny)
  if not view then return nil, err end
  local rect = view.dexRect
  local index = self:frameIndex(view, now)
  local scaled = view.dex[index]
  if scaled == nil then
    local image = self:frameImage(view, index)
    if image and rect then
      scaled = Pack.fitRect(image, rect) or false
    elseif image and view.cell == Pack.BOX then
      scaled = image
    else
      scaled = image and Pack.scaleCell(image, view.cell, Pack.BOX) or false
    end
    view.dex[index] = scaled
  end
  if not scaled then return nil, "could not fit a dex-sized frame" end
  return { image = scaled, w = Pack.BOX, h = Pack.BOX,
           animated = view.frames > 1, shiny = shiny and true or false }
end

-- frame returns the engine's picture entry for one species, side and palette,
-- or nil when the pack does not serve it.
function Pack:frame(nat, side, shiny, now, female)
  local view, err = self:view(nat, side, shiny, female)
  if not view then return nil, err end
  local image = self:frameImage(view, self:frameIndex(view, now))
  if not image then return nil, "could not cut a frame image" end
  return { image = image, w = view.cell, h = view.cell, animated = view.frames > 1, shiny = shiny and true or false }
end

return Pack
