-- LÖVE configuration for the sprite-pack previewer.
function love.conf(t)
  t.identity = "bwsprites"
  t.version = "11.0"
  t.console = false
  t.window.title = "bwsprites preview"
  t.window.width = 1080
  t.window.height = 680
  t.window.resizable = true
  t.window.vsync = 1
  t.modules.joystick = false
  t.modules.physics = false
  t.modules.touch = false
  t.modules.video = false
end
