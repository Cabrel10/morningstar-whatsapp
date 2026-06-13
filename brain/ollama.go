package main

import (
	"encoding/json"
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
// GROQ API STRUCTURES (OpenAI-compatible)
// ============================================================================

type GroqRequest struct {
	Model       string        `json:"model"`
	Messages    []GroqMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	TopP        float64       `json:"top_p"`
	Stream      bool          `json:"stream"`
}

type GroqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GroqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

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
		return 1024
	case IntentStory:
		return 800
	case IntentGame:
		return 200
	case IntentGreeting:
		return 100
	case IntentSearch:
		return 200
	default:
		return 400
	}
}

// ============================================================================
// GROQ API CALL (PRIMARY - Fast cloud inference)
// ============================================================================

func callGroq(systemPrompt, userPrompt string, intent Intent) (string, error) {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GROQ_API_KEY not set")
	}

	model := os.Getenv("GROQ_MODEL")
	if model == "" {
		model = "llama-3.3-70b-versatile"
	}

	temperature := getTemperatureForIntent(intent)
	maxTokens := getMaxTokensForIntent(intent)

	messages := []GroqMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	reqBody := GroqRequest{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		TopP:        0.9,
		Stream:      false,
	}

	client := resty.New()
	client.SetTimeout(30 * time.Second) // Groq is fast, 30s is more than enough

	fmt.Printf("[GROQ] Calling %s intent=%s (temp=%.2f, max_tokens=%d)\n",
		model, intent, temperature, maxTokens)

	var result GroqResponse
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetBody(reqBody).
		SetResult(&result).
		Post("https://api.groq.com/openai/v1/chat/completions")

	if err != nil {
		fmt.Printf("[GROQ] Network error: %v\n", err)
		return "", err
	}

	if resp.IsError() {
		// Parse error body
		var errResp GroqResponse
		_ = json.Unmarshal(resp.Body(), &errResp)
		errMsg := resp.String()
		if errResp.Error != nil {
			errMsg = errResp.Error.Message
		}
		fmt.Printf("[GROQ] API Error %d: %s\n", resp.StatusCode(), errMsg)
		return "", fmt.Errorf("groq error %d: %s", resp.StatusCode(), errMsg)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("groq: empty response")
	}

	response := result.Choices[0].Message.Content
	fmt.Printf("[GROQ] OK: %d chars, %d tokens (prompt=%d, completion=%d)\n",
		len(response), result.Usage.TotalTokens, result.Usage.PromptTokens, result.Usage.CompletionTokens)

	return response, nil
}

// ============================================================================
// GEMINI API CALL (SECONDARY - Google free tier)
// ============================================================================

type GeminiRequest struct {
	Contents         []GeminiContent        `json:"contents"`
	SystemInstruction *GeminiContent        `json:"systemInstruction,omitempty"`
	GenerationConfig map[string]interface{} `json:"generationConfig"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func callGemini(systemPrompt, userPrompt string, intent Intent) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY not set")
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
	}

	temperature := getTemperatureForIntent(intent)
	maxTokens := getMaxTokensForIntent(intent)

	reqBody := GeminiRequest{
		SystemInstruction: &GeminiContent{
			Parts: []GeminiPart{{Text: systemPrompt}},
		},
		Contents: []GeminiContent{
			{
				Role:  "user",
				Parts: []GeminiPart{{Text: userPrompt}},
			},
		},
		GenerationConfig: map[string]interface{}{
			"temperature":     temperature,
			"maxOutputTokens": maxTokens,
			"topP":            0.9,
		},
	}

	client := resty.New()
	client.SetTimeout(30 * time.Second)

	fmt.Printf("[GEMINI] Calling %s intent=%s (temp=%.2f, max_tokens=%d)\n",
		model, intent, temperature, maxTokens)

	var result GeminiResponse
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(reqBody).
		SetResult(&result).
		Post(url)

	if err != nil {
		fmt.Printf("[GEMINI] Network error: %v\n", err)
		return "", err
	}

	if resp.IsError() {
		fmt.Printf("[GEMINI] API Error %d: %s\n", resp.StatusCode(), resp.String())
		return "", fmt.Errorf("gemini error %d: %s", resp.StatusCode(), resp.String())
	}

	if result.Error != nil {
		return "", fmt.Errorf("gemini: %s", result.Error.Message)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: empty response")
	}

	response := result.Candidates[0].Content.Parts[0].Text
	fmt.Printf("[GEMINI] OK: %d chars\n", len(response))
	return response, nil
}

// ============================================================================
// OLLAMA LOCAL CALL (FALLBACK)
// ============================================================================

func getOllamaOptions(intent Intent) map[string]interface{} {
	base := map[string]interface{}{
		"num_thread":     4,
		"num_ctx":        1024,
		"top_p":          0.90,
		"repeat_penalty": 1.15,
		"num_predict":    256,
	}

	switch intent {
	case IntentCode:
		base["temperature"] = 0.3
		base["num_predict"] = 512
	case IntentQuestion:
		base["temperature"] = 0.4
		base["num_predict"] = 256
	case IntentStory:
		base["temperature"] = 0.75
		base["num_predict"] = 512
	case IntentGame:
		base["temperature"] = 0.5
		base["num_predict"] = 128
	case IntentGreeting:
		base["temperature"] = 0.6
		base["num_predict"] = 64
	default:
		base["temperature"] = 0.55
		base["num_predict"] = 256
	}

	return base
}

func callOllamaLocal(prompt string, intent Intent, images []string) (string, error) {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen2.5:1.5b"
	}

	client := resty.New()
	client.SetTimeout(90 * time.Second)

	options := getOllamaOptions(intent)

	fmt.Printf("[OLLAMA-FALLBACK] Calling %s for intent=%s\n", model, intent)

	var result OllamaResponse
	resp, err := client.R().
		SetBody(OllamaRequest{
			Model:     model,
			Prompt:    prompt,
			Stream:    false,
			KeepAlive: "5m",
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

// callOllamaWithIntent - PRIMARY ENTRY POINT for all LLM calls
// Strategy: Groq > Gemini > Ollama local (in order of speed)
func callOllamaWithIntent(prompt string, intent Intent, images []string) (string, error) {
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	start := time.Now()

	// Extract system prompt from the full prompt for chat-based APIs
	systemPrompt, userPrompt := splitPrompt(prompt)

	// TRY 1: Groq API (fastest cloud inference)
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey != "" {
		response, err := callGroq(systemPrompt, userPrompt, intent)
		if err == nil {
			fmt.Printf("[LLM] Groq OK in %.1fs\n", time.Since(start).Seconds())
			return response, nil
		}
		fmt.Printf("[LLM] Groq failed: %v, trying next backend...\n", err)
	}

	// TRY 2: Gemini API (Google free tier - 15 RPM)
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey != "" {
		response, err := callGemini(systemPrompt, userPrompt, intent)
		if err == nil {
			fmt.Printf("[LLM] Gemini OK in %.1fs\n", time.Since(start).Seconds())
			return response, nil
		}
		fmt.Printf("[LLM] Gemini failed: %v, trying Ollama fallback...\n", err)
	}

	// TRY 3: Ollama local (slow but works offline)
	if groqKey == "" && geminiKey == "" {
		fmt.Printf("[LLM] WARNING: No GROQ_API_KEY or GEMINI_API_KEY set! Using slow Ollama local.\n")
	}
	response, err := callOllamaLocal(prompt, intent, images)
	if err != nil {
		fmt.Printf("[LLM] ALL backends failed! Groq + Gemini + Ollama. Error: %v\n", err)
		return "", err
	}

	fmt.Printf("[LLM] Ollama fallback OK in %.1fs\n", time.Since(start).Seconds())
	return response, nil
}

// splitPrompt separates the system prompt from the user prompt
// Convention: everything before the first "MESSAGES RECENTS:" or the user's actual query
func splitPrompt(fullPrompt string) (system string, user string) {
	// The system prompt is the persona/instructions at the top
	// The user prompt is the actual conversation context + user message

	// Find markers that indicate the start of user context
	markers := []string{
		"MESSAGES RECENTS:\n",
		"CONNAISSANCES DU GROUPE:\n",
		"CE QUE TU SAIS SUR",
		"RESUME DES DISCUSSIONS",
		"FAITS IMPORTANTS:\n",
		"Contexte recent:\n",
		"Question de ",
		"Demande de ",
		"Recherche de ",
		"Messages:\n",
	}

	// Default: use SystemPrompt as system, full prompt as user
	system = SystemPrompt
	user = fullPrompt

	// Try to split at the first context marker
	for _, marker := range markers {
		idx := strings.Index(fullPrompt, marker)
		if idx > 0 {
			potentialSystem := strings.TrimSpace(fullPrompt[:idx])
			potentialUser := strings.TrimSpace(fullPrompt[idx:])
			if len(potentialSystem) > 20 && len(potentialUser) > 5 {
				system = potentialSystem
				user = potentialUser
				break
			}
		}
	}

	// Ensure system prompt isn't too long (Groq has context limits)
	if len(system) > 1500 {
		system = system[:1500]
	}
	// Ensure user prompt isn't too long
	if len(user) > 3000 {
		user = user[len(user)-3000:]
	}

	return system, user
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
