-- Run from the repository root: `rex do scripts/rex/new-agent-session_test.lua`. Fake servers; no
-- session is touched.
local M = dofile("scripts/rex/new-agent-session.lua")

local function lists(...)
  local sessions = { ... }
  return function() return { sessions = sessions } end
end
local here = lists({ session_id = "s1", label = "burns" })
local remote = { Two = { call = lists({ session_id = "s2", label = "my work" }) } }
local servers = function() return { "Gone", "Two" } end
local server = function(name)
  if not remote[name] then error("unknown server " .. name) end
  return remote[name]
end

local function check(cond, what) if not cond then error("failed: " .. what, 0) end end

local h, l = M.owner("s1", here, servers, server)
check(h == "" and l == "burns", "a session on this server has no host label")
h, l = M.owner("s2", here, servers, server)
check(h == "Two" and l == "my work", "a remote session names its server, past one that fails")
h, l = M.owner("s9", here, servers, server)
check(h == nil and l == nil, "an unknown session is not found")

check(M.url(nil, nil) == "burns://new", "no session opens the plain panel")
check(M.url("", "burns") == "burns://new?host=&session=burns", "this server is an empty host")
check(M.url("Two", "my work") == "burns://new?host=Two&session=my%20work", "labels are escaped")
check(M.url("Two", "w", "block:b") == "burns://new?host=Two&session=w&block=block%3Ab", "a beside launch names the block")
return "ok"
