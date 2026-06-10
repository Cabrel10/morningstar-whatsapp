package main

import (
	"context"
	"fmt"
	"os"

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

func addFact(remoteJid, content string) error {
	_, err := db.Exec(context.Background(), 
		"INSERT INTO facts (remote_jid, content) VALUES ($1, $2)", 
		remoteJid, content)
	return err
}
