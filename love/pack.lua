-- Loads and validates a sprite pack written by the Go tool (internal/pack).
--
-- The pack lives on the real filesystem, so this uses io.open rather than
-- love.filesystem: the previewer has to be able to open a pack anywhere,
-- including one next to the ROM outside the game's source directory.
local json = require("json")

local Pack = {}

-- field helpers: they return the value or raise a message the caller prefixes
-- with the entry id, so a bad pack names the field that is wrong.
local function integer(value, field)
  if type(value) ~= "number" or value ~= math.floor(value) then
    error(string.format("%s is %s, want an integer", field, tostring(value)), 0)
  end
  return value
end

local function within(value, field, low, high)
  integer(value, field)
  if value < low or value > high then
    error(string.format("%s is %d, want %d..%d", field, value, low, high), 0)
  end
  return value
end

-- readFile reads a whole file from the real filesystem.
local function readFile(path)
  local file, err = io.open(path, "rb")
  if not file then
    return nil, err
  end
  local data = file:read("*a")
  file:close()
  if not data then
    return nil, "could not read " .. path
  end
  return data
end

Pack.readFile = readFile

-- join builds a path with the pack's own separator.
local function join(dir, name)
  if dir:sub(-1) == "/" or dir:sub(-1) == "\\" then
    return dir .. name
  end
  return dir .. "/" .. name
end

-- validate checks one entry's metadata, mirroring internal/pack/verify.go.
local function validateEntry(raw, frameSize)
  if type(raw) ~= "table" then
    error("entry is not an object", 0)
  end
  if type(raw.id) ~= "string" or raw.id == "" then
    error("id is missing", 0)
  end
  local size = within(raw.frameSize, "frameSize", 1, 256)
  if frameSize ~= 0 and size ~= frameSize then
    error(string.format("frameSize %d disagrees with the pack's %d", size, frameSize), 0)
  end
  local frames = within(raw.frames, "frames", 1, 4096)
  within(raw.ground, "ground", 0, size - 1)
  within(raw.loopStart, "loopStart", 0, frames - 1)
  within(raw.cycleTicks, "cycleTicks", 1, 1073741824)
  if type(raw.strip) ~= "string" or raw.strip == "" then
    error("strip is missing", 0)
  end
  if type(raw.durations) ~= "table" or #raw.durations ~= frames then
    error(string.format("%d frames but %d durations", frames,
      type(raw.durations) == "table" and #raw.durations or 0), 0)
  end
  if type(raw.offsets) ~= "table" or #raw.offsets ~= frames then
    error(string.format("%d frames but %d offsets", frames,
      type(raw.offsets) == "table" and #raw.offsets or 0), 0)
  end
  local total = 0
  for i, duration in ipairs(raw.durations) do
    within(duration, string.format("duration %d", i), 1, 1073741824)
    total = total + duration
  end
  if total ~= raw.cycleTicks then
    error(string.format("durations add up to %d ticks but the cycle is %d", total, raw.cycleTicks), 0)
  end
  for i, offset in ipairs(raw.offsets) do
    if type(offset) ~= "table" or #offset ~= 2 then
      error(string.format("offset %d is not a pair", i), 0)
    end
    within(offset[1], string.format("offset %d x", i), 0, size - 1)
    within(offset[2], string.format("offset %d y", i), 0, size - 1)
  end
  local union = raw.union
  if type(union) ~= "table" or #union ~= 4 then
    error("union is not four numbers", 0)
  end
  for i = 1, 4 do
    within(union[i], string.format("union[%d]", i), 0, size)
  end
  if union[3] <= union[1] or union[4] <= union[2] then
    error(string.format("union %d,%d,%d,%d is empty", union[1], union[2], union[3], union[4]), 0)
  end
  return raw
end

-- load reads manifest.json, validates it and returns a pack. Entry images are
-- loaded lazily so one missing strip does not hide the rest.
function Pack.load(dir)
  local text, err = readFile(join(dir, "manifest.json"))
  if not text then
    return nil, string.format("no manifest.json in %s (%s)", dir, tostring(err))
  end
  local manifest, decodeError = json.decode(text)
  if not manifest then
    return nil, "manifest.json: " .. tostring(decodeError)
  end
  if type(manifest) ~= "table" then
    return nil, "manifest.json: top level is not an object"
  end
  if integer(manifest.format, "format") ~= 1 then
    return nil, string.format("manifest.json: format %s, want 1 (rebuild the pack)",
      tostring(manifest.format))
  end
  local tickRate = within(manifest.tickRate, "tickRate", 1, 1000)
  local frameSize = integer(manifest.frameSize, "frameSize")
  if frameSize < 0 or frameSize > 256 then
    return nil, string.format("manifest.json: frameSize %d outside 0..256", frameSize)
  end
  if type(manifest.entries) ~= "table" or #manifest.entries == 0 then
    return nil, "manifest.json: no entries"
  end

  local pack = {
    dir = dir,
    tickRate = tickRate,
    frameSize = frameSize,
    importer = tostring(manifest.importer or ""),
    source = type(manifest.source) == "table" and manifest.source or {},
    entries = {},
    byID = {},
  }
  for index, raw in ipairs(manifest.entries) do
    local id = type(raw) == "table" and type(raw.id) == "string" and raw.id or ""
    local ok, entry = pcall(validateEntry, raw, frameSize)
    if not ok then
      entry = {
        id = id ~= "" and id or string.format("#%d", index),
        error = tostring(entry),
        frames = 0,
        frameSize = 64,
        ground = 63,
        durations = {},
        offsets = {},
        union = { 0, 0, 64, 64 },
      }
    end
    entry.label = entry.label or ""
    pack.entries[index] = entry
    pack.byID[entry.id] = entry
  end
  return pack
end

-- image loads an entry's strip once and caches it. It returns the image, or
-- nil and a message when the strip is missing, unreadable or the wrong size.
function Pack.image(pack, entry)
  if entry.image then return entry.image end
  if entry.imageError then return nil, entry.imageError end
  local path = join(pack.dir, entry.strip)
  local ok, result, err = pcall(function()
    local bytes, readError = readFile(path)
    if not bytes then return nil, readError end
    local filedata = love.filesystem.newFileData(bytes, entry.strip)
    local imagedata = love.image.newImageData(filedata)
    local width, height = imagedata:getDimensions()
    local wantWidth, wantHeight = entry.frameSize * entry.frames, entry.frameSize
    if width ~= wantWidth or height ~= wantHeight then
      return nil, string.format("%s is %dx%d, want %dx%d", entry.strip,
        width, height, wantWidth, wantHeight)
    end
    local image = love.graphics.newImage(imagedata)
    image:setFilter("nearest", "nearest")
    return image, nil
  end)
  if not ok then
    entry.imageError = tostring(result)
    return nil, entry.imageError
  end
  if not result then
    entry.imageError = tostring(err)
    return nil, entry.imageError
  end
  entry.image = result
  return result, nil
end

return Pack
