package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"forum/internal/metrics"
	"forum/internal/models"

	"github.com/redis/go-redis/v9"
)

const postsTTL = 5 * time.Minute

type Cache struct {
	client *redis.Client
}

func New(url string) (*Cache, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Cache{client: client}, nil
}

func (c *Cache) Close() error { return c.client.Close() }

// ---- Cache-aside for posts ----

func (c *Cache) GetPosts(ctx context.Context, key string) ([]*models.Post, error) {
	val, err := c.client.Get(ctx, "posts:"+key).Bytes()
	if err == redis.Nil {
		metrics.CacheHits.WithLabelValues("miss").Inc()
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var posts []*models.Post
	if err := json.Unmarshal(val, &posts); err != nil {
		return nil, err
	}
	metrics.CacheHits.WithLabelValues("hit").Inc()
	return posts, nil
}

func (c *Cache) SetPosts(ctx context.Context, key string, posts []*models.Post) error {
	data, err := json.Marshal(posts)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, "posts:"+key, data, postsTTL).Err()
}

func (c *Cache) InvalidatePosts(ctx context.Context) error {
	// Use SCAN instead of KEYS to avoid blocking Redis on large keyspaces.
	var cursor uint64
	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(ctx, cursor, "posts:*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		if cursor == 0 {
			break
		}
	}
	return nil
}

// ---- Rate limiting ----

// allowScript is a fixed-window counter. The increment and the expiry are set
// in one script so they are atomic: with separate INCR and EXPIRE calls, a
// crash (or a lost connection) between the two leaves a counter with no TTL,
// which would lock that key out permanently. A counter found without a TTL is
// repaired here as well. The expiry is armed only when the window opens;
// re-arming it on every call would let a busy key keep pushing its own reset
// out and stay limited indefinitely.
var allowScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if count == 1 or ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
end
return {count, ttl}
`)

// Allow counts one request against key and reports whether it is within
// limit requests per window. When it is not, the returned duration is the
// time until the window resets. The error is non-nil when Redis could not be
// consulted; the caller decides whether to fail open or fall back to another
// limiter.
func (c *Cache) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	res, err := allowScript.Run(ctx, c.client, []string{key}, window.Milliseconds()).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit script: %w", err)
	}
	if len(res) != 2 {
		return false, 0, fmt.Errorf("rate limit script: unexpected reply %v", res)
	}
	count, ttlMillis := res[0], res[1]
	if count > int64(limit) {
		return false, time.Duration(ttlMillis) * time.Millisecond, nil
	}
	return true, 0, nil
}

// ---- WebSocket pub/sub ----

func (c *Cache) Publish(ctx context.Context, channel string, msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.client.Publish(ctx, channel, data).Err()
}

func (c *Cache) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	return c.client.Subscribe(ctx, channel)
}
