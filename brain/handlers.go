package main

import (
	"context"
	"encoding/base64"
	"fmt"
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
		rolesStr := "Aucun badge"
		if len(roles) > 0 {
			rolesStr = strings.Join(roles, ", ")
		}
		style := ResponseStyle{
			Title:      fmt.Sprintf("Profil de %s", name),
			TitleEmoji: "👤",
			Sections: []Section{
				{
					Title:      "Informations",
					TitleEmoji: "ℹ️",
					Items: []string{
						fmt.Sprintf("Identifiant : @%s", strings.Split(targetJid, "@")[0]),
						fmt.Sprintf("Badges : *%s*", rolesStr),
						fmt.Sprintf("Réputation : *%d points*", pts),
					},
				},
			},
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
		participants, err := getGroupMetadata(instance, remoteJid)
		if err != nil {
			response = "❌ Erreur de récupération des membres."
			break
		}
		var mentions []string
		var sb strings.Builder
		sb.WriteString("📢 *APPEL GÉNÉRAL*\n\n")
		for _, p := range participants {
			sb.WriteString(fmt.Sprintf("@%s ", strings.Split(p, "@")[0]))
			mentions = append(mentions, p)
		}
		if args != "" {
			sb.WriteString("\n\n*Message :* " + args)
		}
		_, _ = sendWhatsAppMessageWithMentions(instance, remoteJid, sb.String(), mentions)
		return

	case "stats":
		cartography, _ := getGroupCartography(remoteJid)
		if cartography == "" {
			response = "📈 Pas encore assez de données pour ce groupe."
		} else {
			style := ResponseStyle{
				Title:      "Statistiques du Groupe",
				TitleEmoji: "📊",
				Sections: []Section{
					{Content: cartography},
				},
			}
			response = RenderWhatsApp(style)
		}

	case "memoire", "faits":
		facts, _ := GetGroupFacts(remoteJid)
		if len(facts) == 0 {
			response = "🧠 Ma mémoire est vide pour ce groupe. Utilise `.fact <clé> : <valeur>` pour m'apprendre des choses !"
		} else {
			var list []string
			for _, f := range facts {
				list = append(list, fmt.Sprintf("*%s* : %s", f.Key, f.Value))
			}
			style := ResponseStyle{
				Title:      "Mémoire du Groupe",
				TitleEmoji: "🧠",
				Sections: []Section{
					{Items: list},
				},
			}
			response = RenderWhatsApp(style)
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

	case "aide", "help", "menu":
		style := ResponseStyle{
			Title:      "Menu d'Aide Poulga",
			TitleEmoji: "📋",
			Sections: []Section{
				{
					Title:      "Identité & Profil",
					TitleEmoji: "👥",
					Items:      []string{".aide", ".qui-es-tu", ".je-suis <Nom>", ".qui", ".profil", ".stats"},
				},
				{
					Title:      "Outils & Utilitaires",
					TitleEmoji: "🛠️",
					Items:      []string{".note <texte>", ".rappel <texte>", ".sondage <question>", ".code <langue>", ".tagall"},
				},
				{
					Title:      "Web & Recherche",
					TitleEmoji: "🌐",
					Items:      []string{".lire <url>", ".google <recherche>", ".recherche <terme>", ".resume"},
				},
				{
					Title:      "Média & Fun",
					TitleEmoji: "📥",
					Items:      []string{".yt <url>", ".audio <url>", ".sticker", ".jeu"},
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
					Items: []string{
						fmt.Sprintf("Uptime : %s", strings.TrimSpace(string(uptimeOut))),
						fmt.Sprintf("CPU : %s%%", strings.TrimSpace(string(cpuOut))),
						fmt.Sprintf("RAM : %s", strings.TrimSpace(string(memOut))),
					},
				},
			},
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
		url := strings.TrimSpace(args)
		_, _ = sendWhatsAppMessage(instance, remoteJid, "🔍 *Lecture du site en cours...*", "", senderJid)
		content, err := scrapeURL(url)
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
				response = cleanResponse(resp)
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
		_, _ = sendWhatsAppMessage(instance, remoteJid, response, msgId, senderJid)
	}
}

// ============================================================================
// ASYNC HANDLERS
// ============================================================================

func handleWebSearchCommand(ctx MessageContext, query string) {
	_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "🔎 *Recherche sur le web...*", "", ctx.SenderJid)

	// We can use a simple Google Search link scraper or a dedicated search API
	// For now, let's use a very basic search results extractor or just provide a summary of what's found
	// Since we don't have a Search API key, we will simulate a search via scraping or just use LLM for synthesis
	
	searchURL := fmt.Sprintf("https://www.google.com/search?q=%s", strings.ReplaceAll(query, " ", "+"))
	content, err := scrapeURL(searchURL)
	if err != nil {
		_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "❌ Erreur de recherche.", ctx.MsgId, ctx.SenderJid)
		return
	}

	prompt := fmt.Sprintf("Voici les résultats d'une recherche Google pour '%s' : %s\n\nSynthétise les informations les plus pertinentes.", query, content)
	resp, err := callOllamaWithIntent(prompt, IntentSearch, nil)
	if err == nil {
		_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, cleanResponse(resp), ctx.MsgId, ctx.SenderJid)
	}
}

func handleSearchCommand(ctx MessageContext, query string) {
	sendTypingStatus(ctx.Instance, ctx.RemoteJid)
	prompt := BuildSearchPrompt(ctx, query)
	resp, err := callOllamaWithIntent(prompt, IntentSearch, nil)
	if err == nil {
		_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, cleanResponse(resp), ctx.MsgId, ctx.SenderJid)
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
			_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, cleanResponse(resp), ctx.MsgId, ctx.SenderJid)
		}
	}
}

func handleCodeCommand(ctx MessageContext, args string) {
	sendTypingStatus(ctx.Instance, ctx.RemoteJid)
	ctx.Text = args
	prompt := BuildCodePrompt(ctx)
	resp, err := callOllamaWithIntent(prompt, IntentCode, nil)
	if err == nil {
		_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, cleanResponse(resp), ctx.MsgId, ctx.SenderJid)
	}
}

func handleStickerCommand(ctx MessageContext) {
	target := ctx.QuotedMsgId
	if target == "" {
		target = ctx.MsgId
	}
	b64, err := getMediaBase64(ctx.Instance, target)
	if err == nil {
		_ = sendSticker(ctx.Instance, ctx.RemoteJid, b64)
	}
}

func handleDownload(ctx MessageContext, cmd, url string) {
	fmt.Printf("[DOWNLOAD] %s | url=%s\n", cmd, url)

	_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "⏳ Téléchargement en cours...", ctx.MsgId, ctx.SenderJid)

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
		"--extractor-args", "youtube:player_client=web,mweb",
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
		_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, RenderWhatsApp(style), ctx.MsgId, ctx.SenderJid)
		return
	}

	matches, _ := filepath.Glob(outputFile + ".*")
	if len(matches) == 0 {
		_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "❌ Fichier introuvable.", ctx.MsgId, ctx.SenderJid)
		return
	}
	realPath := matches[0]
	defer os.Remove(realPath)

	data, err := os.ReadFile(realPath)
	if err != nil {
		_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "❌ Erreur lecture.", ctx.MsgId, ctx.SenderJid)
		return
	}

	base64Data := base64.StdEncoding.EncodeToString(data)
	_, err = sendWhatsAppMedia(ctx.Instance, ctx.RemoteJid, base64Data, filepath.Base(realPath), "", mediaType)
	if err != nil {
		_, _ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "❌ Erreur envoi.", ctx.MsgId, ctx.SenderJid)
	}
}

func extractJid(args string, quoted string) string {
	if quoted != "" {
		return quoted
	}
	target := strings.TrimPrefix(strings.Fields(args)[0], "@")
	if !strings.Contains(target, "@") {
		target += "@s.whatsapp.net"
	}
	return target
}
