-- sliding_window_counter.lua
local key = KEYS[1]
local window_ms = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

local data = redis.call('HMGET', key, 'curr', 'prev', 'start')
local curr = tonumber(data[1]) or 0
local prev = tonumber(data[2]) or 0
local start = tonumber(data[3]) or now_ms

if now_ms - start >= window_ms then
  prev = curr
  curr = 0
  start = now_ms
end

local elapsed = now_ms - start
local weight = 1.0 - (elapsed / window_ms)
if weight < 0 then weight = 0 end
local estimated = prev * weight + curr

if math.floor(estimated) >= limit then
  local retry_ms = math.max(1, window_ms - elapsed)
  redis.call('HMSET', key, 'curr', curr, 'prev', prev, 'start', start)
  redis.call('PEXPIRE', key, window_ms * 2 + 1000)
  return {0, retry_ms, math.floor(estimated)}
end

curr = curr + 1
redis.call('HMSET', key, 'curr', curr, 'prev', prev, 'start', start)
redis.call('PEXPIRE', key, window_ms * 2 + 1000)
return {1, 0, limit - math.ceil(estimated) - 1}
