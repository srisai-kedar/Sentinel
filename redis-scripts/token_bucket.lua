-- token_bucket.lua
-- Atomic token bucket: read + refill + check + decrement in one Redis script execution.
-- KEYS[1] = rate limit key
-- ARGV[1] = capacity, ARGV[2] = refill_rate (tokens/sec), ARGV[3] = now_ms

local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

local data = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(data[1])
local last_refill = tonumber(data[2])

if tokens == nil then
  tokens = capacity
  last_refill = now_ms
end

local elapsed = (now_ms - last_refill) / 1000.0
tokens = math.min(capacity, tokens + elapsed * refill_rate)

if tokens >= 1 then
  tokens = tokens - 1
  redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now_ms)
  local ttl_ms = math.ceil((capacity / refill_rate) * 1000) + 1000
  redis.call('PEXPIRE', key, ttl_ms)
  return {1, 0, math.floor(tokens)}
end

local deficit = 1 - tokens
local retry_ms = math.ceil((deficit / refill_rate) * 1000)
redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now_ms)
return {0, retry_ms, 0}
