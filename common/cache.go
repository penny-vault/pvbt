// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"context"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	lru "github.com/hashicorp/golang-lru"

	"github.com/rs/zerolog/log"
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
			log.Error().Err(err).Msg("could not parse redis URL")
			os.Exit(1)
		}

		rdb = redis.NewClient(opt)
	}

	cache, err = lru.New(viper.GetInt("cache.local_size"))
	if err != nil {
		log.Error().Err(err).Msg("could not create LRU cache")
		os.Exit(1)
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
