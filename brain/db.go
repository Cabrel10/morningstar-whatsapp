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
	rows, err := db.Query(context.Background(), 
		"SELECT content FROM facts WHERE remote_jid = $1 ORDER BY created_at DESC", 
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
		facts = append(facts, content)
	}
	return facts, nil
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

func addFact(remoteJid, content string) error {
	_, err := db.Exec(context.Background(), 
		"INSERT INTO facts (remote_jid, content) VALUES ($1, $2)", 
		remoteJid, content)
	return err
}
