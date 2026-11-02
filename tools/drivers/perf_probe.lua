-- Driver: measure the imported-pack runtime after the frame-clock and texture
-- release fixes: raw call cost and real frame pacing in a live battle.
io.stdout:setvbuf("no")
local U = require("tests.drivers.util")
return function(game)
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
  local foeSp = internalFor(6)
  session.party = {}
  Party.giveMon(session, internalFor(25), 50)
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
  for _ = 1, 120 do U.wait(1) end
  Pokemon.frontPic(foeSp, 0, false)
  local t0 = love.timer.getTime()
  for _ = 1, 20000 do Pokemon.frontPic(foeSp, 0, false) end
  local perCall = (love.timer.getTime() - t0) / 20000 * 1000
  print(string.format("perf: frontPic %.3f ms/call", perCall))
  local frames, t1 = 600, love.timer.getTime()
  for _ = 1, frames do U.wait(1) end
  local perFrame = (love.timer.getTime() - t1) / frames * 1000
  print(string.format("perf: %.2f ms/frame over %d frames", perFrame, frames))
  local ok = perCall < 0.25 and perFrame < 25
  print(ok and "perf: OK" or "perf: SLOW")
  love.event.quit(ok and 0 or 1)
end
