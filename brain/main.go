package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

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
		processResponse(job.Instance, job.RemoteJid, job.UserText)
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
	
	response, err := callOllama(prompt, nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"summary": response})
}

func handleWebhook(c echo.Context) error {
	var payload WebhookPayload
	if err := c.Bind(&payload); err != nil {
		return err
	}

	if payload.Event != "messages.upsert" {
		return c.NoContent(http.StatusOK)
	}

	instance := payload.Instance
	if instance == "" {
		instance = os.Getenv("INSTANCE_NAME")
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
	if senderJid == "" {
		senderJid = remoteJid
	}

	// 1. Extract content and type
	var m MessageContent
	_ = json.Unmarshal(data.Message, &m)
	text := GetMessageText(data.Message)

	// 2. Handle Social Graph: Citations/Replies
	if m.ExtendedText != nil && m.ExtendedText.ContextInfo != nil {
		quotedParticipant := m.ExtendedText.ContextInfo.Participant
		if quotedParticipant != "" {
			go recordInteraction(remoteJid, senderJid, quotedParticipant, "reply")
		}
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
	
	fmt.Printf("[%s] Received message from %s (%s): %s\n", instance, data.PushName, senderJid, text)

	// Update last message time in Redis
	rdb.Set(context.Background(), "last_msg:"+remoteJid, time.Now().Unix(), 0)

	// VÉRIFIER LES COMMANDES D'ABORD
	if cmd, args, isCmd := IsCommand(text); isCmd {
		go handleCommand(instance, remoteJid, cmd, args)
		return c.NoContent(http.StatusOK)
	}

	// LOGIQUE DE DÉCLENCHEMENT (Privé / Mention / Reply)
	botJid := "237620864894@s.whatsapp.net"
	
	isPrivateChat := !strings.HasSuffix(remoteJid, "@g.us")
	isMentioned := strings.Contains(strings.ToLower(text), "@poulga")
	isReplyToBot := false

	// Vérifier si on répond à l'un des messages de Poulga
	if m.ExtendedText != nil && m.ExtendedText.ContextInfo != nil {
		if m.ExtendedText.ContextInfo.Participant == botJid {
			isReplyToBot = true
		}
	}

	// DÉCISION FINALE : Répondre si privé OU mentionné OU réponse au bot
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
			Instance:  instance,
			RemoteJid: remoteJid,
			UserText:  text,
			Type:      jobType,
		}
	}

	return c.NoContent(http.StatusOK)
}

func processResponse(instance, remoteJid, userText string) {
	fmt.Printf("[%s] Processing response for %s...\n", instance, remoteJid)
	start := time.Now()

	// ÉTAPE 1 : Réponse rapide codée (< 5ms)
	if fastReply, ok := IsFastReply(userText); ok {
		elapsed := time.Since(start)
		fmt.Printf("[TIMING] FAST_REPLY: %.0fms\n", elapsed.Seconds()*1000)
		_ = sendWhatsAppMessage(instance, remoteJid, fastReply)
		return
	}

	// ÉTAPE 2 : Détection d'intention
	intentStart := time.Now()
	intent := DetectIntent(userText)
	intentTime := time.Since(intentStart)
	fmt.Printf("[TIMING] INTENT: %.1fms (detected: %s)\n", intentTime.Seconds()*1000, intent)

	go sendTypingStatus(instance, remoteJid)

	// ÉTAPE 3 : Routing par intention
	switch intent {
	case IntentGame:
		handleGame(instance, remoteJid, userText, start)

	case IntentSummary:
		handleSummary(instance, remoteJid, start)

	case IntentSearch:
		handleSearch(instance, remoteJid, userText, start)

	case IntentGreeting:
		// Salutations : réponse TRÈS légère sans historique
		userTextClean := strings.TrimSpace(userText)
		userTextClean = strings.ReplaceAll(strings.ToLower(userTextClean), "@poulga", "")
		userTextClean = strings.TrimSpace(userTextClean)

		promptGreeting := fmt.Sprintf(`Tu es Poulga, une amie chaleureuse. Réponds très brièvement et de façon amicale à ce message. Ne te présente pas.

Message de l'utilisateur : %s

Poulga :`, userTextClean)

		ollamaStart := time.Now()
		response, _ := callOllama(promptGreeting, nil)
		ollamaTime := time.Since(ollamaStart)
		fmt.Printf("[TIMING] OLLAMA_GREETING: %.1fms\n", ollamaTime.Seconds()*1000)
		fmt.Printf("[TIMING] TOTAL: %.1fms\n", time.Since(start).Seconds()*1000)

		response = cleanResponse(response)
		_ = sendWhatsAppMessage(instance, remoteJid, response)

	default: // IntentChat
		// Chat normal : contexte minimal
		pgStart := time.Now()
		history, _ := getRecentMessages(remoteJid, 5)
		facts, _ := getFacts(remoteJid)
		cartography, _ := getGroupCartography(remoteJid)
		pgTime := time.Since(pgStart)
		fmt.Printf("[TIMING] DB_CHAT: %.1fms\n", pgTime.Seconds()*1000)

		historyStr := strings.Join(history, "\n")
		factsStr := strings.Join(facts, "\n")
		if factsStr == "" {
			factsStr = "(Aucun fait)"
		}

		// Vérifier si un persona personnalisé existe
		customPersona := GetGroupPersona(remoteJid)
		var prompt string
		if customPersona != "" {
			prompt = fmt.Sprintf("%s\n\nHistorique :\n%s\n\nFaits :\n%s\n\nRéponds :", customPersona, historyStr, factsStr)
		} else {
			prompt = fmt.Sprintf(PersonaPrompt, cartography, factsStr, historyStr)
		}
		fmt.Printf("[DEBUG] PROMPT_START: %.100s...\n", prompt)
		fmt.Printf("[DEBUG] USER_TEXT: %s\n", userText)
		
		ollamaStart := time.Now()
		response, _ := callOllama(prompt, nil)
		ollamaTime := time.Since(ollamaStart)
		fmt.Printf("[TIMING] OLLAMA_CHAT: %.1fms\n", ollamaTime.Seconds()*1000)
		fmt.Printf("[DEBUG] RESPONSE_START: %.100s...\n", response)
		fmt.Printf("[TIMING] TOTAL: %.1fms\n", time.Since(start).Seconds()*1000)
		
		response = cleanResponse(response)
		fmt.Printf("[DEBUG] RESPONSE_AFTER_CLEAN: %s\n", response)
		_ = sendWhatsAppMessage(instance, remoteJid, response)
	}
}

func extractAndSaveFacts(remoteJid, conversation string) {
	// Désactivée pendant la réponse pour économiser CPU
	// Activable ultérieurement si ressources disponibles
}
