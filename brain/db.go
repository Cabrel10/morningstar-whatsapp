package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
)

var db *pgx.Conn

func initDB() error {
	connStr := os.Getenv("DATABASE_CONNECTION_URI")
	if connStr == "" {
		return fmt.Errorf("DATABASE_CONNECTION_URI is not set")
	}

	conn, err := pgx.Connect(context.Background(), connStr)
	if err != nil {
		return fmt.Errorf("unable to connect to database: %v", err)
	}

	db = conn
	return nil
}

func getRecentMessages(remoteJid string, limit int) ([]string, error) {
	// Query the existing Message table from Evolution API
	rows, err := db.Query(context.Background(), 
		`SELECT COALESCE(message->>'conversation', message->'extendedTextMessage'->>'text') as content, COALESCE("pushName", 'Utilisateur') 
		 FROM public."Message" 
		 WHERE key->>'remoteJid' = $1
		 AND (message->>'conversation' IS NOT NULL OR message->'extendedTextMessage'->>'text' IS NOT NULL)
		 ORDER BY "messageTimestamp" DESC LIMIT $2`, 
		 remoteJid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []string
	for rows.Next() {
		var content, pushName string
		if err := rows.Scan(&content, &pushName); err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			continue
		}
		messages = append([]string{fmt.Sprintf("%s: %s", pushName, content)}, messages...)
	}
	return messages, nil
}

func getFacts(remoteJid string) ([]string, error) {
	// 1. Get textual facts
	rows, err := db.Query(context.Background(), 
		"SELECT content FROM facts WHERE remote_jid = $1 ORDER BY created_at DESC LIMIT 5", 
		remoteJid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			continue
		}
		facts = append(facts, "[Fait] "+content)
	}

	// 2. Get media context (What was shared recently)
	mediaRows, err := db.Query(context.Background(),
		"SELECT media_type, description FROM media_metadata WHERE remote_jid = $1 ORDER BY created_at DESC LIMIT 3",
		remoteJid)
	if err == nil {
		defer mediaRows.Close()
		for mediaRows.Next() {
			var mType, desc string
			if err := mediaRows.Scan(&mType, &desc); err == nil {
				facts = append(facts, fmt.Sprintf("[Média:%s] %s", mType, desc))
			}
		}
	}

	return facts, nil
}

func recordInteraction(groupJid, sourceJid, targetJid, iType string) error {
	if sourceJid == targetJid || targetJid == "" {
		return nil
	}
	_, err := db.Exec(context.Background(),
		`INSERT INTO member_interactions (group_jid, source_jid, target_jid, interaction_type)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (group_jid, source_jid, target_jid, interaction_type) DO UPDATE SET
		 weight = member_interactions.weight + 1,
		 last_interaction = CURRENT_TIMESTAMP`,
		 groupJid, sourceJid, targetJid, iType)
	return err
}

func recordStickerUsage(jid, sha256 string) error {
	_, err := db.Exec(context.Background(),
		`INSERT INTO member_sticker_usage (jid, sticker_file_sha256)
		 VALUES ($1, $2)
		 ON CONFLICT (jid, sticker_file_sha256) DO UPDATE SET
		 usage_count = member_sticker_usage.usage_count + 1,
		 last_used = CURRENT_TIMESTAMP`,
		 jid, sha256)
	return err
}

func recordMediaMetadata(remoteJid, messageId, mType, desc string) error {
	_, err := db.Exec(context.Background(),
		`INSERT INTO media_metadata (remote_jid, message_id, media_type, description)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (message_id) DO UPDATE SET
		 description = EXCLUDED.description`,
		 remoteJid, messageId, mType, desc)
	return err
}

func upsertMember(jid, groupJid, pushName string) error {
	_, err := db.Exec(context.Background(), 
		`INSERT INTO group_members (jid, group_jid, push_name, message_count, last_seen) 
		 VALUES ($1, $2, $3, 1, CURRENT_TIMESTAMP)
		 ON CONFLICT (jid, group_jid) DO UPDATE SET 
		 push_name = EXCLUDED.push_name,
		 message_count = group_members.message_count + 1,
		 last_seen = CURRENT_TIMESTAMP`, 
		 jid, groupJid, pushName)
	return err
}

func updateMemberProfile(groupJid, pushName, skill, interest string) error {
	// Simple append of skills/interests to existing ones
	_, err := db.Exec(context.Background(), 
		`UPDATE group_members SET 
		 skills = CASE WHEN skills IS NULL THEN $3 ELSE skills || ', ' || $3 END,
		 interests = CASE WHEN interests IS NULL THEN $4 ELSE interests || ', ' || $4 END
		 WHERE group_jid = $1 AND push_name = $2`, 
		 groupJid, pushName, skill, interest)
	return err
}

func getMemberProfiles(groupJid string) (string, error) {
	rows, err := db.Query(context.Background(), 
		`SELECT push_name, skills, interests, message_count 
		 FROM group_members 
		 WHERE group_jid = $1 
		 ORDER BY message_count DESC LIMIT 10`, 
		 groupJid)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var profiles []string
	for rows.Next() {
		var name, skills, interests string
		var count int
		var s, i *string
		if err := rows.Scan(&name, &s, &i, &count); err != nil {
			continue
		}
		if s != nil { skills = *s } else { skills = "Non identifiées" }
		if i != nil { interests = *i } else { interests = "Non identifiés" }
		
		profiles = append(profiles, fmt.Sprintf("- %s (%d msgs) | Compétences: %s | Intérêts: %s", name, count, skills, interests))
	}
	return strings.Join(profiles, "\n"), nil
}

func getGroupCartography(remoteJid string) (string, error) {
	// Faster pre-aggregated cartography
	return getMemberProfiles(remoteJid)
}

func cleanupOldMessages(days int) error {
	_, err := db.Exec(context.Background(), 
		`DELETE FROM public."Message" 
		 WHERE "messageTimestamp" < extract(epoch from (now() - interval '`+fmt.Sprintf("%d", days)+` days'))`)
	return err
}

func addFact(remoteJid, content string) error {
	_, err := db.Exec(context.Background(), 
		"INSERT INTO facts (remote_jid, content) VALUES ($1, $2)", 
		remoteJid, content)
	return err
}
