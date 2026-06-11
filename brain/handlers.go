package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// handleGame gère les jeux (morpion, etc.)
func handleGame(instance, remoteJid, userText, msgId, senderJid string, start time.Time) {
	// Détecter quel jeu
	gameType := "morpion"
	if strings.Contains(strings.ToLower(userText), "echecs") || strings.Contains(strings.ToLower(userText), "chess") {
		gameType = "echecs"
	}

	// Pour le morpion, créer une grille simple
	gameState := "---------" // 9 cases vides
	if strings.Contains(strings.ToLower(userText), "commence") {
		gameState = "O--------" // Poulga commence
	}

	pgStart := time.Now()
	history, _ := getRecentMessages(remoteJid, 3)
	pgTime := time.Since(pgStart)
	fmt.Printf("[TIMING] DB_GAME: %.1fms\n", pgTime.Seconds()*1000)

	historyStr := strings.Join(history, "\n")

	prompt := fmt.Sprintf(GamePrompt, gameType, gameState, historyStr)
	
	ollamaStart := time.Now()
	response, _ := callOllama(prompt, nil, 0.3)
	ollamaTime := time.Since(ollamaStart)
	fmt.Printf("[TIMING] OLLAMA_GAME: %.1fms\n", ollamaTime.Seconds()*1000)
	fmt.Printf("[TIMING] TOTAL: %.1fms\n", time.Since(start).Seconds()*1000)
	
	response = cleanResponse(response)
	_ = sendWhatsAppMessage(instance, remoteJid, response, msgId, senderJid)
}

// handleSearch cherche dans la mémoire
func handleSearch(instance, remoteJid, userText, msgId, senderJid string, start time.Time) {
	pgStart := time.Now()
	facts, _ := getFacts(remoteJid)
	pgTime := time.Since(pgStart)
	fmt.Printf("[TIMING] DB_SEARCH: %.1fms\n", pgTime.Seconds()*1000)

	factsStr := strings.Join(facts, "\n")
	if factsStr == "" {
		factsStr = "(Aucun souvenir trouvé)"
	}

	prompt := fmt.Sprintf(SearchPrompt, factsStr, userText)
	
	ollamaStart := time.Now()
	response, _ := callOllama(prompt, nil, 0.4)
	ollamaTime := time.Since(ollamaStart)
	fmt.Printf("[TIMING] OLLAMA_SEARCH: %.1fms\n", ollamaTime.Seconds()*1000)
	fmt.Printf("[TIMING] TOTAL: %.1fms\n", time.Since(start).Seconds()*1000)
	
	response = cleanResponse(response)
	_ = sendWhatsAppMessage(instance, remoteJid, response, msgId, senderJid)
}

// handleSummary génère un résumé du groupe
func handleSummary(instance, remoteJid, msgId, senderJid string, start time.Time) {
	pgStart := time.Now()
	profiles, _ := getMemberProfiles(remoteJid)
	history, _ := getRecentMessages(remoteJid, 100)
	pgTime := time.Since(pgStart)
	fmt.Printf("[TIMING] DB_SUMMARY: %.1fms\n", pgTime.Seconds()*1000)

	historyStr := strings.Join(history, "\n")

	prompt := fmt.Sprintf(SummaryPrompt, profiles, historyStr)
	
	ollamaStart := time.Now()
	response, _ := callOllama(prompt, nil, 0.3)
	ollamaTime := time.Since(ollamaStart)
	fmt.Printf("[TIMING] OLLAMA_SUMMARY: %.1fms\n", ollamaTime.Seconds()*1000)
	fmt.Printf("[TIMING] TOTAL: %.1fms\n", time.Since(start).Seconds()*1000)
	
	response = cleanResponse(response)
	_ = sendWhatsAppMessage(instance, remoteJid, response, msgId, senderJid)
}


// cleanResponse nettoie les présentations indésirables sans tuer la réponse
func cleanResponse(text string) string {
	text = strings.TrimSpace(text)
	prefixes := []string{
		"Bonjour à tous !",
		"Bonjour à tous",
		"Je suis Poulga",
		"Poulga :",
		"Poulga:",
		"En tant que Poulga,",
	}

	// Retirer proprement les préfixes inutiles
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.ToLower(text), strings.ToLower(prefix)) {
			// Coupe juste le préfixe, garde le reste du message
			text = strings.TrimSpace(text[len(prefix):])
		}
	}

	// Si jamais la réponse est vide après nettoyage
	if text == "" {
		return "Je suis là ! 😊"
	}
	return text
}


// handleCommand traite les commandes Poulga (.help, .stats, .tagall, etc.)
func handleCommand(instance, remoteJid, cmd, args, msgId, senderJid, quotedMsgId string) {
	var response string

	switch cmd {
	case "help", "menu":
		response = `Je suis Poulga, votre partenaire et associée du groupe ! 🚀

📚 Je peux :
  • Répondre à vos questions
  • Résumer une discussion
  • Retrouver une information passée
  • Télécharger (YouTube, FB, TikTok)
  • Créer des stickers

⚙️ Commandes :
  .help - Affiche cette aide
  .ping - Vérifie si je suis en ligne
  .yt [url] - Télécharge une vidéo YouTube
  .audio [url] - Télécharge l'audio
  .fb [url] - Vidéo Facebook
  .tt [url] - Vidéo TikTok
  .tagall - Mentionne tout le monde
  .sticker - Crée un sticker (cite une image)`

	case "ping":
		response = "Pong ! 🏓 Bot Poulga actif et prêt à vous aider."

	case "yt", "fb", "tt", "video", "audio":
		if args == "" {
			response = fmt.Sprintf("Usage: .%s [URL]", cmd)
		} else {
			go handleDownload(instance, remoteJid, cmd, args, msgId, senderJid)
			return
		}

	case "tagall":
		if !strings.HasSuffix(remoteJid, "@g.us") {
			response = "Cette commande ne fonctionne que dans les groupes. 👥"
		} else {
			participants, err := getGroupMetadata(instance, remoteJid)
			if err != nil {
				response = "Erreur lors de la récupération des membres. ❌"
			} else {
				var mentions []string
				var text strings.Builder
				text.WriteString("📣 Appel à tous les membres :\n\n")
				for _, p := range participants {
					number := strings.Split(p, "@")[0]
					text.WriteString(fmt.Sprintf("@%s ", number))
					mentions = append(mentions, p)
				}
				
				// Envoi spécial avec toutes les mentions
				evoURL := os.Getenv("EVOLUTION_URL")
				if evoURL == "" {
					evoURL = "http://evolution-api:8080"
				}
				apiKey := os.Getenv("AUTHENTICATION_API_KEY")

				body := EvolutionSendMessageRequest{
					Number:    strings.Split(remoteJid, "@")[0],
					Text:      text.String(),
					Mentioned: mentions,
				}
				
				client := resty.New()
				_, _ = client.R().
					SetHeader("apikey", apiKey).
					SetBody(body).
					Post(fmt.Sprintf("%s/message/sendText/%s", evoURL, instance))
				return
			}
		}

	case "sticker":
		// On cherche d'abord dans le message cité
		targetMsgId := quotedMsgId
		if targetMsgId == "" {
			// Si pas de citation, on regarde si le message actuel est une image
			targetMsgId = msgId
		}

		_ = sendWhatsAppMessage(instance, remoteJid, "⏳ Conversion en sticker... Un instant ! ✨", msgId, senderJid)
		
		// Récupérer le base64 (soit du message cité, soit du message actuel)
		b64, err := getMediaBase64(instance, targetMsgId)
		if err != nil {
			fmt.Printf("[STICKER] Error fetching media for %s: %v\n", targetMsgId, err)
			response = "❌ Impossible de transformer ça en sticker. Assure-toi de citer une image ou d'en envoyer une avec la légende .sticker"
		} else {
			err = sendSticker(instance, remoteJid, b64)
			if err != nil {
				fmt.Printf("[STICKER] Error sending sticker: %v\n", err)
				response = "❌ Erreur lors de la création du sticker."
			} else {
				return // Succès
			}
		}


	case "stats":
		cartography, _ := getGroupCartography(remoteJid)
		if cartography == "" {
			response = "Aucune donnée de groupe pour le moment. 📊"
		} else {
			response = "📊 Cartographie du groupe :\n\n" + cartography
		}

	case "persona":
		if args == "" {
			response = "Usage: @poulga !persona [description]\nExemple: @poulga !persona Tu es une experte en cryptomonnaie"
		} else {
			err := SetGroupPersona(remoteJid, args)
			if err != nil {
				response = "Erreur lors de la mise à jour. ❌"
			} else {
				response = "Personnalité mise à jour avec succès ! ✅"
			}
		}

	case "confidentialité":
		response = `🔒 Politique de Confidentialité

Je suis une partenaire transparente. Je lis les messages pour :
  • Construire les statistiques (qui parle le plus)
  • Extraire les faits importants liés aux projets
  • Comprendre le contexte des discussions

Je ne fais PAS :
  • Conserver le texte brut des messages indéfiniment
  • Partager vos données hors du groupe
  • Analyser les messages privés sans être sollicitée

Pour refuser complètement : !optout`

	default:
		response = "Commande inconnue. Tapez @poulga !help pour la liste."
	}

	_ = sendWhatsAppMessage(instance, remoteJid, response, msgId, senderJid)
}

// handleDownload gère les téléchargements via yt-dlp
func handleDownload(instance, remoteJid, cmd, url, msgId, senderJid string) {
	fmt.Printf("[DOWNLOAD] Starting %s download for %s\n", cmd, url)
	
	// Message d'attente
	_ = sendWhatsAppMessage(instance, remoteJid, "⏳ Téléchargement en cours... Je m'en occupe ! 🚀", msgId, senderJid)

	outputFile := fmt.Sprintf("/tmp/poulga_%d", time.Now().UnixNano())
	args := []string{}
	mediaType := "video"

	if cmd == "audio" {
		mediaType = "audio"
		args = []string{
			"--extract-audio",
			"--audio-format", "mp3",
			"--max-filesize", "50M",
			"--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"--geo-bypass",
			"-o", outputFile + ".%(ext)s",
			url,
		}
	} else {
		args = []string{
			"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
			"--max-filesize", "50M",
			"--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"--geo-bypass",
			"-o", outputFile + ".%(ext)s",
			url,
		}
	}

	cmdExec := exec.Command("yt-dlp", args...)
	output, err := cmdExec.CombinedOutput()
	if err != nil {
		fmt.Printf("[DOWNLOAD] Error: %v, Output: %s\n", err, string(output))
		errorMsg := "❌ Désolée, je n'ai pas pu télécharger cette vidéo. Vérifie le lien ou réessaie plus tard."
		if strings.Contains(string(output), "Sign in to confirm you’re not a bot") {
			errorMsg = "❌ Le téléchargement a été bloqué par la sécurité de la plateforme (Protection Anti-Bot YouTube). YouTube bloque les serveurs Cloud."
		}
		_ = sendWhatsAppMessage(instance, remoteJid, errorMsg, msgId, senderJid)
		return
	}

	// Trouver le fichier réel (car yt-dlp ajoute l'extension)
	matches, _ := filepath.Glob(outputFile + ".*")
	if len(matches) == 0 {
		_ = sendWhatsAppMessage(instance, remoteJid, "❌ Erreur interne : fichier introuvable.", msgId, senderJid)
		return
	}
	realPath := matches[0]
	defer os.Remove(realPath)

	// Lire le fichier et encoder en base64
	data, err := os.ReadFile(realPath)
	if err != nil {
		_ = sendWhatsAppMessage(instance, remoteJid, "❌ Erreur lors de la lecture du fichier.", msgId, senderJid)
		return
	}

	// Vérifier la taille (50MB max pour Evolution API / WhatsApp)
	if len(data) > 50*1024*1024 {
		_ = sendWhatsAppMessage(instance, remoteJid, "❌ Le fichier est trop volumineux (max 50 Mo).", msgId, senderJid)
		return
	}

	base64Data := base64.StdEncoding.EncodeToString(data)
	fileName := filepath.Base(realPath)
	
	caption := "Voici ton contenu ! 😊"
	if cmd == "audio" {
		caption = ""
	}

	err = sendWhatsAppMedia(instance, remoteJid, base64Data, fileName, caption, mediaType)
	if err != nil {
		fmt.Printf("[DOWNLOAD] Send Error: %v\n", err)
		_ = sendWhatsAppMessage(instance, remoteJid, "❌ Erreur lors de l'envoi du fichier sur WhatsApp.", msgId, senderJid)
	}
}
