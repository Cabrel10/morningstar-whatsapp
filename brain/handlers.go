package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// handleCommand routes all bot commands
func handleCommand(ctx MessageContext, cmd, args string) {
	var response string
	instance := ctx.Instance
	remoteJid := ctx.RemoteJid
	senderJid := ctx.SenderJid
	msgId := ctx.MsgId

	fmt.Printf("[CMD] %s | args=%q | from=%s\n", cmd, args, ctx.PushName)

	switch cmd {

	// ========================================================================
	// IDENTITY & PRESENTATION
	// ========================================================================

	case "je-suis", "presenter", "presentation":
		if args == "" {
			style := ResponseStyle{
				Title:      "Erreur de format",
				TitleEmoji: "⚠️",
				Sections: []Section{
					{
						Content: "Pour te présenter officiellement à Poulga, utilise la commande comme ceci :",
						Items: []string{
							"*.je-suis [Ton Prénom / Pseudo]*",
							"Exemple : `.je-suis Herold`",
						},
					},
				},
			}
			response = RenderWhatsApp(style)
			break
		}
		name := strings.TrimSpace(args)
		err := SaveMemberName(senderJid, remoteJid, name)
		if err != nil {
			response = "❌ Erreur technique lors de l'enregistrement."
		} else {
			_ = LogProfileChange(senderJid, remoteJid, "custom_name", "", name, "user")
			_ = UpdateMemberPoints(senderJid, remoteJid, 10)
			style := ResponseStyle{
				Title:      "Présentation validée !",
				TitleEmoji: "✨",
				Sections: []Section{
					{
						Content: fmt.Sprintf("Enchantée, *%s* ! J'ai enregistré ton identité. Je t'appellerai désormais par ton nom.", name),
					},
					{
						Title:      "Récompense",
						TitleEmoji: "🏆",
						Content:    "Tu as reçu *+10 points* pour ta présentation !",
					},
				},
			}
			response = RenderWhatsApp(style)
		}

	case "nommer":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Commande réservée aux administrateurs."
			break
		}
		parts := strings.Fields(args)
		if len(parts) < 2 || !strings.Contains(parts[0], "@") {
			response = "⚠️ Usage : `.nommer @user [Nouveau Nom]`"
			break
		}
		targetJid := extractJid(parts[0], "")
		newName := strings.Join(parts[1:], " ")
		oldName := GetMemberName(targetJid, remoteJid, "")
		err := SaveMemberName(targetJid, remoteJid, newName)
		if err != nil {
			response = "❌ Erreur."
		} else {
			_ = LogProfileChange(targetJid, remoteJid, "custom_name", oldName, newName, senderJid)
			response = fmt.Sprintf("✅ @%s est maintenant *%s*.", strings.Split(targetJid, "@")[0], newName)
		}

	case "qui":
		rows, err := db.Query(context.Background(), "SELECT jid, custom_name FROM member_profiles WHERE group_jid = $1", remoteJid)
		if err != nil {
			response = "❌ Erreur base de données."
			break
		}
		defer rows.Close()
		var list []string
		for rows.Next() {
			var jid, name string
			if err := rows.Scan(&jid, &name); err == nil {
				pts, _ := GetMemberPoints(jid, remoteJid)
				list = append(list, fmt.Sprintf("@%s : *%s* (%d pts)", strings.Split(jid, "@")[0], name, pts))
			}
		}
		if len(list) == 0 {
			response = "📭 Aucun membre enregistré. Tapez `.je-suis [Nom]` !"
		} else {
			style := ResponseStyle{
				Title:      "Membres Enregistrés",
				TitleEmoji: "👥",
				Sections:   []Section{{Items: list}},
			}
			response = RenderWhatsApp(style)
		}

	// ========================================================================
	// ROLES & REPUTATION
	// ========================================================================

	case "role", "badge":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		parts := strings.Fields(args)
		if len(parts) < 2 || !strings.Contains(parts[0], "@") {
			response = "⚠️ Usage : `.role @user [Badge]`"
			break
		}
		targetJid := extractJid(parts[0], "")
		role := strings.Join(parts[1:], " ")
		err := SaveMemberRole(targetJid, remoteJid, role)
		if err != nil {
			response = "❌ Erreur d'attribution."
		} else {
			_ = LogProfileChange(targetJid, remoteJid, "role_added", "", role, senderJid)
			response = fmt.Sprintf("🛡️ Badge *%s* attribué à @%s.", role, strings.Split(targetJid, "@")[0])
		}

	case "retirer-role":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		parts := strings.Fields(args)
		if len(parts) < 2 || !strings.Contains(parts[0], "@") {
			response = "⚠️ Usage : `.retirer-role @user [Badge]`"
			break
		}
		targetJid := extractJid(parts[0], "")
		role := strings.Join(parts[1:], " ")
		err := RemoveMemberRole(targetJid, remoteJid, role)
		if err != nil {
			response = "❌ Erreur."
		} else {
			_ = LogProfileChange(targetJid, remoteJid, "role_removed", role, "", senderJid)
			response = fmt.Sprintf("🗑️ Badge *%s* retiré à @%s.", role, strings.Split(targetJid, "@")[0])
		}

	case "points":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		parts := strings.Fields(args)
		if len(parts) < 2 || !strings.Contains(parts[0], "@") {
			response = "⚠️ Usage : `.points @user [Nombre]`"
			break
		}
		targetJid := extractJid(parts[0], "")
		delta, err := strconv.Atoi(parts[1])
		if err != nil {
			response = "⚠️ Le nombre de points doit être un entier."
			break
		}
		err = UpdateMemberPoints(targetJid, remoteJid, delta)
		if err != nil {
			response = "❌ Erreur."
		} else {
			newPts, _ := GetMemberPoints(targetJid, remoteJid)
			_ = LogProfileChange(targetJid, remoteJid, "points", "", strconv.Itoa(newPts), senderJid)
			response = fmt.Sprintf("🏆 Réputation de @%s : *%d points* (%+d).", strings.Split(targetJid, "@")[0], newPts, delta)
		}

	case "profil":
		targetJid := extractJid(args, ctx.QuotedSender)
		if targetJid == "" {
			targetJid = senderJid
		}
		
		name := GetMemberName(targetJid, remoteJid, strings.Split(targetJid, "@")[0])
		pts, _ := GetMemberPoints(targetJid, remoteJid)
		roles, _ := GetMemberRoles(targetJid, remoteJid)
		
		// Detailed profile from DB
		profile, _ := GetUserProfile(targetJid)
		
		// Seniority
		memberDetails, _ := GetMemberDetails(targetJid, remoteJid)
		seniorityBadge := "🌱 Nouveau"
		if !memberDetails.CreatedAt.IsZero() {
			days := time.Since(memberDetails.CreatedAt).Hours() / 24
			if days > 30 { seniorityBadge = "💎 Vétéran" } else if days > 7 { seniorityBadge = "🛡️ Habitué" }
		}

		rolesStr := "Membre standard"
		if len(roles) > 0 { rolesStr = strings.Join(roles, ", ") }
		
		style := ResponseStyle{
			Title:      "PROFIL DE " + strings.ToUpper(name),
			TitleEmoji: "👤",
			Sections: []Section{
				{
					Title: "Statut & Identité", TitleEmoji: "🆔",
					KeyValues: []KeyValue{
						{Key: "Nom", Value: name, Emoji: "🏷️"},
						{Key: "Rôle", Value: rolesStr, Emoji: "🛡️"},
						{Key: "Ancienneté", Value: seniorityBadge, Emoji: "✨"},
					},
				},
				{
					Title: "Activité & Réputation", TitleEmoji: "📊",
					KeyValues: []KeyValue{
						{Key: "XP", Value: strconv.Itoa(pts) + " points", Emoji: "🏆"},
						{Key: "Messages", Value: strconv.Itoa(memberDetails.MessageCount), Emoji: "💬"},
					},
				},
			},
			Footer: "Poulga Supreme Edition",
		}
		if profile.Profession != "" {
			style.Sections = append(style.Sections, Section{
				Title: "Détails Bio", TitleEmoji: "💼",
				Content: "Métier : *" + profile.Profession + "*\nNote : " + profile.Facts,
			})
		}
		response = RenderWhatsApp(style)

	case "ouvrir":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		err := setGroupAnnouncement(instance, remoteJid, false)
		if err != nil {
			response = "❌ Erreur."
		} else {
			response = "🔓 Groupe ouvert. Tout le monde peut participer."
		}

	case "fermer":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		err := setGroupAnnouncement(instance, remoteJid, true)
		if err != nil {
			response = "❌ Erreur."
		} else {
			response = "🔒 Groupe fermé. Seuls les admins peuvent parler."
		}

	case "lien":
		if !strings.HasSuffix(remoteJid, "@g.us") {
			response = "❌ Commande de groupe uniquement."
			break
		}
		link, err := getGroupInviteLink(instance, remoteJid)
		if err != nil {
			response = "❌ Impossible de récupérer le lien."
		} else {
			response = "🔗 *Lien d'invitation :*\n" + link
		}

	case "regles":
		if args == "" {
			rules, err := getRules(remoteJid)
			if err != nil || len(rules) == 0 {
				response = "📜 Aucune règle définie. Admin: `.regles add <texte>`"
			} else {
				style := ResponseStyle{
					Title:      "Règles du Groupe",
					TitleEmoji: "📜",
					Sections: []Section{
						{Items: rules},
					},
				}
				response = RenderWhatsApp(style)
			}
		} else {
			parts := strings.SplitN(args, " ", 2)
			subCmd := strings.ToLower(parts[0])
			switch subCmd {
			case "add":
				isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
				if !isAdmin {
					response = "👑 Réservé aux admins."
					break
				}
				if len(parts) < 2 {
					response = "⚠️ Usage: `.regles add <texte>`"
				} else {
					err := addRule(remoteJid, parts[1], senderJid)
					if err == nil {
						response = "✅ Règle ajoutée."
					}
				}
			case "del":
				isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
				if !isAdmin {
					response = "👑 Réservé aux admins."
					break
				}
				if len(parts) < 2 {
					response = "⚠️ Usage: `.regles del <id>`"
				} else {
					id, _ := strconv.Atoi(parts[1])
					deleteRule(remoteJid, id)
					response = "🗑️ Règle supprimée."
				}
			}
		}

	case "persona":
		if args == "" {
			current := GetGroupPersona(remoteJid)
			if current == "" {
				response = "🎭 Aucune personnalité personnalisée. Usage: `.persona <description>`"
			} else {
				response = "🎭 *Personnalité actuelle :*\n" + current
			}
		} else if strings.ToLower(args) == "reset" {
			SetGroupPersona(remoteJid, "")
			response = "🎭 Personnalité réinitialisée."
		} else {
			err := SetGroupPersona(remoteJid, args)
			if err == nil {
				response = "🎭 Personnalité mise à jour."
			}
		}

	case "langue":
		if args == "" {
			lang := GetGroupLanguage(remoteJid)
			response = fmt.Sprintf("🌐 Langue actuelle: *%s*\nUsage: `.langue fr` ou `.langue en`.", lang)
		} else {
			lang := strings.ToLower(strings.TrimSpace(args))
			SetGroupLanguage(remoteJid, lang)
			response = fmt.Sprintf("🌐 Langue changée en *%s*.", lang)
		}

	case "tagall":
		if !strings.HasSuffix(remoteJid, "@g.us") {
			response = "❌ Cette commande ne fonctionne que dans les groupes."
			break
		}
		
		members, err := GetGroupMembersDetailed(remoteJid)
		if err != nil || len(members) == 0 {
			// BEAUTIFUL FALLBACK if DB is empty
			participants, err := getGroupMetadata(instance, remoteJid)
			if err != nil {
				response = "❌ Erreur de récupération des membres."
				break
			}
			var mentions []string
			var sb strings.Builder
			
			msgTitle := "📢 *APPEL GÉNÉRAL*"
			if args != "" {
				msgTitle = "📢 *" + strings.ToUpper(args) + "*"
			}
			
			sb.WriteString(msgTitle + "\n")
			sb.WriteString("━━━━━━━━━━━━━━━\n\n")
			sb.WriteString("✨ *LISTE DES PRÉSENTS*\n")
			
			for _, p := range participants {
				sb.WriteString(fmt.Sprintf("👤 @%s\n", strings.Split(p, "@")[0]))
				mentions = append(mentions, p)
			}
			
			sb.WriteString("\n━━━━━━━━━━━━━━━\n")
			sb.WriteString("_Poulga a réveillé tout le monde._")
			
			_, _ = sendWhatsAppMessageWithMentions(instance, remoteJid, sb.String(), mentions)
			return
		}

		var mentions []string
		var veteranList []string
		var activeList []string
		var newList []string
		
		now := time.Now()
		for _, m := range members {
			mentions = append(mentions, m.Jid)
			
			days := now.Sub(m.CreatedAt).Hours() / 24
			
			// Display number without @s.whatsapp.net
			num := strings.Split(m.Jid, "@")[0]
			entry := fmt.Sprintf("• @%s (%d msgs)", num, m.MessageCount)
			
			if days > 30 {
				veteranList = append(veteranList, "💎 "+entry)
			} else if days > 7 {
				activeList = append(activeList, "🛡️ "+entry)
			} else {
				newList = append(newList, "🆕 "+entry)
			}
		}

		msgTitle := "📢 *CONVOCATION GÉNÉRALE*"
		if args != "" {
			msgTitle = "📢 *" + strings.ToUpper(args) + "*"
		}

		var sb strings.Builder
		sb.WriteString(msgTitle + "\n")
		sb.WriteString("━━━━━━━━━━━━━━━\n\n")

		if len(veteranList) > 0 {
			sb.WriteString("🏆 *LES PILIERS*\n")
			for _, v := range veteranList { sb.WriteString(v + "\n") }
			sb.WriteString("\n")
		}
		
		if len(activeList) > 0 {
			sb.WriteString("🔥 *LES HABITUÉS*\n")
			for _, a := range activeList { sb.WriteString(a + "\n") }
			sb.WriteString("\n")
		}
		
		if len(newList) > 0 {
			sb.WriteString("🌱 *LES NOUVEAUX*\n")
			for _, n := range newList { sb.WriteString(n + "\n") }
			sb.WriteString("\n")
		}

		sb.WriteString("━━━━━━━━━━━━━━━\n")
		sb.WriteString("_Design by Poulga Supreme_")
		
		_, _ = sendWhatsAppMessageWithMentions(instance, remoteJid, sb.String(), mentions)
		return

	case "stats":
		members, err := GetGroupMembersDetailed(remoteJid)
		if err != nil || len(members) == 0 {
			response = "📈 Pas encore assez de données pour ce groupe."
			break
		}
		
		var topList []string
		totalMsgs := 0
		// Sort by message count already done in DB? No, GetGroupMembersDetailed sorts by CreatedAt
		// Let's sort manually here or use a better query
		
		for i, m := range members {
			if i < 7 { // Show top 7
				topList = append(topList, fmt.Sprintf("%d. @%s — *%d msgs*", i+1, strings.Split(m.Jid, "@")[0], m.MessageCount))
			}
			totalMsgs += m.MessageCount
		}
		
		style := ResponseStyle{
			Title: "DASHBOARD DU GROUPE", TitleEmoji: "📊",
			Sections: []Section{
				{Title: "Activité Globale", TitleEmoji: "📈", Content: fmt.Sprintf("Total de messages analysés : *%d*", totalMsgs)},
				{Title: "Top Membres", TitleEmoji: "🔥", Items: topList},
				{Title: "Communauté", TitleEmoji: "👥", Content: fmt.Sprintf("Nombre d'identités suivies : *%d*", len(members))},
			},
		}
		response = RenderWhatsApp(style)

	case "top", "leaderboard":
		rows, err := db.Query(context.Background(), `
			SELECT jid, points FROM member_points 
			WHERE group_jid = $1 ORDER BY points DESC LIMIT 10`, remoteJid)
		if err != nil {
			response = "❌ Erreur de récupération du classement."
			break
		}
		defer rows.Close()

		var items []string
		i := 1
		for rows.Next() {
			var jid string
			var pts int
			if err := rows.Scan(&jid, &pts); err == nil {
				name := GetMemberName(jid, remoteJid, strings.Split(jid, "@")[0])
				medal := "🏅"
				if i == 1 { medal = "🥇" } else if i == 2 { medal = "🥈" } else if i == 3 { medal = "🥉" }
				items = append(items, fmt.Sprintf("%s *%s* : %d pts", medal, name, pts))
				i++
			}
		}

		if len(items) == 0 {
			response = "🏆 Aucun point attribué pour le moment. Jouez pour en gagner !"
		} else {
			style := ResponseStyle{
				Title:      "CLASSEMENT DE RÉPUTATION",
				TitleEmoji: "🏆",
				Sections:   []Section{{Items: items}},
				Footer:     "Gagne des points en jouant aux jeux !",
			}
			response = RenderWhatsApp(style)
		}

	// Alias pour la mémoire
	case "memoire", "faits", "facts":
		facts, _ := GetGroupFacts(remoteJid)
		if len(facts) == 0 {
			response = RenderWhatsApp(ResponseStyle{
				Title:      "Mémoire",
				TitleEmoji: "🧠",
				Sections: []Section{
					{Content: "Ma mémoire est vide pour ce groupe."},
					{Content: "Utilise `.fact <clé> : <valeur>` pour m'apprendre des choses !"},
				},
			})
		} else {
			var list []string
			for _, f := range facts {
				list = append(list, fmt.Sprintf("*%s* → %s", f.Key, f.Value))
			}
			response = RenderWhatsApp(ResponseStyle{
				Title:      "Mémoire du Groupe",
				TitleEmoji: "🧠",
				Sections: []Section{
					{Title: fmt.Sprintf("%d faits retenus", len(list)), TitleEmoji: "📚", Items: list},
				},
			})
		}

	case "fact":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		if args == "" {
			response = "⚠️ Usage : `.fact <clé> : <valeur>` ou `.fact del <clé>`"
			break
		}
		if strings.HasPrefix(strings.ToLower(args), "del ") {
			key := strings.TrimSpace(args[4:])
			_, err := db.Exec(context.Background(), "DELETE FROM group_facts WHERE group_jid = $1 AND key = $2", remoteJid, key)
			if err == nil {
				response = fmt.Sprintf("✅ Fait '%s' oublié.", key)
			}
			break
		}
		parts := strings.SplitN(args, ":", 2)
		if len(parts) < 2 {
			response = "⚠️ Format incorrect. Utilise le colon `:` pour séparer la clé et la valeur."
			break
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		err := SaveGroupFact(remoteJid, key, val)
		if err == nil {
			response = fmt.Sprintf("✅ J'ai mémorisé : *%s*.", key)
		}

	// Le jeu interactif (Morpion)
	case "jeu", "morpion", "tictactoe":
		go handleMorpionGame(ctx, args)
		return

	case "pendu", "hangman":
		go handlePenduGame(ctx, args)
		return

	case "quiz", "quizz":
		go handleQuizGame(ctx, args)
		return

	case "bienvenue":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		val := strings.ToLower(args) == "on"
		updateGroupSetting(remoteJid, "welcome_enabled", val)
		status := "activés"
		if !val {
			status = "désactivés"
		}
		response = fmt.Sprintf("✨ Messages de bienvenue *%s*.", status)

	case "anti-lien":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		val := strings.ToLower(args) == "on"
		updateGroupSetting(remoteJid, "antilink_enabled", val)
		status := "activé"
		if !val {
			status = "désactivé"
		}
		response = fmt.Sprintf("🛡️ Anti-lien *%s*.", status)

	case "outils", "tools":
		style := ResponseStyle{
			Title:      "Capacités Techniques",
			TitleEmoji: "🛠️",
			Sections: []Section{
				{
					Title: "Navigation & Recherche", TitleEmoji: "🌐",
					Items: []string{
						"`.lire [url]` — Analyse complète d'un site web",
						"`.google [recherche]` — Recherche web en temps réel",
					},
				},
				{
					Title: "Mémoire & Organisation", TitleEmoji: "🧠",
					Items: []string{
						"`.fact [clé] : [valeur]` — Apprendre une information au bot",
						"`.note [texte]` — Enregistrer une note personnelle",
						"`.rappel [texte]` — Créer un rappel",
					},
				},
				{
					Title: "Social & Interaction", TitleEmoji: "🎮",
					Items: []string{
						"`.top` — Voir le classement des membres",
						"`.jeu` / `.quiz` / `.pendu` — Lancer un jeu",
						"`.sondage Q?|O1|O2` — Créer un sondage",
						"`.sticker` — Convertir une image citée",
					},
				},
			},
			Footer: "Poulga Intelligent Engine",
		}
		response = RenderWhatsApp(style)

	case "evolve":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		if args == "analyse" || args == "analyze" {
			go handleEvolveAnalyse(ctx)
			return
		}
		
		// List suggestions
		rows, err := db.Query(context.Background(), "SELECT id, suggestion, reason FROM bot_suggestions WHERE group_jid = $1 AND status = 'pending' ORDER BY created_at DESC", remoteJid)
		if err != nil {
			response = "❌ Erreur de lecture des suggestions."
			break
		}
		defer rows.Close()
		
		var items []string
		for rows.Next() {
			var id int
			var sug, reason string
			if err := rows.Scan(&id, &sug, &reason); err == nil {
				items = append(items, fmt.Sprintf("*[%d]* : %s\n_Pourquoi : %s_", id, sug, reason))
			}
		}
		
		if len(items) == 0 {
			response = "💡 *Aucune suggestion pour le moment.*\nUtilise `.evolve analyse` pour que j'étudie le groupe !"
		} else {
			style := ResponseStyle{
				Title: "Suggestions d'Amélioration", TitleEmoji: "💡",
				Sections: []Section{{Items: items}},
				Footer: "Utilise .evolve clear pour tout oublier.",
			}
			response = RenderWhatsApp(style)
		}

	case "aide", "help", "menu":
		style := ResponseStyle{
			Title:      "Menu d'Aide Poulga",
			TitleEmoji: "📋",
			Sections: []Section{
				{
					Title:      "Identité & Profil",
					TitleEmoji: "👥",
					Items:      []string{".aide", ".outils", ".qui-es-tu", ".je-suis <Nom>", ".qui", ".profil", ".stats", ".top"},
				},
				{
					Title:      "Outils & Utilitaires",
					TitleEmoji: "🛠️",
					Items:      []string{".note <texte>", ".rappel <texte>", ".sondage <question>", ".code <langue>", ".tagall"},
				},
				{
					Title:      "Web & Recherche",
					TitleEmoji: "🌐",
					Items:      []string{".lire <url>", ".google <recherche>", ".resume"},
				},
				{
					Title:      "Média & Fun",
					TitleEmoji: "📥",
					Items:      []string{".yt <url>", ".audio <url>", ".sticker", ".jeu", ".pendu", ".quiz"},
				},
				{
					Title:      "Mémoire & Faits",
					TitleEmoji: "🧠",
					Items:      []string{".memoire", ".fact <info>", ".facts"},
				},
				{
					Title:      "Groupe & Admin",
					TitleEmoji: "🛡️",
					Items:      []string{".warn @user", ".warn-list", ".warn-reset @user", ".kick @user", ".ouvrir", ".fermer", ".regles", ".lien"},
				},
			},
		}
		response = RenderWhatsApp(style)

	case "statut-serveur":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		cpuOut, _ := exec.Command("sh", "-c", "top -bn1 | grep 'Cpu(s)' | awk '{print $2}'").Output()
		memOut, _ := exec.Command("sh", "-c", "free -m | awk 'NR==2{printf \"%.1f%% (%d/%d MB)\", $3*100/$2, $3, $2}'").Output()
		uptimeOut, _ := exec.Command("uptime", "-p").Output()
		style := ResponseStyle{
			Title:      "Statut Serveur",
			TitleEmoji: "💻",
			Sections: []Section{
				{
					KeyValues: []KeyValue{
						{Key: "Uptime", Value: strings.TrimSpace(string(uptimeOut)), Emoji: "⏱️"},
						{Key: "CPU", Value: strings.TrimSpace(string(cpuOut)) + "%", Emoji: "🧠"},
						{Key: "RAM", Value: strings.TrimSpace(string(memOut)), Emoji: "💾"},
					},
				},
			},
			Footer: "MorningStar Infrastructure",
		}
		response = RenderWhatsApp(style)

	case "qui-es-tu":
		response = "Je suis *Poulga*, ton associée intelligente. J'analyse, je mémorise et je gère ce groupe. Tape `.aide` pour en savoir plus."

	case "ping":
		response = fmt.Sprintf("Pong ! 🏓 (%s)", time.Now().Format("15:04:05"))

	case "warn":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Commande réservée aux admins."
			break
		}
		targetJid := extractJid(args, ctx.QuotedSender)
		if targetJid == "" {
			response = "⚠️ Usage : `.warn @user`"
			break
		}
		count, _ := addWarning(targetJid, remoteJid)
		response = fmt.Sprintf("⚠️ Avertissement pour @%s (%d/3).", strings.Split(targetJid, "@")[0], count)
		if count >= 3 {
			response += "\nExpulsion en cours..."
			go kickUser(instance, remoteJid, targetJid)
			go resetWarnings(targetJid, remoteJid)
		}

	case "warn-list":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		list, _ := listWarnings(remoteJid)
		response = "*Avertissements :*\n" + list

	case "warn-reset":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		targetJid := extractJid(args, ctx.QuotedSender)
		if targetJid == "" {
			response = "⚠️ Usage : `.warn-reset @user`"
			break
		}
		err := resetWarnings(targetJid, remoteJid)
		if err == nil {
			response = fmt.Sprintf("✅ Avertissements réinitialisés pour @%s.", strings.Split(targetJid, "@")[0])
		}

	case "kick":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		targetJid := extractJid(args, ctx.QuotedSender)
		if targetJid != "" {
			err := kickUser(instance, remoteJid, targetJid)
			if err == nil {
				response = fmt.Sprintf("Bye @%s ! 👋", strings.Split(targetJid, "@")[0])
			}
		}

	case "note":
		if args == "" {
			notes, err := getNotes(senderJid, remoteJid)
			if err != nil || len(notes) == 0 {
				response = "📝 Aucune note. Usage: `.note <texte>`"
			} else {
				response = "📝 *Tes notes :*\n" + strings.Join(notes, "\n")
			}
		} else {
			parts := strings.SplitN(args, " ", 2)
			if parts[0] == "del" && len(parts) > 1 {
				id, _ := strconv.Atoi(parts[1])
				deleteNote(senderJid, remoteJid, id)
				response = "🗑️ Note supprimée."
			} else {
				err := addNote(senderJid, remoteJid, args)
				if err == nil {
					response = "✅ Note enregistrée."
				}
			}
		}

	case "lire", "visit", "web":
		if args == "" {
			style := ResponseStyle{
				Title:      "Lien manquant",
				TitleEmoji: "⚠️",
				Sections: []Section{
					{Content: "Tu dois fournir l'URL d'un site web à analyser.\n👉 Exemple : `.lire https://fr.wikipedia.org/wiki/Go_(langage)`"},
				},
			}
			response = RenderWhatsApp(style)
			break
		}
		targetURL := strings.TrimSpace(args)
		_, _ = sendWhatsAppMessage(instance, remoteJid, "🔍 *Lecture du site en cours...*", "", senderJid)
		content, err := scrapeURL(targetURL)
		if err != nil {
			style := ResponseStyle{
				Title:      "Échec de navigation",
				TitleEmoji: "❌",
				Sections: []Section{
					{Content: fmt.Sprintf("Impossible d'accéder au site web : %v", err)},
				},
			}
			response = RenderWhatsApp(style)
		} else {
			prompt := fmt.Sprintf("Voici le contenu d'un site web que je viens de lire : %s\n\nFais-en une synthèse claire et précise.", content)
			resp, err := callOllamaWithIntent(prompt, IntentSearch, nil)
			if err != nil {
				response = "❌ Erreur de synthèse."
			} else {
				resp = cleanResponse(resp)
				response = RenderWhatsApp(ResponseStyle{
					Title:      "Synthèse de Lecture Web",
					TitleEmoji: "🌐",
					Sections: []Section{
						{
							Content: fmt.Sprintf("✅ *[Poulga a visité et lu avec succès le lien suivant]* :\n🔗 %s", targetURL),
						},
						{
							Title:      "Analyse du Contenu",
							TitleEmoji: "📝",
							Content:    resp,
						},
					},
				})
			}
		}

	case "google", "search_web":
		if args == "" {
			response = "🔎 Usage: `.google <votre recherche>`"
			break
		}
		go handleWebSearchCommand(ctx, args)
		return

	case "rappel":
		if args == "" {
			response = "⏰ Usage: `.rappel <texte>`"
		} else {
			err := addNote(senderJid, remoteJid, "[RAPPEL] "+args)
			if err == nil {
				response = "⏰ Rappel enregistré."
			}
		}

	case "sondage":
		if args == "" {
			response = "📊 Usage: `.sondage Question ? | Opt1 | Opt2`"
		} else {
			parts := strings.Split(args, "|")
			if len(parts) < 3 {
				response = "❌ Il faut au moins une question et 2 options."
			} else {
				question := strings.TrimSpace(parts[0])
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📊 *SONDAGE :* %s\n\n", question))
				emojis := []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣"}
				for i, opt := range parts[1:] {
					if i >= len(emojis) {
						break
					}
					sb.WriteString(fmt.Sprintf("%s %s\n", emojis[i], strings.TrimSpace(opt)))
				}
				sb.WriteString("\n_Réagissez avec le numéro correspondant !_")
				response = sb.String()
			}
		}

	case "annonce":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "👑 Réservé aux admins."
			break
		}
		if args == "" {
			response = "📣 Usage: `.annonce <message>`"
		} else {
			announcement := fmt.Sprintf("📣 *ANNONCE OFFICIELLE*\n\n%s\n\n_Par @%s_", args, strings.Split(senderJid, "@")[0])
			_, _ = sendWhatsAppMessage(instance, remoteJid, announcement, "", senderJid)
			return
		}

	case "recherche":
		if args != "" {
			go handleSearchCommand(ctx, args)
			return
		}
		response = "🔍 Que cherches-tu ?"

	case "resume":
		go handleSummaryCommand(ctx)
		return

	case "code":
		if args != "" {
			go handleCodeCommand(ctx, args)
			return
		}
		response = "💻 Pose ta question technique."

	case "sticker":
		go handleStickerCommand(ctx)
		return

	case "yt", "audio":
		if args != "" {
			go handleDownload(ctx, cmd, args)
			return
		}
		response = "📥 Donne-moi un lien."

	default:
		if cmd != "" {
			response = fmt.Sprintf("Commande `.%s` inconnue.", cmd)
		}
	}

	if response != "" {
		_, _ = sendAndSaveBotMessage(ctx, response, msgId)
	}
}

// ============================================================================
// ASYNC HANDLERS
// ============================================================================

func handleWebSearchCommand(ctx MessageContext, query string) {
	_, _ = sendAndSaveBotMessage(ctx, "🔎 *Recherche sur le web...*", "")

	results, err := WebSearch(query, 5)
	if err != nil {
		fmt.Printf("[SEARCH] Error: %v\n", err)
		_, _ = sendAndSaveBotMessage(ctx, "❌ Erreur lors de la recherche web.", ctx.MsgId)
		return
	}

	if len(results) == 0 {
		_, _ = sendAndSaveBotMessage(ctx, "🔍 Aucun résultat trouvé pour cette recherche.", ctx.MsgId)
		return
	}

	// Format results for WhatsApp
	formatted := FormatSearchResults(query, results)
	
	// Optional: Let LLM synthesize if needed, but the formatted results are often enough.
	// For now, let's send the formatted results directly for speed and reliability.
	_, _ = sendAndSaveBotMessage(ctx, formatted, ctx.MsgId)
}

func handleSearchCommand(ctx MessageContext, query string) {
	sendTypingStatus(ctx.Instance, ctx.RemoteJid)
	prompt := BuildSearchPrompt(ctx, query)
	resp, err := callOllamaWithIntent(prompt, IntentSearch, nil)
	if err == nil {
		_, _ = sendAndSaveBotMessage(ctx, cleanResponse(resp), ctx.MsgId)
	}
}

func handleSummaryCommand(ctx MessageContext) {
	sendTypingStatus(ctx.Instance, ctx.RemoteJid)
	profiles, _ := getMemberProfiles(ctx.RemoteJid)
	history, _ := GetConversationContext(ctx.RemoteJid, 50)
	if len(history) > 0 {
		prompt := BuildSummaryPrompt(ctx, profiles, history)
		resp, err := callOllamaWithIntent(prompt, IntentSummary, nil)
		if err == nil {
			_, _ = sendAndSaveBotMessage(ctx, cleanResponse(resp), ctx.MsgId)
		}
	}
}

func handleCodeCommand(ctx MessageContext, args string) {
	sendTypingStatus(ctx.Instance, ctx.RemoteJid)
	ctx.Text = args
	prompt := BuildCodePrompt(ctx)
	resp, err := callOllamaWithIntent(prompt, IntentCode, nil)
	if err == nil {
		_, _ = sendAndSaveBotMessage(ctx, cleanResponse(resp), ctx.MsgId)
	}
}

func handleStickerCommand(ctx MessageContext) {
	target := ctx.QuotedMsgId
	if target == "" {
		target = ctx.MsgId
	}

	// Notify user we are working on it
	// _, _ = sendAndSaveBotMessage(ctx, "🎨 *Création du sticker...*", ctx.MsgId)

	b64, err := getMediaBase64(ctx.Instance, target)
	if err != nil {
		fmt.Printf("[STICKER] Error fetching media: %v\n", err)
		_, _ = sendAndSaveBotMessage(ctx, "❌ Impossible de récupérer l'image. Assure-toi de bien citer une image ou d'en envoyer une.", ctx.MsgId)
		return
	}

	err = sendSticker(ctx.Instance, ctx.RemoteJid, b64)
	if err != nil {
		fmt.Printf("[STICKER] Error sending sticker: %v\n", err)
		_, _ = sendAndSaveBotMessage(ctx, "❌ Erreur lors de la conversion en sticker.", ctx.MsgId)
	} else {
		botJid := os.Getenv("BOT_JID")
		if botJid == "" {
			botJid = "237620864894@s.whatsapp.net"
		}
		_ = SaveMessage("sticker_"+time.Now().String(), ctx.RemoteJid, botJid, "Poulga", "[Sticker]", true, ctx.MsgId)
	}
}

func handleDownload(ctx MessageContext, cmd, url string) {
	fmt.Printf("[DOWNLOAD] %s | url=%s\n", cmd, url)

	_, _ = sendAndSaveBotMessage(ctx, "⏳ Téléchargement en cours...", ctx.MsgId)

	outputFile := fmt.Sprintf("/tmp/poulga_%d", time.Now().UnixNano())
	var ytdlpArgs []string
	mediaType := "video"

	commonArgs := []string{
		"--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"--geo-bypass",
		"--no-check-certificates",
		"--no-warnings",
		"--max-filesize", "50M",
		"--socket-timeout", "30",
		"--retries", "3",
		"-o", outputFile + ".%(ext)s",
	}

	cookieFile := "/app/cookies.txt"
	if info, err := os.Stat(cookieFile); err == nil && info.Size() > 200 {
		commonArgs = append([]string{"--cookies", cookieFile}, commonArgs...)
	}

	if cmd == "audio" {
		mediaType = "audio"
		ytdlpArgs = append(commonArgs, "--extract-audio", "--audio-format", "mp3", "--audio-quality", "128K")
	} else {
		ytdlpArgs = append(commonArgs, "-f", "best[ext=mp4][filesize<50M]/bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best")
	}
	ytdlpArgs = append(ytdlpArgs, url)

	cmdExec := exec.Command("yt-dlp", ytdlpArgs...)
	output, err := cmdExec.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		fmt.Printf("[DOWNLOAD] Error: %v | Output: %s\n", err, outputStr)

		errorTitle := "Erreur de téléchargement"
		errorContent := "Impossible de traiter ce lien."

		if strings.Contains(outputStr, "Sign in to confirm") {
			errorContent = "YouTube bloque ce serveur (détection anti-bot). Essaie TikTok ou Facebook qui sont plus souples."
		} else if strings.Contains(outputStr, "Video unavailable") {
			errorContent = "La vidéo est privée ou a été supprimée."
		} else if strings.Contains(outputStr, "Unsupported URL") {
			errorContent = "Ce site n'est pas supporté par mon moteur de téléchargement."
		} else if strings.Contains(outputStr, "File is larger") {
			errorContent = "Le fichier dépasse la limite de 50 Mo autorisée par WhatsApp."
		}

		style := ResponseStyle{
			Title:      errorTitle,
			TitleEmoji: "❌",
			Sections: []Section{
				{Content: errorContent},
			},
		}
		_, _ = sendAndSaveBotMessage(ctx, RenderWhatsApp(style), ctx.MsgId)
		return
	}

	matches, _ := filepath.Glob(outputFile + ".*")
	if len(matches) == 0 {
		_, _ = sendAndSaveBotMessage(ctx, "❌ Fichier introuvable.", ctx.MsgId)
		return
	}
	realPath := matches[0]
	defer os.Remove(realPath)

	data, err := os.ReadFile(realPath)
	if err != nil {
		_, _ = sendAndSaveBotMessage(ctx, "❌ Erreur lecture.", ctx.MsgId)
		return
	}

	base64Data := base64.StdEncoding.EncodeToString(data)
	botMsgId, err := sendWhatsAppMedia(ctx.Instance, ctx.RemoteJid, base64Data, filepath.Base(realPath), "", mediaType)
	if err != nil {
		_, _ = sendAndSaveBotMessage(ctx, "❌ Erreur envoi.", ctx.MsgId)
	} else if botMsgId != "" {
		botJid := os.Getenv("BOT_JID")
		if botJid == "" {
			botJid = "237620864894@s.whatsapp.net"
		}
		_ = SaveMessage(botMsgId, ctx.RemoteJid, botJid, "Poulga", "[Média: "+mediaType+"]", true, ctx.MsgId)
	}
}

func handleEvolveAnalyse(ctx MessageContext) {
	_, _ = sendAndSaveBotMessage(ctx, "🧠 *Analyse des dynamiques du groupe en cours...*", "")

	history, _ := GetConversationContext(ctx.RemoteJid, 40)
	historyStr := FormatConversationHistory(history)

	prompt := fmt.Sprintf(`Tu es Poulga, l'IA associée de ce groupe. Analyse l'historique suivant et propose UNE SEULE amélioration concrète pour rendre le groupe plus actif, mieux géré ou plus fun.
	
Historique :
%s

Réponds UNIQUEMENT au format JSON suivant :
{
  "suggestion": "Titre court de l'idée",
  "reason": "Explication courte du pourquoi"
}`, historyStr)

	resp, err := callOllamaWithIntent(prompt, IntentChat, nil)
	if err != nil {
		_, _ = sendAndSaveBotMessage(ctx, "❌ Échec de l'analyse.", ctx.MsgId)
		return
	}

	// Simple JSON extraction
	var sugData struct {
		Suggestion string `json:"suggestion"`
		Reason     string `json:"reason"`
	}
	
	// Find JSON in response
	startIdx := strings.Index(resp, "{")
	endIdx := strings.LastIndex(resp, "}")
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		jsonStr := resp[startIdx : endIdx+1]
		err = json.Unmarshal([]byte(jsonStr), &sugData)
	}

	if err != nil || sugData.Suggestion == "" {
		// Fallback if LLM didn't return perfect JSON
		sugData.Suggestion = "Amélioration des interactions"
		sugData.Reason = cleanResponse(resp)
	}

	err = saveBotSuggestion(ctx.RemoteJid, sugData.Suggestion, sugData.Reason)
	if err != nil {
		_, _ = sendAndSaveBotMessage(ctx, "❌ Erreur lors de l'enregistrement de l'idée.", ctx.MsgId)
	} else {
		style := ResponseStyle{
			Title: "Nouvelle Idée d'Évolution", TitleEmoji: "💡",
			Sections: []Section{
				{Title: sugData.Suggestion, Content: sugData.Reason},
			},
			Footer: "Tape .evolve pour voir toutes les idées.",
		}
		_, _ = sendAndSaveBotMessage(ctx, RenderWhatsApp(style), ctx.MsgId)
	}
}

func saveBotSuggestion(groupJid, suggestion, reason string) error {
	_, err := db.Exec(context.Background(),
		"INSERT INTO bot_suggestions (group_jid, suggestion, reason) VALUES ($1, $2, $3)",
		groupJid, suggestion, reason)
	return err
}

func extractJid(args string, quoted string) string {
	if quoted != "" {
		return quoted
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return ""
	}
	target := strings.TrimPrefix(fields[0], "@")
	if !strings.Contains(target, "@") {
		target += "@s.whatsapp.net"
	}
	return target
}

type MorpionState struct {
	Grid   [3][3]string `json:"grid"`
	Turn   string       `json:"turn"` // "user" ou "bot"
	Active bool         `json:"active"`
}

func handleMorpionGame(ctx MessageContext, args string) {
	key := fmt.Sprintf("game:morpion:%s", ctx.RemoteJid)
	fmt.Printf("[GAME] handleMorpionGame key=%s args=%q\n", key, args)

	// 1. Démarrer une nouvelle partie
	if args == "" || strings.ToLower(args) == "reset" || strings.ToLower(args) == "start" {
		fmt.Printf("[GAME] Starting new game for %s\n", ctx.RemoteJid)
		state := MorpionState{
			Grid:   [3][3]string{{"-", "-", "-"}, {"-", "-", "-"}, {"-", "-", "-"}},
			Turn:   "user",
			Active: true,
		}
		saveMorpionState(key, state)

		style := ResponseStyle{
			Title:      "Morpion (Tic-Tac-Toe)",
			TitleEmoji: "❌",
			Sections: []Section{
				{Content: "🎮 *La partie commence !* Tu joues les *❌* et Poulga joue les *⭕*.\n\nPour jouer, réponds avec les coordonnées de la case (ligne,colonne).\n👉 Exemple : `.jeu 2,2` (pour jouer au centre)"},
				{Content: renderMorpionGrid(state.Grid)},
			},
		}
		_, _ = sendAndSaveBotMessage(ctx, RenderWhatsApp(style), ctx.MsgId)
		return
	}

	// 2. Récupérer la partie en cours
	state, found := getMorpionState(key)
	if !found || !state.Active {
		_, _ = sendAndSaveBotMessage(ctx, "⚠️ *Aucune partie en cours !* Écris `.jeu` ou `.jeu start` pour commencer. 🎮", ctx.MsgId)
		return
	}

	// 3. Parser le coup de l'utilisateur (format: l,c)
	parts := strings.Split(strings.TrimSpace(args), ",")
	if len(parts) != 2 {
		_, _ = sendAndSaveBotMessage(ctx, "⚠️ *Coordonnées invalides !* Utilise le format : `ligne,colonne` (ex: `.jeu 1,3`).", ctx.MsgId)
		return
	}
	r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	c, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || r < 1 || r > 3 || c < 1 || c > 3 {
		_, _ = sendAndSaveBotMessage(ctx, "⚠️ Les lignes et colonnes doivent être comprises entre *1 et 3*.", ctx.MsgId)
		return
	}

	rIdx, cIdx := r - 1, c - 1
	if state.Grid[rIdx][cIdx] != "-" {
		_, _ = sendAndSaveBotMessage(ctx, "⚠️ *Case déjà occupée !* Choisis une autre coordonnée.", ctx.MsgId)
		return
	}

	// Appliquer le coup de l'utilisateur
	state.Grid[rIdx][cIdx] = "X"

	// Vérifier la victoire de l'utilisateur
	if checkMorpionWinner(state.Grid, "X") {
		state.Active = false
		saveMorpionState(key, state)
		style := ResponseStyle{
			Title:      "Morpion — Victoire !",
			TitleEmoji: "🏆",
			Sections: []Section{
				{Content: fmt.Sprintf("🎉 Félicitations @%s, tu as battu Poulga !", strings.Split(ctx.SenderJid, "@")[0])},
				{Content: renderMorpionGrid(state.Grid)},
			},
		}
		_, _ = sendAndSaveBotMessage(ctx, RenderWhatsApp(style), ctx.MsgId)
		return
	}

	// Vérifier si la grille est pleine (Match nul)
	if isMorpionGridFull(state.Grid) {
		state.Active = false
		saveMorpionState(key, state)
		style := ResponseStyle{
			Title:      "Morpion — Match Nul",
			TitleEmoji: "🤝",
			Sections: []Section{
				{Content: "La grille est pleine. Bien joué !"},
				{Content: renderMorpionGrid(state.Grid)},
			},
		}
		_, _ = sendAndSaveBotMessage(ctx, RenderWhatsApp(style), ctx.MsgId)
		return
	}

	// 4. Tour du Bot (Intelligence Artificielle Minimax)
	botRow, botCol := getBestMorpionMove(state.Grid)
	state.Grid[botRow][botCol] = "O"

	// Vérifier la victoire du Bot
	if checkMorpionWinner(state.Grid, "O") {
		state.Active = false
		saveMorpionState(key, state)
		style := ResponseStyle{
			Title:      "Morpion — Poulga l'emporte",
			TitleEmoji: "⭕",
			Sections: []Section{
				{Content: "Désolée, j'ai gagné cette manche ! Mieux vaut de la chance au jeu qu'en amour. 😉"},
				{Content: renderMorpionGrid(state.Grid)},
			},
		}
		_, _ = sendAndSaveBotMessage(ctx, RenderWhatsApp(style), ctx.MsgId)
		return
	}

	saveMorpionState(key, state)
	style := ResponseStyle{
		Title:      "Morpion (À toi de jouer)",
		TitleEmoji: "❌",
		Sections: []Section{
			{Content: fmt.Sprintf("Poulga a joué en *%d,%d*. C'est ton tour !", botRow+1, botCol+1)},
			{Content: renderMorpionGrid(state.Grid)},
		},
	}
	_, _ = sendAndSaveBotMessage(ctx, RenderWhatsApp(style), ctx.MsgId)
}

// Helpers Morpion
func renderMorpionGrid(grid [3][3]string) string {
	var sb strings.Builder
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			switch grid[r][c] {
			case "X":
				sb.WriteString("❌")
			case "O":
				sb.WriteString("⭕")
			default:
				sb.WriteString("⬜")
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func checkMorpionWinner(g [3][3]string, s string) bool {
	for i := 0; i < 3; i++ {
		if g[i][0] == s && g[i][1] == s && g[i][2] == s {
			return true
		}
		if g[0][i] == s && g[1][i] == s && g[2][i] == s {
			return true
		}
	}
	if g[0][0] == s && g[1][1] == s && g[2][2] == s {
		return true
	}
	if g[0][2] == s && g[1][1] == s && g[2][0] == s {
		return true
	}
	return false
}

func isMorpionGridFull(g [3][3]string) bool {
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			if g[r][c] == "-" {
				return false
			}
		}
	}
	return true
}

// sendAndSaveBotMessage sends a message and persists it to history
func sendAndSaveBotMessage(ctx MessageContext, text string, quotedMsgId string) (string, error) {
	botMsgId, err := sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, text, quotedMsgId, ctx.SenderJid)
	if err == nil && botMsgId != "" {
		botJid := os.Getenv("BOT_JID")
		if botJid == "" {
			botJid = "237620864894@s.whatsapp.net"
		}
		// Save to DB
		_ = SaveMessage(botMsgId, ctx.RemoteJid, botJid, "Poulga", text, true, quotedMsgId)
	}
	return botMsgId, err
}

func getBestMorpionMove(g [3][3]string) (int, int) {
	// Minimax implementation for a perfect AI
	bestScore := -1000
	var move [2]int
	move[0], move[1] = -1, -1

	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			if g[r][c] == "-" {
				g[r][c] = "O"
				score := minimax(g, 0, false)
				g[r][c] = "-"
				if score > bestScore {
					bestScore = score
					move[0], move[1] = r, c
				}
			}
		}
	}

	if move[0] == -1 {
		return 0, 0
	}
	return move[0], move[1]
}

func minimax(g [3][3]string, depth int, isMaximizing bool) int {
	if checkMorpionWinner(g, "O") { return 10 - depth }
	if checkMorpionWinner(g, "X") { return depth - 10 }
	if isMorpionGridFull(g) { return 0 }

	if isMaximizing {
		bestScore := -1000
		for r := 0; r < 3; r++ {
			for c := 0; c < 3; c++ {
				if g[r][c] == "-" {
					g[r][c] = "O"
					score := minimax(g, depth+1, false)
					g[r][c] = "-"
					if score > bestScore { bestScore = score }
				}
			}
		}
		return bestScore
	} else {
		bestScore := 1000
		for r := 0; r < 3; r++ {
			for c := 0; c < 3; c++ {
				if g[r][c] == "-" {
					g[r][c] = "X"
					score := minimax(g, depth+1, true)
					g[r][c] = "-"
					if score < bestScore { bestScore = score }
				}
			}
		}
		return bestScore
	}
}

func saveMorpionState(key string, s MorpionState) {
	data, _ := json.Marshal(s)
	rdb.Set(context.Background(), key, string(data), 15*time.Minute)
}

func getMorpionState(key string) (MorpionState, bool) {
	val, err := rdb.Get(context.Background(), key).Result()
	if err != nil { return MorpionState{}, false }
	var s MorpionState
	json.Unmarshal([]byte(val), &s)
	return s, true
}

type PenduState struct {
	Word      string   `json:"word"`
	Guessed   []string `json:"guessed"`
	Attempts  int      `json:"attempts"`
	Active    bool     `json:"active"`
}

func handlePenduGame(ctx MessageContext, args string) {
	key := fmt.Sprintf("game:pendu:%s", ctx.RemoteJid)
	
	// Start new game
	if args == "" || strings.ToLower(args) == "start" {
		word := PenduWords[rand.Intn(len(PenduWords))]
		state := PenduState{
			Word:     word,
			Guessed:  []string{},
			Attempts: 0,
			Active:   true,
		}
		savePenduState(key, state)
		
		response := "🎮 *Nouveau Pendu démarré !*\n\nMot à deviner :\n`" + renderPenduWord(state) + "`\n\nPropose une lettre avec `.pendu <lettre>`"
		_, _ = sendAndSaveBotMessage(ctx, response, ctx.MsgId)
		return
	}

	state, found := getPenduState(key)
	if !found || !state.Active {
		_, _ = sendAndSaveBotMessage(ctx, "⚠️ Aucune partie de Pendu en cours. Tape `.pendu start`.", ctx.MsgId)
		return
	}

	// Process guess
	guess := strings.ToUpper(strings.TrimSpace(args))
	if len(guess) != 1 {
		_, _ = sendAndSaveBotMessage(ctx, "⚠️ Propose une seule lettre !", ctx.MsgId)
		return
	}

	// Check if already guessed
	for _, l := range state.Guessed {
		if l == guess {
			_, _ = sendAndSaveBotMessage(ctx, "⚠️ Tu as déjà proposé la lettre "+guess+".", ctx.MsgId)
			return
		}
	}

	state.Guessed = append(state.Guessed, guess)
	
	// Check if correct
	foundInWord := false
	for i := 0; i < len(state.Word); i++ {
		if string(state.Word[i]) == guess {
			foundInWord = true
			break
		}
	}

	if !foundInWord {
		state.Attempts++
	}

	// Check win/loss
	rendered := renderPenduWord(state)
	if !strings.Contains(rendered, "_") {
		state.Active = false
		savePenduState(key, state)
		msg := "🎉 *VICTOIRE !* Vous avez trouvé le mot : *" + state.Word + "*\n\nRécompense : *+5 points* pour tout le groupe !"
		_ = UpdateMemberPoints(ctx.SenderJid, ctx.RemoteJid, 5)
		_, _ = sendAndSaveBotMessage(ctx, msg, ctx.MsgId)
		return
	}

	if state.Attempts >= 7 {
		state.Active = false
		savePenduState(key, state)
		msg := "💀 *PERDU !* Le mot était : *" + state.Word + "*\nDommage, réessayez !"
		_, _ = sendAndSaveBotMessage(ctx, msg, ctx.MsgId)
		return
	}

	savePenduState(key, state)
	status := fmt.Sprintf("🎮 *Pendu* (%d/7 erreurs)\n\n`%s`\n\nLettres : %s", state.Attempts, rendered, strings.Join(state.Guessed, ", "))
	_, _ = sendAndSaveBotMessage(ctx, status, ctx.MsgId)
}

func renderPenduWord(s PenduState) string {
	res := ""
	for i := 0; i < len(s.Word); i++ {
		char := string(s.Word[i])
		found := false
		for _, g := range s.Guessed {
			if g == char {
				found = true
				break
			}
		}
		if found {
			res += char + " "
		} else {
			res += "_ "
		}
	}
	return strings.TrimSpace(res)
}

func savePenduState(key string, s PenduState) {
	data, _ := json.Marshal(s)
	rdb.Set(context.Background(), key, string(data), 15*time.Minute)
}

func getPenduState(key string) (PenduState, bool) {
	val, err := rdb.Get(context.Background(), key).Result()
	if err != nil { return PenduState{}, false }
	var s PenduState
	json.Unmarshal([]byte(val), &s)
	return s, true
}

type QuizState struct {
	QuestionIndex int  `json:"question_index"`
	Active        bool `json:"active"`
}

func handleQuizGame(ctx MessageContext, args string) {
	key := fmt.Sprintf("game:quiz:%s", ctx.RemoteJid)
	
	if args == "" || strings.ToLower(args) == "start" {
		idx := rand.Intn(len(QuizQuestions))
		q := QuizQuestions[idx]
		state := QuizState{
			QuestionIndex: idx,
			Active:        true,
		}
		saveQuizState(key, state)
		
		var sb strings.Builder
		sb.WriteString("❓ *QUIZ POULGA*\n\n")
		sb.WriteString("*" + q.Question + "*\n\n")
		for i, opt := range q.Options {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, opt))
		}
		sb.WriteString("\nRéponds avec le numéro du bon choix !")
		_, _ = sendAndSaveBotMessage(ctx, sb.String(), ctx.MsgId)
		return
	}

	state, found := getQuizState(key)
	if !found || !state.Active {
		_, _ = sendAndSaveBotMessage(ctx, "⚠️ Aucun Quiz en cours. Tape `.quiz start`.", ctx.MsgId)
		return
	}

	// Process answer
	choice, err := strconv.Atoi(strings.TrimSpace(args))
	if err != nil {
		_, _ = sendAndSaveBotMessage(ctx, "⚠️ Réponds avec un numéro (ex: `2`).", ctx.MsgId)
		return
	}

	q := QuizQuestions[state.QuestionIndex]
	if choice-1 == q.Answer {
		state.Active = false
		saveQuizState(key, state)
		msg := fmt.Sprintf("🎉 *BRAVO !* C'est la bonne réponse.\n\nRécompense : *+%d points* pour @%s !", q.XP, strings.Split(ctx.SenderJid, "@")[0])
		_ = UpdateMemberPoints(ctx.SenderJid, ctx.RemoteJid, q.XP)
		_, _ = sendAndSaveBotMessage(ctx, msg, ctx.MsgId)
	} else {
		msg := "❌ *DOMMAGE !* Ce n'est pas la bonne réponse. Réessaie !"
		_, _ = sendAndSaveBotMessage(ctx, msg, ctx.MsgId)
	}
}

func saveQuizState(key string, s QuizState) {
	data, _ := json.Marshal(s)
	rdb.Set(context.Background(), key, string(data), 10*time.Minute)
}

func getQuizState(key string) (QuizState, bool) {
	val, err := rdb.Get(context.Background(), key).Result()
	if err != nil { return QuizState{}, false }
	var s QuizState
	json.Unmarshal([]byte(val), &s)
	return s, true
}
