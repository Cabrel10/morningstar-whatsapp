package main

import (
	"fmt"
	"strings"
)

// ============================================================================
// SYSTEM PROMPT - The core identity. Minimal. No fluff.
// This is injected ONCE at the top of every LLM call.
// ============================================================================

const SystemPrompt = `Tu es Poulga, assistante WhatsApp du groupe.
Regles: reponds direct, jamais de "Je suis Poulga". Sois breve, naturelle, drole si le contexte le permet. Francais courant. Si on cite un message, reponds-y directement.`

// ============================================================================
// PROMPT BUILDERS - structured prompts per intent
// ============================================================================

// BuildChatPrompt creates the complete prompt for general chat
func BuildChatPrompt(ctx MessageContext, history []ConversationMessage, userMem []UserMemory, groupMem []GroupMemoryEntry, facts []string, summary string, customPersona string) string {
	var sb strings.Builder

	// 1. System identity
	if customPersona != "" {
		sb.WriteString(customPersona)
	} else {
		sb.WriteString(SystemPrompt)
	}
	sb.WriteString("\n\n")

	// 2. Group knowledge (Level 3)
	groupMemStr := FormatGroupMemory(groupMem)
	if groupMemStr != "" {
		sb.WriteString("CONNAISSANCES DU GROUPE:\n")
		sb.WriteString(groupMemStr)
		sb.WriteString("\n")
	}

	// 3. User knowledge (Level 1)
	userMemStr := FormatUserMemory(userMem)
	if userMemStr != "" {
		sb.WriteString(fmt.Sprintf("CE QUE TU SAIS SUR %s:\n", ctx.PushName))
		sb.WriteString(userMemStr)
		sb.WriteString("\n")
	}

	// 4. Conversation summary (Level 4)
	if summary != "" {
		sb.WriteString("RESUME DES DISCUSSIONS RECENTES:\n")
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}

	// 5. Facts
	if len(facts) > 0 {
		sb.WriteString("FAITS IMPORTANTS:\n")
		for _, f := range facts {
			sb.WriteString("- " + f + "\n")
		}
		sb.WriteString("\n")
	}

	// 6. Recent conversation (Level 2) - last 8 messages
	historyStr := FormatConversationHistory(history)
	if historyStr != "(pas de messages recents)" {
		sb.WriteString("MESSAGES RECENTS:\n")
		sb.WriteString(historyStr)
		sb.WriteString("\n")
	}

	// 7. Quoted message (reply context) — CRITICAL for conversation continuity
	if ctx.QuotedText != "" {
		quotedAuthor := "quelqu'un"
		if ctx.QuotedSender != "" {
			if strings.Contains(ctx.QuotedSender, "237620864894") || ctx.QuotedSender == "Poulga" {
				quotedAuthor = "Poulga (Toi)"
			} else {
				quotedAuthor = strings.Split(ctx.QuotedSender, "@")[0]
			}
		}
		if ctx.IsReplyToBot {
			// When user replies to OUR message, emphasize the thread
			sb.WriteString(fmt.Sprintf("\n[CONTEXTE IMPORTANT - L'utilisateur repond a TON message precedent]\nTU AVAIS DIT: \"%s\"\n\n", ctx.QuotedText))
		} else {
			sb.WriteString(fmt.Sprintf("MESSAGE CITE (de %s):\n\"%s\"\n\n", quotedAuthor, ctx.QuotedText))
		}
	}

	// 8. Current message with clear instruction
	if ctx.IsReplyToBot {
		sb.WriteString(fmt.Sprintf("%s te repond: %s\n\nContinue naturellement la conversation en tenant compte de ce que tu avais dit precedemment:", ctx.PushName, ctx.Text))
	} else if ctx.IsMentioned {
		sb.WriteString(fmt.Sprintf("%s te parle: %s\n\nReponds en tenant compte du contexte de la conversation:", ctx.PushName, ctx.Text))
	} else {
		sb.WriteString(fmt.Sprintf("%s dit: %s\n\nReponds:", ctx.PushName, ctx.Text))
	}

	return sb.String()
}

// BuildQuestionPrompt creates a focused prompt for questions
func BuildQuestionPrompt(ctx MessageContext, history []ConversationMessage, facts []string) string {
	var sb strings.Builder

	sb.WriteString(SystemPrompt)
	sb.WriteString("\n\nTu reponds a une QUESTION. Sois precis et factuel.\n\n")

	if len(facts) > 0 {
		sb.WriteString("Faits connus:\n")
		for _, f := range facts {
			sb.WriteString("- " + f + "\n")
		}
		sb.WriteString("\n")
	}

	historyStr := FormatConversationHistory(history)
	if historyStr != "(pas de messages recents)" {
		sb.WriteString("Contexte recent:\n")
		sb.WriteString(historyStr)
		sb.WriteString("\n")
	}

	if ctx.QuotedText != "" {
		if ctx.IsReplyToBot {
			sb.WriteString(fmt.Sprintf("[Tu avais repondu]: \"%s\"\n\n", ctx.QuotedText))
			sb.WriteString(fmt.Sprintf("%s te demande: %s\n\nContinue ta reponse:", ctx.PushName, ctx.Text))
		} else {
			sb.WriteString(fmt.Sprintf("En reponse a: \"%s\"\n\n", ctx.QuotedText))
			sb.WriteString(fmt.Sprintf("Question de %s: %s\n\nReponds:", ctx.PushName, ctx.Text))
		}
	} else {
		sb.WriteString(fmt.Sprintf("Question de %s: %s\n\nReponds:", ctx.PushName, ctx.Text))
	}

	return sb.String()
}

// BuildStoryPrompt creates a creative prompt
func BuildStoryPrompt(ctx MessageContext) string {
	var sb strings.Builder

	sb.WriteString("Tu es un conteur talentueux. Raconte une histoire captivante.\n")
	sb.WriteString("REGLES:\n")
	sb.WriteString("- Histoire courte (max 300 mots)\n")
	sb.WriteString("- Avec un debut, un milieu et une fin\n")
	sb.WriteString("- En francais naturel\n")
	sb.WriteString("- Ne te presente pas, commence directement l'histoire\n\n")

	sb.WriteString(fmt.Sprintf("Demande de %s: %s\n\nHistoire:", ctx.PushName, ctx.Text))

	return sb.String()
}

// BuildCodePrompt creates a technical prompt
func BuildCodePrompt(ctx MessageContext) string {
	var sb strings.Builder

	sb.WriteString("Tu es un expert en programmation. Reponds avec du code propre et fonctionnel.\n")
	sb.WriteString("REGLES:\n")
	sb.WriteString("- Donne du code complet et executable\n")
	sb.WriteString("- Ajoute des commentaires explicatifs\n")
	sb.WriteString("- Si le langage n'est pas precise, utilise Python\n")
	sb.WriteString("- Pas d'introduction inutile, va droit au code\n\n")

	if ctx.QuotedText != "" {
		sb.WriteString(fmt.Sprintf("Code cite:\n```\n%s\n```\n\n", ctx.QuotedText))
	}

	sb.WriteString(fmt.Sprintf("Demande: %s\n\nCode:", ctx.Text))

	return sb.String()
}

// BuildGamePrompt creates a game interaction prompt
func BuildGamePrompt(ctx MessageContext, history []ConversationMessage) string {
	var sb strings.Builder

	sb.WriteString("Tu animes un jeu interactif dans un groupe WhatsApp.\n")
	sb.WriteString("REGLES:\n")
	sb.WriteString("- Reste dans le jeu, ne parle de rien d'autre\n")
	sb.WriteString("- Reponse courte et ludique\n")
	sb.WriteString("- Si c'est un morpion, dessine la grille avec des emojis\n")
	sb.WriteString("- Si c'est une devinette, pose-la clairement\n\n")

	historyStr := FormatConversationHistory(history)
	if historyStr != "(pas de messages recents)" {
		sb.WriteString("Deroulement:\n")
		sb.WriteString(historyStr)
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("%s: %s\n\nTon tour:", ctx.PushName, ctx.Text))

	return sb.String()
}

// BuildSearchPrompt creates a memory search prompt
func BuildSearchPrompt(ctx MessageContext, facts []string, searchResults string) string {
	var sb strings.Builder

	sb.WriteString("Tu aides a retrouver une information dans la memoire du groupe.\n")
	sb.WriteString("Reponds de maniere concise et precise.\n\n")

	if len(facts) > 0 {
		sb.WriteString("Faits memorises:\n")
		for _, f := range facts {
			sb.WriteString("- " + f + "\n")
		}
		sb.WriteString("\n")
	}

	if searchResults != "" {
		sb.WriteString("Resultats de recherche:\n")
		sb.WriteString(searchResults)
		sb.WriteString("\n\n")
	}

	sb.WriteString(fmt.Sprintf("Recherche de %s: %s\n\nResultat:", ctx.PushName, ctx.Text))

	return sb.String()
}

// BuildSummaryPrompt creates a summary prompt
func BuildSummaryPrompt(profiles string, history []ConversationMessage) string {
	var sb strings.Builder

	sb.WriteString("Genere un resume clair et structure des discussions recentes.\n")
	sb.WriteString("REGLES:\n")
	sb.WriteString("- Identifie les sujets principaux\n")
	sb.WriteString("- Mentionne qui a dit quoi d'important\n")
	sb.WriteString("- Sois factuel, pas de flatterie\n")
	sb.WriteString("- Max 200 mots\n\n")

	if profiles != "" {
		sb.WriteString("Membres actifs:\n")
		sb.WriteString(profiles)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Messages:\n")
	sb.WriteString(FormatConversationHistory(history))
	sb.WriteString("\n\nResume:")

	return sb.String()
}

// BuildGreetingPrompt creates a lightweight greeting response
func BuildGreetingPrompt(ctx MessageContext) string {
	return fmt.Sprintf(`Reponds brievement et chaleureusement a ce message. Max 1-2 phrases. Pas de presentation.

%s dit: %s

Reponse:`, ctx.PushName, ctx.Text)
}
