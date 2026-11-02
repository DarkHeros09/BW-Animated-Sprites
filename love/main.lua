-- Preview and runtime for an animated-sprite pack written by the Go importer.
--
-- The pack is the whole contract: manifest.json plus one strip per entry. This
-- app draws the strip's frames at the measured cell, on the measured ground
-- row, and reports what it read, so the same data can be checked by eye and by
-- the host game.
local Pack = require("pack")

local SIDEBAR = 320
local ROW = 34
local PAD = 16

local app = {
  pack = nil,
  error = nil,
  tried = {},
  index = 1,
  frame = 1,       -- 1-based frame inside the entry
  tick = 0,        -- ticks elapsed inside the current frame
  playing = true,
  debug = true,
  host = false,
  scale = 0,       -- 0 fits the window
  shot = nil,      -- --shot PATH
  selftest = false,
}

local fonts = {}

-- wrap breaks text to width using the given font.
local function wrap(text, font, width)
  local out, line = {}, ""
  for word in tostring(text):gmatch("%S+") do
    local candidate = line == "" and word or (line .. " " .. word)
    if font:getWidth(candidate) > width and line ~= "" then
      out[#out + 1] = line
      line = word
    else
      line = candidate
    end
  end
  if line ~= "" then out[#out + 1] = line end
  return out
end

-- fitText shortens text to width, so a long label cannot run into the next
-- column.
local function fitText(text, font, width)
  text = tostring(text)
  if font:getWidth(text) <= width then return text end
  while #text > 1 and font:getWidth(text .. "...") > width do
    text = text:sub(1, -2)
  end
  return text .. "..."
end

-- current returns the selected entry, or nil when the pack has none.
local function current()
  local pack = app.pack
  if not pack then return nil end
  return pack.entries[app.index]
end

-- selectEntry resets the animation state for a new entry.
local function selectEntry(index)
  local pack = app.pack
  if not pack then return end
  app.index = math.max(1, math.min(#pack.entries, index))
  app.frame = 1
  app.tick = 0
end

-- resolvePackDir finds a pack directory: the argument when given, otherwise
-- the demo pack next to this app. Returns the directory and what it tried.
local function resolvePackDir(given)
  local source = love.filesystem.getSource()
  local candidates = {}
  if given then
    candidates = { given, source .. "/" .. given, source .. "/../" .. given }
  else
    candidates = { source .. "/../demo/pack", source .. "/demo/pack", "demo/pack" }
  end
  for _, dir in ipairs(candidates) do
    local file = io.open(dir .. "/manifest.json", "rb")
    if file then
      file:close()
      return dir, candidates
    end
  end
  return nil, candidates
end

-- parseArgs separates our flags from the pack path.
local function parseArgs(args)
  local path, i = nil, 1
  while i <= #args do
    local value = args[i]
    if value == "--selftest" then
      app.selftest = true
    elseif value == "--no-debug" then
      app.debug = false
    elseif value == "--host" then
      app.host = true
    elseif value == "--shot" then
      i = i + 1
      app.shot = args[i]
    elseif value == "--entry" then
      i = i + 1
      app.shotEntry = tonumber(args[i])
    elseif value == "--frame" then
      i = i + 1
      app.shotFrame = tonumber(args[i])
    elseif value:sub(1, 2) ~= "--" then
      path = value
    end
    i = i + 1
  end
  return path
end

-- failLoad records a load failure and, under --selftest, exits instead of
-- waiting on a window nobody will look at.
local function failLoad(message)
  app.error = message
  if not app.selftest then return end
  print("FAIL  pack: " .. tostring(message))
  for _, dir in ipairs(app.tried or {}) do print("      tried " .. dir) end
  print("selftest: no pack loaded")
  love.event.quit(1)
end

function love.load(args)
  fonts.small = love.graphics.newFont(12)
  fonts.body = love.graphics.newFont(14)
  fonts.title = love.graphics.newFont(18)
  fonts.mono = love.graphics.newFont(13)
  love.graphics.setFont(fonts.body)

  local given = parseArgs(args or {})
  local dir, tried = resolvePackDir(given)
  app.tried = tried
  if not dir then
    failLoad("no sprite pack found")
    return
  end
  local pack, err = Pack.load(dir)
  if not pack then
    failLoad(err)
    return
  end
  app.pack = pack
  selectEntry(1)
  if app.shotEntry then selectEntry(app.shotEntry) end
  if app.shotFrame then
    local entry = current()
    for i = 1, math.min(app.shotFrame, entry.frames) - 1 do
      app.tick = app.tick + entry.durations[i]
    end
    app.frame = math.min(app.shotFrame, entry.frames)
    app.playing = false
  end
  if app.shot then
    app.playing = false
    app.shotPending = true
  end
  love.graphics.setBackgroundColor(0.07, 0.08, 0.10)
  if app.selftest then runSelftest() end
end

-- advance moves the playhead by dt seconds, using the entry's tick durations.
local function advance(entry, dt)
  if not app.playing or entry.error then return end
  app.tick = app.tick + dt * app.pack.tickRate
  local steps = 0
  while app.tick >= (entry.durations[app.frame] or 1) do
    app.tick = app.tick - entry.durations[app.frame]
    local next = app.frame + 1
    if next > entry.frames then next = entry.loopStart + 1 end
    app.frame = next
    steps = steps + 1
    if steps > 1000 then
      app.tick = 0
      break
    end
  end
end

function love.update(dt)
  if not app.pack then return end
  local entry = current()
  if entry then advance(entry, dt) end
end

-- drawError renders the load failure with what was tried.
local function drawError()
  local x, y, width = PAD, 60, love.graphics.getWidth() - PAD * 2
  love.graphics.setColor(1, 0.45, 0.4)
  love.graphics.print("cannot load a sprite pack", x, y, 0, 1.4, 1.4)
  y = y + 34
  love.graphics.setColor(1, 0.85, 0.8)
  for _, line in ipairs(wrap(app.error or "unknown error", fonts.body, width)) do
    love.graphics.print(line, x, y)
    y = y + fonts.body:getHeight() + 2
  end
  y = y + 10
  love.graphics.setColor(0.75, 0.8, 0.85)
  love.graphics.print("tried:", x, y)
  y = y + fonts.body:getHeight() + 2
  for _, dir in ipairs(app.tried) do
    love.graphics.print("  " .. dir, x, y)
    y = y + fonts.body:getHeight() + 2
  end
  y = y + 10
  love.graphics.print("build one with:  go run ./cmd/bwsprites demo -out demo/pack", x, y)
  y = y + fonts.body:getHeight() + 2
  love.graphics.print("or:              go run ./cmd/bwsprites export -rom ROM.nds -out pack", x, y)
  y = y + fonts.body:getHeight() + 2
  love.graphics.print("then:            love love/ demo/pack", x, y)
end

-- sidebar draws the entry list.
local function drawSidebar()
  local pack = app.pack
  local height = love.graphics.getHeight()
  love.graphics.setColor(0.10, 0.11, 0.13)
  love.graphics.rectangle("fill", 0, 0, SIDEBAR, height)
  love.graphics.setColor(0.18, 0.20, 0.24)
  love.graphics.rectangle("fill", SIDEBAR - 1, 0, 1, height)

  local y = PAD
  love.graphics.setFont(fonts.title)
  love.graphics.setColor(0.9, 0.94, 0.98)
  love.graphics.print("sprite pack", PAD, y)
  y = y + fonts.title:getHeight() + 2
  love.graphics.setFont(fonts.small)
  love.graphics.setColor(0.55, 0.62, 0.70)
  love.graphics.print(pack.importer .. "  " .. tostring(pack.source.gameCode or ""), PAD, y)
  y = y + fonts.small:getHeight() + 10

  local visible = math.floor((height - y - 90) / ROW)
  local first = math.max(1, math.min(app.index - math.floor(visible / 2), #pack.entries - visible + 1))
  first = math.max(1, first)
  for i = first, math.min(#pack.entries, first + visible - 1) do
    local entry = pack.entries[i]
    local rowY = y + (i - first) * ROW
    if i == app.index then
      love.graphics.setColor(0.20, 0.45, 0.42)
      love.graphics.rectangle("fill", 0, rowY - 2, SIDEBAR - 1, ROW)
    end
    love.graphics.setFont(fonts.mono)
    love.graphics.setColor(i == app.index and 1 or 0.78, 0.92, 0.88)
    love.graphics.print(fitText(entry.id, fonts.mono, SIDEBAR - PAD * 2), PAD, rowY)
    local text = entry.label ~= "" and entry.label or string.format("%d frames", entry.frames)
    if entry.error then text = "broken: " .. entry.error end
    love.graphics.setFont(fonts.small)
    love.graphics.setColor(entry.error and 1 or 0.55, entry.error and 0.45 or 0.62, 0.62)
    love.graphics.print(fitText(text, fonts.small, SIDEBAR - PAD * 2 - 8), PAD + 8, rowY + 16)
  end

  local footer = height - 74
  love.graphics.setColor(0.18, 0.20, 0.24)
  love.graphics.rectangle("fill", PAD - 8, footer - 8, SIDEBAR - PAD, 1)
  love.graphics.setFont(fonts.small)
  love.graphics.setColor(0.55, 0.62, 0.70)
  love.graphics.print("up/down  entry", PAD, footer)
  love.graphics.print("space    play/pause", PAD, footer + 15)
  love.graphics.print("left/right frame", PAD, footer + 30)
  love.graphics.print("d debug  h host cell  r restart  esc quit", PAD, footer + 45)
end

-- fitScale picks the integer zoom that keeps the cell inside the stage.
local function fitScale(entry)
  if app.scale ~= 0 then return app.scale end
  local width = love.graphics.getWidth() - SIDEBAR - PAD * 2
  local height = love.graphics.getHeight() - 150
  local scale = math.floor(math.min(width / entry.frameSize, height / entry.frameSize))
  return math.max(1, math.min(scale, 8))
end

-- stage draws one frame plus the measurement overlay.
local function drawStage()
  local entry = current()
  local u = entry.union
  local width = love.graphics.getWidth()
  local height = love.graphics.getHeight()
  local stageX, stageY = SIDEBAR, 0
  local stageW, stageH = width - SIDEBAR, height - 96

  love.graphics.setColor(0.07, 0.08, 0.10)
  love.graphics.rectangle("fill", stageX, stageY, stageW, stageH)

  love.graphics.setFont(fonts.title)
  love.graphics.setColor(0.9, 0.94, 0.98)
  love.graphics.print(entry.id, stageX + PAD, PAD)
  love.graphics.setFont(fonts.small)
  love.graphics.setColor(0.55, 0.62, 0.70)
  love.graphics.print(entry.label ~= "" and entry.label or "", stageX + PAD, PAD + 26)

  if entry.error then
    love.graphics.setColor(1, 0.5, 0.45)
    local y = stageY + stageH / 2 - 20
    for _, line in ipairs(wrap("entry is unusable: " .. entry.error, fonts.body, stageW - PAD * 2)) do
      love.graphics.print(line, stageX + PAD, y)
      y = y + fonts.body:getHeight() + 2
    end
    return
  end

  local scale = fitScale(entry)
  local size = entry.frameSize * scale
  local image, err = Pack.image(app.pack, entry)
  local top, bottom = stageY + 76, stageY + stageH - 14
  local x = stageX + math.floor((stageW - size) / 2)
  local y = top + math.max(0, math.floor((bottom - top - size) / 2))

  -- the measurement frame
  love.graphics.setColor(0.13, 0.15, 0.18)
  love.graphics.rectangle("fill", x, y, size, size)
  if image then
    love.graphics.setColor(1, 1, 1)
    if not entry.quads then
      entry.quads = {}
      for i = 1, entry.frames do
        entry.quads[i] = love.graphics.newQuad((i - 1) * entry.frameSize, 0,
          entry.frameSize, entry.frameSize, image:getDimensions())
      end
    end
    love.graphics.draw(image, entry.quads[app.frame], x, y, 0, scale, scale)
  else
    love.graphics.setColor(1, 0.5, 0.45)
    love.graphics.print("strip missing: " .. tostring(err), x + 8, y + 8)
  end

  if app.debug then
    -- 8px grid, so the cel grid of the source data stays visible
    love.graphics.setColor(1, 1, 1, 0.05)
    for gx = 0, entry.frameSize, 8 do
      love.graphics.line(x + gx * scale, y, x + gx * scale, y + size)
      love.graphics.line(x, y + gx * scale, x + size, y + gx * scale)
    end
    -- union box
    love.graphics.setColor(0.35, 0.85, 1, 0.9)
    love.graphics.setLineWidth(1)
    love.graphics.rectangle("line", x + u[1] * scale, y + u[2] * scale,
      (u[3] - u[1]) * scale, (u[4] - u[2]) * scale)
    -- ground row
    love.graphics.setColor(1, 0.35, 0.35, 0.9)
    love.graphics.line(x, y + entry.ground * scale + scale - 1,
      x + size, y + entry.ground * scale + scale - 1)
    love.graphics.setFont(fonts.small)
    love.graphics.print(string.format("ground row %d%s", entry.ground,
      entry.grounded and "" or "  (not grounded, pinned to the top)"),
      x + 4, y + size + 4)
    -- anchor
    if entry.anchorX >= 0 and entry.anchorX < entry.frameSize
      and entry.anchorY >= 0 and entry.anchorY < entry.frameSize then
      local ax, ay = x + entry.anchorX * scale, y + entry.anchorY * scale
      love.graphics.setColor(1, 0.75, 0.2, 0.95)
      love.graphics.line(ax - 6, ay, ax + 6, ay)
      love.graphics.line(ax, ay - 6, ax, ay + 6)
    end
    -- frame cell border
    love.graphics.setColor(1, 1, 1, 0.3)
    love.graphics.rectangle("line", x, y, size, size)
  end

  if app.host then
    -- how a 64x64 host cell consumes this measurement: the cell's bottom row
    -- is the ground, and the sprite is centred on its union.
    local hostSize = 64
    local unionCX = (u[1] + u[3]) / 2
    local hx = x + math.floor((unionCX - hostSize / 2) * scale)
    local hy = y + (entry.ground + 1 - hostSize) * scale
    love.graphics.setColor(1, 0.9, 0.4, 0.8)
    love.graphics.rectangle("line", hx, hy, hostSize * scale, hostSize * scale)
    love.graphics.print("host 64x64 cell", hx + 4, hy + 4)
    if entry.frameSize > hostSize then
      love.graphics.setColor(1, 0.75, 0.2)
      love.graphics.print(string.format("cell %dpx > host %dpx: draw at 1:%d",
        entry.frameSize, hostSize, entry.frameSize), x + 4, y - 18)
    end
  end

  love.graphics.setFont(fonts.small)
  love.graphics.setColor(0.55, 0.62, 0.70)
  local hud = string.format(
    "frame %d/%d  duration %d ticks  cell %dpx  union %dx%d  anchor %d,%d  cycle %d ticks%s",
    app.frame, entry.frames, entry.durations[app.frame] or 0, entry.frameSize,
    u[3] - u[1], u[4] - u[2], entry.anchorX, entry.anchorY, entry.cycleTicks,
    entry.cycleCapped and "  (capped)" or "")
  local barY = height - 92
  love.graphics.setColor(0.10, 0.11, 0.13)
  love.graphics.rectangle("fill", stageX, barY, stageW, height - barY)
  love.graphics.setColor(0.18, 0.20, 0.24)
  love.graphics.rectangle("fill", stageX, barY, stageW, 1)
  love.graphics.setColor(0.72, 0.80, 0.86)
  love.graphics.print(hud, stageX + PAD, barY + 10)
  love.graphics.setColor(0.55, 0.62, 0.70)
  love.graphics.print(string.format("%s  zoom x%d  tick %.2f  %s",
    app.playing and "playing" or "paused", fitScale(entry), app.tick,
    app.host and "host cell on" or "h: host cell"), stageX + PAD, barY + 30)
end

function love.draw()
  love.graphics.clear(0.07, 0.08, 0.10)
  if app.error then
    drawError()
    return
  end
  if app.selftest then return end
  drawSidebar()
  drawStage()
  if app.shotPending then
    app.shotPending = false
    captureShot()
  end
end

function love.keypressed(key)
  if key == "escape" then
    love.event.quit(0)
    return
  end
  if not app.pack then return end
  local entry = current()
  if key == "space" then
    app.playing = not app.playing
  elseif key == "up" then
    selectEntry(app.index - 1)
  elseif key == "down" then
    selectEntry(app.index + 1)
  elseif key == "r" then
    app.frame = 1
    app.tick = 0
  elseif key == "d" then
    app.debug = not app.debug
  elseif key == "h" then
    app.host = not app.host
  elseif key == "=" or key == "kp+" then
    app.scale = math.min(8, fitScale(entry) + 1)
  elseif key == "-" or key == "kp-" then
    app.scale = math.max(1, fitScale(entry) - 1)
  elseif key == "0" then
    app.scale = 0
  elseif key == "right" and entry and not entry.error then
    app.playing = false
    app.frame = app.frame % entry.frames + 1
    app.tick = 0
  elseif key == "left" and entry and not entry.error then
    app.playing = false
    app.frame = (app.frame - 2) % entry.frames + 1
    app.tick = 0
  end
end

-- selftest checks the pack headlessly: metadata invariants plus a simulated
-- cycle, so it runs without a visible window.
function runSelftest()
  local pack = app.pack
  local failures = 0
  local function check(ok, message)
    print((ok and "PASS  " or "FAIL  ") .. message)
    if not ok then failures = failures + 1 end
  end
  check(pack.tickRate == 60, "tick rate is 60")
  check(#pack.entries > 0, string.format("%d entries", #pack.entries))
  for _, entry in ipairs(pack.entries) do
    if entry.error then
      check(false, entry.id .. ": " .. entry.error)
    else
      local total = 0
      for _, duration in ipairs(entry.durations) do total = total + duration end
      check(total == entry.cycleTicks,
        string.format("%s: durations sum to %d of %d", entry.id, total, entry.cycleTicks))
      check(entry.union[4] - 1 <= entry.ground,
        string.format("%s: union bottom %d within ground %d", entry.id, entry.union[4] - 1, entry.ground))
      check(entry.frameSize >= 64,
        string.format("%s: cell %d >= facets 64", entry.id, entry.frameSize))
      local image, err = Pack.image(pack, entry)
      check(image ~= nil, string.format("%s: strip %s", entry.id, image and "loaded" or tostring(err)))
      if image then
        -- simulate one full cycle and confirm it returns to the loop start
        local frame, ticks, steps = 1, 0, 0
        while steps < entry.cycleTicks * 2 + 4 do
          ticks = ticks + entry.durations[frame]
          frame = frame + 1
          if frame > entry.frames then frame = entry.loopStart + 1 end
          steps = steps + 1
          if frame == entry.loopStart + 1 and ticks >= entry.cycleTicks then break end
        end
        check(ticks == entry.cycleTicks,
          string.format("%s: cycle advances %d ticks, metadata says %d", entry.id, ticks, entry.cycleTicks))
      end
    end
  end
  print(failures == 0 and "selftest: OK" or string.format("selftest: %d failure(s)", failures))
  love.event.quit(failures == 0 and 0 or 1)
end

-- captureShot writes one frame of the demo to app.shot, so the preview can be
-- checked in CI or pasted into the README without a human at the keyboard.
function captureShot()
  love.graphics.captureScreenshot(function(imagedata)
    local filedata = imagedata:encode("png")
    local file, err = io.open(app.shot, "wb")
    if not file then
      print("cannot write " .. tostring(app.shot) .. ": " .. tostring(err))
      love.event.quit(1)
      return
    end
    file:write(filedata:getString())
    file:close()
    print("wrote " .. app.shot)
    love.event.quit(0)
  end)
end