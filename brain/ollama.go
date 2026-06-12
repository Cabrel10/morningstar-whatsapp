package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

var (
	semaphore = make(chan struct{}, 2) // Max 2 concurrent Ollama requests
)

// ============================================================================
// OLLAMA CONFIGURATION for Gemma3 on 4vCPU / 8GB RAM
// ============================================================================

// getOllamaOptions returns tuned parameters based on intent
// Temperature varies by task: low for code/facts, medium for chat, higher for stories
func getOllamaOptions(intent Intent) map[string]interface{} {
	base := map[string]interface{}{
		"num_thread":     4,     // Use all 4 vCPUs
		"num_ctx":        2048,  // Reduced from 4096 - saves time
		"top_p":          0.90,
		"repeat_penalty": 1.15,  // Prevent repetition loops
		"num_predict":    512,   // Reduced from 1024 - faster responses
	}

	switch intent {
	case IntentCode:
		base["temperature"] = 0.3   // Very precise for code
		base["num_predict"] = 1024  // Code needs more tokens but not 2048
		base["repeat_penalty"] = 1.1
	case IntentQuestion:
		base["temperature"] = 0.4   // Factual, precise
		base["num_predict"] = 512
	case IntentStory:
		base["temperature"] = 0.75  // Creative but not chaotic
		base["num_predict"] = 1024  // Stories need length but not 2048
		base["repeat_penalty"] = 1.2
	case IntentGame:
		base["temperature"] = 0.5
		base["num_predict"] = 256   // Games are short
	case IntentSummary:
		base["temperature"] = 0.3   // Factual summary
		base["num_predict"] = 512
	case IntentSearch:
		base["temperature"] = 0.3
		base["num_predict"] = 256
	case IntentGreeting:
		base["temperature"] = 0.6
		base["num_predict"] = 128   // Short greetings
	default: // IntentChat
		base["temperature"] = 0.55  // Balanced
		base["num_predict"] = 512
	}

	return base
}

// callOllama sends a prompt to Ollama and returns the response
func callOllama(prompt string, images []string, temperature float64) (string, error) {
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "gemma3:4b"
	}

	client := resty.New()
	client.SetTimeout(60 * time.Second) // 60s max (Ollama peut être lent sur 4vCPU)

	// If temperature is provided explicitly, use it; otherwise default
	if temperature == 0 {
		temperature = 0.55
	}

	options := map[string]interface{}{
		"num_thread":     4,
		"num_ctx":        4096,
		"temperature":    temperature,
		"top_p":          0.90,
		"repeat_penalty": 1.15,
		"num_predict":    1024,
	}

	fmt.Printf("[OLLAMA] Calling %s (temp=%.2f, predict=%v)\n", model, temperature, options["num_predict"])

	var result OllamaResponse
	resp, err := client.R().
		SetBody(OllamaRequest{
			Model:     model,
			Prompt:    prompt,
			Stream:    false,
			KeepAlive: "24h",
			Options:   options,
			Images:    images,
		}).
		SetResult(&result).
		Post(fmt.Sprintf("%s/api/generate", ollamaURL))

	if err != nil {
		fmt.Printf("[OLLAMA] Error: %v\n", err)
		return "", err
	}

	if resp.IsError() {
		fmt.Printf("[OLLAMA] API Error: %s\n", resp.String())
		return "", fmt.Errorf("ollama error: %s", resp.String())
	}

	fmt.Printf("[OLLAMA] Response: %d chars\n", len(result.Response))
	return result.Response, nil
}

// callOllamaWithIntent sends a prompt using intent-specific parameters
func callOllamaWithIntent(prompt string, intent Intent, images []string) (string, error) {
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "gemma3:4b"
	}

	client := resty.New()
	client.SetTimeout(60 * time.Second) // 60s max (Ollama peut être lent sur 4vCPU)

	options := getOllamaOptions(intent)

	fmt.Printf("[OLLAMA] Calling %s for intent=%s (temp=%.2f, predict=%v)\n",
		model, intent, options["temperature"], options["num_predict"])

	var result OllamaResponse
	resp, err := client.R().
		SetBody(OllamaRequest{
			Model:     model,
			Prompt:    prompt,
			Stream:    false,
			KeepAlive: "24h",
			Options:   options,
			Images:    images,
		}).
		SetResult(&result).
		Post(fmt.Sprintf("%s/api/generate", ollamaURL))

	if err != nil {
		fmt.Printf("[OLLAMA] Error: %v\n", err)
		return "", err
	}

	if resp.IsError() {
		fmt.Printf("[OLLAMA] API Error: %s\n", resp.String())
		return "", fmt.Errorf("ollama error: %s", resp.String())
	}

	fmt.Printf("[OLLAMA] Response: %d chars\n", len(result.Response))
	return result.Response, nil
}

// ============================================================================
// RESPONSE CLEANING
// ============================================================================

// cleanResponse removes unwanted preambles and self-introductions
func cleanResponse(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "..."
	}

	// Remove common LLM self-introduction artifacts
	unwantedPrefixes := []string{
		"Bonjour ! Je suis Poulga",
		"Bonjour! Je suis Poulga",
		"Bonjour \u00e0 tous !",
		"Bonjour \u00e0 tous",
		"Je suis Poulga,",
		"Je suis Poulga.",
		"En tant que Poulga,",
		"En tant qu'assistante,",
		"Poulga :",
		"Poulga:",
		"Bien s\u00fbr !",
		"Bien s\u00fbr,",
		"Absolument !",
	}

	for _, prefix := range unwantedPrefixes {
		if strings.HasPrefix(text, prefix) {
			text = strings.TrimSpace(text[len(prefix):])
		}
	}

	// If cleaning emptied the response
	if text == "" {
		return "..."
	}

	// Remove trailing incomplete sentences (cut off by num_predict)
	// If the response doesn't end with punctuation, try to trim to last complete sentence
	if len(text) > 100 {
		lastChar := text[len(text)-1]
		if lastChar != '.' && lastChar != '!' && lastChar != '?' && lastChar != ')' && lastChar != '"' {
			// Find last sentence-ending punctuation
			lastDot := strings.LastIndexAny(text, ".!?")
			if lastDot > len(text)/2 { // Only trim if we keep at least half
				text = text[:lastDot+1]
			}
		}
	}

	return text
}
