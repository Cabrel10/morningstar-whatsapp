package main

import (
	"fmt"
	"strings"
)

type Intent string

const (
	IntentGreeting Intent = "greeting"
	IntentGame     Intent = "game"
	IntentQuestion Intent = "question"
	IntentSearch   Intent = "search"
	IntentSummary  Intent = "summary"
	IntentChat     Intent = "chat"
)

// Fast replies codées - pas de LLM
var fastReplies = map[string]string{
	"bonjour": "Bonjour 👋",
	"salut":   "Salut 😊",
	"merci":   "Avec plaisir 😊",
	"ok":      "D'accord 👍",
	"oui":     "Oui, c'est bon 👍",
	"non":     "Non, ça me convient pas",
	"a+":      "À bientôt 👋",
	"coucou":  "Coucou! 😊",
	"ca va?":  "Ça va bien, et toi? 😊",
	"ça va?":  "Ça va bien, et toi? 😊",
	"thanks":  "You're welcome 😊",
	"👍":      "👍",
	"😂":      "😂",
	"❤️":      "❤️",
}

// DetectIntent analyse rapidement le message
func DetectIntent(text string) Intent {
	lower := strings.ToLower(strings.TrimSpace(text))

	// Retirer la mention @poulga
	lower = strings.ReplaceAll(lower, "@poulga", "")
	lower = strings.TrimSpace(lower)

	// Jeux
	if strings.Contains(lower, "morpion") || strings.Contains(lower, "tic tac toe") ||
		strings.Contains(lower, "echecs") || strings.Contains(lower, "devinette") ||
		strings.Contains(lower, "jouons") || strings.Contains(lower, "joue avec moi") {
		return IntentGame
	}

	// Résumé
	if strings.Contains(lower, "resume") || strings.Contains(lower, "résumé") ||
		strings.Contains(lower, "synthese") || strings.Contains(lower, "synthèse") ||
		strings.Contains(lower, "recap") || strings.Contains(lower, "recapitulatif") {
		return IntentSummary
	}

	// Recherche
	if strings.Contains(lower, "qui a dit") || strings.Contains(lower, "rappelle-moi") ||
		strings.Contains(lower, "qu'est-ce qu'on a dit") || strings.Contains(lower, "trouve-moi") ||
		strings.Contains(lower, "cherche") {
		return IntentSearch
	}

	// Salutations simples
	if lower == "bonjour" || lower == "salut" || lower == "coucou" ||
		lower == "hi" || lower == "hello" || lower == "a+" || lower == "au revoir" {
		return IntentGreeting
	}

	// Remerciements simples
	if lower == "merci" || lower == "thanks" || lower == "thx" || lower == "ok" ||
		lower == "oui" || lower == "non" || lower == "ça va?" || lower == "ca va?" {
		return IntentGreeting
	}

	// Emoji seuls
	if len(lower) <= 3 && (strings.Contains(lower, "👍") || strings.Contains(lower, "😂") ||
		strings.Contains(lower, "❤️") || strings.Contains(lower, "😊") || strings.Contains(lower, "🙏")) {
		return IntentGreeting
	}

	// Défaut : chat normal
	return IntentChat
}

// IsFastReply vérifie si c'est une réponse codée rapide
func IsFastReply(text string) (string, bool) {
	cleanText := strings.ToLower(strings.TrimSpace(text))
	// Retirer la mention pour ne garder que l'intention
	cleanText = strings.ReplaceAll(cleanText, "@poulga", "")
	cleanText = strings.TrimSpace(cleanText)

	fastReplies := map[string]string{
		"bonjour":  "Coucou ! 😊",
		"salut":    "Salut ! 👋",
		"merci":    "Avec grand plaisir ! ✨",
		"ok":       "Ça marche ! 👍",
		"ping":     "Pong ! 🏓",
	}

	if reply, exists := fastReplies[cleanText]; exists {
		return reply, true
	}
	return "", false
}

// IsShortMessage vérifie si le message est trop court pour faire des embeddings
func IsShortMessage(text string) bool {
	return len(strings.TrimSpace(text)) < 20
}


// IsCommand détecte les commandes avec ou sans ! ou .
func IsCommand(text string) (string, string, bool) {
	fmt.Printf("[DEBUG] IS_COMMAND_INPUT=%s\n", text)
	clean := strings.TrimSpace(text)
	if clean == "" {
		return "", "", false
	}

	lower := strings.ToLower(clean)

	// Commandes avec préfixes . ou ! (Priorité)
	if strings.HasPrefix(lower, "!") || strings.HasPrefix(lower, ".") {
		parts := strings.Fields(clean)
		if len(parts) == 0 {
			return "", "", false
		}
		// On retire le préfixe de la première partie
		cmd := strings.ToLower(parts[0][1:])
		args := ""
		if len(parts) > 1 {
			args = strings.TrimSpace(clean[len(parts[0]):])
		}
		fmt.Printf("[DEBUG] IS_COMMAND_RESULT=true PREFIXED CMD=%s ARGS=%s\n", cmd, args)
		return cmd, args, true
	}

	// Commandes sans préfixe (help, stats, persona, etc.)
	commands := []string{
		"aide", "help", "menu", "qui-es-tu", "qui", "mémoire", "résumé", "resume", "stats", "statistiques",
		"persona", "personnalité", "confidentialité", "privacy",
		"ping", "pong", "tagall", "mentionner", "sticker",
		"ouvrir", "open", "fermer", "close",
		"avertir", "warn", "avertissements", "warnings", "warn-list", "warn-reset", "reset",
		"bienvenue", "anti-lien", "anti-spam", "anti-suppression",
		"yt", "fb", "tt", "video", "vidéo", "audio", "télécharger", "download", "miniature", "thumbnail", "infos", "info",
		"recherche", "search", "code", "explique", "explain", "débogue", "debug",
		"statut-serveur", "server-status", "logs", "docker", "fait", "fact",
	}
	for _, cmd := range commands {
		if lower == cmd { // Match exact pour commande sans préfixe
			fmt.Printf("[DEBUG] IS_COMMAND_RESULT=true EXACT CMD=%s\n", cmd)
			return cmd, "", true
		}
		if strings.HasPrefix(lower, cmd+" ") { // Match avec arguments
			args := strings.TrimSpace(clean[len(cmd):])
			fmt.Printf("[DEBUG] IS_COMMAND_RESULT=true BARE CMD=%s ARGS=%s\n", cmd, args)
			return cmd, args, true
		}
	}

	fmt.Printf("[DEBUG] IS_COMMAND_RESULT=false\n")
	return "", "", false
}
