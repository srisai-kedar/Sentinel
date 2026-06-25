-- leaky_bucket.lua
-- Atomic leaky bucket with lazy leak computation.
-- KEYS[1] = rate limit key
-- ARGV[1] = capacity, ARGV[2] = leak_rate (per sec), ARGV[3] = now_ms

local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local leak_rate = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

local data = redis.call('HMGET', key, 'volume', 'last_leak')
local volume = tonumber(data[1]) or 0
local last_leak = tonumber(data[2]) or now_ms

local elapsed = (now_ms - last_leak) / 1000.0
volume = math.max(0, volume - elapsed * leak_rate)

if volume + 1 <= capacity then
  volume = volume + 1
  redis.call('HMSET', key, 'volume', volume, 'last_leak', now_ms)
  redis.call('PEXPIRE', key, math.ceil((capacity / leak_rate) * 1000) + 1000)
  return {1, 0, math.floor(capacity - volume)}
end

local overflow = volume + 1 - capacity
local retry_ms = math.ceil((overflow / leak_rate) * 1000)
redis.call('HMSET', key, 'volume', volume, 'last_leak', now_ms)
return {0, retry_ms, 0}
