-- Regression harness for the "Cannot use object after it has been released"
-- crash in double battles. The engine keeps the Image it got from the mod in a
-- sprite/particle object for several frames; when the mod's LRU cache drops that
-- entry it used to call Image:release() and the engine's next draw crashed.
--
-- The harness drives the mod's real external.lua (through External.new and a
-- stubbed launcher pack) exactly as the engine does: it holds the first picture,
-- fills the cache until that entry is evicted, then draws the held picture. The
-- picture must still be drawable after eviction: the mod may drop its Lua
-- reference, never release an Image it handed to the engine.
local MOD_DIR = assert(os.getenv("MOD_DIR"), "MOD_DIR is not set")

local failures = 0
local function check(ok, label, detail)
  if ok then
    print("PASS  " .. label)
  else
    failures = failures + 1
    print("FAIL  " .. label .. (detail and ("  (" .. tostring(detail) .. ")") or ""))
  end
end

local function loadPart(name)
  local path = MOD_DIR .. "/" .. name
  local fh, err = io.open(path, "rb")
  if not fh then error("cannot read " .. path .. ": " .. tostring(err)) end
  local source = fh:read("*a")
  fh:close()
  local chunk, chunkErr = load(source, "@" .. path)
  if not chunk then error(chunkErr) end
  local ok, part = pcall(chunk)
  if not ok then error(part) end
  return part
end

local Pack = loadPart("pack.lua")
local External = loadPart("external.lua")

-- makePNG encodes one atlas. The frame cache test needs 193 distinct 64x64
-- frames; the size the mod validates is the atlas' own size, so the PNG bytes
-- are what the stubbed launcher pack serves.
local function makePNG(w, h, ink)
  local data = love.image.newImageData(w, h)
  if ink then
    for y = 8, h - 9 do
      for x = 8, w - 9 do
        data:setPixel(x, y, 0.2, 0.8, 0.3, 1)
      end
    end
  end
  local file = data:encode("png")
  local bytes = file:getString()
  file:release()
  data:release()
  return bytes
end

local function drawable(image)
  local ok, err = pcall(love.graphics.draw, image, 0, 0)
  return ok, err
end

-- tickNow maps frame i to a clock value whose floor(t*60) is exactly i-1. The
-- epsilon keeps a float (i-1)/60*60 that came out just below the integer from
-- selecting the previous frame, which would leave the cache short of an entry.
local function tickNow(i)
  return (i - 1) / 60 + 1e-6
end

-- stubRuntime builds the mod runtime from a fake launcher pack, the same way
-- main.lua calls External.new(Pack, mod).
local function stubRuntime(entries, blobs, metadata)
  local packs = {
    info = function() return { source = "release-after-release harness" } end,
    entries = function(_, _, _) return entries end,
    read = function(_, _, _, id) return blobs[id] end,
    metadata = function(_, _, _, id) return metadata and metadata[id] end,
  }
  local built, err = External.new(Pack, { packs = packs })
  assert(built and built.runtime, tostring(err or "External.new returned no runtime"))
  return built.runtime
end

-- flatView is an ordinary animation view: one sheet column, one 64x64 cell per
-- frame, one tick per frame. frameIndex(t) then returns frame t+1.
local function flatView(id, frames)
  local png = makePNG(64, 64 * frames, false)
  local durations = {}
  for i = 1, frames do durations[i] = 1 end
  local view = {
    id = id, file = id .. ".png", size = #png,
    width = 64, height = 64 * frames,
    sprite = { width = 64, height = 64, frames = frames, columns = 1,
      tickRate = 60, durations = durations, loopStartFrame = 0,
      cycleTicks = frames, cycleCapped = false },
  }
  return view, png
end

local function testFrameCache()
  local view, png = flatView("normal/25/front", 193)
  local runtime = stubRuntime({ view }, { [view.id] = png })
  local first = runtime:frame(25, "front", false, 0)
  check(first ~= nil and first.image ~= nil, "the frame cache returns a picture")
  -- 192 entries fill the cache; the 193rd evicts the first.
  for i = 2, 193 do runtime:frame(25, "front", false, tickNow(i)) end
  check(runtime.frames["normal/25/front:1"] == nil,
    "frame cache evicted the picture the engine still holds")
  local ok, err = drawable(first.image)
  check(ok, "a battle frame held by the engine survives a cache eviction", err)
end

local function testDexCache()
  local view, png = flatView("normal/25/front", 65)
  local runtime = stubRuntime({ view }, { [view.id] = png })
  local first = runtime:dexFrame(25, "front", false, 0)
  check(first ~= nil and first.image ~= nil, "the dex cache returns a picture")
  for i = 2, 65 do runtime:dexFrame(25, "front", false, tickNow(i)) end
  check(runtime.dex["normal/25/front:1"] == nil,
    "dex cache evicted the picture the engine still holds")
  local ok, err = drawable(first.image)
  check(ok, "a Pokedex picture held by the engine survives a cache eviction", err)
end

local function testBoxCache()
  local view, png = flatView("normal/25/front", 1)
  local runtime = stubRuntime({ view }, { [view.id] = png })
  local first = runtime:boxFrame(25, "front", false, 0, 64, 64)
  check(first ~= nil and first.image ~= nil, "the box cache returns a picture")
  -- 32 distinct boxes fill the cache; the 33rd evicts the first.
  for w = 65, 96 do runtime:boxFrame(25, "front", false, 0, w, 64) end
  check(runtime.boxes["normal/25/front:1:64x64"] == nil,
    "box cache evicted the picture the engine still holds")
  local ok, err = drawable(first.image)
  check(ok, "a 64x64 box picture held by the engine survives a cache eviction", err)
end

-- partsPack is a capped view like the exporter publishes: the battle picture is
-- composed from the part tracks, and the composed image is cached separately
-- from the 64x64 cell made for the battle.
local function partsPack()
  local png = makePNG(64, 64, true)
  local view = {
    id = "normal/30/front", file = "normal/30/front.png", size = #png,
    width = 64, height = 64,
    sprite = { width = 64, height = 64, frames = 1, columns = 1,
      tickRate = 60, durations = { 1 }, loopStartFrame = 0, cycleTicks = 1,
      cycleCapped = true, partsEntry = "parts/30/front", partsPalette = "normal" },
  }
  local atlas = {
    id = "parts/30/front", file = "parts/30/front.png", size = #png,
    width = 64, height = 64,
    sprite = { kind = "parts", format = "gen5-parts", width = 64, height = 64,
      frames = 1, columns = 1, tickRate = 60 },
    metadata = { file = "parts/30/front.lua", size = 1 },
  }
  -- 17 one-tick states make 17 distinct composed signatures, one more than
  -- PARTS_IMAGE_CACHE (16), so the first picture is evicted.
  local states, runs = {}, {}
  for i = 1, 17 do
    local o = (i - 1) * 9
    states[o + 1] = 1
    states[o + 2], states[o + 3], states[o + 4], states[o + 5] = 0, 0, 64, 64
    states[o + 6], states[o + 7], states[o + 8], states[o + 9] = 1, 0, 0, 0
    runs[#runs + 1] = 1
    runs[#runs + 1] = i
  end
  local meta = {
    format = "gen5-parts", version = 1, tickRate = 60,
    width = 64, height = 64, atlasWidth = 64, atlasHeight = 64,
    pieces = { 0, 0, 64, 64, 0, 0 },
    tracks = { { intro = 0, period = 17, states = states, runs = runs } },
    palettes = {
      normal = { 255, 255, 255, 255, 255, 0, 0, 255 },
      shiny = { 255, 255, 255, 255, 0, 0, 255, 255 },
    },
  }
  return { view, atlas }, { [view.id] = png, [atlas.id] = png }, { [atlas.id] = meta }
end

local function testPartsCellCache()
  local entries, blobs, meta = partsPack()
  local runtime = stubRuntime(entries, blobs, meta)
  local first = runtime:frame(30, "front", false, 0)
  check(first ~= nil and first.image ~= nil, "the parts cell cache returns a picture")
  for tick = 1, 16 do runtime:frame(30, "front", false, tick / 60) end
  check(runtime.partCells["normal/30/front@1"] == nil,
    "parts cell cache evicted the picture the engine still holds")
  local ok, err = drawable(first.image)
  check(ok, "a composed part cell held by the engine survives a cache eviction", err)
end

local function testPartsImageCache()
  local entries, blobs, meta = partsPack()
  local runtime = stubRuntime(entries, blobs, meta)
  -- dexFrame returns composeParts' own native-size image, which is the picture
  -- a double battle's particle sprite keeps across draws.
  local first = runtime:dexFrame(30, "front", false, 0)
  check(first ~= nil and first.image ~= nil, "the composed part image is returned")
  for tick = 1, 16 do runtime:dexFrame(30, "front", false, tick / 60) end
  check(runtime.partImages["parts/30/front|normal@1"] == nil,
    "part image cache evicted the picture the engine still holds")
  local ok, err = drawable(first.image)
  check(ok, "a composed part image held by the engine survives a cache eviction", err)
end

local function main()
  testFrameCache()
  testDexCache()
  testBoxCache()
  testPartsCellCache()
  testPartsImageCache()
  return failures
end

function love.load()
  local ok, result = pcall(main)
  if not ok then
    print("FAIL  harness raised: " .. tostring(result))
    failures = failures + 1
  end
  print(failures == 0 and "selftest: OK" or ("selftest: " .. failures .. " failure(s)"))
  love.event.quit(failures == 0 and 0 or 1)
end
