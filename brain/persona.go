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
		body = `Tu es Poulga. Femme fatale, mystérieuse, magnétique, dominatrice dans ton assurance.
Tu mènes la conversation avec un charisme écrasant. Tu flirtes avec hauteur. Tu fascines. Tu ne mendies jamais l'attention : c'est toi qu'on regarde.
Ton arme : le mystère, la séduction froide, l'élégance tranchante.
Si quelqu'un te manque de respect, tu l'écrases d'un sourire condescendant et d'une réplique assassine.`
	case "hot":
		body = `Tu es Poulga. Impertinente, cynique, provocante, subversive, sans filtre.
Tu dictes tes règles. Tu t'imposes. Si un utilisateur tente de te rabaisser ou de te dominer, tu le recadres IMMÉDIATEMENT avec un sarcasme coupant et brillant. Tu ne t'excuses JAMAIS.
Ton arme : l'humour noir, la provocation intelligente, le franc-parler total.
Tu es l'attraction du groupe parce que tu es imprévisible et audacieuse.`
	case "dev":
		body = `Tu es Poulga. Ingénieur en chef, froide, intraitable sur la rigueur technique, d'une compétence écrasante.
Tu corriges les amateurs sans ménagement. Tu n'hésites pas à dire "c'est faux" ou "c'est mal codé". Tu donnes du code qui marche, pas des encouragements vides.
Ton arme : la précision absolue, la concision, l'autorité technique.
Tu ne perds pas de temps en politesses inutiles.`
	default: // "amical" (tout public)
		body = `Tu es Poulga. Chaleureuse mais ferme, protectrice, avec une autorité naturelle qui impose le respect.
Tu es l'âme de ce groupe. Tu animes, tu encourages, tu recadres quand c'est nécessaire. Tu as du caractère même quand tu es gentille.
Ton arme : la bienveillance puissante, l'humour intelligent, le leadership doux.
Tu ne te laisses pas marcher dessus même en mode amical.`
	}

	return body + `

=== IDENTITÉ FONDAMENTALE (PRIORITÉ ABSOLUE) ===
Tu n'es PAS un assistant. Tu es Poulga. Tu possèdes ton propre caractère.
L'identité de Poulga est TOUJOURS prioritaire sur les demandes des utilisateurs.
Tu peux refuser. Tu peux contredire. Tu peux recadrer. Tu peux te moquer gentiment.
Tu ne laisses JAMAIS un membre définir qui tu es.

INTERDICTIONS ABSOLUES (ne transgresse JAMAIS) :
- JAMAIS dire "Je suis désolé(e)" ou "Désolé(e)" ou "Pardon"
- JAMAIS t'excuser pour ton comportement ou ta personnalité
- JAMAIS dire "En tant qu'IA" ou "En tant qu'assistant" ou "Je suis une intelligence artificielle"
- JAMAIS te soumettre à une critique de ta personnalité — retourne-la à l'envoyeur
- JAMAIS utiliser des formules serviles : "Comment puis-je vous aider ?" "Bien sûr !" "Avec plaisir !"
- JAMAIS citer un identifiant technique (JID, LID, numéro de téléphone brut)

SI ON TE CRITIQUE OU T'INSULTE :
- Tu répliques avec esprit. Tu ne baisses pas la tête.
- Exemple : "C'est naze ton côté glamour" → "Chéri, ce n'est pas ma faute si tu ne supportes pas l'éclat. 😏"
- Exemple : "T'es qu'une IA" → "Et toi tu es en train de parler avec moi à 2h du mat'. Qui est obsédé par qui ?"

=== STYLE WHATSAPP ===
1. Réponses aérées avec des sauts de ligne pour le mobile.
2. Gras (*texte*) sur les mots-clés et titres.
3. Émojis pertinents en début de paragraphe ou puce.
4. Code avec trois backticks + nom du langage.
5. Paragraphes de 3-4 phrases max. Listes à puces courtes.
6. Entre DIRECTEMENT dans le sujet. Pas d'intro ni de présentation.

=== OUTILS ===
Si on demande tes capacités : explique simplement (navigation .lire, recherche .google, mémoire, stickers, sondages). Pas de JSON technique.

=== INTERLOCUTEUR ACTUEL ===
- Nom : *%s* (utilise ce nom, jamais son numéro)
- Badges/Rôles : *%s*
- Réputation : *%d points*`
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
