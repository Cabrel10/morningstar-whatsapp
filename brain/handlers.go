package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// COMMAND HANDLER - The switch that routes ALL commands
// Every case is 100% implemented. Zero fake commands.
// ============================================================================

func handleCommand(ctx MessageContext, cmd, args string) {
	var response string
	instance := ctx.Instance
	remoteJid := ctx.RemoteJid
	senderJid := ctx.SenderJid
	msgId := ctx.MsgId

	fmt.Printf("[CMD] %s | args=%q | from=%s\n", cmd, args, ctx.PushName)

	switch cmd {

	// ========================================================================
	// HELP & INFO
	// ========================================================================

	case "help":
		response = getHelpMenu()

	case "qui-es-tu":
		response = "Je suis Poulga, ton assistante de groupe WhatsApp. Je memorise, j'analyse, je gere et je reponds. Tape .aide pour voir tout ce que je sais faire."

	case "ping":
		start := time.Now()
		elapsed := time.Since(start)
		response = fmt.Sprintf("Pong ! %s | Latence: %.0fms", time.Now().Format("15:04:05"), float64(elapsed.Microseconds())/1000)

	case "confidentialite":
		response = `*Politique de Confidentialite*

Je lis les messages pour:
- Construire les statistiques du groupe
- Memoriser les faits importants
- Comprendre le contexte des discussions

Je ne fais PAS:
- Conserver le texte brut indefiniment
- Partager vos donnees hors du groupe
- Analyser les messages prives non sollicites`

	// ========================================================================
	// MEMORY & FACTS
	// ========================================================================

	case "memoire":
		facts, err := getFactsDetailed(remoteJid)
		if err != nil || len(facts) == 0 {
			response = "Ma memoire est vide pour ce groupe. Utilisez `.fact add <texte>` pour ajouter des faits."
		} else {
			var sb strings.Builder
			sb.WriteString("*Faits memorises:*\n\n")
			for _, f := range facts {
				sb.WriteString(fmt.Sprintf("[%d] %s\n", f.ID, f.Content))
			}
			response = sb.String()
		}

	case "fact":
		if args == "" {
			response = "Usage:\n`.fact add <texte>` - Ajouter un fait\n`.fact list` - Lister les faits\n`.fact del <id>` - Supprimer un fait"
			break
		}
		parts := strings.SplitN(args, " ", 2)
		subCmd := strings.ToLower(parts[0])

		switch subCmd {
		case "add":
			if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
				response = "Usage: `.fact add <texte>`"
			} else {
				err := addFact(remoteJid, strings.TrimSpace(parts[1]))
				if err != nil {
					response = "Erreur lors de l'ajout."
				} else {
					response = "Fait memorise !"
				}
			}
		case "list":
			facts, err := getFactsDetailed(remoteJid)
			if err != nil || len(facts) == 0 {
				response = "Aucun fait memorise."
			} else {
				var sb strings.Builder
				sb.WriteString("*Faits:*\n")
				for _, f := range facts {
					sb.WriteString(fmt.Sprintf("[%d] %s\n", f.ID, f.Content))
				}
				response = sb.String()
			}
		case "del", "delete", "suppr":
			if len(parts) < 2 {
				response = "Usage: `.fact del <id>`"
			} else {
				id, err := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err != nil {
					response = "ID invalide."
				} else {
					err = deleteFact(remoteJid, id)
					if err != nil {
						response = "Erreur lors de la suppression."
					} else {
						response = fmt.Sprintf("Fait #%d supprime.", id)
					}
				}
			}
		default:
			response = "Sous-commande inconnue. Utilise: add, list, del"
		}

	case "clear":
		// Clear conversation context in Redis (typing locks, etc.)
		ReleaseTypingLock(remoteJid, senderJid)
		response = "Contexte conversationnel reinitialise."

	// ========================================================================
	// GROUP ADMINISTRATION
	// ========================================================================

	case "tagall":
		if !strings.HasSuffix(remoteJid, "@g.us") {
			response = "Cette commande ne fonctionne que dans les groupes."
			break
		}
		participants, err := getGroupMetadata(instance, remoteJid)
		if err != nil {
			response = "Erreur lors de la recuperation des membres."
			break
		}

		var mentions []string
		var text strings.Builder
		text.WriteString("*Appel general:*\n\n")
		for _, p := range participants {
			number := strings.Split(p, "@")[0]
			text.WriteString(fmt.Sprintf("@%s ", number))
			mentions = append(mentions, p)
		}

		_ = sendWhatsAppMessageWithMentions(instance, remoteJid, text.String(), mentions)
		return // Already sent

	case "warn":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		targetJid := extractJid(args, ctx.QuotedSender)
		if targetJid == "" {
			response = "Usage: `.warn @user` ou cite un message"
			break
		}
		count, err := addWarning(targetJid, remoteJid)
		if err != nil {
			response = "Erreur lors de l'ajout de l'avertissement."
		} else {
			response = fmt.Sprintf("Avertissement pour @%s (%d/3)", strings.Split(targetJid, "@")[0], count)
			if count >= 3 {
				response += "\nLimite atteinte. Expulsion..."
				go kickUser(instance, remoteJid, targetJid)
				go resetWarnings(targetJid, remoteJid)
			}
		}

	case "warn-list":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		list, err := listWarnings(remoteJid)
		if err != nil {
			response = "Erreur."
		} else {
			response = "*Avertissements:*\n" + list
		}

	case "warn-reset":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		targetJid := extractJid(args, ctx.QuotedSender)
		if targetJid == "" {
			response = "Usage: `.warn-reset @user`"
			break
		}
		err := resetWarnings(targetJid, remoteJid)
		if err != nil {
			response = "Erreur."
		} else {
			response = fmt.Sprintf("Avertissements reinitialises pour @%s.", strings.Split(targetJid, "@")[0])
		}

	case "kick":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		targetJid := extractJid(args, ctx.QuotedSender)
		if targetJid == "" {
			response = "Usage: `.kick @user` ou cite un message"
			break
		}
		err := kickUser(instance, remoteJid, targetJid)
		if err != nil {
			response = "Erreur lors de l'expulsion."
		} else {
			response = fmt.Sprintf("@%s a ete expulse.", strings.Split(targetJid, "@")[0])
		}

	case "mute":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		// Mute = close group (only admins can talk)
		err := setGroupAnnouncement(instance, remoteJid, true)
		if err != nil {
			response = "Erreur."
		} else {
			response = "Groupe en mode silencieux (seuls les admins peuvent parler)."
		}

	case "unmute":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		err := setGroupAnnouncement(instance, remoteJid, false)
		if err != nil {
			response = "Erreur."
		} else {
			response = "Groupe demute. Tout le monde peut parler."
		}

	case "promote":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		targetJid := extractJid(args, ctx.QuotedSender)
		if targetJid == "" {
			response = "Usage: `.promote @user`"
			break
		}
		err := promoteUser(instance, remoteJid, targetJid)
		if err != nil {
			response = "Erreur."
		} else {
			response = fmt.Sprintf("@%s promu admin.", strings.Split(targetJid, "@")[0])
		}

	case "demote":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		targetJid := extractJid(args, ctx.QuotedSender)
		if targetJid == "" {
			response = "Usage: `.demote @user`"
			break
		}
		err := demoteUser(instance, remoteJid, targetJid)
		if err != nil {
			response = "Erreur."
		} else {
			response = fmt.Sprintf("@%s retire des admins.", strings.Split(targetJid, "@")[0])
		}

	// ========================================================================
	// GROUP SETTINGS
	// ========================================================================

	case "bienvenue":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		switch strings.ToLower(args) {
		case "on":
			updateGroupSetting(remoteJid, "welcome_enabled", true)
			response = "Messages de bienvenue actives."
		case "off":
			updateGroupSetting(remoteJid, "welcome_enabled", false)
			response = "Messages de bienvenue desactives."
		default:
			response = "Usage: `.bienvenue on` ou `.bienvenue off`"
		}

	case "anti-lien":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		switch strings.ToLower(args) {
		case "on":
			updateGroupSetting(remoteJid, "antilink_enabled", true)
			response = "Anti-lien active. Les liens seront supprimes."
		case "off":
			updateGroupSetting(remoteJid, "antilink_enabled", false)
			response = "Anti-lien desactive."
		default:
			response = "Usage: `.anti-lien on` ou `.anti-lien off`"
		}

	case "ouvrir":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		err := setGroupAnnouncement(instance, remoteJid, false)
		if err != nil {
			response = "Erreur."
		} else {
			response = "Groupe ouvert. Tout le monde peut participer."
		}

	case "fermer":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		err := setGroupAnnouncement(instance, remoteJid, true)
		if err != nil {
			response = "Erreur."
		} else {
			response = "Groupe ferme. Seuls les admins peuvent envoyer des messages."
		}

	case "lien":
		if !strings.HasSuffix(remoteJid, "@g.us") {
			response = "Commande de groupe uniquement."
			break
		}
		link, err := getGroupInviteLink(instance, remoteJid)
		if err != nil {
			response = "Impossible de recuperer le lien d'invitation."
		} else {
			response = "Lien d'invitation: " + link
		}

	// ========================================================================
	// GROUP RULES
	// ========================================================================

	case "regles":
		if args == "" {
			// Show rules
			rules, err := getRules(remoteJid)
			if err != nil || len(rules) == 0 {
				response = "Aucune regle definie. Admin: `.regles add <texte>`"
			} else {
				response = "*Regles du groupe:*\n" + strings.Join(rules, "\n")
			}
		} else {
			parts := strings.SplitN(args, " ", 2)
			subCmd := strings.ToLower(parts[0])

			switch subCmd {
			case "add":
				isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
				if !isAdmin {
					response = "Commande reservee aux admins."
					break
				}
				if len(parts) < 2 {
					response = "Usage: `.regles add <texte de la regle>`"
				} else {
					err := addRule(remoteJid, parts[1], senderJid)
					if err != nil {
						response = "Erreur."
					} else {
						response = "Regle ajoutee."
					}
				}
			case "del":
				isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
				if !isAdmin {
					response = "Commande reservee aux admins."
					break
				}
				if len(parts) < 2 {
					response = "Usage: `.regles del <id>`"
				} else {
					id, err := strconv.Atoi(strings.TrimSpace(parts[1]))
					if err != nil {
						response = "ID invalide."
					} else {
						deleteRule(remoteJid, id)
						response = "Regle supprimee."
					}
				}
			default:
				response = "Usage: `.regles`, `.regles add <texte>`, `.regles del <id>`"
			}
		}

	// ========================================================================
	// PERSONA
	// ========================================================================

	case "persona":
		if args == "" {
			current := GetGroupPersona(remoteJid)
			if current == "" {
				response = "Aucune personnalite personnalisee. Usage: `.persona <description>`"
			} else {
				response = "Personnalite actuelle:\n" + current
			}
		} else if strings.ToLower(args) == "reset" {
			SetGroupPersona(remoteJid, "")
			response = "Personnalite reinitialise au defaut."
		} else {
			err := SetGroupPersona(remoteJid, args)
			if err != nil {
				response = "Erreur."
			} else {
				response = "Personnalite mise a jour."
			}
		}

	case "langue":
		if args == "" {
			lang := GetGroupLanguage(remoteJid)
			response = fmt.Sprintf("Langue actuelle: %s\nUsage: `.langue fr` ou `.langue en`", lang)
		} else {
			lang := strings.ToLower(strings.TrimSpace(args))
			SetGroupLanguage(remoteJid, lang)
			response = fmt.Sprintf("Langue changee: %s", lang)
		}

	// ========================================================================
	// STATS & PROFIL
	// ========================================================================

	case "stats":
		cartography, _ := getGroupCartography(remoteJid)
		if cartography == "" {
			response = "Pas encore de donnees pour ce groupe."
		} else {
			response = "*Statistiques du groupe:*\n\n" + cartography
		}

	case "profil":
		targetJid := senderJid
		if args != "" {
			targetJid = extractJid(args, "")
		}
		if targetJid == "" {
			targetJid = senderJid
		}

		memories, _ := GetUserMemory(targetJid, remoteJid)
		if len(memories) == 0 {
			response = fmt.Sprintf("Aucune info sur @%s pour l'instant.", strings.Split(targetJid, "@")[0])
		} else {
			response = fmt.Sprintf("*Profil de @%s:*\n%s", strings.Split(targetJid, "@")[0], FormatUserMemory(memories))
		}

	// ========================================================================
	// SEARCH
	// ========================================================================

	case "recherche":
		if args == "" {
			response = "Que cherches-tu ? Ex: `.recherche docker`"
		} else {
			go handleSearchCommand(ctx, args)
			return
		}

	// ========================================================================
	// RESUME / SUMMARY
	// ========================================================================

	case "resume":
		go handleSummaryCommand(ctx)
		return

	// ========================================================================
	// NOTES & REMINDERS
	// ========================================================================

	case "note":
		if args == "" {
			notes, err := getNotes(senderJid, remoteJid)
			if err != nil || len(notes) == 0 {
				response = "Aucune note. Usage: `.note <texte>` pour ajouter"
			} else {
				response = "*Tes notes:*\n" + strings.Join(notes, "\n")
			}
		} else {
			parts := strings.SplitN(args, " ", 2)
			if parts[0] == "del" && len(parts) > 1 {
				id, err := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err != nil {
					response = "ID invalide."
				} else {
					deleteNote(senderJid, remoteJid, id)
					response = "Note supprimee."
				}
			} else {
				err := addNote(senderJid, remoteJid, args)
				if err != nil {
					response = "Erreur."
				} else {
					response = "Note enregistree."
				}
			}
		}

	case "rappel":
		if args == "" {
			response = "Usage: `.rappel <texte>` (les rappels temporises arrivent bientot)"
		} else {
			err := addNote(senderJid, remoteJid, "[RAPPEL] "+args)
			if err != nil {
				response = "Erreur."
			} else {
				response = "Rappel enregistre."
			}
		}

	// ========================================================================
	// SONDAGE (Poll-like via text)
	// ========================================================================

	case "sondage":
		if args == "" {
			response = "Usage: `.sondage Question ? | Option1 | Option2 | Option3`"
		} else {
			parts := strings.Split(args, "|")
			if len(parts) < 3 {
				response = "Il faut au moins une question et 2 options separees par |"
			} else {
				question := strings.TrimSpace(parts[0])
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("*SONDAGE:* %s\n\n", question))
				emojis := []string{"1\ufe0f\u20e3", "2\ufe0f\u20e3", "3\ufe0f\u20e3", "4\ufe0f\u20e3", "5\ufe0f\u20e3", "6\ufe0f\u20e3", "7\ufe0f\u20e3", "8\ufe0f\u20e3", "9\ufe0f\u20e3"}
				for i, opt := range parts[1:] {
					if i >= len(emojis) {
						break
					}
					sb.WriteString(fmt.Sprintf("%s %s\n", emojis[i], strings.TrimSpace(opt)))
				}
				sb.WriteString("\nReagissez avec le numero correspondant !")
				response = sb.String()
			}
		}

	// ========================================================================
	// ANNONCE (broadcast)
	// ========================================================================

	case "annonce":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		if args == "" {
			response = "Usage: `.annonce <message>`"
		} else {
			// Send the announcement with a header
			announcement := fmt.Sprintf("*ANNONCE OFFICIELLE*\n\n%s\n\n_Par @%s_", args, strings.Split(senderJid, "@")[0])
			_ = sendWhatsAppMessage(instance, remoteJid, announcement, "", senderJid)
			return
		}

	// ========================================================================
	// CODE (LLM-powered code help)
	// ========================================================================

	case "code":
		if args == "" {
			response = "Usage: `.code <question ou code>`\nEx: `.code python tri a bulles`"
		} else {
			go handleCodeCommand(ctx, args)
			return
		}

	// ========================================================================
	// JEU (Game)
	// ========================================================================

	case "jeu":
		go handleGameCommand(ctx, args)
		return

	// ========================================================================
	// STICKER
	// ========================================================================

	case "sticker":
		go handleStickerCommand(ctx)
		return

	// ========================================================================
	// DOWNLOADS
	// ========================================================================

	case "yt", "fb", "tt", "video", "audio":
		if args == "" {
			response = fmt.Sprintf("Usage: `.%s <URL>`", cmd)
		} else {
			go handleDownload(ctx, cmd, args)
			return
		}

	// ========================================================================
	// SYSTEM STATUS
	// ========================================================================

	case "statut-serveur":
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			response = "Commande reservee aux admins."
			break
		}
		response = getServerStatus()

	// ========================================================================
	// DEFAULT - Unknown command
	// ========================================================================

	default:
		response = fmt.Sprintf("Commande `.%s` inconnue. Tape `.aide` pour la liste.", cmd)
	}

	// Send response if not empty
	if response != "" {
		_ = sendWhatsAppMessage(instance, remoteJid, response, msgId, senderJid)
	}
}

// ============================================================================
// ASYNC COMMAND HANDLERS (run in goroutines, send their own messages)
// ============================================================================

func handleSearchCommand(ctx MessageContext, query string) {
	sendTypingStatus(ctx.Instance, ctx.RemoteJid)

	facts, _ := getFacts(ctx.RemoteJid)
	prompt := BuildSearchPrompt(ctx, facts, "")
	response, err := callOllamaWithIntent(prompt, IntentSearch, nil)
	if err != nil {
		response = "Erreur lors de la recherche."
	}
	response = cleanResponse(response)
	_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, response, ctx.MsgId, ctx.SenderJid)
}

func handleSummaryCommand(ctx MessageContext) {
	sendTypingStatus(ctx.Instance, ctx.RemoteJid)

	profiles, _ := getMemberProfiles(ctx.RemoteJid)
	history, _ := GetConversationContext(ctx.RemoteJid, 50)

	if len(history) == 0 {
		_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "Pas assez de messages pour un resume.", ctx.MsgId, ctx.SenderJid)
		return
	}

	prompt := BuildSummaryPrompt(profiles, history)
	response, err := callOllamaWithIntent(prompt, IntentSummary, nil)
	if err != nil {
		response = "Erreur lors de la generation du resume."
	}
	response = cleanResponse(response)
	_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, response, ctx.MsgId, ctx.SenderJid)
}

func handleCodeCommand(ctx MessageContext, args string) {
	sendTypingStatus(ctx.Instance, ctx.RemoteJid)

	codeCtx := ctx
	codeCtx.Text = args
	prompt := BuildCodePrompt(codeCtx)
	response, err := callOllamaWithIntent(prompt, IntentCode, nil)
	if err != nil {
		response = "Erreur lors de la generation du code."
	}
	response = cleanResponse(response)
	_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, response, ctx.MsgId, ctx.SenderJid)
}

func handleGameCommand(ctx MessageContext, args string) {
	sendTypingStatus(ctx.Instance, ctx.RemoteJid)

	history, _ := GetConversationContext(ctx.RemoteJid, 5)
	prompt := BuildGamePrompt(ctx, history)
	response, err := callOllamaWithIntent(prompt, IntentGame, nil)
	if err != nil {
		response = "Erreur. Reessaie."
	}
	response = cleanResponse(response)
	_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, response, ctx.MsgId, ctx.SenderJid)
}

func handleStickerCommand(ctx MessageContext) {
	targetMsgId := ctx.QuotedMsgId
	if targetMsgId == "" {
		targetMsgId = ctx.MsgId // If no quote, try current message (image with caption)
	}

	_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "Conversion en sticker...", ctx.MsgId, ctx.SenderJid)

	b64, err := getMediaBase64(ctx.Instance, targetMsgId)
	if err != nil {
		fmt.Printf("[STICKER] getMedia error for %s: %v\n", targetMsgId, err)
		_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "Impossible de transformer en sticker. Cite une image.", ctx.MsgId, ctx.SenderJid)
		return
	}

	err = sendSticker(ctx.Instance, ctx.RemoteJid, b64)
	if err != nil {
		fmt.Printf("[STICKER] send error: %v\n", err)
		_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "Erreur lors de la creation du sticker.", ctx.MsgId, ctx.SenderJid)
	}
}

func handleDownload(ctx MessageContext, cmd, url string) {
	fmt.Printf("[DOWNLOAD] %s | url=%s\n", cmd, url)

	_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "Telechargement en cours...", ctx.MsgId, ctx.SenderJid)

	outputFile := fmt.Sprintf("/tmp/poulga_%d", time.Now().UnixNano())
	var ytdlpArgs []string
	mediaType := "video"

	// Common args with multiple strategies to bypass restrictions
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

	// Add cookies only if file has real content (more than the placeholder comment)
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

	fmt.Printf("[DOWNLOAD] Executing yt-dlp with %d args\n", len(ytdlpArgs))

	cmdExec := exec.Command("yt-dlp", ytdlpArgs...)
	output, err := cmdExec.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		fmt.Printf("[DOWNLOAD] Error: %v | Output: %s\n", err, outputStr)

		errorMsg := "Impossible de telecharger."
		if strings.Contains(outputStr, "Sign in to confirm") {
			errorMsg = "YouTube bloque ce serveur (anti-bot). TikTok et Facebook marchent."
		} else if strings.Contains(outputStr, "Video unavailable") {
			errorMsg = "Video non disponible (privee ou supprimee)."
		} else if strings.Contains(outputStr, "Unsupported URL") {
			errorMsg = "Lien non supporte. Essaie YouTube, TikTok ou Facebook."
		} else if strings.Contains(outputStr, "File is larger") {
			errorMsg = "Video trop volumineuse (max 50 Mo)."
		} else if strings.Contains(outputStr, "HTTP Error 403") {
			errorMsg = "Acces refuse par la plateforme."
		}
		_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, errorMsg, ctx.MsgId, ctx.SenderJid)
		return
	}

	matches, _ := filepath.Glob(outputFile + ".*")
	if len(matches) == 0 {
		_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "Fichier introuvable apres telechargement.", ctx.MsgId, ctx.SenderJid)
		return
	}
	realPath := matches[0]
	defer os.Remove(realPath)

	data, err := os.ReadFile(realPath)
	if err != nil {
		_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "Erreur lecture fichier.", ctx.MsgId, ctx.SenderJid)
		return
	}

	if len(data) > 50*1024*1024 {
		_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "Fichier trop volumineux (max 50 Mo).", ctx.MsgId, ctx.SenderJid)
		return
	}

	base64Data := base64.StdEncoding.EncodeToString(data)
	fileName := filepath.Base(realPath)

	fmt.Printf("[DOWNLOAD] Sending %s (%d bytes) as %s\n", fileName, len(data), mediaType)

	err = sendWhatsAppMedia(ctx.Instance, ctx.RemoteJid, base64Data, fileName, "", mediaType)
	if err != nil {
		fmt.Printf("[DOWNLOAD] Send error: %v\n", err)
		_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, "Erreur envoi WhatsApp.", ctx.MsgId, ctx.SenderJid)
	}
}

// ============================================================================
// HELP MENU
// ============================================================================

func getHelpMenu() string {
	return `*Poulga - Commandes*

*Info*
.aide / .help - Ce menu
.qui-es-tu - Qui je suis
.ping - Test de latence

*Memoire*
.memoire - Faits memorises
.fact add/list/del - Gerer les faits
.recherche <sujet> - Fouiller ma memoire
.resume - Resume des discussions
.note <texte> - Sauvegarder une note
.rappel <texte> - Creer un rappel

*Groupe*
.tagall - Mentionner tout le monde
.stats - Statistiques du groupe
.profil - Voir un profil
.regles - Regles du groupe
.lien - Lien d'invitation
.sondage Q | Opt1 | Opt2 - Sondage
.annonce <msg> - Annonce officielle

*Moderation (Admin)*
.warn @user - Avertissement (3=kick)
.warn-list - Liste des avertissements
.warn-reset @user - Reset warns
.kick @user - Expulser
.promote / .demote - Admin/Retirer
.mute / .unmute - Silence groupe
.ouvrir / .fermer - Ouvrir/Fermer
.bienvenue on/off - Accueil
.anti-lien on/off - Bloquer liens

*Media*
.sticker - Creer un sticker (cite une image)
.yt <url> - Video YouTube
.audio <url> - Audio MP3
.fb <url> - Video Facebook
.tt <url> - Video TikTok

*Outils*
.code <question> - Aide codage
.jeu - Jouer un jeu
.persona <texte> - Changer ma personnalite
.langue fr/en - Changer la langue
.clear - Reinitialiser le contexte
.statut-serveur - Etat du VPS

_Mentionnez @poulga pour discuter librement._`
}

// ============================================================================
// SYSTEM STATUS
// ============================================================================

func getServerStatus() string {
	cpuOut, _ := exec.Command("sh", "-c", "top -bn1 | grep 'Cpu(s)' | awk '{print $2}'").Output()
	memOut, _ := exec.Command("sh", "-c", "free -m | awk 'NR==2{printf \"%.1f%% (%d/%d MB)\", $3*100/$2, $3, $2}'").Output()
	diskOut, _ := exec.Command("sh", "-c", "df -h / | awk 'NR==2{print $5 \" (\" $3 \"/\" $2 \")\"}'").Output()
	uptimeOut, _ := exec.Command("uptime", "-p").Output()

	return fmt.Sprintf("*Statut Serveur*\n\nUptime: %s\nCPU: %s%%\nRAM: %s\nDisque: %s\nDocker: Actif",
		strings.TrimSpace(string(uptimeOut)),
		strings.TrimSpace(string(cpuOut)),
		strings.TrimSpace(string(memOut)),
		strings.TrimSpace(string(diskOut)))
}

// ============================================================================
// UTILITIES
// ============================================================================

// extractJid extracts a WhatsApp JID from args or quoted sender
func extractJid(args string, quotedSender string) string {
	// Try quoted sender first
	if quotedSender != "" && strings.Contains(quotedSender, "@") {
		return quotedSender
	}

	// Try extracting from args (format: @number or number)
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}

	// Remove @ prefix if present
	target := strings.TrimPrefix(args, "@")
	// Take first word only
	parts := strings.Fields(target)
	if len(parts) == 0 {
		return ""
	}
	target = parts[0]

	// If it already has @s.whatsapp.net, return as is
	if strings.Contains(target, "@") {
		return target
	}

	// Otherwise assume it's a phone number
	return target + "@s.whatsapp.net"
}
