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
		return c.String(http.StatusOK, "MorningStar Brain is healthy")
	})

	e.POST("/webhook", handleWebhook)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	e.Logger.Fatal(e.Start(":" + port))
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
	fmt.Printf("[%s] Received message from %s: %s\n", instance, data.PushName, text)

	remoteJid := data.Key.RemoteJid
	
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
	// 1. Get history
	history, err := getRecentMessages(remoteJid, 10)
	if err != nil {
		fmt.Printf("Error getting history: %v\n", err)
	}
	fmt.Printf("Found %d history messages\n", len(history))
	historyStr := strings.Join(history, "\n")

	// 2. Get facts
	facts, err := getFacts(remoteJid)
	if err != nil {
		fmt.Printf("Error getting facts: %v\n", err)
	}
	fmt.Printf("Found %d facts\n", len(facts))
	factsStr := strings.Join(facts, "\n")
	if factsStr == "" {
		factsStr = "Aucun fait connu."
	}

	// 3. Prepare prompt
	prompt := fmt.Sprintf(PersonaPrompt, factsStr, historyStr)
	fmt.Printf("Calling Ollama for response...\n")
	
	// 4. Call Ollama
	response, err := callOllama(prompt)
	if err != nil {
		fmt.Printf("Error calling Ollama: %v\n", err)
		return
	}
	fmt.Printf("Ollama response received: %s\n", response)

	// 5. Send message
	err = sendWhatsAppMessage(instance, remoteJid, response)
	if err != nil {
		fmt.Printf("Error sending message: %v\n", err)
	} else {
		fmt.Printf("Message sent successfully to %s\n", remoteJid)
	}
	
	// 6. Extract new facts
	fmt.Printf("Extracting facts...\n")
	go extractAndSaveFacts(remoteJid, historyStr+"\nUser: "+userText+"\nMorningStar: "+response)
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
