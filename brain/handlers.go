package main

import (
	"fmt"
	"strings"
	"time"
)

// handleGame gère les jeux (morpion, etc.)
func handleGame(instance, remoteJid, userText string, start time.Time) {
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
	response, _ := callOllama(prompt, nil)
	ollamaTime := time.Since(ollamaStart)
	fmt.Printf("[TIMING] OLLAMA_GAME: %.1fms\n", ollamaTime.Seconds()*1000)
	fmt.Printf("[TIMING] TOTAL: %.1fms\n", time.Since(start).Seconds()*1000)
	
	response = cleanResponse(response)
	_ = sendWhatsAppMessage(instance, remoteJid, response)
}

// handleSearch cherche dans la mémoire
func handleSearch(instance, remoteJid, userText string, start time.Time) {
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
	response, _ := callOllama(prompt, nil)
	ollamaTime := time.Since(ollamaStart)
	fmt.Printf("[TIMING] OLLAMA_SEARCH: %.1fms\n", ollamaTime.Seconds()*1000)
	fmt.Printf("[TIMING] TOTAL: %.1fms\n", time.Since(start).Seconds()*1000)
	
	response = cleanResponse(response)
	_ = sendWhatsAppMessage(instance, remoteJid, response)
}

// handleSummary génère un résumé du groupe
func handleSummary(instance, remoteJid string, start time.Time) {
	pgStart := time.Now()
	profiles, _ := getMemberProfiles(remoteJid)
	history, _ := getRecentMessages(remoteJid, 100)
	pgTime := time.Since(pgStart)
	fmt.Printf("[TIMING] DB_SUMMARY: %.1fms\n", pgTime.Seconds()*1000)

	historyStr := strings.Join(history, "\n")

	prompt := fmt.Sprintf(SummaryPrompt, profiles, historyStr)
	
	ollamaStart := time.Now()
	response, _ := callOllama(prompt, nil)
	ollamaTime := time.Since(ollamaStart)
	fmt.Printf("[TIMING] OLLAMA_SUMMARY: %.1fms\n", ollamaTime.Seconds()*1000)
	fmt.Printf("[TIMING] TOTAL: %.1fms\n", time.Since(start).Seconds()*1000)
	
	response = cleanResponse(response)
	_ = sendWhatsAppMessage(instance, remoteJid, response)
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


// handleCommand traite les commandes Poulga (!help, !stats, !persona, etc.)
func handleCommand(instance, remoteJid, cmd, args string) {
	var response string

	switch cmd {
	case "help":
		response = `Je suis Poulga ! Voici mes capacités :

📚 Je peux :
  • Répondre à vos questions
  • Résumer une discussion
  • Retrouver une information passée
  • Aider sur des sujets techniques
  • Mémoriser les décisions du groupe

💡 Exemples :
  @poulga résume la discussion
  @poulga qui a parlé de Docker ?
  @poulga explique le RSI

⚙️ Commandes :
  !help - Affiche cette aide
  !stats - Statistiques du groupe
  !confidentialité - Politique de confidentialité
  !persona [texte] - Personnalité custom (admin)`

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

Je lis les messages pour :
  • Construire les statistiques (qui parle le plus)
  • Extraire les faits importants liés aux projets
  • Comprendre le contexte des discussions

Je ne fais PAS :
  • Conserver le texte brut des messages
  • Partager vos données
  • Lire les messages privés sans mentionner

Pour refuser complètement : !optout`

	default:
		response = "Commande inconnue. Tapez @poulga !help pour la liste."
	}

	_ = sendWhatsAppMessage(instance, remoteJid, response)
}
