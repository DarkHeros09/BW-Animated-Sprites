-- Driver: hold a wild Charizard battle long enough to cross the exporter's
-- 240 tick prefix and photograph the foe before and after.
io.stdout:setvbuf("no")
local U = require("tests.drivers.util")
local DIR = os.getenv("POKEPORT_SHOT_DIR") or "build/shots"
return function(game)
  local fails = 0
  local function result(ok, label)
    print((ok and "PASS " or "FAIL ") .. label)
    if not ok then fails = fails + 1 end
    return ok
  end
  local function shot(path)
    os.remove(path)
    love.graphics.captureScreenshot(function(imagedata)
      local f = io.open(path, "wb")
      if f then f:write(imagedata:encode("png"):getString()); f:close() end
    end)
    for _ = 1, 60 do
      U.wait(1)
      local f = io.open(path, "rb")
      if f then f:close(); print("driver: shot " .. path); return true end
    end
    print("driver: shot did not land " .. path)
    return false
  end
  for _ = 1, 900 do
    if game.phase == "boot" and game.boot then break end
    U.wait(1)
  end
  game:_handleBootAction({ action = "new_game", name = "RED" })
  U.wait(180)
  local Runtime = require("src.core.game3.runtime")
  local Party = require("src.core.game3.party")
  local BattleBridge = require("src.core.game3.battle_bridge")
  local Battle = require("src.core.game3.battle")
  local Ui = require("src.core.game3.battle.ui")
  local Pokemon = require("src.core.game3.pokemon")
  local session = Runtime.getSession()
  local function internalFor(nat)
    for sp = 1, 440 do
      local ok, n = pcall(Pokemon.national, sp)
      if ok and n == nat then return sp end
    end
  end
  local foeSp, ownSp = internalFor(6), internalFor(25)
  session.party = {}
  Party.giveMon(session, ownSp, 50)
  BattleBridge.startWild(Runtime._mod, game, { species = foeSp, level = 45 }, { fade = false })
  local lastTap, f = 0, 0
  for _ = 1, 3000 do
    f = f + 1
    if Battle.isActive() and Battle._phase == "command" and Ui._mode == "menu" then break end
    if Ui.dialogPending and Ui.dialogPending() and f - lastTap >= 14 then
      lastTap = f; U.tap(game, "a")
    else
      U.wait(1)
    end
  end
  result(Battle.isActive() and Battle._phase == "command", "the Charizard battle reached the command menu")
  local first = Pokemon.frontPic(foeSp, 0, false)
  result(type(first) == "table" and first.w >= 64, "the foe Charizard front is served (" ..
    tostring(first and first.w) .. "px)")
  shot(DIR .. "/foe-before.png")
  -- 6 seconds at 60 Hz crosses the exporter's 240 tick prefix.
  local changed, seen = false, first and first.image
  for _ = 1, 360 do
    U.wait(1)
    local pic = Pokemon.frontPic(foeSp, 0, false)
    if pic and pic.image ~= seen then changed = true end
    seen = pic and pic.image or seen
  end
  result(changed, "the foe keeps animating past the 240 tick prefix")
  shot(DIR .. "/foe-after.png")
  print("driver done fails=" .. fails)
  love.event.quit(fails == 0 and 0 or 1)
end
