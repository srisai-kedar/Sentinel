package algorithms

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	sredis "github.com/sentinel-project/sentinel/gateway/internal/redis"
)

// RedisLimiter wraps Redis Lua scripts for distributed atomic rate limiting.
//
// Why Lua instead of naive read-check-write:
//   GET count → CHECK limit → INCR count is not atomic across gateway instances.
//   Two instances can both read count=9, both allow, both increment → limit exceeded.
//   Lua executes read+check+update as one atomic unit on the Redis server.
type RedisLimiter struct {
	client   *sredis.Client
	script   string
	capacity int
	rate     float64
	window   int
}

func NewRedisTokenBucket(client *sredis.Client, capacity int, rate float64) *RedisLimiter {
	return &RedisLimiter{client: client, script: "token_bucket", capacity: capacity, rate: rate}
}

func NewRedisSlidingWindowLog(client *sredis.Client, limit, windowSec int) *RedisLimiter {
	return &RedisLimiter{client: client, script: "sliding_window_log", capacity: limit, window: windowSec}
}

func NewRedisSlidingWindowCounter(client *sredis.Client, limit, windowSec int) *RedisLimiter {
	return &RedisLimiter{client: client, script: "sliding_window_counter", capacity: limit, window: windowSec}
}

func NewRedisLeakyBucket(client *sredis.Client, capacity int, rate float64) *RedisLimiter {
	return &RedisLimiter{client: client, script: "leaky_bucket", capacity: capacity, rate: rate}
}

func (rl *RedisLimiter) Allow(ctx context.Context, key string) (Result, error) {
	redisKey := fmt.Sprintf("sentinel:%s:%s", rl.script, key)
	nowMs := time.Now().UnixMilli()

	var raw []interface{}
	var err error

	switch rl.script {
	case "token_bucket":
		raw, err = rl.client.EvalScript(ctx, rl.script, []string{redisKey},
			rl.capacity, rl.rate, nowMs)
	case "sliding_window_log":
		raw, err = rl.client.EvalScript(ctx, rl.script, []string{redisKey},
			rl.window*1000, rl.capacity, nowMs, uuid.NewString())
	case "sliding_window_counter":
		raw, err = rl.client.EvalScript(ctx, rl.script, []string{redisKey},
			rl.window*1000, rl.capacity, nowMs)
	case "leaky_bucket":
		raw, err = rl.client.EvalScript(ctx, rl.script, []string{redisKey},
			rl.capacity, rl.rate, nowMs)
	default:
		return Result{}, fmt.Errorf("unknown redis script: %s", rl.script)
	}
	if err != nil {
		return Result{}, err
	}

	return parseScriptResult(raw)
}

func parseScriptResult(raw []interface{}) (Result, error) {
	if len(raw) < 3 {
		return Result{}, fmt.Errorf("invalid script result length")
	}
	allowed, _ := toInt64(raw[0])
	retryMs, _ := toInt64(raw[1])
	remaining, _ := toInt64(raw[2])

	return Result{
		Allowed:    allowed == 1,
		RetryAfter: time.Duration(retryMs) * time.Millisecond,
		Remaining:  int(remaining),
	}, nil
}

func toInt64(v interface{}) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case string:
		return strconv.ParseInt(n, 10, 64)
	default:
		return 0, fmt.Errorf("unexpected number type %T", v)
	}
}
