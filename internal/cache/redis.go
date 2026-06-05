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

const (
	postsTTL   = 5 * time.Minute
	rateWindow = time.Minute
	rateLimit  = 60 // requests per minute per IP for API
)

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

// ---- Token bucket rate limiting ----

// AllowRequest returns true if the IP is within rate limit, false if exceeded.
func (c *Cache) AllowRequest(ctx context.Context, ip string) (bool, error) {
	key := fmt.Sprintf("rate:%s", ip)
	pipe := c.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, rateWindow)
	if _, err := pipe.Exec(ctx); err != nil {
		return true, nil // fail open
	}
	return incr.Val() <= int64(rateLimit), nil
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
