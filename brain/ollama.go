package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

var (
	semaphore = make(chan struct{}, 3) // Max 3 concurrent LLM requests
)

// ============================================================================
// LLM BACKEND CONFIGURATION
// ============================================================================

// getTemperatureForIntent returns the temperature for a given intent
func getTemperatureForIntent(intent Intent) float64 {
	switch intent {
	case IntentCode:
		return 0.3
	case IntentQuestion:
		return 0.4
	case IntentStory:
		return 0.75
	case IntentGame:
		return 0.5
	case IntentSummary:
		return 0.3
	case IntentSearch:
		return 0.3
	case IntentGreeting:
		return 0.6
	default:
		return 0.55
	}
}

// getMaxTokensForIntent returns max tokens for a given intent
func getMaxTokensForIntent(intent Intent) int {
	switch intent {
	case IntentCode:
		return 256 // Reduced from 1024 for CPU-only inference
	case IntentStory:
		return 128 // Reduced from 800
	case IntentGame:
		return 80 // Reduced from 200
	case IntentGreeting:
		return 50 // Reduced from 100
	case IntentSearch:
		return 100 // Reduced from 200
	default:
		return 128 // Reduced from 400
	}
}

// ============================================================================
// OLLAMA LOCAL CALL (FALLBACK)
// ============================================================================

func getOllamaOptions(intent Intent) map[string]interface{} {
	base := map[string]interface{}{
		"num_thread":     3,    // Using 3 threads to leave one core for system/evolution
		"num_ctx":        4096, // Larger context window as requested
		"temperature":    0.4,  // Stable temperature
		"num_predict":    2048, // Long responses allowed (important for code)
		"top_p":          0.9,
		"repeat_penalty": 1.2,
		"numa":           true,
	}

	switch intent {
	case IntentCode:
		base["temperature"] = 0.2
		base["num_predict"] = 2048
	case IntentQuestion:
		base["temperature"] = 0.4
		base["num_predict"] = 1024
	case IntentStory:
		base["temperature"] = 0.75
		base["num_predict"] = 2048
	case IntentGame:
		base["temperature"] = 0.5
		base["num_predict"] = 512
	case IntentGreeting:
		base["temperature"] = 0.6
		base["num_predict"] = 256
	case IntentSearch:
		base["temperature"] = 0.3
		base["num_predict"] = 1024
	default:
		// Default values from base
	}

	return base
}

func callOllamaLocal(prompt string, intent Intent, images []string) (string, error) {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://172.17.0.1:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "gemma3:4b" // Restore stable gemma3:4b
	}

	// PERFORMANCE: Trim prompt for CPU inference if it's huge
	if len(prompt) > 6000 {
		prompt = prompt[len(prompt)-6000:]
	}

	client := resty.New()
	client.SetTimeout(180 * time.Second) // Increased timeout for heavier model

	options := getOllamaOptions(intent)

	fmt.Printf("[OLLAMA-LOCAL] Calling %s for intent=%s (prompt=%d)\n", model, intent, len(prompt))

	var result OllamaResponse
	resp, err := client.R().
		SetBody(OllamaRequest{
			Model:     model,
			Prompt:    prompt,
			Stream:    false,
			KeepAlive: "-1m", // INFINITE PERSISTENCE - Don't unload
			Options:   options,
			Images:    images,
		}).
		SetResult(&result).
		Post(fmt.Sprintf("%s/api/generate", ollamaURL))

	if err != nil {
		fmt.Printf("[OLLAMA-FALLBACK] Error: %v\n", err)
		return "", err
	}

	if resp.IsError() {
		return "", fmt.Errorf("ollama error: %s", resp.String())
	}

	fmt.Printf("[OLLAMA-FALLBACK] Response: %d chars\n", len(result.Response))
	return result.Response, nil
}

// ============================================================================
// MAIN LLM ENTRY POINTS (Groq first, Ollama fallback)
// ============================================================================

// callOllama is the legacy function - now routes through the unified system
func callOllama(prompt string, images []string, temperature float64) (string, error) {
	return callOllamaWithIntent(prompt, IntentChat, images)
}

func callOllamaWithIntent(prompt string, intent Intent, images []string) (string, error) {
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	start := time.Now()
	fmt.Printf("[LLM] Starting generation for intent=%s (total_prompt_size=%d chars)\n", intent, len(prompt))

	response, err := callOllamaLocal(prompt, intent, images)
	if err != nil {
		fmt.Printf("[LLM] Ollama local call failed. Error: %v\n", err)
		return "", err
	}

	fmt.Printf("[LLM] Ollama local OK in %.1fs\n", time.Since(start).Seconds())
	return response, nil
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
		"Bonjour à tous !",
		"Bonjour à tous",
		"Je suis Poulga,",
		"Je suis Poulga.",
		"En tant que Poulga,",
		"En tant qu'assistante,",
		"Poulga :",
		"Poulga:",
		"Bien sûr !",
		"Bien sûr,",
		"Absolument !",
	}

	for _, prefix := range unwantedPrefixes {
		if strings.HasPrefix(text, prefix) {
			text = strings.TrimSpace(text[len(prefix):])
		}
	}

	if text == "" {
		return "..."
	}

	// Remove trailing incomplete sentences (cut off by max_tokens)
	if len(text) > 100 {
		lastChar := text[len(text)-1]
		if lastChar != '.' && lastChar != '!' && lastChar != '?' && lastChar != ')' && lastChar != '"' && lastChar != '\n' {
			lastDot := strings.LastIndexAny(text, ".!?")
			if lastDot > len(text)/2 {
				text = text[:lastDot+1]
			}
		}
	}

	return text
}
