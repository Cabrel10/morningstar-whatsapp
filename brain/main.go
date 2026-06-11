package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var (
	jobQueue = make(chan Job, 100) // Queue for Ollama requests
)

func main() {
	// Retry initialization
	for {
		err := initDB()
		if err == nil {
			fmt.Println("DB initialized successfully")
			break
		}
		fmt.Printf("Retrying DB initialization: %v\n", err)
		time.Sleep(2 * time.Second)
	}

	for {
		err := initRedis()
		if err == nil {
			fmt.Println("Redis initialized successfully")
			break
		}
		fmt.Printf("Retrying Redis initialization: %v\n", err)
		time.Sleep(2 * time.Second)
	}

	// Start Ollama Worker
	go ollamaWorker()

	// Daily Cleanup Task
	go runDailyCleanup()

	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, "Poulga Brain is healthy")
	})

	e.POST("/webhook", handleWebhook)
	e.GET("/summary/weekly", handleWeeklySummary)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	e.Logger.Fatal(e.Start(":" + port))
}

func ollamaWorker() {
	fmt.Println("Ollama Worker started...")
	for job := range jobQueue {
		processResponse(job.Instance, job.RemoteJid, job.UserText, job.QuotedText, job.QuotedSender, job.QuotedMsgId, job.MsgId, job.SenderJid)
	}
}

func runDailyCleanup() {
	for {
		time.Sleep(24 * time.Hour)
		fmt.Println("Running daily database cleanup...")
		cleanupOldMessages(30) // Delete messages older than 30 days
	}
}

func handleWeeklySummary(c echo.Context) error {
	remoteJid := c.QueryParam("remoteJid")
	if remoteJid == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "remoteJid is required"})
	}

	// 1. Get profiles
	profiles, _ := getMemberProfiles(remoteJid)
	
	// 2. Get recent history for summary
	history, _ := getRecentMessages(remoteJid, 100) // n8n can handle more context
	historyStr := strings.Join(history, "\n")

	prompt := fmt.Sprintf("Tu es Poulga. Génère un résumé hebdomadaire bienveillant et intelligent pour ce groupe WhatsApp.\n\nVoici les profils des membres :\n%s\n\nVoici les messages de la semaine :\n%s", profiles, historyStr)
	
	response, err := callOllama(prompt, nil, 0.3)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"summary": response})
}

func handleWebhook(c echo.Context) error {
	fmt.Printf("[WEBHOOK] Received request. Event type: %s\n", c.Request().Header.Get("Content-Type"))
	
	var payload WebhookPayload
	if err := c.Bind(&payload); err != nil {
		fmt.Printf("[WEBHOOK] Bind error: %v\n", err)
		return err
	}

	fmt.Printf("[WEBHOOK] Payload event: %s\n", payload.Event)

	instance := payload.Instance
	if instance == "" {
		instance = os.Getenv("INSTANCE_NAME")
	}

	// 1. GESTION DES ÉVÉNEMENTS DE GROUPE (Welcome/Goodbye)
	if payload.Event == "group-participants.update" {
		var groupUpdate struct {
			Id           string   `json:"id"`
			Participants []string `json:"participants"`
			Action       string   `json:"action"`
		}
		if err := json.Unmarshal(payload.Data, &groupUpdate); err == nil {
			settings, _ := getGroupSettings(groupUpdate.Id)
			for _, p := range groupUpdate.Participants {
				if groupUpdate.Action == "add" && settings.WelcomeEnabled {
					welcomeMsg := fmt.Sprintf("Bienvenue @%s dans le groupe ! 🎉\nJe suis Poulga, votre associée. Tapez .aide pour voir ce que je peux faire.", strings.Split(p, "@")[0])
					go sendWhatsAppMessage(instance, groupUpdate.Id, welcomeMsg, "", p)
				} else if groupUpdate.Action == "remove" {
					goodbyeMsg := fmt.Sprintf("Au revoir @%s 👋. On espère te revoir bientôt !", strings.Split(p, "@")[0])
					go sendWhatsAppMessage(instance, groupUpdate.Id, goodbyeMsg, "", p)
				}
			}
		}
		return c.NoContent(http.StatusOK)
	}

	if payload.Event != "messages.upsert" {
		return c.NoContent(http.StatusOK)
	}

	// Evolution API v2: data can be an object or an array
	var data MessageData
	if err := json.Unmarshal(payload.Data, &data); err != nil {
		// Try unmarshaling as array and taking first element
		var dataArray []MessageData
		if err := json.Unmarshal(payload.Data, &dataArray); err == nil && len(dataArray) > 0 {
			data = dataArray[0]
		} else {
			return c.NoContent(http.StatusOK)
		}
	}

	if data.Key.FromMe {
		return c.NoContent(http.StatusOK)
	}

	remoteJid := data.Key.RemoteJid
	senderJid := data.Key.Participant
	msgId := data.Key.Id
	if senderJid == "" {
		senderJid = remoteJid
	}

	// 0. DÉDOUBLONNAGE
	if IsDuplicateMessage(msgId) {
		fmt.Printf("[DEBUG] DUPLICATE_MESSAGE_IGNORED: %s\n", msgId)
		return c.NoContent(http.StatusOK)
	}

	// 1. Extract content and type
	var m MessageContent
	_ = json.Unmarshal(data.Message, &m)
	text := GetMessageText(data.Message)

	fmt.Printf("[WEBHOOK] Message received: text=%q type=%T pushName=%s jid=%s\n", text, m, data.PushName, senderJid)

	quotedText := ""
	quotedSender := ""
	quotedMsgId := ""

	// Extract ContextInfo from various message types
	var ctxInfo *ContextInfo
	if m.ExtendedText != nil && m.ExtendedText.ContextInfo != nil {
		ctxInfo = m.ExtendedText.ContextInfo
	} else if m.ImageMessage != nil && m.ImageMessage.ContextInfo != nil {
		ctxInfo = m.ImageMessage.ContextInfo
	} else if m.VideoMessage != nil && m.VideoMessage.ContextInfo != nil {
		ctxInfo = m.VideoMessage.ContextInfo
	}

	if ctxInfo != nil {
		quotedSender = ctxInfo.Participant
		quotedMsgId = ctxInfo.StanzaId
		if ctxInfo.QuotedMessage != nil {
			quotedText = ctxInfo.QuotedMessage.Conversation
		}
	}

	// 2. Handle Social Graph: Citations/Replies
	if quotedSender != "" {
		go recordInteraction(remoteJid, senderJid, quotedSender, "reply")
	}

	// 3. Handle Reactions
	if m.ReactionMessage != nil {
		targetParticipant := m.ReactionMessage.Key.Participant
		if targetParticipant == "" {
			targetParticipant = m.ReactionMessage.Key.RemoteJid
		}
		go recordInteraction(remoteJid, senderJid, targetParticipant, "reaction")
		return c.NoContent(http.StatusOK)
	}

	// 4. Handle Stickers
	if m.StickerMessage != nil {
		go recordStickerUsage(senderJid, m.StickerMessage.FileSha256)
		return c.NoContent(http.StatusOK)
	}

	// 5. Update member profile
	go upsertMember(senderJid, remoteJid, data.PushName)

	// 6. GESTION DES PARAMÈTRES DE GROUPE (Anti-Lien, etc.)
	settings, _ := getGroupSettings(remoteJid)
	if settings.AntiLinkEnabled && (strings.Contains(text, "http://") || strings.Contains(text, "https://") || strings.Contains(text, "www.")) {
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			fmt.Printf("[SECURITY] Anti-Link triggered for %s\n", senderJid)
			// Serait bien d'avoir une fonction deleteMessage
			evoURL := os.Getenv("EVOLUTION_URL")
			if evoURL == "" { evoURL = "http://evolution-api:8080" }
			apiKey := os.Getenv("AUTHENTICATION_API_KEY")
			client := resty.New()
			_, _ = client.R().
				SetHeader("apikey", apiKey).
				SetBody(map[string]interface{}{
					"key": map[string]interface{}{
						"remoteJid": remoteJid,
						"fromMe":    false,
						"id":        msgId,
					},
				}).
				Post(fmt.Sprintf("%s/message/delete/%s", evoURL, instance))
			
			warnMsg := fmt.Sprintf("🚫 @%s, les liens ne sont pas autorisés dans ce groupe.", strings.Split(senderJid, "@")[0])
			go sendWhatsAppMessage(instance, remoteJid, warnMsg, "", senderJid)
			return c.NoContent(http.StatusOK)
		}
	}
	
	fmt.Printf("[%s] Received message from %s (%s): %s\n", instance, data.PushName, senderJid, text)

	// Update last message time in Redis
	rdb.Set(c.Request().Context(), "last_msg:"+remoteJid, time.Now().Unix(), 0)

	// NETTOYER LE TEXTE
	cleanText := strings.TrimSpace(text)
	fmt.Printf("[DEBUG] CLEAN_TEXT_BEFORE=%s\n", cleanText)
	
	// Retirer @poulga de manière insensible à la casse
	isMentioned := false
	lowerText := strings.ToLower(cleanText)
	if strings.Contains(lowerText, "@poulga") {
		isMentioned = true
		idx := strings.Index(lowerText, "@poulga")
		cleanText = strings.TrimSpace(cleanText[:idx] + cleanText[idx+len("@poulga"):])
	}
	fmt.Printf("[DEBUG] CLEAN_TEXT_AFTER=%s IS_MENTIONED=%v\n", cleanText, isMentioned)

	// VÉRIFIER LES COMMANDES D'ABORD (Bypass LLM)
	cmd, cmdArgs, isCmd := IsCommand(cleanText)
	if !isCmd && isMentioned && cleanText != "" {
		// If mentioned and not a '.' command, check if the first word is a command
		parts := strings.Fields(cleanText)
		if len(parts) > 0 {
			cmd = parts[0]
			cmdArgs = strings.TrimSpace(cleanText[len(cmd):])
			isCmd = true // Treat as command
		}
	}

	if isCmd {
		fmt.Printf("[DEBUG] EXECUTING_COMMAND=%s ARGS=%s\n", cmd, cmdArgs)
		go handleCommand(instance, remoteJid, cmd, cmdArgs, msgId, senderJid, quotedMsgId)
		return c.NoContent(http.StatusOK)
	}

	// SI TEXTE VIDE APRÈS MENTION (Bypass LLM)
	if isMentioned && cleanText == "" {
		fmt.Printf("[DEBUG] MENTION_ONLY_DETECTED\n")
		go sendWhatsAppMessage(instance, remoteJid, "Oui ? 😊 Dis-moi ce que tu veux !", msgId, senderJid)
		return c.NoContent(http.StatusOK)
	}

	// LOGIQUE DE DÉCLENCHEMENT (Privé / Mention / Reply)
	botJid := os.Getenv("BOT_JID")
	if botJid == "" {
		botJid = "237620864894@s.whatsapp.net"
	}
	
	isPrivateChat := !strings.HasSuffix(remoteJid, "@g.us")
	isReplyToBot := false

	// Vérifier si on répond à l'un des messages de Poulga
	if ctxInfo != nil && ctxInfo.Participant == botJid {
		isReplyToBot = true
	}

	// DÉCISION FINALE
	shouldRespond := false
	if isPrivateChat || isMentioned || isReplyToBot {
		shouldRespond = true
	}

	if shouldRespond {
		jobType := "text"
		if m.AudioMessage != nil {
			jobType = "audio"
		} else if m.ImageMessage != nil {
			jobType = "image"
		}
		
		jobQueue <- Job{
			Instance:     instance,
			RemoteJid:    remoteJid,
			UserText:     cleanText, // On passe le texte propre au LLM
			QuotedText:   quotedText,
			QuotedSender: quotedSender,
			QuotedMsgId:  quotedMsgId,
			MsgId:        msgId,
			SenderJid:    senderJid,
			Type:         jobType,
		}
	}

	return c.NoContent(http.StatusOK)
}

func processResponse(instance, remoteJid, userText, quotedText, quotedSender, quotedMsgId, msgId, senderJid string) {
	fmt.Printf("[%s] Processing response for %s...\n", instance, remoteJid)
	fmt.Printf("[DEBUG] ROUTER_INPUT=%s QUOTED=%s ID=%s\n", userText, quotedText, quotedMsgId)
	start := time.Now()

	// 1. ÉTAPE 1 : Réponse rapide codée (< 5ms)
	if fastReply, ok := IsFastReply(userText); ok {
		elapsed := time.Since(start)
		fmt.Printf("[TIMING] FAST_REPLY: %.0fms\n", elapsed.Seconds()*1000)
		_ = sendWhatsAppMessage(instance, remoteJid, fastReply, msgId, senderJid)
		return
	}

	// 2. ÉTAPE 2 : Détection d'intention
	intentStart := time.Now()
	intent := DetectIntent(userText)
	intentTime := time.Since(intentStart)
	fmt.Printf("[TIMING] INTENT: %.1fms (detected: %s)\n", intentTime.Seconds()*1000, intent)

	go sendTypingStatus(instance, remoteJid)

	// 3. ÉTAPE 3 : Routing par intention
	switch intent {
	case IntentGame:
		handleGame(instance, remoteJid, userText, msgId, senderJid, start)

	case IntentSummary:
		handleSummary(instance, remoteJid, msgId, senderJid, start)

	case IntentSearch:
		handleSearch(instance, remoteJid, userText, msgId, senderJid, start)

	case IntentGreeting:
		// Salutations : réponse légère avec température modérée (0.5)
		promptGreeting := fmt.Sprintf(`Tu es Poulga, une amie chaleureuse et partenaire du groupe. Réponds très brièvement et de façon amicale à ce message.

Message de l'utilisateur : %s

Poulga :`, userText)

		ollamaStart := time.Now()
		response, _ := callOllama(promptGreeting, nil, 0.5)
		ollamaTime := time.Since(ollamaStart)
		fmt.Printf("[TIMING] OLLAMA_GREETING: %.1fms\n", ollamaTime.Seconds()*1000)
		fmt.Printf("[TIMING] TOTAL: %.1fms\n", time.Since(start).Seconds()*1000)

		response = cleanResponse(response)
		_ = sendWhatsAppMessage(instance, remoteJid, response, msgId, senderJid)

	default: // IntentChat
		fmt.Printf("[DEBUG] ROUTER_FALLBACK_TO_OLLAMA INTENT=chat\n")
		// Chat normal - Version équilibrée
		history, _ := getRecentMessages(remoteJid, 10)
		facts, _ := getFacts(remoteJid)
		if len(facts) > 3 {
			facts = facts[:3]
		}

		historyStr := strings.Join(history, "\n")
		factsStr := strings.Join(facts, "\n")
		if factsStr == "" {
			factsStr = "Aucun fait particulier."
		}

		// Préparer le contexte de citation
		userMessage := userText
		if quotedText != "" {
			userMessage = fmt.Sprintf("[En réponse à @%s qui disait: %s] %s", strings.Split(quotedSender, "@")[0], quotedText, userText)
		}

		// Vérifier si un persona personnalisé existe
		customPersona := GetGroupPersona(remoteJid)
		var prompt string
		if customPersona != "" {
			prompt = fmt.Sprintf("%s\n\nDiscussion :\n%s\n\nUtilisateur : %s\nRéponse directe :", customPersona, historyStr, userMessage)
		} else {
			prompt = fmt.Sprintf(PersonaPrompt, factsStr, historyStr, userMessage)
		}
		
		ollamaStart := time.Now()
		response, _ := callOllama(prompt, nil, 0.95)
		ollamaTime := time.Since(ollamaStart)
		fmt.Printf("[TIMING] OLLAMA_CHAT: %.1fms\n", ollamaTime.Seconds()*1000)
		
		response = cleanResponse(response)
		_ = sendWhatsAppMessage(instance, remoteJid, response, msgId, senderJid)
	}
}

func extractAndSaveFacts(remoteJid, conversation string) {
	// Désactivée pour préserver le CPU
}
