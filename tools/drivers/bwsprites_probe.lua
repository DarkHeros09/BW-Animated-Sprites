-- Driver: boot FireRed, start a wild battle and photograph the player and
-- enemy pictures so the bwsprites mod can be checked in the real renderer.
io.stdout:setvbuf("no")
print("driver: loading")
local U = require("tests.drivers.util")
local DIR = os.getenv("POKEPORT_SHOT_DIR") or "build/shots"
print("driver: util loaded")

return function(game)
  print("driver: coroutine start phase=" .. tostring(game and game.phase))
  local fails = 0
  local function result(ok, label)
    print((ok and "PASS " or "FAIL ") .. label)
    if not ok then fails = fails + 1 end
    return ok
  end

  local function shot(path)
    os.remove(path)
    print("driver: capture request " .. path)
    love.graphics.captureScreenshot(function(imagedata)
      local fd = imagedata:encode("png")
      local f = io.open(path, "wb")
      if f then
        f:write(fd:getString())
        f:close()
      end
    end)
    for _ = 1, 60 do
      U.wait(1)
      local f = io.open(path, "rb")
      if f then
        f:close()
        print("driver: capture landed " .. path)
        return true
      end
    end
    print("driver: capture did not land " .. path)
    return false
  end

  local waited = 0
  for _ = 1, 900 do
    if game.phase == "boot" and game.boot then break end
    waited = waited + 1
    U.wait(1)
  end
  print("driver: boot wait " .. waited .. " frames, phase=" .. tostring(game.phase))
  game:_handleBootAction({ action = "new_game", name = "RED" })
  U.wait(180)

  local Runtime = require("src.core.game3.runtime")
  local Party = require("src.core.game3.party")
  local BattleBridge = require("src.core.game3.battle_bridge")
  local Battle = require("src.core.game3.battle")
  local Ui = require("src.core.game3.battle.ui")
  local Pokemon = require("src.core.game3.pokemon")
  local PicCoords = require("src.core.game3.battle.pic_coords")
  local CENTRED = Ui.PIC_CENTRE == true

  local session = Runtime.getSession()
  if not result(session ~= nil, "a Gen 3 session is on the field") then
    love.event.quit(1)
    return
  end
  session.party = {}

  local function internalFor(nat)
    for sp = 1, 440 do
      local ok, n = pcall(Pokemon.national, sp)
      if ok and n == nat then return sp end
    end
  end
  local foeNat, ownNat = 25, 6
  local foeSp = internalFor(foeNat)
  local ownSp = internalFor(ownNat)
  if not result(foeSp ~= nil and ownSp ~= nil, "species ids resolve (foe " .. tostring(foeSp) ..
    ", own " .. tostring(ownSp) .. ")") then
    love.event.quit(1)
    return
  end

  Party.giveMon(session, ownSp, 50)
  local started, err = BattleBridge.startWild(Runtime._mod, game, { species = foeSp, level = 45 },
    { fade = false })
  result(started == true, "wild battle started " .. tostring(err or ""))

  local lastTap = 0
  local function pump_text(f)
    if Ui.dialogPending and Ui.dialogPending() and f - lastTap >= 14 then
      lastTap = f
      U.tap(game, "a")
      return true
    end
    return false
  end
  local function at_command()
    return Battle.isActive() and Battle._phase == "command" and Ui._mode == "menu"
  end
  local f = 0
  for _ = 1, 3000 do
    f = f + 1
    if at_command() then break end
    if not pump_text(f) then U.wait(1) end
  end
  result(at_command(), "the command menu is up")

  local front = Pokemon.frontPic(foeSp, 0, false)
  result(type(front) == "table" and front.image ~= nil and front.w == front.h
    and front.w >= 64 and front.w <= 160,
    "Pokemon.frontPic returns a " .. tostring(front and front.w) .. "px picture (" ..
      (CENTRED and "centre anchor" or "fixed origin") .. ")")
  result(front ~= nil and front.animated == true, "the foe picture is animated")
  result(type(PicCoords.front[foeSp]) == "number", "the foe keeps a numeric y offset (" ..
    tostring(PicCoords.front[foeSp]) .. ")")
  local back = Pokemon.backPic(ownSp, 0, false)
  result(type(back) == "table" and back.image ~= nil and back.w == back.h
    and back.w >= 64 and back.w <= 112,
    "Pokemon.backPic returns a " .. tostring(back and back.w) .. "px picture")
  -- The user's Charizard must keep Black/White's native back size instead of
  -- being squeezed into the 64px cartridge cell. 112 is the host cap.
  result(back ~= nil and back.w >= 96,
    "the player's Charizard back keeps its native cell (" .. tostring(back and back.w) ..
      "px, want at least 96)")
  result(front ~= nil and front.w == 64,
    "the foe's Pikachu is not enlarged past the 64px host cell (" ..
      tostring(front and front.w) .. "px)")
  -- Fronts keep the Black/White art's native pixels: no 72/120 scale-down, so
  -- moving wings stay sharp. Look species up by national dex because a Gen 3
  -- cartridge's internal species ids are not national dex ids.
  local function nationalFront(nat)
    local ok, sp = pcall(Pokemon.speciesFromNational, nat)
    if not ok or type(sp) ~= "number" then return nil end
    return Pokemon.frontPic(sp, 0, false)
  end
  -- Groudon's capped back atlas is 85x79 while its part tracks compose to
  -- 89x79; the runtime grows that cell to 92 from the metadata. Before the fix
  -- this raised "bad part metadata for parts/383/back" during a battle draw.
  do
    local groudonSp = internalFor(383)
    local gBack = groudonSp and Pokemon.backPic(groudonSp, 0, false)
    result(type(gBack) == "table" and gBack.image ~= nil and gBack.w == 92
      and gBack.h == 92 and gBack.animated == true,
      "Groudon's composed back fits its grown 92px cell ("
        .. tostring(gBack and gBack.w) .. "x" .. tostring(gBack and gBack.h) .. ")")
  end
  local charFront = Pokemon.frontPic(ownSp, 0, false)
  result(charFront ~= nil and charFront.w == 92 and charFront.h == 92,
    "the opponent Charizard keeps its native 92px cell (" ..
      tostring(charFront and charFront.w) .. "px)")
  local pidgeotto = nationalFront(17)
  result(pidgeotto ~= nil and pidgeotto.w == 104 and pidgeotto.h == 104,
    "Pidgeotto's front keeps its native 104px cell (" ..
      tostring(pidgeotto and pidgeotto.w) .. "px)")
  local fearow = nationalFront(22)
  result(fearow ~= nil and fearow.w == 96 and fearow.h == 96,
    "Fearow's front keeps its native 96px cell (" ..
      tostring(fearow and fearow.w) .. "px)")
  local lugia = nationalFront(249)
  result(lugia ~= nil and lugia.w == 156 and lugia.h == 156,
    "Lugia's wide opponent front keeps its native 156px cell (" ..
    tostring(lugia and lugia.w) .. "px)")
  local rayquaza = nationalFront(382)
  result(rayquaza ~= nil and rayquaza.w == 100 and rayquaza.h == 100,
    "Rayquaza's front keeps its native 100px cell (" ..
    tostring(rayquaza and rayquaza.w) .. "px)")
  -- Freeze the animation clock so the two calls pick the same frame; the cache
  -- then hands back the same image object only when the same view is served.
  local realGetTime = love.timer.getTime
  love.timer.getTime = function() return 1000.0 end
  local malePika = Pokemon.frontPic(foeSp, 0, false)
  local femalePika = Pokemon.frontPic(foeSp, 0, false, 42, nil, { gender = "F" })
  result(femalePika ~= nil and femalePika.image ~= nil and malePika ~= nil
    and femalePika.image ~= malePika.image,
    "a female Pikachu gets the distinct female front picture")
  local maleAgain = Pokemon.frontPic(foeSp, 0, false, 42, nil, { gender = "M" })
  -- The engine's own battler path goes through Ui.sidePic, which still has the
  -- battler; some builds' Pokemon.backPic shim drops it, so check the live seam.
  local maleBack = Ui.sidePic(true, false, foeSp, 0, false, nil, { gender = "M" })
  local femaleBack = Ui.sidePic(true, false, foeSp, 0, false, nil, { gender = "F" })
  result(femaleBack ~= nil and femaleBack.image ~= nil and maleBack ~= nil
    and femaleBack.image ~= maleBack.image and femaleBack.w == 64,
    "a female Pikachu gets the distinct female back picture")
  love.timer.getTime = realGetTime
  -- The harness proves the male view is chosen; here only require that saying
  -- "male" resolves a picture rather than falling through to the cartridge.
  result(maleAgain ~= nil and maleAgain.image ~= nil and maleAgain.animated == true
    and maleAgain.w == malePika.w,
    "a male Pikachu resolves the male front picture")
  -- Alpha bounding box of a picture at its own resolution, so a native cell
  -- (100x100 and up) can be inspected just like a 64x64 dex frame.
  local function inkBox(image)
    local iw, ih = image:getDimensions()
    local canvas = love.graphics.newCanvas(iw, ih, { dpiscale = 1 })
    canvas:renderTo(function()
      love.graphics.push("all")
      love.graphics.origin()
      love.graphics.setScissor()
      love.graphics.setStencilTest()
      love.graphics.setShader()
      love.graphics.clear(0, 0, 0, 0)
      love.graphics.setBlendMode("replace")
      love.graphics.setColor(1, 1, 1, 1)
      -- Explicit origin 0,0 so the mod's Pokedex placement wrapper leaves the
      -- measurement alone; a bare draw would be moved into the card area.
      love.graphics.draw(image, 0, 0, 0, 1, 1, 0, 0)
      love.graphics.pop()
    end)
    local data = canvas:newImageData()
    canvas:release()
    local minX, minY, maxX, maxY = iw, ih, -1, -1
    for y = 0, ih - 1 do
      for x = 0, iw - 1 do
        local _, _, _, a = data:getPixel(x, y)
        if a > 0 then
          if x < minX then minX = x end
          if y < minY then minY = y end
          if x > maxX then maxX = x end
          if y > maxY then maxY = y end
        end
      end
    end
    data:release()
    return minX, minY, maxX - minX + 1, maxY - minY + 1
  end
  local function inkSize(image)
    local _, _, w, h = inkBox(image)
    return w, h
  end

  local displayedBack = PicCoords.back[ownSp]
  -- The enemy healthbox covers scene rows 14..45 in singles, so a tall own
  -- back's first opaque row must be on row 46 or later. Only the y padding may
  -- move; the cell size above must stay native.
  local ownBase = 80
  do
    local ok, base = pcall(PicCoords.battlerCoords, false, 0)
    if ok and type(base) == "table" and tonumber(base.y) then ownBase = base.y end
  end
  if back and displayedBack then
    local originY = CENTRED and back.w / 2 or 32
    local cellTop = ownBase + displayedBack + 4 - originY
    local _, inkY = inkBox(back.image)
    local inkTop = cellTop + inkY
    result(inkTop >= 46, "the player's back starts below the enemy healthbox (ink top " ..
      tostring(inkTop) .. ", cell top " .. tostring(cellTop) .. ", want 46 or lower)")
  end

  -- The OPTION menu seam: the mod appends a BW SPRITES group with one row per
  -- side. Open the real menu, open the group, flip MY POKEMON to DEFAULT and
  -- check the cartridge picture and its y padding come back. FireRed/LeafGreen
  -- and Emerald have separate OPTION menus behind one redirect, so open the
  -- redirected one and read whichever answered.
  local FRMenu = require("src.ui.game3.option_menu")
  local RseMenu = require("src.ui.game3.rse.option_menu")

  local function openMenu()
    FRMenu.show({ session = session, game = game })
    U.wait(5)
    if type(FRMenu._pages) == "table" and #FRMenu._pages > 0 then
      return FRMenu, FRMenu._pages, FRMenu._ctx
    end
    local st = RseMenu._st
    return RseMenu, st and st.pages, st and st.ctx
  end

  local function rowsOf(page)
    local byId = {}
    for _, row in ipairs(page and page.rows or {}) do byId[row.id] = row end
    return byId
  end

  local menu, pages, ctx = openMenu()
  local groupRow = rowsOf(pages and pages[1])["group.bwSprites"]
  result(groupRow ~= nil, "OPTION lists a BW SPRITES row")
  if groupRow then
    result(tostring(groupRow.value(ctx)):find("2 OPTIONS", 1, true) ~= nil,
      "the group reports its two rows (" .. tostring(groupRow.value(ctx)) .. ")")
    result(shot(DIR .. "/bwsprites_options.png"), "captured the OPTION page")
    groupRow.activate(ctx)
    U.wait(4)
    local member = rowsOf(pages and pages[#pages])
    result(member.bwPlayer ~= nil and member.bwFoe ~= nil,
      "the group opens MY POKEMON and OPPONENT")
    result(shot(DIR .. "/bwsprites_options_bw.png"), "captured the BW SPRITES page")
    if member.bwPlayer and member.bwFoe then
      member.bwPlayer.step(ctx, 1)
      U.wait(2)
      local cartridgeBack = PicCoords.back[ownSp]
      result(type(cartridgeBack) == "number", "MY POKEMON at DEFAULT restores the y padding (got " ..
        tostring(cartridgeBack) .. ")")
      local vanillaBack = Pokemon.backPic(ownSp, 0, false)
      result(type(vanillaBack) == "table" and vanillaBack.image ~= nil
        and vanillaBack.animated ~= true, "MY POKEMON at DEFAULT serves the cartridge back picture")
      local stillBw = Pokemon.frontPic(foeSp, 0, false)
      result(stillBw ~= nil and stillBw.animated == true,
        "the opponent keeps BW art while MY POKEMON is DEFAULT")
      menu.close()
      U.wait(20)
      result(shot(DIR .. "/bwsprites_mixed.png"), "captured a battle with mixed art")
      menu, pages, ctx = openMenu()
      groupRow = rowsOf(pages and pages[1])["group.bwSprites"]
      if groupRow then
        groupRow.activate(ctx)
        U.wait(4)
        local again = rowsOf(pages and pages[#pages])
        again.bwPlayer.step(ctx, 1)
        U.wait(2)
        local bwBack = Pokemon.backPic(ownSp, 0, false)
        result(bwBack ~= nil and bwBack.animated == true, "the player is animated again")
        if bwBack then
          result(PicCoords.back[ownSp] == displayedBack,
            "MY POKEMON back on BW re-pins the same native cell and top clearance (got " ..
              tostring(PicCoords.back[ownSp]) .. ", want " .. tostring(displayedBack) .. ")")
        end
      end
      menu.close()
      U.wait(4)
      result(displayedBack ~= nil, "the engine had a y padding to restore")
    else
      menu.close()
    end
  else
    menu.close()
  end


  U.wait(10)
  result(shot(DIR .. "/bwsprites_battle_a.png"), "captured the first battle frame")
  local firstImage = Pokemon.frontPic(foeSp, 0, false)
  -- Frame durations vary and the animation loops, so wait until the picture
  -- actually changes instead of guessing a fixed delay.
  local laterImage = firstImage
  for _ = 1, 240 do
    U.wait(1)
    laterImage = Pokemon.frontPic(foeSp, 0, false)
    if firstImage and laterImage and laterImage.image ~= firstImage.image then break end
  end
  result(shot(DIR .. "/bwsprites_battle_b.png"), "captured a later battle frame")
  result(firstImage ~= nil and laterImage ~= nil and firstImage.image ~= laterImage.image,
    "the foe picture changed between captures")

  local dex = Pokemon.dexFrontPic(foeSp, 0)
  result(type(dex) == "table" and dex.image ~= nil and dex.w == dex.image:getWidth()
    and dex.h == dex.image:getHeight() and dex.w >= 32 and dex.h >= 32,
    "the Pokedex picture is served at its native size (" .. tostring(dex and dex.w) .. "x" ..
      tostring(dex and dex.h) .. ")")
  if dex and dex.image then
    local w, h = inkSize(dex.image)
    result(w >= 35 and h >= 40,
      "Pikachu's dex picture keeps its own size (" .. w .. "x" .. h .. ")")
  end
  local dexChar = Pokemon.dexFrontPic(ownSp, 0)
  if dexChar and dexChar.image then
    -- The dex plays the idle animation inside a fixed fit sized by the largest
    -- pose, so one sample may be a wing-down frame. Sample a full cycle and
    -- check the largest visible pose, which must keep the v1.9 dex size.
    local maxW, maxH = inkSize(dexChar.image)
    local dexChanged = false
    for _ = 1, 60 do
      local laterDex = Pokemon.dexFrontPic(ownSp, 0)
      if laterDex ~= nil and laterDex.image ~= nil then
        local w, h = inkSize(laterDex.image)
        if w > maxW then maxW = w end
        if h > maxH then maxH = h end
        if laterDex.image ~= dexChar.image then dexChanged = true end
      end
      U.wait(4)
    end
    result(maxH >= 60 and maxW >= 55 and dexChar.w > 64,
      "Charizard's dex peak pose is native (" .. maxW .. "x" .. maxH .. ")")
    result(dexChanged, "the Pokedex picture animates instead of staying on one pose")
  else
    result(false, "Charizard's dex picture resolves")
  end
  local dexOther = Pokemon.dexFrontPic(1, 0)
  result(type(dexOther) == "table" and dexOther.image ~= nil,
    "the Pokedex still resolves an unserved species")

  print("driver done fails=" .. fails)
  love.event.quit(fails == 0 and 0 or 1)
end