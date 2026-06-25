-- sliding_window_log.lua
-- Atomic sliding window log using a sorted set of request timestamps.
-- KEYS[1] = rate limit key
-- ARGV[1] = window_ms, ARGV[2] = limit, ARGV[3] = now_ms, ARGV[4] = request_id

local key = KEYS[1]
local window_ms = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local req_id = ARGV[4]

local cutoff = now_ms - window_ms
redis.call('ZREMRANGEBYSCORE', key, '-inf', cutoff)
local count = redis.call('ZCARD', key)

if count >= limit then
  local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
  local retry_ms = 1000
  if oldest[2] then
    retry_ms = math.max(1, tonumber(oldest[2]) + window_ms - now_ms)
  end
  return {0, retry_ms, count}
end

redis.call('ZADD', key, now_ms, req_id)
redis.call('PEXPIRE', key, window_ms + 1000)
return {1, 0, limit - count - 1}
