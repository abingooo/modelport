local preserved_prefixes = {
    "refresh_token:",
    "user_refresh_tokens:",
    "token_family:",
}

local function should_preserve(key)
    for _, prefix in ipairs(preserved_prefixes) do
        if string.sub(key, 1, string.len(prefix)) == prefix then
            return true
        end
    end
    return false
end

local scanned = 0
local preserved = 0
local removed = 0
local passes = 0

repeat
    local cursor = "0"
    local removed_this_pass = 0
    repeat
        local page = redis.call("SCAN", cursor, "COUNT", 1000)
        cursor = page[1]
        for _, key in ipairs(page[2]) do
            scanned = scanned + 1
            if should_preserve(key) then
                preserved = preserved + 1
            else
                redis.call("UNLINK", key)
                removed = removed + 1
                removed_this_pass = removed_this_pass + 1
            end
        end
    until cursor == "0"
    passes = passes + 1
until removed_this_pass == 0 or passes == 10

return {scanned, preserved, removed, passes}
