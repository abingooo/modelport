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
local refresh_tokens = 0
local user_sets = 0
local family_sets = 0
local invalid_refresh_json = 0
local bound_refresh_tokens = 0
local unbound_refresh_tokens = 0
local ttl_lt_1h = 0
local ttl_lt_1d = 0
local ttl_lt_7d = 0
local ttl_lt_30d = 0
local ttl_gte_30d = 0
local ttl_sum_ms = 0
local ttl_min_ms = nil
local ttl_max_ms = nil
local fingerprints = {}

for _, key in ipairs(keys) do
    local key_type = redis.call("TYPE", key)["ok"]
    local ttl = redis.call("PTTL", key)
    if ttl >= 0 then
        expiring = expiring + 1
        ttl_sum_ms = ttl_sum_ms + ttl
        if ttl_min_ms == nil or ttl < ttl_min_ms then
            ttl_min_ms = ttl
        end
        if ttl_max_ms == nil or ttl > ttl_max_ms then
            ttl_max_ms = ttl
        end
        if ttl < 60 * 60 * 1000 then
            ttl_lt_1h = ttl_lt_1h + 1
        elseif ttl < 24 * 60 * 60 * 1000 then
            ttl_lt_1d = ttl_lt_1d + 1
        elseif ttl < 7 * 24 * 60 * 60 * 1000 then
            ttl_lt_7d = ttl_lt_7d + 1
        elseif ttl < 30 * 24 * 60 * 60 * 1000 then
            ttl_lt_30d = ttl_lt_30d + 1
        else
            ttl_gte_30d = ttl_gte_30d + 1
        end
    elseif ttl == -1 then
        persistent = persistent + 1
    end

    local expected_type = nil
    if string.sub(key, 1, string.len("refresh_token:")) == "refresh_token:" then
        refresh_tokens = refresh_tokens + 1
        expected_type = "string"
    elseif string.sub(key, 1, string.len("user_refresh_tokens:")) == "user_refresh_tokens:" then
        user_sets = user_sets + 1
        expected_type = "set"
    elseif string.sub(key, 1, string.len("token_family:")) == "token_family:" then
        family_sets = family_sets + 1
        expected_type = "set"
    end
    if key_type ~= expected_type then
        invalid_types = invalid_types + 1
    end

    if key_type == "string" then
        strings = strings + 1
        local value = redis.call("GET", key)
        table.insert(fingerprints, key .. "|string|" .. redis.sha1hex(value))
        if expected_type == "string" then
            local ok, data = pcall(cjson.decode, value)
            if not ok or type(data) ~= "table" then
                invalid_refresh_json = invalid_refresh_json + 1
            elseif data["binding_hash"] ~= nil and data["binding_hash"] ~= "" then
                bound_refresh_tokens = bound_refresh_tokens + 1
            else
                unbound_refresh_tokens = unbound_refresh_tokens + 1
            end
        end
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
    end
end

if ttl_min_ms == nil then
    ttl_min_ms = -1
end
if ttl_max_ms == nil then
    ttl_max_ms = -1
end

return {
    "total", #keys,
    "refresh_tokens", refresh_tokens,
    "user_sets", user_sets,
    "family_sets", family_sets,
    "strings", strings,
    "sets", sets,
    "invalid_types", invalid_types,
    "invalid_refresh_json", invalid_refresh_json,
    "bound_refresh_tokens", bound_refresh_tokens,
    "unbound_refresh_tokens", unbound_refresh_tokens,
    "dangling_members", dangling_members,
    "expiring", expiring,
    "persistent", persistent,
    "ttl_lt_1h", ttl_lt_1h,
    "ttl_lt_1d", ttl_lt_1d,
    "ttl_lt_7d", ttl_lt_7d,
    "ttl_lt_30d", ttl_lt_30d,
    "ttl_gte_30d", ttl_gte_30d,
    "ttl_sum_ms", ttl_sum_ms,
    "ttl_min_ms", ttl_min_ms,
    "ttl_max_ms", ttl_max_ms,
    "fingerprint", redis.sha1hex(table.concat(fingerprints, "\n")),
}
