package main

import (
	"context"
	"os"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client

func initRedis() error {
	redisURI := os.Getenv("REDIS_URI")
	if redisURI == "" {
		redisURI = "redis://redis:6379"
	}

	opt, err := redis.ParseURL(redisURI)
	if err != nil {
		return err
	}

	rdb = redis.NewClient(opt)
	return rdb.Ping(context.Background()).Err()
}
