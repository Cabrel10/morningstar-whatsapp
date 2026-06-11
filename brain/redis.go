package main

import (
	"context"
	"os"
	"time"

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


// SetGroupPersona sauvegarde un persona personnalisé pour un groupe
func SetGroupPersona(jid, persona string) error {
	return rdb.Set(context.Background(), "persona:"+jid, persona, 0).Err()
}

// GetGroupPersona récupère le persona personnalisé d'un groupe
func GetGroupPersona(jid string) string {
	val, err := rdb.Get(context.Background(), "persona:"+jid).Result()
	if err != nil {
		return ""
	}
	return val
}

// IsDuplicateMessage vérifie si le message a déjà été traité (TTL 10 min)
func IsDuplicateMessage(msgId string) bool {
	key := "processed:" + msgId
	val, _ := rdb.Get(context.Background(), key).Result()
	if val != "" {
		return true
	}
	// On marque comme traité
	rdb.Set(context.Background(), key, "1", 10*time.Minute)
	return false
}
