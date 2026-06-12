package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var db *pgxpool.Pool

func initDB() error {
	connStr := os.Getenv("DATABASE_CONNECTION_URI")
	if connStr == "" {
		return fmt.Errorf("DATABASE_CONNECTION_URI is not set")
	}

	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return fmt.Errorf("unable to parse connection string: %v", err)
	}

	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return fmt.Errorf("unable to create connection pool: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("unable to ping database: %v", err)
	}

	db = pool

	// Run migrations (create tables if not exist)
	if err := runMigrations(); err != nil {
		fmt.Printf("[DB] Migration warning (non-fatal): %v\n", err)
	}

	return nil
}

func runMigrations() error {
	migrations := []string{
		// Conversation history - Level 2 memory
		`CREATE TABLE IF NOT EXISTS conversation_history (
			id SERIAL PRIMARY KEY,
			group_jid TEXT NOT NULL,
			sender_jid TEXT NOT NULL,
			sender_name TEXT DEFAULT '',
			message TEXT NOT NULL,
			is_from_bot BOOLEAN DEFAULT false,
			quoted_msg_id TEXT DEFAULT '',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_convhist_group_time ON conversation_history(group_jid, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_convhist_sender ON conversation_history(sender_jid, created_at DESC)`,

		// User memory - Level 1 memory (per user per group)
		`CREATE TABLE IF NOT EXISTS user_memory (
			id SERIAL PRIMARY KEY,
			user_jid TEXT NOT NULL,
			group_jid TEXT NOT NULL DEFAULT '',
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			UNIQUE(user_jid, group_jid, key)
		)`,

		// Group memory - Level 3 memory
		`CREATE TABLE IF NOT EXISTS group_memory (
			id SERIAL PRIMARY KEY,
			group_jid TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			UNIQUE(group_jid, key)
		)`,

		// Conversation summaries - Level 4 memory (compression)
		`CREATE TABLE IF NOT EXISTS conversation_summaries (
			id SERIAL PRIMARY KEY,
			remote_jid VARCHAR(255) NOT NULL,
			summary TEXT NOT NULL,
			message_count INTEGER DEFAULT 0,
			period_start TIMESTAMP WITH TIME ZONE,
			period_end TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_summaries_remote_jid ON conversation_summaries(remote_jid, created_at DESC)`,

		// Notes / Reminders
		`CREATE TABLE IF NOT EXISTS user_notes (
			id SERIAL PRIMARY KEY,
			user_jid TEXT NOT NULL,
			group_jid TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			remind_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_user ON user_notes(user_jid, group_jid)`,

		// Group rules
		`CREATE TABLE IF NOT EXISTS group_rules (
			id SERIAL PRIMARY KEY,
			group_jid TEXT NOT NULL,
			rule_text TEXT NOT NULL,
			added_by TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_rules_group ON group_rules(group_jid)`,
	}

	for _, sql := range migrations {
		_, err := db.Exec(context.Background(), sql)
		if err != nil {
			fmt.Printf("[DB] Migration error: %v (SQL: %.80s...)\n", err, sql)
		}
	}

	return nil
}

// ============================================================================
// LEVEL 2: CONVERSATION HISTORY
// ============================================================================

// SaveMessage stores a message in conversation history
func SaveMessage(groupJid, senderJid, senderName, message string, isFromBot bool, quotedMsgId string) error {
	if message == "" {
		return nil
	}
	// Truncate very long messages to save space
	if len(message) > 500 {
		message = message[:500] + "..."
	}
	_, err := db.Exec(context.Background(),
		`INSERT INTO conversation_history (group_jid, sender_jid, sender_name, message, is_from_bot, quoted_msg_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		groupJid, senderJid, senderName, message, isFromBot, quotedMsgId)
	return err
}

// GetConversationContext returns the last N messages from a group
func GetConversationContext(groupJid string, limit int) ([]ConversationMessage, error) {
	rows, err := db.Query(context.Background(),
		`SELECT id, group_jid, sender_jid, sender_name, message, is_from_bot, quoted_msg_id, created_at
		 FROM conversation_history
		 WHERE group_jid = $1
		 ORDER BY created_at DESC LIMIT $2`,
		groupJid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ConversationMessage
	for rows.Next() {
		var m ConversationMessage
		if err := rows.Scan(&m.ID, &m.GroupJid, &m.SenderJid, &m.SenderName, &m.Message, &m.IsFromBot, &m.QuotedMsgId, &m.CreatedAt); err != nil {
			continue
		}
		messages = append(messages, m)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// GetUserMessages returns last N messages from a specific user in a group
func GetUserMessages(groupJid, senderJid string, limit int) ([]ConversationMessage, error) {
	rows, err := db.Query(context.Background(),
		`SELECT id, group_jid, sender_jid, sender_name, message, is_from_bot, quoted_msg_id, created_at
		 FROM conversation_history
		 WHERE group_jid = $1 AND (sender_jid = $2 OR (is_from_bot = true AND quoted_msg_id != ''))
		 ORDER BY created_at DESC LIMIT $2`,
		groupJid, senderJid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ConversationMessage
	for rows.Next() {
		var m ConversationMessage
		if err := rows.Scan(&m.ID, &m.GroupJid, &m.SenderJid, &m.SenderName, &m.Message, &m.IsFromBot, &m.QuotedMsgId, &m.CreatedAt); err != nil {
			continue
		}
		messages = append(messages, m)
	}

	// Reverse
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// FormatConversationHistory formats messages as a readable "theater script"
func FormatConversationHistory(messages []ConversationMessage) string {
	if len(messages) == 0 {
		return "(pas de messages recents)"
	}
	var sb strings.Builder
	for _, m := range messages {
		name := m.SenderName
		if name == "" {
			name = strings.Split(m.SenderJid, "@")[0]
		}
		if m.IsFromBot {
			name = "Poulga (Toi)"
		}
		sb.WriteString(fmt.Sprintf("- %s: %s\n", name, m.Message))
	}
	return sb.String()
}

// getRecentMessages returns formatted messages (legacy compat)
func getRecentMessages(remoteJid string, limit int) ([]string, error) {
	msgs, err := GetConversationContext(remoteJid, limit)
	if err != nil {
		// Fallback to Evolution's Message table
		return getRecentMessagesFromEvolution(remoteJid, limit)
	}
	if len(msgs) == 0 {
		return getRecentMessagesFromEvolution(remoteJid, limit)
	}

	var result []string
	for _, m := range msgs {
		name := m.SenderName
		if name == "" {
			name = strings.Split(m.SenderJid, "@")[0]
		}
		if m.IsFromBot {
			name = "Poulga"
		}
		result = append(result, fmt.Sprintf("%s: %s", name, m.Message))
	}
	return result, nil
}

// getRecentMessagesFromEvolution reads from Evolution API's Message table (fallback)
func getRecentMessagesFromEvolution(remoteJid string, limit int) ([]string, error) {
	rows, err := db.Query(context.Background(),
		`SELECT COALESCE(message->>'conversation', message->'extendedTextMessage'->>'text') as content, 
		 COALESCE("pushName", 'Utilisateur') 
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
			continue
		}
		messages = append([]string{fmt.Sprintf("%s: %s", pushName, content)}, messages...)
	}
	return messages, nil
}

// ============================================================================
// LEVEL 1: USER MEMORY (per-user knowledge)
// ============================================================================

func SaveUserMemory(userJid, groupJid, key, value string) error {
	_, err := db.Exec(context.Background(),
		`INSERT INTO user_memory (user_jid, group_jid, key, value)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_jid, group_jid, key) DO UPDATE SET
		 value = EXCLUDED.value, updated_at = NOW()`,
		userJid, groupJid, key, value)
	return err
}

func GetUserMemory(userJid, groupJid string) ([]UserMemory, error) {
	rows, err := db.Query(context.Background(),
		`SELECT id, user_jid, group_jid, key, value, created_at
		 FROM user_memory
		 WHERE user_jid = $1 AND (group_jid = $2 OR group_jid = '')
		 ORDER BY updated_at DESC LIMIT 20`,
		userJid, groupJid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []UserMemory
	for rows.Next() {
		var m UserMemory
		if err := rows.Scan(&m.ID, &m.UserJid, &m.GroupJid, &m.Key, &m.Value, &m.CreatedAt); err != nil {
			continue
		}
		memories = append(memories, m)
	}
	return memories, nil
}

func FormatUserMemory(memories []UserMemory) string {
	if len(memories) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, m := range memories {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", m.Key, m.Value))
	}
	return sb.String()
}

// ============================================================================
// LEVEL 3: GROUP MEMORY
// ============================================================================

func SaveGroupMemory(groupJid, key, value string) error {
	_, err := db.Exec(context.Background(),
		`INSERT INTO group_memory (group_jid, key, value)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (group_jid, key) DO UPDATE SET
		 value = EXCLUDED.value, updated_at = NOW()`,
		groupJid, key, value)
	return err
}

func GetGroupMemory(groupJid string) ([]GroupMemoryEntry, error) {
	rows, err := db.Query(context.Background(),
		`SELECT id, group_jid, key, value, created_at
		 FROM group_memory
		 WHERE group_jid = $1
		 ORDER BY updated_at DESC LIMIT 20`,
		groupJid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []GroupMemoryEntry
	for rows.Next() {
		var e GroupMemoryEntry
		if err := rows.Scan(&e.ID, &e.GroupJid, &e.Key, &e.Value, &e.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func FormatGroupMemory(entries []GroupMemoryEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", e.Key, e.Value))
	}
	return sb.String()
}

// ============================================================================
// LEVEL 4: CONVERSATION SUMMARIES (compression)
// ============================================================================

func SaveConversationSummary(remoteJid, summary string, msgCount int, periodStart, periodEnd time.Time) error {
	_, err := db.Exec(context.Background(),
		`INSERT INTO conversation_summaries (remote_jid, summary, message_count, period_start, period_end)
		 VALUES ($1, $2, $3, $4, $5)`,
		remoteJid, summary, msgCount, periodStart, periodEnd)
	return err
}

func GetLatestSummary(remoteJid string) (string, error) {
	var summary string
	err := db.QueryRow(context.Background(),
		`SELECT summary FROM conversation_summaries 
		 WHERE remote_jid = $1 ORDER BY created_at DESC LIMIT 1`,
		remoteJid).Scan(&summary)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return summary, err
}

func GetMessageCountSinceLastSummary(remoteJid string) (int, error) {
	var lastSummaryTime time.Time
	err := db.QueryRow(context.Background(),
		`SELECT COALESCE(MAX(period_end), '1970-01-01'::timestamp) FROM conversation_summaries WHERE remote_jid = $1`,
		remoteJid).Scan(&lastSummaryTime)
	if err != nil {
		return 0, err
	}

	var count int
	err = db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM conversation_history WHERE group_jid = $1 AND created_at > $2`,
		remoteJid, lastSummaryTime).Scan(&count)
	return count, err
}

// ============================================================================
// FACTS (existing system, kept for backward compat)
// ============================================================================

func getFacts(remoteJid string) ([]string, error) {
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
		facts = append(facts, content)
	}
	return facts, nil
}

func getFactsDetailed(remoteJid string) ([]FactEntry, error) {
	rows, err := db.Query(context.Background(),
		"SELECT id, content FROM facts WHERE remote_jid = $1 ORDER BY created_at DESC LIMIT 20",
		remoteJid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []FactEntry
	for rows.Next() {
		var f FactEntry
		if err := rows.Scan(&f.ID, &f.Content); err == nil {
			facts = append(facts, f)
		}
	}
	return facts, nil
}

func addFact(remoteJid, content string) error {
	_, err := db.Exec(context.Background(),
		"INSERT INTO facts (remote_jid, content) VALUES ($1, $2)",
		remoteJid, content)
	return err
}

func deleteFact(remoteJid string, id int) error {
	_, err := db.Exec(context.Background(),
		"DELETE FROM facts WHERE remote_jid = $1 AND id = $2",
		remoteJid, id)
	return err
}

// ============================================================================
// SOCIAL GRAPH
// ============================================================================

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

// ============================================================================
// MEMBER PROFILES
// ============================================================================

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
		 ORDER BY message_count DESC LIMIT 15`,
		groupJid)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var profiles []string
	for rows.Next() {
		var name string
		var skills, interests *string
		var count int
		if err := rows.Scan(&name, &skills, &interests, &count); err != nil {
			continue
		}
		s := "?"
		i := "?"
		if skills != nil {
			s = *skills
		}
		if interests != nil {
			i = *interests
		}
		profiles = append(profiles, fmt.Sprintf("- %s (%d msgs) | Skills: %s | Interets: %s", name, count, s, i))
	}
	return strings.Join(profiles, "\n"), nil
}

func getGroupCartography(remoteJid string) (string, error) {
	return getMemberProfiles(remoteJid)
}

// ============================================================================
// GROUP SETTINGS
// ============================================================================

func getGroupSettings(groupJid string) (GroupSettings, error) {
	var s GroupSettings
	err := db.QueryRow(context.Background(),
		`SELECT welcome_enabled, antilink_enabled, antispam_enabled, antisuppression_enabled, is_closed
		 FROM group_settings WHERE group_jid = $1`,
		groupJid).Scan(&s.WelcomeEnabled, &s.AntiLinkEnabled, &s.AntiSpamEnabled, &s.AntiSuppressionEnabled, &s.IsClosed)

	if err == pgx.ErrNoRows {
		return GroupSettings{WelcomeEnabled: true}, nil
	}
	return s, err
}

func updateGroupSetting(groupJid, column string, value bool) error {
	query := fmt.Sprintf(`INSERT INTO group_settings (group_jid, %s) VALUES ($1, $2)
		 ON CONFLICT (group_jid) DO UPDATE SET %s = EXCLUDED.%s, updated_at = CURRENT_TIMESTAMP`,
		column, column, column)
	_, err := db.Exec(context.Background(), query, groupJid, value)
	return err
}

// ============================================================================
// WARNINGS
// ============================================================================

func addWarning(jid, groupJid string) (int, error) {
	var count int
	err := db.QueryRow(context.Background(),
		`INSERT INTO user_warnings (jid, group_jid, warning_count) VALUES ($1, $2, 1)
		 ON CONFLICT (jid, group_jid) DO UPDATE SET
		 warning_count = user_warnings.warning_count + 1,
		 last_warning = CURRENT_TIMESTAMP
		 RETURNING warning_count`,
		jid, groupJid).Scan(&count)
	return count, err
}

func getWarnings(jid, groupJid string) (int, error) {
	var count int
	err := db.QueryRow(context.Background(),
		"SELECT warning_count FROM user_warnings WHERE jid = $1 AND group_jid = $2",
		jid, groupJid).Scan(&count)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return count, err
}

func resetWarnings(jid, groupJid string) error {
	_, err := db.Exec(context.Background(),
		"DELETE FROM user_warnings WHERE jid = $1 AND group_jid = $2",
		jid, groupJid)
	return err
}

func listWarnings(groupJid string) (string, error) {
	rows, err := db.Query(context.Background(),
		"SELECT jid, warning_count FROM user_warnings WHERE group_jid = $1 ORDER BY warning_count DESC",
		groupJid)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb strings.Builder
	found := false
	for rows.Next() {
		var jid string
		var count int
		if err := rows.Scan(&jid, &count); err == nil {
			sb.WriteString(fmt.Sprintf("- @%s : %d/3\n", strings.Split(jid, "@")[0], count))
			found = true
		}
	}
	if !found {
		return "Aucun utilisateur averti. Tout va bien !", nil
	}
	return sb.String(), nil
}

// ============================================================================
// NOTES & REMINDERS
// ============================================================================

func addNote(userJid, groupJid, content string) error {
	_, err := db.Exec(context.Background(),
		`INSERT INTO user_notes (user_jid, group_jid, content) VALUES ($1, $2, $3)`,
		userJid, groupJid, content)
	return err
}

func getNotes(userJid, groupJid string) ([]string, error) {
	rows, err := db.Query(context.Background(),
		`SELECT id, content FROM user_notes WHERE user_jid = $1 AND group_jid = $2 ORDER BY created_at DESC LIMIT 10`,
		userJid, groupJid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []string
	for rows.Next() {
		var id int
		var content string
		if err := rows.Scan(&id, &content); err == nil {
			notes = append(notes, fmt.Sprintf("[%d] %s", id, content))
		}
	}
	return notes, nil
}

func deleteNote(userJid, groupJid string, id int) error {
	_, err := db.Exec(context.Background(),
		`DELETE FROM user_notes WHERE id = $1 AND user_jid = $2 AND group_jid = $3`,
		id, userJid, groupJid)
	return err
}

// ============================================================================
// GROUP RULES
// ============================================================================

func addRule(groupJid, ruleText, addedBy string) error {
	_, err := db.Exec(context.Background(),
		`INSERT INTO group_rules (group_jid, rule_text, added_by) VALUES ($1, $2, $3)`,
		groupJid, ruleText, addedBy)
	return err
}

func getRules(groupJid string) ([]string, error) {
	rows, err := db.Query(context.Background(),
		`SELECT id, rule_text FROM group_rules WHERE group_jid = $1 ORDER BY id ASC`,
		groupJid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []string
	for rows.Next() {
		var id int
		var text string
		if err := rows.Scan(&id, &text); err == nil {
			rules = append(rules, fmt.Sprintf("%d. %s", id, text))
		}
	}
	return rules, nil
}

func deleteRule(groupJid string, id int) error {
	_, err := db.Exec(context.Background(),
		`DELETE FROM group_rules WHERE id = $1 AND group_jid = $2`,
		id, groupJid)
	return err
}

// ============================================================================
// CLEANUP
// ============================================================================

func cleanupOldMessages(days int) error {
	// Clean conversation_history older than N days
	_, err := db.Exec(context.Background(),
		fmt.Sprintf(`DELETE FROM conversation_history WHERE created_at < NOW() - INTERVAL '%d days'`, days))
	if err != nil {
		fmt.Printf("[DB] Cleanup conversation_history error: %v\n", err)
	}

	// Also clean Evolution's Message table
	_, err2 := db.Exec(context.Background(),
		fmt.Sprintf(`DELETE FROM public."Message" 
		 WHERE "messageTimestamp" < extract(epoch from (now() - interval '%d days'))`, days))
	if err2 != nil {
		fmt.Printf("[DB] Cleanup Message table error: %v\n", err2)
	}

	return err
}
