package main

import (
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


// IsCommand détecte si le message est une commande (!help, !stats, etc.)
func IsCommand(text string) (string, string, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.ReplaceAll(text, "@poulga", "")
	text = strings.TrimSpace(text)

	if !strings.HasPrefix(text, "!") {
		return "", "", false
	}

	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", "", false
	}

	cmd := strings.TrimPrefix(parts[0], "!")
	args := ""
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}

	return cmd, args, true
}
