package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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
	
	response, err := callOllama(prompt)
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

	text := GetMessageText(data.Message)
	remoteJid := data.Key.RemoteJid
	
	// Update member profile
	go upsertMember(data.Key.Id, remoteJid, data.PushName)
	
	fmt.Printf("[%s] Received message from %s: %s\n", instance, data.PushName, text)

	// Update last message time in Redis
	rdb.Set(context.Background(), "last_msg:"+remoteJid, time.Now().Unix(), 0)

	shouldRespond := false
	if strings.Contains(strings.ToLower(text), "@poulga") {
		shouldRespond = true
	}

	if shouldRespond {
		go respond(instance, remoteJid, text)
	}

	return c.NoContent(http.StatusOK)
}

func respond(instance, remoteJid, userText string) {
	fmt.Printf("[%s] Responding to %s...\n", instance, remoteJid)
	
	// 1. Get history (Reduced to 15 for better balance)
	history, err := getRecentMessages(remoteJid, 15)
	if err != nil {
		fmt.Printf("Error getting history: %v\n", err)
	}
	historyStr := strings.Join(history, "\n")

	// 2. Get facts (Limit to last 5 facts to keep prompt small)
	facts, _ := getFacts(remoteJid)
	if len(facts) > 5 {
		facts = facts[:5]
	}
	factsStr := strings.Join(facts, "\n")
	if factsStr == "" {
		factsStr = "Aucun fait récent."
	}

	// 2b. Get Group Cartography (Pre-calculated profiles)
	cartography, _ := getGroupCartography(remoteJid)

	// 3. Prepare prompt
	prompt := fmt.Sprintf(PersonaPrompt, cartography, factsStr, historyStr)
	
	// 4. Send typing status
	go sendTypingStatus(instance, remoteJid)

	// 5. Call Ollama
	response, err := callOllama(prompt)
	if err != nil {
		fmt.Printf("Error calling Ollama: %v\n", err)
		return
	}

	// 6. Send message
	_ = sendWhatsAppMessage(instance, remoteJid, response)
	
	// 7. Extract new facts (Deferred to a much later time or separate queue)
	go func() {
		time.Sleep(10 * time.Second) // Let system fully cool down
		extractAndSaveFacts(remoteJid, historyStr+"\nUser: "+userText+"\nPoulga: "+response)
	}()
}

func extractAndSaveFacts(remoteJid, conversation string) {
	prompt := fmt.Sprintf(FactExtractionPrompt, conversation)
	response, err := callOllama(prompt)
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
		// Basic cleaning of bullet points
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		
		fmt.Printf("Saving new fact for %s: %s\n", remoteJid, line)
		addFact(remoteJid, line)
	}
}
