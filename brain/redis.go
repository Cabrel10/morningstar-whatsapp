package main

import (
	"context"
	"os"
	"strings"
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

// ============================================================================
// GROUP PERSONA (custom personality per group)
// ============================================================================

func SetGroupPersona(jid, persona string) error {
	return rdb.Set(context.Background(), "persona:"+jid, persona, 0).Err()
}

func GetGroupPersona(jid string) string {
	val, err := rdb.Get(context.Background(), "persona:"+jid).Result()
	if err != nil {
		return ""
	}
	return val
}

// ============================================================================
// DEDUPLICATION (prevent processing same message twice)
// ============================================================================

func IsDuplicateMessage(msgId string) bool {
	key := "processed:" + msgId
	val, _ := rdb.Get(context.Background(), key).Result()
	if val != "" {
		return true
	}
	rdb.Set(context.Background(), key, "1", 10*time.Minute)
	return false
}

// ============================================================================
// RATE LIMITING (prevent spam / abuse)
// ============================================================================

// CheckRateLimit returns true if the user is rate-limited
func CheckRateLimit(senderJid string, maxPerMinute int) bool {
	key := "ratelimit:" + senderJid
	ctx := context.Background()

	count, _ := rdb.Incr(ctx, key).Result()
	if count == 1 {
		rdb.Expire(ctx, key, 60*time.Second)
	}

	return count > int64(maxPerMinute)
}

// ============================================================================
// TYPING LOCK (prevent multiple LLM calls for same user)
// ============================================================================

func AcquireTypingLock(remoteJid, senderJid string) bool {
	key := "typing:" + remoteJid + ":" + senderJid
	ctx := context.Background()

	set, err := rdb.SetNX(ctx, key, "1", 120*time.Second).Result()
	if err != nil {
		return true // On error, allow
	}
	return set
}

func ReleaseTypingLock(remoteJid, senderJid string) {
	key := "typing:" + remoteJid + ":" + senderJid
	rdb.Del(context.Background(), key)
}

// ============================================================================
// LANGUAGE PREFERENCE (per group)
// ============================================================================

func SetGroupLanguage(jid, lang string) error {
	return rdb.Set(context.Background(), "lang:"+jid, lang, 0).Err()
}

func GetGroupLanguage(jid string) string {
	val, err := rdb.Get(context.Background(), "lang:"+jid).Result()
	if err != nil {
		return "fr" // Default: French
	}
	return val
}

// ============================================================================
// HUMEUR / MOOD (per group dynamic personality)
// ============================================================================

// GetGroupHumeur retrieves the active mood for a group (default: amical)
func GetGroupHumeur(groupJid string) string {
	groupJid = strings.ReplaceAll(groupJid, " ", "")
	val, err := rdb.Get(context.Background(), "humeur:"+groupJid).Result()
	if err != nil || val == "" {
		return "amical"
	}
	return val
}

// SetGroupHumeur saves the mood for a group in Redis
func SetGroupHumeur(groupJid, humeur string) error {
	groupJid = strings.ReplaceAll(groupJid, " ", "")
	return rdb.Set(context.Background(), "humeur:"+groupJid, humeur, 0).Err()
}
