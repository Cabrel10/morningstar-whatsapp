package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"
	"github.com/go-shiori/go-readability"
	"github.com/ollama/ollama/api"
)

// ============================================================================
// TOOL DEFINITIONS — exposed to the LLM via Ollama native tool calling
//
// DESIGN (2026-06-25): Every tool must satisfy three contracts:
//   1. Declared here in GetOllamaTools()  → so the model sees it
//   2. Arg-mapped in buildToolCallArgs()  → so we extract arguments generically
//   3. Executed in ExecuteTool()           → so the action actually happens
//
// Tools are split into categories:
//   • Web       : google_search, web_read
//   • Memory    : add_note, reminder
//   • Group     : tagall, summary, stats, rules, poll
//   • Admin     : kick_user, ban_user, mute_user, promote_user, demote_user
// ============================================================================

func GetOllamaTools() []api.Tool {
	var tools []api.Tool

	// ---- WEB TOOLS ----

	t1 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "google_search",
		Description: "Chercher des informations actuelles sur internet (actualités, sport, météo, etc.). Utilise cet outil quand on te demande des infos que tu ne connais pas ou qui changent souvent.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"query":{"type":"string","description":"Les mots-clés de recherche"}},"required":["query"]}`), &t1.Function.Parameters)
	tools = append(tools, t1)

	t2 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "web_read",
		Description: "Lire et extraire le contenu d'une page web à partir de son URL.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"url":{"type":"string","description":"URL complète du site"}},"required":["url"]}`), &t2.Function.Parameters)
	tools = append(tools, t2)

	// ---- MEMORY TOOLS ----

	t3 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "add_note",
		Description: "Enregistrer une note personnelle pour l'utilisateur.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"text":{"type":"string","description":"Contenu de la note"}},"required":["text"]}`), &t3.Function.Parameters)
	tools = append(tools, t3)

	t9 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "reminder",
		Description: "Créer un rappel pour l'utilisateur.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"text":{"type":"string","description":"Le texte du rappel"}},"required":["text"]}`), &t9.Function.Parameters)
	tools = append(tools, t9)

	// ---- GROUP TOOLS ----

	t4 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "tagall",
		Description: "Mentionner tous les membres du groupe WhatsApp.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"message":{"type":"string","description":"Message optionnel à envoyer avec la mention"}},"required":[]}`), &t4.Function.Parameters)
	tools = append(tools, t4)

	t5 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "summary",
		Description: "Générer un résumé des messages récents du groupe.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{},"required":[]}`), &t5.Function.Parameters)
	tools = append(tools, t5)

	t6 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "stats",
		Description: "Afficher les statistiques d'activité du groupe.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{},"required":[]}`), &t6.Function.Parameters)
	tools = append(tools, t6)

	t7 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "rules",
		Description: "Afficher les règles du groupe.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{},"required":[]}`), &t7.Function.Parameters)
	tools = append(tools, t7)

	t8 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "poll",
		Description: "Créer un sondage dans le groupe.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"question":{"type":"string","description":"La question du sondage"},"options":{"type":"string","description":"Options séparées par | (ex: Oui|Non|Peut-être)"}},"required":["question","options"]}`), &t8.Function.Parameters)
	tools = append(tools, t8)

	// ---- ADMIN TOOLS ----
	// These require bot admin rights in the WhatsApp group.
	// ExecuteTool checks isUserAdmin() before executing.

	tKick := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "kick_user",
		Description: "Expulser (retirer) un membre du groupe WhatsApp. Nécessite les droits admin.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"user":{"type":"string","description":"Le numéro ou @mention de l'utilisateur à expulser"},"reason":{"type":"string","description":"Raison de l'expulsion (optionnel)"}},"required":["user"]}`), &tKick.Function.Parameters)
	tools = append(tools, tKick)

	tBan := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "ban_user",
		Description: "Bannir un membre du groupe (expulsion + blocage). Nécessite les droits admin.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"user":{"type":"string","description":"Le numéro ou @mention de l'utilisateur à bannir"},"reason":{"type":"string","description":"Raison du ban (optionnel)"}},"required":["user"]}`), &tBan.Function.Parameters)
	tools = append(tools, tBan)

	tMute := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "mute_user",
		Description: "Rétrograder un membre pour qu'il ne puisse plus envoyer de messages (demote). Nécessite les droits admin.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"user":{"type":"string","description":"Le numéro ou @mention de l'utilisateur à muter"}},"required":["user"]}`), &tMute.Function.Parameters)
	tools = append(tools, tMute)

	tPromote := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "promote_user",
		Description: "Promouvoir un membre en administrateur du groupe. Nécessite les droits admin.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"user":{"type":"string","description":"Le numéro ou @mention de l'utilisateur à promouvoir"}},"required":["user"]}`), &tPromote.Function.Parameters)
	tools = append(tools, tPromote)

	tDemote := api.Tool{Type: "function", Function: api.ToolFunction{
		Name:        "demote_user",
		Description: "Rétrograder un administrateur en simple membre. Nécessite les droits admin.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"user":{"type":"string","description":"Le numéro ou @mention de l'utilisateur à rétrograder"}},"required":["user"]}`), &tDemote.Function.Parameters)
	tools = append(tools, tDemote)

	return tools
}

// ============================================================================
// TOOL CALL TYPES
// ============================================================================

type ToolCall struct {
	Tool string `json:"tool"`
	Args string `json:"args"`
}

type ToolResult struct {
	Success  bool
	Response string
}

// ============================================================================
// buildToolCallArgs — GENERIC argument extraction from Ollama native tool calls
//
// Instead of per-tool if/else in main.go, this function handles ALL tools
// uniformly. It extracts the "primary" argument for simple tools (single string
// param) and compound arguments for complex tools (poll, admin actions).
// ============================================================================

func buildToolCallArgs(toolName string, arguments map[string]any) string {
	// Helper: safely extract a string from the map
	str := func(key string) string {
		if v, ok := arguments[key]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}

	switch toolName {
	// Simple single-argument tools
	case "google_search":
		return str("query")
	case "web_read":
		return str("url")
	case "add_note":
		return str("text")
	case "reminder":
		return str("text")
	case "tagall":
		return str("message")

	// Compound tools
	case "poll":
		q := str("question")
		o := str("options")
		if q == "" {
			return ""
		}
		return q + "|" + o

	// Admin tools — user is the primary arg, reason is secondary
	case "kick_user", "ban_user", "mute_user", "promote_user", "demote_user":
		user := str("user")
		reason := str("reason")
		if reason != "" {
			return user + "|" + reason
		}
		return user

	// No-argument tools
	case "summary", "stats", "rules":
		return ""

	default:
		// Fallback: try common param names
		for _, key := range []string{"query", "text", "url", "user", "message"} {
			if v := str(key); v != "" {
				return v
			}
		}
		return ""
	}
}

// ParseToolCall is kept for backward compatibility but is no longer the primary
// path. The native Ollama api.ToolCall mechanism in processLLMResponse is used
// instead. This function handles the RARE case where a model outputs a JSON
// tool call as plain text (no native tool support).
func ParseToolCall(response string) (ToolCall, bool) {
	// Try to find JSON {"tool":"...", "args":"..."} in the response
	startIdx := strings.Index(response, `{"tool"`)
	if startIdx == -1 {
		return ToolCall{}, false
	}
	endIdx := strings.Index(response[startIdx:], "}")
	if endIdx == -1 {
		return ToolCall{}, false
	}
	jsonStr := response[startIdx : startIdx+endIdx+1]
	var tc ToolCall
	if err := json.Unmarshal([]byte(jsonStr), &tc); err != nil {
		return ToolCall{}, false
	}
	if tc.Tool == "" {
		return ToolCall{}, false
	}
	return tc, true
}

// ============================================================================
// TOOL EXECUTION — runs the actual tool logic
//
// Admin tools verify that the REQUESTING user (ctx.SenderJid) has admin rights
// before executing. The LLM cannot bypass this check.
// ============================================================================

func ExecuteTool(tc ToolCall, ctx MessageContext) ToolResult {
	fmt.Printf("[TOOL] Executing tool=%q args=%q from=%s\n", tc.Tool, tc.Args, ctx.PushName)

	switch tc.Tool {

	// ---- WEB ----

	case "google_search":
		if tc.Args == "" {
			return ToolResult{false, "Mots-clés manquants pour la recherche."}
		}
		results, err := WebSearch(tc.Args, 5)
		if err != nil {
			return ToolResult{false, fmt.Sprintf("Erreur recherche web : %v", err)}
		}
		if len(results) == 0 {
			return ToolResult{true, "Aucun résultat trouvé sur le web pour : " + tc.Args}
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Résultats de recherche pour '%s' :\n\n", tc.Args))
		for i, r := range results {
			sb.WriteString(fmt.Sprintf("[%d] %s\nURL: %s\nExtrait: %s\n\n", i+1, r.Title, r.URL, r.Snippet))
		}
		return ToolResult{true, sb.String()}

	case "web_read":
		if tc.Args == "" {
			return ToolResult{false, "URL manquante."}
		}
		text, err := scrapeURL(tc.Args)
		if err != nil {
			return ToolResult{false, fmt.Sprintf("Erreur lors de la lecture du site : %v", err)}
		}
		return ToolResult{true, fmt.Sprintf("Contenu extrait du site %s :\n\n%s", tc.Args, text)}

	// ---- MEMORY ----

	case "add_note":
		if tc.Args == "" {
			return ToolResult{false, "Il faut préciser le contenu de la note."}
		}
		err := addNote(ctx.SenderJid, ctx.RemoteJid, tc.Args)
		if err != nil {
			return ToolResult{false, "Erreur lors de l'ajout de la note."}
		}
		return ToolResult{true, fmt.Sprintf("Note enregistrée : \"%s\"", tc.Args)}

	case "reminder":
		if tc.Args == "" {
			return ToolResult{false, "Contenu du rappel manquant."}
		}
		err := addNote(ctx.SenderJid, ctx.RemoteJid, "[RAPPEL] "+tc.Args)
		if err != nil {
			return ToolResult{false, "Erreur lors de la création du rappel."}
		}
		return ToolResult{true, "Rappel enregistré : " + tc.Args}

	// ---- GROUP ----

	case "tagall":
		if !strings.HasSuffix(ctx.RemoteJid, "@g.us") {
			return ToolResult{false, "Cette commande ne fonctionne que dans les groupes."}
		}
		participants, err := getGroupMetadata(ctx.Instance, ctx.RemoteJid)
		if err != nil {
			return ToolResult{false, "Erreur lors de la récupération des membres."}
		}
		var mentions []string
		var text strings.Builder
		msg := tc.Args
		if msg == "" {
			msg = "Appel général"
		}
		text.WriteString(fmt.Sprintf("*📢 %s:*\n\n", msg))
		for _, p := range participants {
			number := strings.Split(p, "@")[0]
			text.WriteString(fmt.Sprintf("@%s ", number))
			mentions = append(mentions, p)
		}
		_, _ = sendWhatsAppMessageWithMentions(ctx.Instance, ctx.RemoteJid, text.String(), mentions)
		return ToolResult{true, "Appel général effectué avec succès."}

	case "summary":
		history, _ := GetConversationContext(ctx.RemoteJid, 50)
		if len(history) == 0 {
			return ToolResult{true, "Pas de messages récents pour faire un résumé."}
		}
		res := FormatConversationHistory(history)
		return ToolResult{true, "Voici l'historique récent des messages :\n\n" + res + "\n\nFais-en une synthèse."}

	case "stats":
		cartography, _ := getGroupCartography(ctx.RemoteJid)
		if cartography == "" {
			return ToolResult{true, "Pas encore assez de données pour ce groupe."}
		}
		return ToolResult{true, "Voici les statistiques du groupe :\n\n" + cartography}

	case "rules":
		rulesList, err := getRules(ctx.RemoteJid)
		if err != nil || len(rulesList) == 0 {
			return ToolResult{true, "Aucune règle définie pour ce groupe."}
		}
		return ToolResult{true, "*Règles du groupe:*\n" + strings.Join(rulesList, "\n")}

	case "poll":
		parts := strings.Split(tc.Args, "|")
		if len(parts) < 2 {
			return ToolResult{false, "Format: question|option1|option2|..."}
		}
		question := strings.TrimSpace(parts[0])
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("*📊 SONDAGE:* %s\n\n", question))
		emojis := []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣"}
		for i, opt := range parts[1:] {
			if i >= len(emojis) {
				break
			}
			sb.WriteString(fmt.Sprintf("%s %s\n", emojis[i], strings.TrimSpace(opt)))
		}
		sb.WriteString("\nRéagissez avec le numéro correspondant !")
		return ToolResult{true, sb.String()}

	// ---- ADMIN TOOLS ----
	// All admin tools verify that the SENDER has admin rights before executing.
	// The target user JID is resolved from the args (number, @mention, or name).

	case "kick_user":
		return executeAdminAction(ctx, tc.Args, "kick")

	case "ban_user":
		return executeAdminAction(ctx, tc.Args, "ban")

	case "mute_user":
		return executeAdminAction(ctx, tc.Args, "mute")

	case "promote_user":
		return executeAdminAction(ctx, tc.Args, "promote")

	case "demote_user":
		return executeAdminAction(ctx, tc.Args, "demote")

	default:
		return ToolResult{false, fmt.Sprintf("Outil '%s' non reconnu.", tc.Tool)}
	}
}

// ============================================================================
// ADMIN ACTION EXECUTOR
//
// Centralizes the logic for all admin tools:
//   1. Verify the requesting user is admin
//   2. Resolve the target user JID from the argument (number, @mention, name)
//   3. Execute the Evolution API call
//   4. Return a human-readable result for the LLM to relay
// ============================================================================

func executeAdminAction(ctx MessageContext, args string, action string) ToolResult {
	// Check if it's a group
	if !strings.HasSuffix(ctx.RemoteJid, "@g.us") {
		return ToolResult{false, "Les actions admin ne fonctionnent que dans les groupes."}
	}

	// Check admin rights of the SENDER (not the target)
	isAdmin, err := isUserAdmin(ctx.Instance, ctx.RemoteJid, ctx.SenderJid)
	if err != nil || !isAdmin {
		return ToolResult{false, "Tu n'as pas les droits administrateur pour cette action."}
	}

	// Parse args: "user|reason" or just "user"
	parts := strings.SplitN(args, "|", 2)
	userArg := strings.TrimSpace(parts[0])
	reason := ""
	if len(parts) > 1 {
		reason = strings.TrimSpace(parts[1])
	}

	if userArg == "" {
		return ToolResult{false, "Tu dois préciser l'utilisateur ciblé."}
	}

	// Resolve the target JID
	targetJid := resolveUserJid(userArg, ctx.RemoteJid)
	if targetJid == "" {
		return ToolResult{false, fmt.Sprintf("Impossible de trouver l'utilisateur '%s'. Utilise un numéro ou @mention.", userArg)}
	}

	// Prevent self-actions
	senderClean := strings.Split(strings.Split(ctx.SenderJid, "@")[0], ":")[0]
	targetClean := strings.Split(strings.Split(targetJid, "@")[0], ":")[0]
	if senderClean == targetClean {
		return ToolResult{false, "Tu ne peux pas exécuter cette action sur toi-même."}
	}

	targetName := GetMemberName(targetJid, ctx.RemoteJid, targetClean)
	reasonStr := ""
	if reason != "" {
		reasonStr = fmt.Sprintf(" (Raison : %s)", reason)
	}

	// Execute the action via Evolution API
	switch action {
	case "kick":
		err := kickUser(ctx.Instance, ctx.RemoteJid, targetJid)
		if err != nil {
			fmt.Printf("[ADMIN] kick error: %v\n", err)
			return ToolResult{false, fmt.Sprintf("Erreur lors de l'expulsion de %s : %v", targetName, err)}
		}
		return ToolResult{true, fmt.Sprintf("✅ %s a été expulsé du groupe.%s", targetName, reasonStr)}

	case "ban":
		// Ban = kick (WhatsApp doesn't have a native ban, kick is the equivalent)
		err := kickUser(ctx.Instance, ctx.RemoteJid, targetJid)
		if err != nil {
			fmt.Printf("[ADMIN] ban(kick) error: %v\n", err)
			return ToolResult{false, fmt.Sprintf("Erreur lors du ban de %s : %v", targetName, err)}
		}
		return ToolResult{true, fmt.Sprintf("🚫 %s a été banni du groupe.%s", targetName, reasonStr)}

	case "mute":
		// Mute = demote (remove ability to send in announcement groups)
		err := demoteUser(ctx.Instance, ctx.RemoteJid, targetJid)
		if err != nil {
			fmt.Printf("[ADMIN] mute(demote) error: %v\n", err)
			return ToolResult{false, fmt.Sprintf("Erreur lors du mute de %s : %v", targetName, err)}
		}
		return ToolResult{true, fmt.Sprintf("🔇 %s a été muté (rétrogradé).%s", targetName, reasonStr)}

	case "promote":
		err := promoteUser(ctx.Instance, ctx.RemoteJid, targetJid)
		if err != nil {
			fmt.Printf("[ADMIN] promote error: %v\n", err)
			return ToolResult{false, fmt.Sprintf("Erreur lors de la promotion de %s : %v", targetName, err)}
		}
		return ToolResult{true, fmt.Sprintf("👑 %s a été promu administrateur.", targetName)}

	case "demote":
		err := demoteUser(ctx.Instance, ctx.RemoteJid, targetJid)
		if err != nil {
			fmt.Printf("[ADMIN] demote error: %v\n", err)
			return ToolResult{false, fmt.Sprintf("Erreur lors de la rétrogradation de %s : %v", targetName, err)}
		}
		return ToolResult{true, fmt.Sprintf("⬇️ %s a été rétrogradé en membre simple.", targetName)}

	default:
		return ToolResult{false, "Action admin inconnue."}
	}
}

// resolveUserJid converts a user reference (number, @mention, name) into a
// WhatsApp JID (number@s.whatsapp.net).
func resolveUserJid(input string, groupJid string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	// Already a full JID
	if strings.Contains(input, "@s.whatsapp.net") {
		return input
	}

	// @number format (WhatsApp mention)
	if strings.HasPrefix(input, "@") {
		num := strings.TrimPrefix(input, "@")
		// If it's all digits, it's a phone number
		if isAllDigits(num) {
			return num + "@s.whatsapp.net"
		}
		// Otherwise it might be a name — search in members
		return resolveNameToJid(num, groupJid)
	}

	// Pure number
	cleaned := strings.ReplaceAll(input, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "+", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	if isAllDigits(cleaned) && len(cleaned) >= 7 {
		return cleaned + "@s.whatsapp.net"
	}

	// Fallback: try resolving as a name
	return resolveNameToJid(input, groupJid)
}

// resolveNameToJid searches group members by custom_name or push_name to find
// the JID matching the given name (case-insensitive).
func resolveNameToJid(name string, groupJid string) string {
	if name == "" || groupJid == "" {
		return ""
	}
	lowerName := strings.ToLower(name)

	// Search in member_profiles (custom names)
	rows, err := db.Query(contextBG(), `
		SELECT jid FROM member_profiles 
		WHERE group_jid = $1 AND LOWER(custom_name) = $2
		LIMIT 1`, groupJid, lowerName)
	if err == nil {
		defer rows.Close()
		if rows.Next() {
			var jid string
			if rows.Scan(&jid) == nil {
				return jid
			}
		}
	}

	// Search in member_details (push names)
	rows2, err := db.Query(contextBG(), `
		SELECT jid FROM member_details 
		WHERE group_jid = $1 AND LOWER(push_name) = $2
		LIMIT 1`, groupJid, lowerName)
	if err == nil {
		defer rows2.Close()
		if rows2.Next() {
			var jid string
			if rows2.Scan(&jid) == nil {
				return jid
			}
		}
	}

	// Partial match (contains)
	rows3, err := db.Query(contextBG(), `
		SELECT jid FROM member_profiles 
		WHERE group_jid = $1 AND LOWER(custom_name) LIKE '%' || $2 || '%'
		LIMIT 1`, groupJid, lowerName)
	if err == nil {
		defer rows3.Close()
		if rows3.Next() {
			var jid string
			if rows3.Scan(&jid) == nil {
				return jid
			}
		}
	}

	return ""
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func contextBG() context.Context {
	return context.Background()
}

// ============================================================================
// KEYWORD-BASED TOOL DETECTION (fallback for non-tool-calling models)
// ============================================================================

func DetectToolFromKeywords(text string) (ToolCall, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ToolCall{}, false
	}
	notePatterns := []string{"ajoute une note", "note :", "note:", "prends note", "enregistre", "sauvegarde"}
	for _, p := range notePatterns {
		if strings.Contains(lower, p) {
			content := extractAfterKeyword(text, p)
			if content != "" {
				return ToolCall{Tool: "add_note", Args: content}, true
			}
		}
	}
	return ToolCall{}, false
}

func extractAfterKeyword(text, keyword string) string {
	lower := strings.ToLower(text)
	keyLower := strings.ToLower(keyword)
	idx := strings.Index(lower, keyLower)
	if idx == -1 {
		return ""
	}
	after := strings.TrimSpace(text[idx+len(keyword):])
	after = strings.TrimLeft(after, " :!?,.")
	return strings.TrimSpace(after)
}

// ============================================================================
// WEB SCRAPING
// ============================================================================

func scrapeURL(targetURL string) (string, error) {
	fmt.Printf("[SCRAPE] Starting: %s\n", targetURL)
	if !strings.HasPrefix(targetURL, "http") {
		targetURL = "https://" + targetURL
	}
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("URL invalide")
	}
	hostname := parsedURL.Hostname()
	ips, err := net.LookupIP(hostname)
	if err == nil {
		for _, ip := range ips {
			if isInternalIP(ip) {
				return "", fmt.Errorf("accès interdit aux réseaux locaux")
			}
		}
	}
	client := resty.New().SetTimeout(20 * time.Second).
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	resp, err := client.R().SetDoNotParseResponse(true).Get(targetURL)
	if err != nil {
		return "", err
	}
	defer resp.RawBody().Close()
	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("status code: %d", resp.StatusCode())
	}
	const maxBodySize = 5 * 1024 * 1024
	limitedReader := io.LimitReader(resp.RawBody(), maxBodySize)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("erreur de lecture")
	}
	return processHTML(string(bodyBytes), targetURL)
}

func isInternalIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 10:
			return true
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return true
		case ip4[0] == 192 && ip4[1] == 168:
			return true
		}
	}
	return false
}

func processHTML(html, targetURL string) (string, error) {
	article, err := readability.FromReader(strings.NewReader(html), nil)
	if err == nil && len(article.TextContent) > 200 {
		text := cleanContentText(article.TextContent)
		if len(text) > 3000 {
			text = text[:3000] + "..."
		}
		return text, nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}
	doc.Find("script, style, head, nav, footer, iframe, header, noscript").Remove()
	text := cleanContentText(doc.Find("body").Text())
	if len(text) > 3000 {
		text = text[:3000] + "..."
	}
	if len(text) < 10 {
		return "", fmt.Errorf("could not extract meaningful text from page")
	}
	return text, nil
}

func cleanContentText(text string) string {
	reSpace := regexp.MustCompile(`\s+`)
	text = reSpace.ReplaceAllString(text, " ")
	reNewlines := regexp.MustCompile(`\n{3,}`)
	return strings.TrimSpace(reNewlines.ReplaceAllString(text, "\n\n"))
}
