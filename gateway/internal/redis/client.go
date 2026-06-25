package redis

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

//go:embed scripts/*.lua
var scriptFS embed.FS

// Client wraps go-redis with pre-loaded Lua scripts.
type Client struct {
	rdb      *goredis.Client
	scripts  map[string]string
	sha      map[string]string
	failOpen bool
}

// NewClient connects to Redis and loads Lua scripts from the embedded filesystem.
func NewClient(redisURL string, failOpen bool) (*Client, error) {
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	rdb := goredis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	c := &Client{
		rdb:      rdb,
		scripts:  make(map[string]string),
		sha:      make(map[string]string),
		failOpen: failOpen,
	}

	entries, err := fs.ReadDir(scriptFS, "scripts")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".lua")
		body, err := scriptFS.ReadFile("scripts/" + e.Name())
		if err != nil {
			return nil, err
		}
		c.scripts[name] = string(body)
		sha, err := rdb.ScriptLoad(ctx, string(body)).Result()
		if err != nil {
			return nil, fmt.Errorf("script load %s: %w", name, err)
		}
		c.sha[name] = sha
	}

	return c, nil
}

// EvalScript runs a named Lua script atomically.
// Returns raw Redis result slice. On Redis error, behavior depends on failOpen flag.
func (c *Client) EvalScript(ctx context.Context, name string, keys []string, args ...interface{}) ([]interface{}, error) {
	sha, ok := c.sha[name]
	if !ok {
		return nil, fmt.Errorf("unknown script: %s", name)
	}

	res, err := c.rdb.EvalSha(ctx, sha, keys, args...).Result()
	if err != nil {
		if c.failOpen {
			// Fail-open: allow traffic when Redis is down (configurable, not default).
			return []interface{}{int64(1), int64(0), int64(999)}, nil
		}
		return nil, err
	}
	slice, ok := res.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected script result type")
	}
	return slice, nil
}

// Ping checks Redis connectivity.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}
