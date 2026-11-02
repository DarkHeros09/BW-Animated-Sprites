-- Loads a bwsprites pack and turns its sheets into frame images on demand.
-- Format 2 packs store square cells in a grid, one cell per frame at the art's
-- own size: the cell is the animation's union, so a view is never resampled and
-- a cell can be larger than the host's battle cell. Every entry point takes the
-- reader it needs, so this file can be exercised without the engine.
local Pack = {}

Pack.BOX = 64
Pack.MIN_SIZE = 64
-- A native cell is never shrunk to a cap: 256 is headroom for the art, not a
-- scale limit.
Pack.MAX_SIZE = 256
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

-- cropRect cuts a rectangle out of the sheet at 1:1 and returns it as an image.
-- The Pokedex picture is the animation's whole pose window, so every pose still
-- fits while it plays, and no pixel is resampled or dropped: the picture the
-- engine draws is the dump's own art.
function Pack.cropRect(data, rect)
  local ok, out = pcall(love.image.newImageData, rect.w, rect.h)
  if not ok or not out then return nil, "could not allocate the dex window" end
  local pasted, err = pcall(out.paste, out, data, 0, 0, rect.x, rect.y, rect.w, rect.h)
  if not pasted then
    out:release()
    return nil, tostring(err)
  end
  local made, image = pcall(love.graphics.newImage, out)
  out:release()
  if not made or not image then return nil, "could not make the dex picture" end
  image:setFilter("nearest", "nearest")
  return image
end

-- fitRect reduces the same pose window into a boxW x boxH picture, keeping the
-- whole window. Only the layouts that hold the cartridge's own 64x64 scale need
-- it -- the dex's size comparison beside the trainer and the credits roll -- and
-- it never enlarges.
function Pack.fitRect(image, rect, boxW, boxH)
  boxW, boxH = boxW or Pack.BOX, boxH or Pack.BOX
  local sheetW, sheetH = image:getDimensions()
  if type(rect) ~= "table" or (rect.w or 0) < 1 or (rect.h or 0) < 1
    or (rect.x or 0) < 0 or (rect.y or 0) < 0
    or rect.x + rect.w > sheetW or rect.y + rect.h > sheetH then
    return nil, "the pose window is outside its frame"
  end
  local okCanvas, canvas = pcall(love.graphics.newCanvas, boxW, boxH, { dpiscale = 1 })
  if not okCanvas or not canvas then
    okCanvas, canvas = pcall(love.graphics.newCanvas, boxW, boxH)
  end
  if not okCanvas or not canvas then return nil end
  local scale = math.min(boxW / rect.w, boxH / rect.h, 1)
  local width, height = rect.w * scale, rect.h * scale
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
          (boxW - width) / 2, boxH - height, 0, scale, scale)
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

-- dexFrame is the Pokedex's picture: the current animation frame cropped to the
-- window that encloses every pose, at 1:1. The window is fixed and never
-- reduced, so the dex shows the dump's pixels and the picture does not change
-- scale as the idle plays; the mod places it in the card and keeps it on screen.
-- A pack built before that metadata existed crops its whole cell instead.
function Pack:dexFrame(nat, side, shiny, now)
  local view, err = self:view(nat, side, shiny)
  if not view then return nil, err end
  local rect = view.dexRect or { x = 0, y = 0, w = view.cell, h = view.cell }
  local index = self:frameIndex(view, now)
  local cached = view.dex[index]
  if cached == nil then
    -- rect is in cell coordinates; the sheet holds the cells in a grid, so the
    -- frame's own cell has to be added or every pose would crop the first cell.
    local col = (index - 1) % view.columns
    local row = math.floor((index - 1) / view.columns)
    local at = { x = rect.x + col * view.cell, y = rect.y + row * view.cell, w = rect.w, h = rect.h }
    cached = Pack.cropRect(view.data, at) or false
    view.dex[index] = cached
  end
  if not cached then return nil, "could not cut a dex-sized frame" end
  return { image = cached, w = rect.w, h = rect.h,
           animated = view.frames > 1, shiny = shiny and true or false }
end

-- boxFrame is the picture the dex's size comparison and the credits roll want:
-- the same pose window reduced into the 64x64 box those layouts reserve, because
-- they compare it against the cartridge's own trainer and sprite scale.
function Pack:boxFrame(nat, side, shiny, now, boxW, boxH)
  boxW, boxH = boxW or Pack.BOX, boxH or Pack.BOX
  local view, err = self:view(nat, side, shiny)
  if not view then return nil, err end
  local rect = view.dexRect or { x = 0, y = 0, w = view.cell, h = view.cell }
  local index = self:frameIndex(view, now)
  local key = index .. "/" .. boxW .. "x" .. boxH
  local scaled = view.dex[key]
  if scaled == nil then
    local image = self:frameImage(view, index)
    scaled = image and Pack.fitRect(image, rect, boxW, boxH) or false
    view.dex[key] = scaled
  end
  if not scaled then return nil, "could not fit a boxed dex frame" end
  return { image = scaled, w = boxW, h = boxH,
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
