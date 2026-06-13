package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"
	"github.com/go-shiori/go-readability"
)

// ToolCall represents a structured tool invocation from the LLM
type ToolCall struct {
	Tool string `json:"tool"`
	Args string `json:"args"`
}

// ToolResult is what a tool handler returns
type ToolResult struct {
	Success  bool
	Response string
}

// ToolCatalog - injected into LLM system prompt
const ToolCatalog = `
OUTILS DISPONIBLES (reponds UNIQUEMENT avec le JSON si tu veux utiliser un outil):

{"tool":"web_read","args":"<url>"}
  → Lire et extraire le contenu d'un site web ou d'un lien URL pour répondre à l'utilisateur

{"tool":"add_note","args":"<contenu de la note>"}
  → Ajouter une note personnelle

{"tool":"list_notes","args":""}
  → Lister les notes de l'utilisateur

{"tool":"del_note","args":"<id>"}
  → Supprimer une note par son ID

{"tool":"reminder","args":"<texte du rappel>"}
  → Creer un rappel

{"tool":"poll","args":"<question>|<option1>|<option2>|..."}
  → Creer un sondage

{"tool":"tagall","args":"<message optionnel>"}
  → Mentionner tout le monde

{"tool":"search","args":"<requete de recherche>"}
  → Chercher dans la memoire du groupe

{"tool":"summary","args":""}
  → Resume des discussions recentes

{"tool":"rules","args":""}
  → Afficher les regles du groupe

{"tool":"profile","args":"<@user optionnel>"}
  → Afficher un profil

{"tool":"stats","args":""}
  → Statistiques du groupe

Si aucun outil n'est pertinent, reponds normalement en texte libre.
IMPORTANT: Ne mets PAS de texte avant ou apres le JSON si tu utilises un outil.`

// ParseToolCall tries to extract a tool call from LLM response.
func ParseToolCall(response string) (ToolCall, bool) {
	response = strings.TrimSpace(response)
	if response == "" {
		return ToolCall{}, false
	}

	var tc ToolCall
	if err := json.Unmarshal([]byte(response), &tc); err == nil && tc.Tool != "" {
		return tc, true
	}

	start := strings.Index(response, `{"tool"`)
	if start == -1 {
		start = strings.Index(response, `{ "tool"`)
	}
	if start >= 0 {
		depth := 0
		end := -1
		for i := start; i < len(response); i++ {
			if response[i] == '{' {
				depth++
			} else if response[i] == '}' {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
		}
		if end > start {
			jsonStr := response[start:end]
			if err := json.Unmarshal([]byte(jsonStr), &tc); err == nil && tc.Tool != "" {
				return tc, true
			}
		}
	}

	return ToolCall{}, false
}

// ExecuteTool runs the actual tool handler and returns the result
func ExecuteTool(tc ToolCall, ctx MessageContext) ToolResult {
	fmt.Printf("[TOOL] Executing tool=%q args=%q from=%s\n", tc.Tool, tc.Args, ctx.PushName)

	switch tc.Tool {

	case "web_read":
		if tc.Args == "" {
			return ToolResult{false, "URL manquante."}
		}
		text, err := scrapeURL(tc.Args)
		if err != nil {
			return ToolResult{false, fmt.Sprintf("Erreur lors de la lecture du site : %v", err)}
		}
		return ToolResult{true, fmt.Sprintf("Contenu extrait du site %s :\n\n%s", tc.Args, text)}

	case "add_note":
		if tc.Args == "" {
			return ToolResult{false, "Il faut preciser le contenu de la note."}
		}
		err := addNote(ctx.SenderJid, ctx.RemoteJid, tc.Args)
		if err != nil {
			return ToolResult{false, "Erreur lors de l'ajout de la note."}
		}
		return ToolResult{true, fmt.Sprintf("Note enregistree: \"%s\"", tc.Args)}

	case "list_notes":
		notes, err := getNotes(ctx.SenderJid, ctx.RemoteJid)
		if err != nil || len(notes) == 0 {
			return ToolResult{true, "Tu n'as aucune note pour l'instant."}
		}
		return ToolResult{true, "*Tes notes:*\n" + strings.Join(notes, "\n")}

	case "del_note":
		if tc.Args == "" {
			return ToolResult{false, "Precise l'ID de la note a supprimer."}
		}
		var id int
		fmt.Sscanf(tc.Args, "%d", &id)
		if id <= 0 {
			return ToolResult{false, "ID invalide."}
		}
		deleteNote(ctx.SenderJid, ctx.RemoteJid, id)
		return ToolResult{true, fmt.Sprintf("Note #%d supprimee.", id)}

	case "reminder":
		if tc.Args == "" {
			return ToolResult{false, "Il faut preciser le contenu du rappel."}
		}
		err := addNote(ctx.SenderJid, ctx.RemoteJid, "[RAPPEL] "+tc.Args)
		if err != nil {
			return ToolResult{false, "Erreur lors de la creation du rappel."}
		}
		return ToolResult{true, fmt.Sprintf("Rappel enregistre: \"%s\"", tc.Args)}

	case "poll":
		parts := strings.Split(tc.Args, "|")
		if len(parts) < 3 {
			return ToolResult{false, "Format: question|option1|option2|..."}
		}
		question := strings.TrimSpace(parts[0])
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("*SONDAGE:* %s\n\n", question))
		emojis := []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣"}
		for i, opt := range parts[1:] {
			if i >= len(emojis) {
				break
			}
			sb.WriteString(fmt.Sprintf("%s %s\n", emojis[i], strings.TrimSpace(opt)))
		}
		sb.WriteString("\nReagissez avec le numero correspondant !")
		return ToolResult{true, sb.String()}

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
		if msg == "" {
			msg = "Appel general"
		}
		text.WriteString(fmt.Sprintf("*%s:*\n\n", msg))
		for _, p := range participants {
			number := strings.Split(p, "@")[0]
			text.WriteString(fmt.Sprintf("@%s ", number))
			mentions = append(mentions, p)
		}
		_, _ = sendWhatsAppMessageWithMentions(ctx.Instance, ctx.RemoteJid, text.String(), mentions)
		return ToolResult{true, ""}

	case "search":
		if tc.Args == "" {
			return ToolResult{false, "Que cherches-tu ?"}
		}
		go handleSearchCommand(ctx, tc.Args)
		return ToolResult{true, ""}

	case "summary":
		go handleSummaryCommand(ctx)
		return ToolResult{true, ""}

	case "rules":
		rules, err := getRules(ctx.RemoteJid)
		if err != nil || len(rules) == 0 {
			return ToolResult{true, "Aucune regle definie pour ce groupe."}
		}
		return ToolResult{true, "*Regles du groupe:*\n" + strings.Join(rules, "\n")}

	case "profile":
		targetJid := ctx.SenderJid
		if tc.Args != "" {
			extracted := extractJid(tc.Args, "")
			if extracted != "" {
				targetJid = extracted
			}
		}
		// Profile fetching should ideally be upgraded to use new profile system
		name := GetMemberName(targetJid, ctx.RemoteJid, strings.Split(targetJid, "@")[0])
		return ToolResult{true, fmt.Sprintf("*Profil de %s*", name)}

	case "stats":
		cartography, _ := getGroupCartography(ctx.RemoteJid)
		if cartography == "" {
			return ToolResult{true, "Pas encore de donnees pour ce groupe."}
		}
		return ToolResult{true, "*Statistiques du groupe:*\n\n" + cartography}

	default:
		return ToolResult{false, ""}
	}
}

// DetectToolFromKeywords checks if the user message clearly implies a tool
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
	if lower == "mes notes" || lower == "liste mes notes" || lower == "montre mes notes" {
		return ToolCall{Tool: "list_notes"}, true
	}

	reminderPatterns := []string{"rappelle-moi", "rappelle moi", "rappel:", "cree un rappel"}
	for _, p := range reminderPatterns {
		if strings.Contains(lower, p) {
			content := extractAfterKeyword(text, p)
			if content != "" {
				return ToolCall{Tool: "reminder", Args: content}, true
			}
		}
	}

	if strings.Contains(lower, "sondage") || strings.Contains(lower, "vote") {
		if strings.Contains(text, "|") {
			content := extractAfterKeyword(text, "sondage")
			if content == "" {
				content = extractAfterKeyword(text, "vote")
			}
			if content != "" {
				return ToolCall{Tool: "poll", Args: content}, true
			}
		}
	}

	if lower == "tague tout le monde" || lower == "tag tout le monde" ||
		lower == "tagall" || lower == "appelle tout le monde" ||
		strings.HasPrefix(lower, "tague tout le monde") {
		msg := extractAfterKeyword(text, "tague tout le monde")
		if msg == "" {
			msg = extractAfterKeyword(text, "tag tout le monde")
		}
		return ToolCall{Tool: "tagall", Args: msg}, true
	}

	if lower == "les regles" || lower == "quelles sont les regles" || lower == "regles du groupe" {
		return ToolCall{Tool: "rules"}, true
	}

	if lower == "statistiques" || lower == "stats du groupe" || lower == "les stats" {
		return ToolCall{Tool: "stats"}, true
	}

	if lower == "resume la discussion" || lower == "resume les messages" ||
		lower == "fais un resume" || strings.HasPrefix(lower, "resume") {
		return ToolCall{Tool: "summary"}, true
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
	after = strings.TrimSpace(after)
	return after
}

// ============================================================================
// WEB SCRAPING ENGINE
// ============================================================================

// scrapeURL fetches a webpage and returns the cleaned, readable text content.
func scrapeURL(targetURL string) (string, error) {
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	// 1. SSRF PROTECTION: Block access to local/private networks
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

	client := resty.New().
		SetTimeout(15 * time.Second).
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	// 2. CAMOUFOX INTEGRATION (Optional)
	camoufoxURL := os.Getenv("CAMOUFOX_API_URL")
	if camoufoxURL != "" {
		var result struct {
			Content string `json:"content"`
		}
		resp, err := client.R().
			SetBody(map[string]string{"url": targetURL}).
			SetResult(&result).
			Post(camoufoxURL)

		if err == nil && resp.StatusCode() == 200 && result.Content != "" {
			return processHTML(result.Content, targetURL)
		}
	}

	// 3. NATIVE SCRAPER FALLBACK with Size Limit
	resp, err := client.R().
		SetDoNotParseResponse(true).
		Get(targetURL)

	if err != nil {
		return "", err
	}
	defer resp.RawBody().Close()

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("status code: %d", resp.StatusCode())
	}

	// Limit to 5MB raw HTML
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


// processHTML cleans HTML and extracts the main readable text using Readability and GoQuery.
func processHTML(html, targetURL string) (string, error) {
	// 1. First pass with go-readability to extract main content
	article, err := readability.FromReader(strings.NewReader(html), nil)
	if err == nil && len(article.TextContent) > 200 {
		text := cleanContentText(article.TextContent)
		if len(text) > 4000 {
			text = text[:4000] + "..."
		}
		return text, nil
	}

	// 2. Fallback to GoQuery for manual extraction if Readability fails
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	// Remove noise
	doc.Find("script, style, head, nav, footer, iframe, header, noscript").Remove()

	// Extract text from body
	text := doc.Find("body").Text()
	text = cleanContentText(text)

	if len(text) > 4000 {
		text = text[:4000] + "..."
	}

	if len(text) < 10 {
		return "", fmt.Errorf("could not extract meaningful text from page")
	}

	return text, nil
}

// cleanContentText normalizes whitespace and removes excessive line breaks
func cleanContentText(text string) string {
	// Replace multiple spaces with one
	reSpace := regexp.MustCompile(`[ \t]+`)
	text = reSpace.ReplaceAllString(text, " ")

	// Replace multiple newlines with maximum two
	reNewlines := regexp.MustCompile(`\n{3,}`)
	text = reNewlines.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}
