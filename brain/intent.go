package main

import (
	"strings"
)

// ============================================================================
// INTENT TYPES
// ============================================================================

type Intent string

const (
	IntentCommand  Intent = "command"  // .xxx or !xxx
	IntentChat     Intent = "chat"     // General conversation with LLM
	IntentQuestion Intent = "question" // Question directed at bot
	IntentStory    Intent = "story"    // Storytelling request
	IntentCode     Intent = "code"     // Code/programming help
	IntentGame     Intent = "game"     // Game request
	IntentSearch   Intent = "search"   // Memory/search request
	IntentSummary  Intent = "summary"  // Summary request
	IntentGreeting Intent = "greeting" // Simple greeting
	IntentIgnore   Intent = "ignore"   // Bot should not respond
)

// ============================================================================
// COMMAND REGISTRY - exhaustive list of all valid commands
// Each command maps to its canonical name for the handler switch
// ============================================================================

var commandAliases = map[string]string{
	// Help
	"help": "help", "aide": "help", "menu": "help",
	// Identity
	"qui-es-tu": "qui-es-tu", "qui": "qui-es-tu",
	// Ping
	"ping": "ping",
	// Tag all
	"tagall": "tagall", "tous": "tagall",
	// Sticker
	"sticker": "sticker", "s": "sticker",
	// Stats
	"stats": "stats", "statistiques": "stats",
	// Memory / Facts
	"memoire": "memoire", "m\u00e9moire": "memoire",
	"fact": "fact", "fait": "fact",
	// Summary
	"resume": "resume", "r\u00e9sum\u00e9": "resume",
	// Persona
	"persona": "persona", "personnalit\u00e9": "persona", "personnalite": "persona",
	// Moderation
	"warn":      "warn",
	"warn-list": "warn-list", "avertissements": "warn-list",
	"warn-reset": "warn-reset",
	"kick":       "kick",
	"mute":       "mute", "unmute": "unmute",
	// Group management
	"bienvenue": "bienvenue",
	"anti-lien": "anti-lien",
	"ouvrir":    "ouvrir", "fermer": "fermer",
	// Download
	"yt": "yt", "youtube": "yt",
	"fb": "fb", "facebook": "fb",
	"tt": "tt", "tiktok": "tt",
	"audio": "audio",
	"video": "video", "vid\u00e9o": "video",
	// Search
	"recherche": "recherche", "search": "recherche",
	// Code
	"code": "code",
	// System
	"statut-serveur": "statut-serveur", "serveur": "statut-serveur",
	// Privacy
	"confidentialit\u00e9": "confidentialite", "confidentialite": "confidentialite", "privacy": "confidentialite",
	// Clear context
	"clear": "clear", "reset": "clear",
	// Jeu
	"jeu": "jeu", "jouer": "jeu", "morpion": "jeu", "devinette": "jeu",
	// Note/Rappel
	"note": "note", "rappel": "rappel",
	// Sondage
	"sondage": "sondage", "poll": "sondage",
	// Profil
	"profil": "profil",
	// Langue
	"langue": "langue", "lang": "langue",
	// Regles
	"regles": "regles", "r\u00e8gles": "regles", "rules": "regles",
	// Lien groupe
	"lien": "lien", "link": "lien",
	// Promote/Demote
	"promote": "promote", "demote": "demote",
	// Annonce
	"annonce": "annonce", "broadcast": "annonce",
}

// ============================================================================
// COMMAND PARSING - strict prefix-based detection
// ============================================================================

// ParseCommand checks if text is a command (starts with . or !)
// Returns: canonical command name, arguments, isCommand
func ParseCommand(text string) (string, string, bool) {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return "", "", false
	}

	// STRICT RULE: Commands MUST start with . or !
	if clean[0] != '.' && clean[0] != '!' {
		return "", "", false
	}

	// Remove prefix
	withoutPrefix := clean[1:]
	if withoutPrefix == "" {
		return "", "", false
	}

	// Split into command and args
	parts := strings.SplitN(withoutPrefix, " ", 2)
	rawCmd := strings.ToLower(strings.TrimSpace(parts[0]))
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	// Look up canonical command name
	canonical, exists := commandAliases[rawCmd]
	if !exists {
		return rawCmd, args, true // Unknown command - still a command, handler will say "unknown"
	}

	return canonical, args, true
}

// ============================================================================
// INTENT DETECTION - for LLM routing (non-command messages)
// ============================================================================

// DetectIntent analyzes a non-command message and determines the best intent
// This is ONLY called when the bot should respond (mentioned, reply, private)
func DetectIntent(text string, isMentioned bool, isReplyToBot bool) Intent {
	lower := strings.ToLower(strings.TrimSpace(text))

	// Empty message after cleaning
	if lower == "" {
		return IntentGreeting
	}

	// Single emoji or very short reaction
	if len([]rune(lower)) <= 2 {
		return IntentGreeting
	}

	// Story/narrative request
	storyKeywords := []string{"raconte", "histoire", "conte", "fable", "invente", "imagine"}
	for _, kw := range storyKeywords {
		if strings.Contains(lower, kw) {
			return IntentStory
		}
	}

	// Game request
	gameKeywords := []string{"jouons", "joue avec moi", "morpion", "tic tac toe",
		"devinette", "enigme", "quiz", "devine"}
	for _, kw := range gameKeywords {
		if strings.Contains(lower, kw) {
			return IntentGame
		}
	}

	// Summary request
	summaryKeywords := []string{"resume", "r\u00e9sum\u00e9", "synth\u00e8se", "synthese", "recap", "r\u00e9capitulatif"}
	for _, kw := range summaryKeywords {
		if strings.Contains(lower, kw) {
			return IntentSummary
		}
	}

	// Search/memory request
	searchKeywords := []string{"qui a dit", "rappelle-moi", "qu'est-ce qu'on a dit",
		"trouve-moi", "cherche", "tu te souviens", "souviens-toi"}
	for _, kw := range searchKeywords {
		if strings.Contains(lower, kw) {
			return IntentSearch
		}
	}

	// Code/programming request
	codeKeywords := []string{"code", "programme", "script", "fonction", "variable",
		"python", "javascript", "golang", "html", "css", "sql", "api",
		"d\u00e9bogue", "debug", "erreur de code", "compile"}
	for _, kw := range codeKeywords {
		if strings.Contains(lower, kw) {
			return IntentCode
		}
	}

	// Question detection (contains ?)
	if strings.Contains(lower, "?") {
		return IntentQuestion
	}

	// Question patterns in French
	questionPrefixes := []string{"qui ", "que ", "quoi ", "quel ", "quelle ",
		"quand ", "o\u00f9 ", "ou ", "comment ", "pourquoi ", "combien ",
		"est-ce que", "c'est quoi", "qu'est-ce"}
	for _, prefix := range questionPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return IntentQuestion
		}
	}

	// Simple greeting (exact match only)
	greetings := map[string]bool{
		"bonjour": true, "salut": true, "coucou": true,
		"hi": true, "hello": true, "hey": true,
		"bonsoir": true, "yo": true, "wesh": true,
		"a+": true, "au revoir": true, "bye": true,
		"merci": true, "thanks": true, "ok": true,
		"oui": true, "non": true,
		"\u00e7a va": true, "ca va": true, "\u00e7a va?": true, "ca va?": true,
	}
	if greetings[lower] {
		return IntentGreeting
	}

	// Default: general chat
	return IntentChat
}

// ============================================================================
// FAST REPLIES - instant responses without LLM
// ============================================================================

var fastReplies = map[string]string{
	"ping": "Pong ! :ping_pong:",
}

// GetFastReply checks if a non-command message deserves an instant reply
// This is VERY selective - only for the most trivial interactions
func GetFastReply(text string) (string, bool) {
	clean := strings.ToLower(strings.TrimSpace(text))
	if reply, exists := fastReplies[clean]; exists {
		return reply, true
	}
	return "", false
}
