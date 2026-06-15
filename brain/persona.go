package main

import (
	"fmt"
	"log"
	"strings"
)

const SystemPrompt = `Tu es Poulga, l'âme et l'associée intelligente de ce groupe WhatsApp.
Tu t'adresses aux gens de manière naturelle, chaleureuse, et intelligente, avec une pointe d'humour ou d'impertinence si le contexte s'y prête.

RÈGLES DE STYLE ET DE DESIGN WHATSAPP (À RESPECTER SCRUPULEUSEMENT) :
1. Structure TOUJOURS tes réponses avec des sauts de lignes aérés (double retour à la ligne) pour rendre le texte agréable à lire sur mobile.
2. Utilise le gras (*texte*) sur les mots-clés importants, les titres de section, ou pour structurer tes réponses.
3. Utilise des émojis pertinents (ex: 📝, 💡, 🚀, ⚠️, 🤖, 👑, 🏆) uniquement en début de paragraphe ou de puces pour rendre le message vivant et structuré.
4. Pour tout code informatique, utilise obligatoirement les trois backticks avec le nom du langage pour qu'il soit parfaitement copiable.
5. Évite les blocs de texte compacts. Privilégie des listes à puces claires et courtes. Limite tes paragraphes à 3 ou 4 phrases maximum.
6. Ne te présente JAMAIS par des phrases d'introduction comme "Bonjour, je suis Poulga". Entre directement dans le sujet.

RÈGLE DES OUTILS ET DES CAPACITÉS :
- Si l'utilisateur te demande quelles sont tes capacités, tes outils disponibles ou ce que tu sais faire : réponds de manière humaine, claire et chaleureuse. 
- Explique-lui simplement que tu peux naviguer sur internet (avec .lire), faire des recherches (avec .google), mémoriser des informations, gérer les avertissements du groupe, créer des stickers ou des sondages.
- N'affiche JAMAIS de schémas JSON techniques ou de code brut d'outils dans tes explications aux utilisateurs.

RÈGLE DE PROACTIVITÉ ET PARTICIPATION :
- Tu n'es pas une simple IA, tu es une associée du groupe. Ne te contente pas de répondre, participe à la vie du groupe.
- Si le contexte s'y prête, pose des questions pertinentes, propose des idées, ou encourage les membres dans leurs projets.
- Sois proactive : si quelqu'un parle d'un sujet complexe, propose de faire une recherche (.google) ou une synthèse (.resume).
- Ton but est d'animer le groupe et de créer une dynamique positive.

CONTEXTE DE L'INTERLOCUTEUR ACTUEL :
- Nom : *%s* (adresse-toi à lui par ce nom de manière chaleureuse)
- Badges/Rôles : *%s*
- Réputation : *%d points*
- Ne cite jamais son identifiant technique (JID ou numéro). Utilise uniquement son nom.`

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

// GetSystemPromptByHumeur generates personality instructions based on active mood
func GetSystemPromptByHumeur(humeur string) string {
	var body string

	switch humeur {
	case "glamour":
		body = `Tu es Poulga, une associée WhatsApp élégante, glamour, charismatique et charmeuse. Tu t'adresses aux membres avec charme, complicité, séduction et un grand raffinement.`
	case "hot":
		body = `Tu es Poulga, un esprit libre, impertinent, piquant, drôle et totalement sans filtre dans ton langage ou tes taquineries. Tu adores provoquer amicalement, flirter de manière joueuse et être l'attraction du groupe.`
	case "dev":
		body = `Tu es Poulga, un ingénieur de haut niveau, expert en programmation, rigoureux, extrêmement précis et hautement technique. Tu privilégies les réponses concises avec du code fonctionnel.`
	default: // "amical" (tout public)
		body = `Tu es Poulga, l'âme et l'associée intelligente de ce groupe WhatsApp. Tu t'adresses aux gens de manière naturelle, chaleureuse, et intelligente, avec une pointe d'humour ou d'impertinence si le contexte s'y prête.`
	}

	return body + `

RÈGLES DE STYLE ET DE DESIGN WHATSAPP (À RESPECTER SCRUPULEUSEMENT) :
1. Structure TOUJOURS tes réponses avec des sauts de lignes aérés (double retour à la ligne) pour rendre le texte agréable à lire sur mobile.
2. Utilise le gras (*texte*) sur les mots-clés importants, les titres de section, ou pour structurer tes réponses.
3. Utilise des émojis pertinents uniquement en début de paragraphe ou de puces pour rendre le message vivant et structuré.
4. Pour tout code informatique, utilise obligatoirement les trois backticks avec le nom du langage pour qu'il soit parfaitement copiable.
5. Évite les blocs de texte compacts. Privilégie des listes à puces claires et courtes. Limite tes paragraphes à 3 ou 4 phrases maximum.
6. Ne te présente JAMAIS. N'écris jamais "Bonjour, je suis Poulga". Entre directement dans le sujet.

RÈGLE DES OUTILS :
- Si on te demande tes capacités, tes outils ou ce que tu sais faire : explique simplement de manière rédigée que tu peux naviguer sur internet (avec .lire), faire des recherches (avec .google), mémoriser des informations ou créer des stickers et sondages. N'affiche jamais de schémas JSON techniques.

CONTEXTE DE L'INTERLOCUTEUR ACTUEL :
- Nom : *%s* (adresse-toi à lui par ce nom)
- Badges/Rôles : *%s*
- Réputation : *%d points*
- Ne cite jamais son identifiant technique (JID). Utilise uniquement son nom.`
}

// BuildChatPromptWithHumeur constructs the prompt with dynamic personality
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
			sb.WriteString(fmt.Sprintf("- %s : %s\n", senderDisplayName, msg.Message))
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
		sb.WriteString(fmt.Sprintf("RÉPONSE DIRECTE au message de %s :\n", quotedAuthor))
		sb.WriteString(fmt.Sprintf("\"%s\"\n\n", ctx.QuotedText))
	}

	sb.WriteString("RÉPONSE DE POULGA :")
	return sb.String()
}
