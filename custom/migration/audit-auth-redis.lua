local preserved_prefixes = {
    "refresh_token:",
    "user_refresh_tokens:",
    "token_family:",
}

local function should_audit(key)
    for _, prefix in ipairs(preserved_prefixes) do
        if string.sub(key, 1, string.len(prefix)) == prefix then
            return true
        end
    end
    return false
end

local keys = {}
local cursor = "0"
repeat
    local page = redis.call("SCAN", cursor, "COUNT", 1000)
    cursor = page[1]
    for _, key in ipairs(page[2]) do
        if should_audit(key) then
            table.insert(keys, key)
        end
    end
until cursor == "0"
table.sort(keys)

local strings = 0
local sets = 0
local invalid_types = 0
local dangling_members = 0
local expiring = 0
local persistent = 0
local fingerprints = {}

for _, key in ipairs(keys) do
    local key_type = redis.call("TYPE", key)["ok"]
    local ttl = redis.call("PTTL", key)
    if ttl >= 0 then
        expiring = expiring + 1
    elseif ttl == -1 then
        persistent = persistent + 1
    end

    if key_type == "string" then
        strings = strings + 1
        table.insert(fingerprints, key .. "|string|" .. redis.sha1hex(redis.call("GET", key)))
    elseif key_type == "set" then
        sets = sets + 1
        local members = redis.call("SMEMBERS", key)
        table.sort(members)
        table.insert(fingerprints, key .. "|set|" .. redis.sha1hex(table.concat(members, "\n")))
        for _, member in ipairs(members) do
            if redis.call("EXISTS", "refresh_token:" .. member) == 0 then
                dangling_members = dangling_members + 1
            end
        end
    else
        invalid_types = invalid_types + 1
    end
end

return {
    "total", #keys,
    "strings", strings,
    "sets", sets,
    "invalid_types", invalid_types,
    "dangling_members", dangling_members,
    "expiring", expiring,
    "persistent", persistent,
    "fingerprint", redis.sha1hex(table.concat(fingerprints, "\n")),
}
