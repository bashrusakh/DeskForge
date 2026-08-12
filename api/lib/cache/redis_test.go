package cache

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/go-redis/redis/v8"
)

// Redis integration tests are opt-in and use the address supplied by this variable.
const redisTestAddrEnv = "DESKFORGE_TEST_REDIS_ADDR"
const invalidRedisTestEndpoint = "invalid Redis test endpoint"

type redisTestCache struct {
	*RedisCache
	prefix string
}

func redisTestParseErrorMessage(string, error) string {
	return invalidRedisTestEndpoint
}

func newRedisTestCache(t testing.TB) *redisTestCache {
	t.Helper()
	addr := os.Getenv(redisTestAddrEnv)
	if addr == "" {
		t.Skipf("Redis integration test skipped: %s is unset", redisTestAddrEnv)
	}

	// Plain host:port endpoints use default options; redis:// and rediss:// URLs
	// retain the endpoint's password, database, and TLS settings.
	options := &redis.Options{Addr: addr}
	if strings.Contains(addr, "://") {
		var err error
		options, err = redis.ParseURL(addr)
		if err != nil {
			t.Fatal(redisTestParseErrorMessage(addr, err))
		}
	}

	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("generate Redis test namespace: %v", err)
	}
	prefix := fmt.Sprintf("deskforge:test:%d:%x:", os.Getpid(), random)
	cache := &redisTestCache{
		RedisCache: RedisCacheInit(options),
		prefix:     prefix,
	}
	t.Cleanup(func() {
		cache.cleanup(t)
	})
	return cache
}

func (c *redisTestCache) key(key string) string {
	return c.prefix + key
}

func (c *redisTestCache) Set(key string, value interface{}, exp int) error {
	return c.RedisCache.Set(c.key(key), value, exp)
}

func (c *redisTestCache) Get(key string, value interface{}) error {
	return c.RedisCache.Get(c.key(key), value)
}

func (c *redisTestCache) cleanup(t testing.TB) {
	t.Helper()
	ctx := context.Background()
	var cursor uint64
	for {
		keys, nextCursor, err := c.rdb.Scan(ctx, cursor, c.prefix+"*", 0).Result()
		if err != nil {
			t.Fatalf("Redis test cleanup scan failed: %v", err)
		}
		if len(keys) > 0 {
			if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
				t.Fatalf("Redis test cleanup delete failed: %v", err)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	if err := c.rdb.Close(); err != nil {
		t.Fatalf("Redis test client close failed: %v", err)
	}
}

func TestRedisSet(t *testing.T) {
	rc := newRedisTestCache(t)
	err := rc.Set("123", "ddd", 0)
	if err != nil {
		t.Fatalf("Redis set failed: %v", err)
	}
}

func TestRedisGet(t *testing.T) {
	rc := newRedisTestCache(t)
	err := rc.Set("123", "451156", 300)
	if err != nil {
		t.Fatalf("Redis set failed: %v", err)
	}
	res := ""
	err = rc.Get("123", &res)
	if err != nil {
		t.Fatalf("Redis get failed: %v", err)
	}
	fmt.Println("res", res)
}

func TestRedisGetJson(t *testing.T) {
	rc := newRedisTestCache(t)
	type r struct {
		Aa string `json:"a"`
		B  string `json:"c"`
	}
	old := &r{
		Aa: "ab", B: "cdc",
	}
	err := rc.Set("1233", old, 300)
	if err != nil {
		t.Fatalf("Redis set failed: %v", err)
	}

	res := &r{}
	err2 := rc.Get("1233", res)
	if err2 != nil {
		t.Fatalf("Redis get failed: %v", err2)
	}
	if !reflect.DeepEqual(res, old) {
		t.Fatalf("Redis JSON value mismatch: got %v, want %v", res, old)
	}
	fmt.Println(res, res.Aa)
}

func BenchmarkRSet(b *testing.B) {
	rc := newRedisTestCache(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := rc.Set("123", "{dsv}", 1000); err != nil {
			b.Fatalf("Redis set failed: %v", err)
		}
	}
}

func BenchmarkRGet(b *testing.B) {
	rc := newRedisTestCache(b)
	if err := rc.Set("123", "{dsv}", 1000); err != nil {
		b.Fatalf("Redis seed failed: %v", err)
	}
	b.ResetTimer()
	v := ""
	for i := 0; i < b.N; i++ {
		if err := rc.Get("123", &v); err != nil {
			b.Fatalf("Redis get failed: %v", err)
		}
	}
}
