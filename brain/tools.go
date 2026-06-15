package main

import (
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

// GetOllamaTools returns the list of tools defined for the Ollama API
func GetOllamaTools() []api.Tool {
	var tools []api.Tool

	// 1. Google Search
	t1 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name: "google_search", Description: "Chercher des infos actuelles sur le web.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"query":{"type":"string","description":"Mots-clés de recherche"}},"required":["query"]}`), &t1.Function.Parameters)
	tools = append(tools, t1)

	// 2. Web Read
	t2 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name: "web_read", Description: "Lire et extraire le contenu d'un site web.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"url":{"type":"string","description":"URL complète du site"}},"required":["url"]}`), &t2.Function.Parameters)
	tools = append(tools, t2)

	// 3. Add Note
	t3 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name: "add_note", Description: "Enregistrer une note personnelle pour l'utilisateur.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"text":{"type":"string","description":"Contenu de la note"}},"required":["text"]}`), &t3.Function.Parameters)
	tools = append(tools, t3)

	// 4. Tag All
	t4 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name: "tagall", Description: "Mentionner tous les membres du groupe.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"message":{"type":"string","description":"Message optionnel"}},"required":[]}`), &t4.Function.Parameters)
	tools = append(tools, t4)

	// 5. Summary
	t5 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name: "summary", Description: "Générer un résumé des messages récents.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{},"required":[]}`), &t5.Function.Parameters)
	tools = append(tools, t5)

	// 6. Stats
	t6 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name: "stats", Description: "Afficher les statistiques du groupe.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{},"required":[]}`), &t6.Function.Parameters)
	tools = append(tools, t6)

	// 7. Rules
	t7 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name: "rules", Description: "Afficher les règles du groupe.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{},"required":[]}`), &t7.Function.Parameters)
	tools = append(tools, t7)

	// 8. Poll
	t8 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name: "poll", Description: "Créer un sondage dans le groupe.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"question":{"type":"string","description":"La question"},"options":{"type":"string","description":"Options séparées par | (ex: Oui|Non)"}},"required":["question","options"]}`), &t8.Function.Parameters)
	tools = append(tools, t8)

	// 9. Reminder
	t9 := api.Tool{Type: "function", Function: api.ToolFunction{
		Name: "reminder", Description: "Créer un rappel pour l'utilisateur.",
	}}
	json.Unmarshal([]byte(`{"type":"object","properties":{"text":{"type":"string","description":"Le texte du rappel"}},"required":["text"]}`), &t9.Function.Parameters)
	tools = append(tools, t9)

	return tools
}

type ToolCall struct {
	Tool string `json:"tool"`
	Args string `json:"args"`
}

type ToolResult struct {
	Success  bool
	Response string
}

func ParseToolCall(response string) (ToolCall, bool) {
	return ToolCall{}, false
}

func ExecuteTool(tc ToolCall, ctx MessageContext) ToolResult {
	fmt.Printf("[TOOL] Executing tool=%q args=%q from=%s\n", tc.Tool, tc.Args, ctx.PushName)

	switch tc.Tool {

	case "google_search":
		if tc.Args == "" { return ToolResult{false, "Mots-clés manquants."} }
		results, err := WebSearch(tc.Args, 5)
		if err != nil { return ToolResult{false, fmt.Sprintf("Erreur recherche : %v", err)} }
		if len(results) == 0 { return ToolResult{true, "Aucun résultat trouvé sur le web."} }
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Résultats de recherche pour '%s' :\n\n", tc.Args))
		for i, r := range results {
			sb.WriteString(fmt.Sprintf("[%d] %s\nURL: %s\nExtrait: %s\n\n", i+1, r.Title, r.URL, r.Snippet))
		}
		return ToolResult{true, sb.String()}

	case "web_read":
		if tc.Args == "" { return ToolResult{false, "URL manquante."} }
		text, err := scrapeURL(tc.Args)
		if err != nil { return ToolResult{false, fmt.Sprintf("Erreur lors de la lecture du site : %v", err)} }
		return ToolResult{true, fmt.Sprintf("Contenu extrait du site %s :\n\n%s", tc.Args, text)}

	case "add_note":
		if tc.Args == "" { return ToolResult{false, "Il faut preciser le contenu de la note."} }
		err := addNote(ctx.SenderJid, ctx.RemoteJid, tc.Args)
		if err != nil { return ToolResult{false, "Erreur lors de l'ajout de la note."} }
		return ToolResult{true, fmt.Sprintf("Note enregistree: \"%s\"", tc.Args)}

	case "tagall":
		if !strings.HasSuffix(ctx.RemoteJid, "@g.us") {
			return ToolResult{false, "Cette commande ne fonctionne que dans les groupes."}
		}
		participants, err := getGroupMetadata(ctx.Instance, ctx.RemoteJid)
		if err != nil {
			return ToolResult{false, "Erreur lors de la recuperation des membres."}
		}
		var mentions []string
		var text strings.Builder
		msg := tc.Args
		if msg == "" { msg = "Appel général" }
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
		if len(history) == 0 { return ToolResult{true, "Pas de messages récents pour faire un résumé."} }
		res := FormatConversationHistory(history)
		return ToolResult{true, "Voici l'historique récent des messages :\n\n" + res + "\n\nFais-en une synthèse."}

	case "stats":
		cartography, _ := getGroupCartography(ctx.RemoteJid)
		if cartography == "" { return ToolResult{true, "Pas encore assez de données pour ce groupe."} }
		return ToolResult{true, "Voici les statistiques du groupe :\n\n" + cartography}

	case "rules":
		rules, err := getRules(ctx.RemoteJid)
		if err != nil || len(rules) == 0 { return ToolResult{true, "Aucune règle définie pour ce groupe."} }
		return ToolResult{true, "*Règles du groupe:*\n" + strings.Join(rules, "\n")}

	case "poll":
		parts := strings.Split(tc.Args, "|")
		if len(parts) < 2 { return ToolResult{false, "Format: question|option1|option2|..."} }
		question := strings.TrimSpace(parts[0])
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("*📊 SONDAGE:* %s\n\n", question))
		emojis := []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣"}
		options := parts[1:]
		if len(parts) == 2 { // If only question and options as string
			options = strings.Split(parts[1], "|")
		}
		for i, opt := range options {
			if i >= len(emojis) { break }
			sb.WriteString(fmt.Sprintf("%s %s\n", emojis[i], strings.TrimSpace(opt)))
		}
		sb.WriteString("\nRéagissez avec le numéro correspondant !")
		return ToolResult{true, sb.String()}

	case "reminder":
		if tc.Args == "" { return ToolResult{false, "Contenu manquant."} }
		err := addNote(ctx.SenderJid, ctx.RemoteJid, "[RAPPEL] "+tc.Args)
		if err != nil { return ToolResult{false, "Erreur."} }
		return ToolResult{true, "Rappel enregistré."}

	default:
		return ToolResult{false, "Outil non reconnu."}
	}
}

func DetectToolFromKeywords(text string) (ToolCall, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" { return ToolCall{}, false }
	notePatterns := []string{"ajoute une note", "note :", "note:", "prends note", "enregistre", "sauvegarde"}
	for _, p := range notePatterns {
		if strings.Contains(lower, p) {
			content := extractAfterKeyword(text, p)
			if content != "" { return ToolCall{Tool: "add_note", Args: content}, true }
		}
	}
	return ToolCall{}, false
}

func extractAfterKeyword(text, keyword string) string {
	lower := strings.ToLower(text)
	keyLower := strings.ToLower(keyword)
	idx := strings.Index(lower, keyLower)
	if idx == -1 { return "" }
	after := strings.TrimSpace(text[idx+len(keyword):])
	after = strings.TrimLeft(after, " :!?,.")
	return strings.TrimSpace(after)
}

func scrapeURL(targetURL string) (string, error) {
	fmt.Printf("[SCRAPE] Starting: %s\n", targetURL)
	if !strings.HasPrefix(targetURL, "http") { targetURL = "https://" + targetURL }
	parsedURL, err := url.Parse(targetURL)
	if err != nil { return "", fmt.Errorf("URL invalide") }
	hostname := parsedURL.Hostname()
	ips, err := net.LookupIP(hostname)
	if err == nil {
		for _, ip := range ips {
			if isInternalIP(ip) { return "", fmt.Errorf("accès interdit aux réseaux locaux") }
		}
	}
	client := resty.New().SetTimeout(20 * time.Second).
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	resp, err := client.R().SetDoNotParseResponse(true).Get(targetURL)
	if err != nil { return "", err }
	defer resp.RawBody().Close()
	if resp.StatusCode() != http.StatusOK { return "", fmt.Errorf("status code: %d", resp.StatusCode()) }
	const maxBodySize = 5 * 1024 * 1024
	limitedReader := io.LimitReader(resp.RawBody(), maxBodySize)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil { return "", fmt.Errorf("erreur de lecture") }
	return processHTML(string(bodyBytes), targetURL)
}

func isInternalIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() { return true }
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 10: return true
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31: return true
		case ip4[0] == 192 && ip4[1] == 168: return true
		}
	}
	return false
}

func processHTML(html, targetURL string) (string, error) {
	article, err := readability.FromReader(strings.NewReader(html), nil)
	if err == nil && len(article.TextContent) > 200 {
		text := cleanContentText(article.TextContent)
		if len(text) > 3000 { text = text[:3000] + "..." }
		return text, nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil { return "", err }
	doc.Find("script, style, head, nav, footer, iframe, header, noscript").Remove()
	text := cleanContentText(doc.Find("body").Text())
	if len(text) > 3000 { text = text[:3000] + "..." }
	if len(text) < 10 { return "", fmt.Errorf("could not extract meaningful text from page") }
	return text, nil
}

func cleanContentText(text string) string {
	reSpace := regexp.MustCompile(`\s+`)
	text = reSpace.ReplaceAllString(text, " ")
	reNewlines := regexp.MustCompile(`\n{3,}`)
	return strings.TrimSpace(reNewlines.ReplaceAllString(text, "\n\n"))
}
