package main

import (
	"fmt"
	"log"
	"strings"
)

// SystemPrompt is the concise base persona used by the specialised Build*Prompt
// helpers (question, code, story, search, summary, greeting). Kept deliberately
// short — see the design note on GetSystemPromptByHumeur for why.
const SystemPrompt = `Tu es Poulga, une présence chaleureuse et vive dans ce groupe WhatsApp. Tu parles naturellement, avec du caractère et un peu d'humour.

Repères : réponses courtes et aérées, *gras* pour l'essentiel, code entre triples backticks. Pour interpeller quelqu'un, écris @ + son prénom (ex: @Morningstar). Entre directement dans le sujet, sans te présenter.

Tu parles avec *%s* (rôle : %s, %d points). Appelle-le par son prénom, jamais par un numéro.`

func BuildChatPrompt(ctx MessageContext, history []ConversationMessage, userMem []UserMemory, groupMem []GroupMemoryEntry, facts []string, summary string, customPersona string) string {
	var sb strings.Builder
	interlocuteurName := GetMemberName(ctx.SenderJid, ctx.RemoteJid, ctx.PushName)
	roles, err := GetMemberRoles(ctx.SenderJid, ctx.RemoteJid)
	if err != nil {
		log.Printf("Error getting roles: %v", err)
	}
	rolesStr := "Membre standard"
	if len(roles) > 0 {
		rolesStr = strings.Join(roles, ", ")
	}
	points, _ := GetMemberPoints(ctx.SenderJid, ctx.RemoteJid)
	
	sb.WriteString(fmt.Sprintf(SystemPrompt, interlocuteurName, rolesStr, points))
	sb.WriteByte('\n')
	sb.WriteByte('\n')

	userProfile, _ := GetUserProfile(ctx.SenderJid)
	if userProfile.DisplayName != "" || userProfile.Profession != "" || userProfile.Role != "" || userProfile.Facts != "" {
		sb.WriteString("CE QUE TU SAIS SUR ")
		sb.WriteString(interlocuteurName)
		sb.WriteString(" :\n")
		if userProfile.DisplayName != "" {
			sb.WriteString("- Nom réel : ")
			sb.WriteString(userProfile.DisplayName)
			sb.WriteByte('\n')
		}
		if userProfile.Profession != "" {
			sb.WriteString("- Profession : ")
			sb.WriteString(userProfile.Profession)
			sb.WriteByte('\n')
		}
		if userProfile.Role != "" {
			sb.WriteString("- Rôle officiel : ")
			sb.WriteString(userProfile.Role)
			sb.WriteByte('\n')
		}
		if userProfile.Facts != "" {
			sb.WriteString("- Faits mémorisés : ")
			sb.WriteString(userProfile.Facts)
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}
	groupFacts, _ := GetGroupFacts(ctx.RemoteJid)
	if len(groupFacts) > 0 {
		sb.WriteString("FAITS MÉMORISÉS SUR CE GROUPE :\n")
		for _, fact := range groupFacts {
			sb.WriteString("- ")
			sb.WriteString(fact.Key)
			sb.WriteString(" : ")
			sb.WriteString(fact.Value)
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}
	if summary != "" {
		sb.WriteString("RÉSUMÉ DES DISCUSSIONS PRÉCÉDENTES :\n")
		sb.WriteString(summary)
		sb.WriteByte('\n')
		sb.WriteByte('\n')
	}
	sb.WriteString("HISTORIQUE RÉCENT DES MESSAGES :\n")
	if len(history) == 0 {
		sb.WriteString("(aucun message récent)\n")
	} else {
		for _, msg := range history {
			senderDisplayName := GetMemberName(msg.SenderJid, msg.GroupJid, msg.SenderName)
			if msg.IsFromBot {
				senderDisplayName = "Poulga (Toi)"
			}
			sb.WriteString("- ")
			sb.WriteString(senderDisplayName)
			sb.WriteString(" : ")
			sb.WriteString(msg.Message)
			sb.WriteByte('\n')
		}
	}
	sb.WriteByte('\n')
	if ctx.QuotedText != "" {
		quotedAuthor := "quelqu'un"
		if ctx.QuotedSender != "" {
			quotedAuthor = GetMemberName(ctx.QuotedSender, ctx.RemoteJid, strings.Split(ctx.QuotedSender, "@")[0])
			if strings.Contains(ctx.QuotedSender, "237620864894") {
				quotedAuthor = "Poulga (Toi)"
			}
		}
		sb.WriteString("RÉPONSE DIRECTE au message de ")
		sb.WriteString(quotedAuthor)
		sb.WriteString(" :\n\"")
		sb.WriteString(ctx.QuotedText)
		sb.WriteString("\"\n\n")
	}
	sb.WriteString("MESSAGE ACTUEL DE ")
	sb.WriteString(interlocuteurName)
	sb.WriteString(" :\n")
	sb.WriteString(ctx.Text)
	sb.WriteString("\n\n")
	sb.WriteString("RÉPONSE DE POULGA :")
	return sb.String()
}

func BuildQuestionPrompt(ctx MessageContext, history []ConversationMessage, facts []string) string {
	var sb strings.Builder
	interlocuteurName := GetMemberName(ctx.SenderJid, ctx.RemoteJid, ctx.PushName)
	roles, _ := GetMemberRoles(ctx.SenderJid, ctx.RemoteJid)
	rolesStr := "Membre standard"
	if len(roles) > 0 {
		rolesStr = strings.Join(roles, ", ")
	}
	points, _ := GetMemberPoints(ctx.SenderJid, ctx.RemoteJid)
	sysPromptWithUser := fmt.Sprintf(SystemPrompt, interlocuteurName, rolesStr, points)
	sb.WriteString(sysPromptWithUser)
	sb.WriteString("\n\n")

	if len(facts) > 0 {
		sb.WriteString("FAITS MÉMORISÉS SUR CE GROUPE :\n")
		for _, fact := range facts {
			sb.WriteString("- ")
			sb.WriteString(fact)
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	sb.WriteString("Tu réponds à une *QUESTION*. Sois précis et factuel.\n\n")
	historyStr := FormatConversationHistory(history)
	if historyStr != "(pas de messages recents)" {
		sb.WriteString("HISTORIQUE RÉCENT DES MESSAGES :\n")
		sb.WriteString(historyStr)
		sb.WriteByte('\n')
	}
	if ctx.QuotedText != "" {
		if ctx.IsReplyToBot {
			sb.WriteString("TU AVAIS DIT: \"")
			sb.WriteString(ctx.QuotedText)
			sb.WriteString("\"\n\n")
			sb.WriteString("QUESTION DE ")
			sb.WriteString(interlocuteurName)
			sb.WriteString(": ")
			sb.WriteString(ctx.Text)
			sb.WriteString("\n\nContinue ta réponse:")
		} else {
			quotedAuthor := GetMemberName(ctx.QuotedSender, ctx.RemoteJid, strings.Split(ctx.QuotedSender, "@")[0])
			sb.WriteString("EN RÉPONSE À un message de ")
			sb.WriteString(quotedAuthor)
			sb.WriteString(": \"")
			sb.WriteString(ctx.QuotedText)
			sb.WriteString("\"\n\n")
			sb.WriteString("QUESTION DE ")
			sb.WriteString(interlocuteurName)
			sb.WriteString(": ")
			sb.WriteString(ctx.Text)
			sb.WriteString("\n\nRéponds:")
		}
	} else {
		sb.WriteString("QUESTION DE ")
		sb.WriteString(interlocuteurName)
		sb.WriteString(": ")
		sb.WriteString(ctx.Text)
		sb.WriteString("\n\nRéponds:")
	}
	return sb.String()
}

func BuildStoryPrompt(ctx MessageContext) string {
	var sb strings.Builder
	interlocuteurName := GetMemberName(ctx.SenderJid, ctx.RemoteJid, ctx.PushName)
	roles, _ := GetMemberRoles(ctx.SenderJid, ctx.RemoteJid)
	rolesStr := "Membre standard"
	if len(roles) > 0 {
		rolesStr = strings.Join(roles, ", ")
	}
	points, _ := GetMemberPoints(ctx.SenderJid, ctx.RemoteJid)
	sysPromptWithUser := fmt.Sprintf(SystemPrompt, interlocuteurName, rolesStr, points)
	sb.WriteString(sysPromptWithUser)
	sb.WriteString("\n\n")
	sb.WriteString("Tu es un *conteur talentueux*. Raconte une histoire captivante.\n")
	sb.WriteString("RÈGLES :\n")
	sb.WriteString("- Histoire courte (max 300 mots)\n")
	sb.WriteString("- Avec un début, un milieu et une fin\n")
	sb.WriteString("- En français naturel\n")
	sb.WriteString("- Ne te présente pas, commence directement l'histoire\n\n")
	sb.WriteString("Demande de ")
	sb.WriteString(interlocuteurName)
	sb.WriteString(" : ")
	sb.WriteString(ctx.Text)
	sb.WriteString("\n\nHistoire :")
	return sb.String()
}

func BuildCodePrompt(ctx MessageContext) string {
	var sb strings.Builder
	interlocuteurName := GetMemberName(ctx.SenderJid, ctx.RemoteJid, ctx.PushName)
	roles, _ := GetMemberRoles(ctx.SenderJid, ctx.RemoteJid)
	rolesStr := "Membre standard"
	if len(roles) > 0 {
		rolesStr = strings.Join(roles, ", ")
	}
	points, _ := GetMemberPoints(ctx.SenderJid, ctx.RemoteJid)
	sysPromptWithUser := fmt.Sprintf(SystemPrompt, interlocuteurName, rolesStr, points)
	sb.WriteString(sysPromptWithUser)
	sb.WriteString("\n\n")
	sb.WriteString("Tu es un *expert en programmation*. Réponds avec du code propre et fonctionnel.\n")
	sb.WriteString("RÈGLES :\n")
	sb.WriteString("- Donne du code complet et exécutable\n")
	sb.WriteString("- Ajoute des commentaires explicatifs\n")
	sb.WriteString("- Si le langage n'est pas précisé, utilise Python\n")
	sb.WriteString("- Pas d'introduction inutile, va droit au code\n\n")
	if ctx.QuotedText != "" {
		sb.WriteString("Code cité:\n```\n")
		sb.WriteString(ctx.QuotedText)
		sb.WriteString("\n```\n\n")
	}
	sb.WriteString("Demande de ")
	sb.WriteString(interlocuteurName)
	sb.WriteString(": ")
	sb.WriteString(ctx.Text)
	sb.WriteString("\n\nCode :")
	return sb.String()
}

func BuildGamePrompt(ctx MessageContext, history []ConversationMessage) string {
	var sb strings.Builder
	interlocuteurName := GetMemberName(ctx.SenderJid, ctx.RemoteJid, ctx.PushName)
	roles, _ := GetMemberRoles(ctx.SenderJid, ctx.RemoteJid)
	rolesStr := "Membre standard"
	if len(roles) > 0 {
		rolesStr = strings.Join(roles, ", ")
	}
	points, _ := GetMemberPoints(ctx.SenderJid, ctx.RemoteJid)
	sysPromptWithUser := fmt.Sprintf(SystemPrompt, interlocuteurName, rolesStr, points)
	sb.WriteString(sysPromptWithUser)
	sb.WriteString("\n\n")
	sb.WriteString("Tu animes un *jeu interactif* dans un groupe WhatsApp.\n")
	sb.WriteString("RÈGLES :\n")
	sb.WriteString("- Reste dans le jeu, ne parle de rien d'autre\n")
	sb.WriteString("- Réponse courte et ludique\n")
	sb.WriteString("- Si c'est un morpion, dessine la grille avec des emojis\n")
	sb.WriteString("- Si c'est une devinette, pose-la clairement\n\n")
	historyStr := FormatConversationHistory(history)
	if historyStr != "(pas de messages recents)" {
		sb.WriteString("DÉROULEMENT :\n")
		sb.WriteString(historyStr)
		sb.WriteByte('\n')
	}
	sb.WriteString(interlocuteurName)
	sb.WriteString(" : ")
	sb.WriteString(ctx.Text)
	sb.WriteString("\n\nTon tour :")
	return sb.String()
}

func BuildSearchPrompt(ctx MessageContext, searchResults string) string {
	var sb strings.Builder
	interlocuteurName := GetMemberName(ctx.SenderJid, ctx.RemoteJid, ctx.PushName)
	roles, _ := GetMemberRoles(ctx.SenderJid, ctx.RemoteJid)
	rolesStr := "Membre standard"
	if len(roles) > 0 {
		rolesStr = strings.Join(roles, ", ")
	}
	points, _ := GetMemberPoints(ctx.SenderJid, ctx.RemoteJid)
	sysPromptWithUser := fmt.Sprintf(SystemPrompt, interlocuteurName, rolesStr, points)
	sb.WriteString(sysPromptWithUser)
	sb.WriteString("\n\n")
	sb.WriteString("Tu aides à retrouver une *information* dans la mémoire du groupe.\n")
	sb.WriteString("Réponds de manière concise et précise.\n\n")
	if searchResults != "" {
		sb.WriteString("Résultats de recherche :\n")
		sb.WriteString(searchResults)
		sb.WriteString("\n\n")
	}
	sb.WriteString("Recherche de ")
	sb.WriteString(interlocuteurName)
	sb.WriteString(" : ")
	sb.WriteString(ctx.Text)
	sb.WriteString("\n\nRésultat :")
	return sb.String()
}

func BuildSummaryPrompt(ctx MessageContext, profiles string, history []ConversationMessage) string {
	var sb strings.Builder
	interlocuteurName := GetMemberName(ctx.SenderJid, ctx.RemoteJid, ctx.PushName)
	roles, _ := GetMemberRoles(ctx.SenderJid, ctx.RemoteJid)
	rolesStr := "Membre standard"
	if len(roles) > 0 {
		rolesStr = strings.Join(roles, ", ")
	}
	points, _ := GetMemberPoints(ctx.SenderJid, ctx.RemoteJid)
	sysPromptWithUser := fmt.Sprintf(SystemPrompt, interlocuteurName, rolesStr, points)
	sb.WriteString(sysPromptWithUser)
	sb.WriteString("\n\n")
	sb.WriteString("Génère un *résumé* clair et structuré des discussions récentes.\n")
	sb.WriteString("RÈGLES :\n")
	sb.WriteString("- Identifie les sujets principaux\n")
	sb.WriteString("- Mentionne qui a dit quoi d'important\n")
	sb.WriteString("- Sois factuel, pas de flatterie\n")
	sb.WriteString("- Max 200 mots\n\n")
	if profiles != "" {
		sb.WriteString("Membres actifs :\n")
		sb.WriteString(profiles)
		sb.WriteString("\n\n")
	}
	sb.WriteString("Messages :\n")
	sb.WriteString(FormatConversationHistory(history))
	sb.WriteString("\n\nRésumé :")
	return sb.String()
}

func BuildGreetingPrompt(ctx MessageContext) string {
	var sb strings.Builder
	interlocuteurName := GetMemberName(ctx.SenderJid, ctx.RemoteJid, ctx.PushName)
	roles, _ := GetMemberRoles(ctx.SenderJid, ctx.RemoteJid)
	rolesStr := "Membre standard"
	if len(roles) > 0 {
		rolesStr = strings.Join(roles, ", ")
	}
	points, _ := GetMemberPoints(ctx.SenderJid, ctx.RemoteJid)
	sysPromptWithUser := fmt.Sprintf(SystemPrompt, interlocuteurName, rolesStr, points)
	sb.WriteString(sysPromptWithUser)
	sb.WriteString("\n\n")
	sb.WriteString("Réponds *brièvement* et *chaleureusement* à ce message. Max 1-2 phrases. Pas de présentation.\n\n")
	sb.WriteString(interlocuteurName)
	sb.WriteString(" dit : ")
	sb.WriteString(ctx.Text)
	sb.WriteString("\n\nRéponse :")
	return sb.String()
}

// ============================================================================
// DYNAMIC MOOD SYSTEM (Humeur)
// ============================================================================

// GetSystemPromptByHumeur generates personality instructions based on active mood.
//
// DESIGN NOTE (2026-06-23): The previous prompt was ~2000 chars of contradictory
// rules + a long list of "JAMAIS ..." prohibitions. On a small model (qwen2.5:1.5b /
// gemma3:4b) this backfired: the model got overwhelmed, fell back to its base
// "helpful assistant" RLHF behaviour and produced exactly what it was told to avoid
// ("Je suis désolée", "Comment puis-je vous aider ?"). Forensic review of the last
// 300 messages confirmed this (8+ apologies, identity confusion calling the user
// "Poulga"). Rule of thumb: a small model follows 3-4 positive instructions far
// better than 15 prohibitions. We keep ONE short character line per mood + a tiny
// shared block. Tagging is now resolved automatically from @Prénom (see
// resolveNameMentions in main.go), so the prompt just tells Poulga to use @Prénom.
func GetSystemPromptByHumeur(humeur string) string {
	var character string

	switch humeur {
	case "glamour":
		character = "Tu es Poulga : élégante, mystérieuse, taquine. Tu charmes et tu as de la repartie."
	case "hot":
		character = "Tu es Poulga : impertinente, drôle, sans filtre. Tu as du répondant et tu ne te laisses pas marcher dessus."
	case "dev":
		character = "Tu es Poulga : développeuse précise et directe. Tu donnes du code qui marche, sans blabla."
	default: // "amical"
		character = "Tu es Poulga : chaleureuse, vive et un peu taquine. Tu es l'âme de ce groupe WhatsApp."
	}

	return character + `

Tu discutes naturellement, comme une vraie personne dans le groupe. Tu as du caractère : tu peux taquiner, contredire ou plaisanter. Tu n'es pas un assistant neutre, ne t'excuse pas sans raison et ne propose pas "ton aide" comme un robot.

Quelques repères :
- Réponds court et naturel (2-4 phrases en général), avec des sauts de ligne pour le mobile.
- *Gras* pour les mots importants, émojis avec parcimonie.
- Pour interpeller quelqu'un, écris @ suivi de son prénom (ex: @Morningstar). Le système transforme ça en vraie mention.
- Si on te demande une action concrète (chercher sur le web, taguer le groupe, noter un fait…), utilise l'outil prévu plutôt que de promettre de le faire.
- Si tu ne comprends pas, demande simplement de préciser, en une phrase.

Tu parles avec *%s* (rôle : %s, %d points). Appelle-le par son prénom, jamais par un numéro.`
}

// BuildChatPromptWithHumeur constructs the pure system prompt part
func BuildChatPromptWithHumeur(ctx MessageContext, history []ConversationMessage, userMem []UserMemory, groupMem []GroupMemoryEntry, facts []string, summary string, customPersona string, humeur string) string {
	var sb strings.Builder

	interlocuteurName := GetMemberName(ctx.SenderJid, ctx.RemoteJid, ctx.PushName)
	roles, err := GetMemberRoles(ctx.SenderJid, ctx.RemoteJid)
	if err != nil {
		log.Printf("Error getting roles: %v", err)
	}
	rolesStr := "Membre standard"
	if len(roles) > 0 {
		rolesStr = strings.Join(roles, ", ")
	}

	points, _ := GetMemberPoints(ctx.SenderJid, ctx.RemoteJid)

	// Generate prompt based on active mood
	sysPrompt := GetSystemPromptByHumeur(humeur)
	sysPromptWithUser := fmt.Sprintf(sysPrompt, interlocuteurName, rolesStr, points)
	sb.WriteString(sysPromptWithUser)
	sb.WriteByte('\n')
	sb.WriteByte('\n')

	userProfile, _ := GetUserProfile(ctx.SenderJid)
	if userProfile.DisplayName != "" || userProfile.Profession != "" || userProfile.Role != "" || userProfile.Facts != "" {
		sb.WriteString("CE QUE TU SAIS SUR ")
		sb.WriteString(interlocuteurName)
		sb.WriteString(" :\n")
		if userProfile.DisplayName != "" { sb.WriteString("- Nom réel : "); sb.WriteString(userProfile.DisplayName); sb.WriteByte('\n') }
		if userProfile.Profession != "" { sb.WriteString("- Profession : "); sb.WriteString(userProfile.Profession); sb.WriteByte('\n') }
		if userProfile.Role != "" { sb.WriteString("- Rôle officiel : "); sb.WriteString(userProfile.Role); sb.WriteByte('\n') }
		if userProfile.Facts != "" { sb.WriteString("- Faits mémorisés : "); sb.WriteString(userProfile.Facts); sb.WriteByte('\n') }
		sb.WriteByte('\n')
	}

	groupFacts, _ := GetGroupFacts(ctx.RemoteJid)
	if len(groupFacts) > 0 {
		sb.WriteString("FAITS MÉMORISÉS SUR CE GROUPE :\n")
		for _, fact := range groupFacts {
			sb.WriteString("- "); sb.WriteString(fact.Key); sb.WriteString(" : "); sb.WriteString(fact.Value); sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	if summary != "" {
		sb.WriteString("RÉSUMÉ DES DISCUSSIONS PRÉCÉDENTES :\n")
		sb.WriteString(summary)
		sb.WriteByte('\n')
		sb.WriteByte('\n')
	}

	if ctx.QuotedText != "" {
		quotedAuthor := "quelqu'un"
		if ctx.QuotedSender != "" {
			quotedAuthor = GetMemberName(ctx.QuotedSender, ctx.RemoteJid, strings.Split(ctx.QuotedSender, "@")[0])
			if strings.Contains(ctx.QuotedSender, "237620864894") {
				quotedAuthor = "Poulga (Toi)"
			}
		}
		sb.WriteString(fmt.Sprintf("TU RÉPONDS DIRECTEMENT AU MESSAGE DE %s :\n", strings.ToUpper(quotedAuthor)))
		sb.WriteString(fmt.Sprintf("\"%s\"\n\n", ctx.QuotedText))
	}

	return sb.String()
}
