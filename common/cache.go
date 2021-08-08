package common

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	lru "github.com/hashicorp/golang-lru"
	log "github.com/sirupsen/logrus"

	"github.com/spf13/viper"
)

var ctx = context.Background()
var rdb *redis.Client
var cache *lru.Cache

func SetupCache() {
	var err error
	if viper.GetBool("cache.redis") {
		opt, err := redis.ParseURL(viper.GetString("cache.redis_url"))
		if err != nil {
			log.Fatal(err)
		}

		rdb = redis.NewClient(opt)
	}

	cache, err = lru.New(viper.GetInt("cache.local_size"))
	if err != nil {
		log.Fatal(err)
	}
}

func CacheSet(key string, bytes []byte) error {
	b2, err := Compress(bytes)
	if err != nil {
		return err
	}
	cache.Add(key, b2)

	if viper.GetBool("cache.redis") {
		expires := time.Duration(viper.GetInt("cache.ttl")) * time.Second
		return rdb.Set(ctx, key, b2, expires).Err()
	}
	return nil
}

func CacheGet(key string) ([]byte, error) {
	var val []byte
	var err error

	v2, ok := cache.Get(key)

	if ok {
		val = v2.([]byte)
		return Decompress(val)
	} else if viper.GetBool("cache.redis") {
		expires := time.Duration(viper.GetInt("cache.ttl")) * time.Second
		val, err = rdb.GetEx(ctx, key, expires).Bytes()
		if err != nil {
			return []byte{}, err
		}
		return Decompress(val)
	}

	return []byte{}, err
}
