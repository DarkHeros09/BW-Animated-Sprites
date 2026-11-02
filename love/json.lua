-- Minimal JSON decoder for the sprite pack's sidecars.
--
-- The pack writer emits plain JSON, so this accepts exactly that: objects,
-- arrays, strings, numbers, booleans and null. Anything else fails with the
-- byte offset, so a hand-edited pack reports where it went wrong instead of
-- loading half-way.
local json = {}

local MAX_DEPTH = 64

local escapes = {
  ['"'] = '"',
  ["\\"] = "\\",
  ["/"] = "/",
  b = "\b",
  f = "\f",
  n = "\n",
  r = "\r",
  t = "\t",
}

local function fail(pos, message)
  error(string.format("json: %s at byte %d", message, pos), 0)
end

-- utf8 encodes a code point as UTF-8.
local function utf8(cp)
  if cp < 0x80 then
    return string.char(cp)
  elseif cp < 0x800 then
    return string.char(0xC0 + math.floor(cp / 0x40), 0x80 + cp % 0x40)
  elseif cp < 0x10000 then
    return string.char(0xE0 + math.floor(cp / 0x1000),
      0x80 + math.floor(cp / 0x40) % 0x40, 0x80 + cp % 0x40)
  end
  return string.char(0xF0 + math.floor(cp / 0x40000),
    0x80 + math.floor(cp / 0x1000) % 0x40,
    0x80 + math.floor(cp / 0x40) % 0x40, 0x80 + cp % 0x40)
end

local parseValue

local function skip(text, pos)
  while true do
    local c = text:sub(pos, pos)
    if c == " " or c == "\t" or c == "\n" or c == "\r" then
      pos = pos + 1
    else
      return pos
    end
  end
end

local function parseString(text, pos)
  -- pos is on the opening quote.
  local out, i = {}, pos + 1
  while true do
    local c = text:sub(i, i)
    if c == "" then fail(i, "unterminated string") end
    if c == '"' then
      return table.concat(out), i + 1
    elseif c == "\\" then
      local e = text:sub(i + 1, i + 1)
      if e == "u" then
        local hex = text:sub(i + 2, i + 5)
        if not hex:match("^%x%x%x%x$") then fail(i, "bad \\u escape") end
        out[#out + 1] = utf8(tonumber(hex, 16))
        i = i + 6
      else
        local mapped = escapes[e]
        if not mapped then fail(i, "bad escape \\" .. e) end
        out[#out + 1] = mapped
        i = i + 2
      end
    else
      out[#out + 1] = c
      i = i + 1
    end
  end
end

local function parseNumber(text, pos)
  local s, e = text:find("^-?%d+%.?%d*[eE]?[-+]?%d*", pos)
  if not s then fail(pos, "bad number") end
  local value = tonumber(text:sub(s, e))
  if not value then fail(pos, "bad number") end
  return value, e + 1
end

local function parseArray(text, pos, depth)
  local out, i = {}, pos + 1
  i = skip(text, i)
  if text:sub(i, i) == "]" then return out, i + 1 end
  while true do
    local value
    value, i = parseValue(text, i, depth + 1)
    out[#out + 1] = value
    i = skip(text, i)
    local c = text:sub(i, i)
    if c == "]" then return out, i + 1 end
    if c ~= "," then fail(i, "expected , or ]") end
    i = skip(text, i + 1)
  end
end

local function parseObject(text, pos, depth)
  local out, i = {}, pos + 1
  i = skip(text, i)
  if text:sub(i, i) == "}" then return out, i + 1 end
  while true do
    if text:sub(i, i) ~= '"' then fail(i, "expected a key") end
    local key
    key, i = parseString(text, i)
    i = skip(text, i)
    if text:sub(i, i) ~= ":" then fail(i, "expected :") end
    local value
    value, i = parseValue(text, i + 1, depth + 1)
    out[key] = value
    i = skip(text, i)
    local c = text:sub(i, i)
    if c == "}" then return out, i + 1 end
    if c ~= "," then fail(i, "expected , or }") end
    i = skip(text, i + 1)
  end
end

parseValue = function(text, pos, depth)
  if depth > MAX_DEPTH then fail(pos, "nested too deeply") end
  pos = skip(text, pos)
  local c = text:sub(pos, pos)
  if c == "{" then return parseObject(text, pos, depth) end
  if c == "[" then return parseArray(text, pos, depth) end
  if c == '"' then return parseString(text, pos) end
  if c == "-" or c:match("%d") then return parseNumber(text, pos) end
  if text:sub(pos, pos + 3) == "true" then return true, pos + 4 end
  if text:sub(pos, pos + 4) == "false" then return false, pos + 5 end
  if text:sub(pos, pos + 3) == "null" then return nil, pos + 4 end
  fail(pos, "unexpected " .. (c == "" and "end of input" or ("'" .. c .. "'")))
end

-- decode parses text and returns the value, or nil and an error message.
function json.decode(text)
  if type(text) ~= "string" then
    return nil, "json: input is a " .. type(text) .. ", not a string"
  end
  local ok, value = pcall(function()
    local out, pos = parseValue(text, 1, 0)
    pos = skip(text, pos)
    if pos <= #text then fail(pos, "trailing data") end
    return out
  end)
  if ok then return value end
  return nil, tostring(value)
end

return json
