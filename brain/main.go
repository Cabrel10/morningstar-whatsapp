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

	data := payload.Data
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

	shouldRespond := false
	if strings.Contains(strings.ToLower(text), "@poulga") {
		shouldRespond = true
	}

	if shouldRespond {
		jobQueue <- Job{
			Instance:  instance,
			RemoteJid: remoteJid,
			UserText:  text,
		}
	}

	return c.NoContent(http.StatusOK)
}

func processResponse(instance, remoteJid, userText string) {
	fmt.Printf("[%s] Processing response for %s...\n", instance, remoteJid)
	
	// 1. Get history
	history, err := getRecentMessages(remoteJid, 15)
	if err != nil {
		fmt.Printf("Error getting history: %v\n", err)
	}
	
	var historyLines []string
	for _, line := range history {
		if len(line) > 500 {
			line = line[:500] + "..."
		}
		historyLines = append(historyLines, line)
	}
	historyStr := strings.Join(historyLines, "\n")

	// 2. Get facts and media context
	facts, _ := getFacts(remoteJid)
	factsStr := strings.Join(facts, "\n")
	if factsStr == "" {
		factsStr = "Aucun fait ou média récent mémorisé."
	}

	// 2b. Get Group Cartography
	cartography, _ := getGroupCartography(remoteJid)

	// 3. Prepare prompt
	prompt := fmt.Sprintf(PersonaPrompt, cartography, factsStr, historyStr)
	
	// 4. Send typing status
	go sendTypingStatus(instance, remoteJid)

	// 5. Call Ollama
	response, err := callOllama(prompt, nil)
	if err != nil {
		fmt.Printf("Error calling Ollama: %v\n", err)
		return
	}

	// 6. Send message
	_ = sendWhatsAppMessage(instance, remoteJid, response)
	
	// 7. Extract new facts
	go func() {
		time.Sleep(10 * time.Second) // Let system fully cool down
		extractAndSaveFacts(remoteJid, historyStr+"\nUser: "+userText+"\nPoulga: "+response)
	}()
}

func respond(instance, remoteJid, userText string) {
	// Replaced by worker
}

func extractAndSaveFacts(remoteJid, conversation string) {
	prompt := fmt.Sprintf(FactExtractionPrompt, conversation)
	response, err := callOllama(prompt, nil)
	if err != nil {
		fmt.Printf("Error extracting facts: %v\n", err)
		return
	}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.ToUpper(line) == "NONE" {
			continue
		}

		if strings.HasPrefix(line, "PROFILE:") {
			parts := strings.Split(strings.TrimPrefix(line, "PROFILE:"), "|")
			if len(parts) >= 2 {
				name := strings.TrimSpace(parts[0])
				info := strings.TrimSpace(parts[1])
				fmt.Printf("Updating profile for %s in %s: %s\n", name, remoteJid, info)
				updateMemberProfile(remoteJid, name, info, info) // Using same info for both for now
			}
		} else if strings.HasPrefix(line, "FACT:") {
			fact := strings.TrimSpace(strings.TrimPrefix(line, "FACT:"))
			fmt.Printf("Saving new fact for %s: %s\n", remoteJid, fact)
			addFact(remoteJid, fact)
		} else {
			// Fallback for old style or unformatted lines
			line = strings.TrimPrefix(line, "- ")
			line = strings.TrimPrefix(line, "* ")
			addFact(remoteJid, line)
		}
	}
}
