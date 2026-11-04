-- New Agent Session (docs/SPEC.md, "Launch"): opens Burns' launch panel on the session the action
-- ran in. Load it from the Mac's init.lua and bind a key:
--
--   dofile(os.getenv("HOME") .. "/path/to/burns/scripts/rex/new-agent-session.lua")
--   rex.bind("shift+cmd+a", "burns_new")
--   rex.bind("alt+shift+cmd+a", "burns_beside")
--
-- burns_beside also names the focused block, so the panel starts beside it, in its folder.
--
-- Rex runs a config action on the server that loaded it, the Mac's own, even from another host's
-- tab, so `open` reaches the Burns app. `ctx` names the session but not its server, so the action
-- looks for it on this server, then on each server the asking client can reach.

local M = {}

-- The session's server label ("" for this one) and the session's label, or nil.
function M.owner(id, local_call, servers, server)
  local function find(call)
    local list = call("session.list")
    for _, s in ipairs((list or {}).sessions or {}) do
      if s.session_id == id then return s.label end
    end
  end
  local label = find(local_call)
  if label then return "", label end
  for _, name in ipairs(servers()) do
    local ok, remote = pcall(server, name)
    if ok then
      label = find(remote.call)
      if label then return name, label end
    end
  end
end

local function encode(s)
  return (s:gsub("[^%w%-%._~]", function(c) return string.format("%%%02X", string.byte(c)) end))
end

-- burns://new, with the session when it was found, and the block for a beside launch. Without a
-- session the panel opens on its defaults.
function M.url(host, label, block)
  if not label then return "burns://new" end
  local url = "burns://new?host=" .. encode(host) .. "&session=" .. encode(label)
  if block then url = url .. "&block=" .. encode(block) end
  return url
end

local function open_panel(ctx, beside)
  local host, label
  if ctx.session_id then host, label = M.owner(ctx.session_id, rex.call, rex.servers, rex.server) end
  local block = beside and ctx.block_id or nil
  local r, err = rex.exec({ "open", M.url(host, label, block) }, { timeout = 10 })
  if r == nil then error(err, 0) end
  if not r.ok then error(r.stderr, 0) end
  return { host = host, session = label, block = block }
end

rex.action{
  name = "burns_new",
  title = "New Agent Session",
  description = "Open Burns' launch panel on this session",
  keywords = "burns agent launch claude codex",
  run = function(ctx) return open_panel(ctx, false) end,
}

rex.action{
  name = "burns_beside",
  title = "New Agent Beside This Pane",
  description = "Open Burns' launch panel to split this pane, in its folder",
  keywords = "burns agent launch split beside",
  run = function(ctx) return open_panel(ctx, true) end,
}

return M
