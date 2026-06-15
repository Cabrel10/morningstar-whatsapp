package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

var (
	semaphore = make(chan struct{}, 3) // Max 3 concurrent LLM requests
)

// ChatWithOllama calls the Ollama API with structured messages and tools.
// If the model doesn't support tools (400 error), it automatically retries without tools.
func ChatWithOllama(messages []api.Message, intent Intent, tools []api.Tool) (*api.Message, error) {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://172.17.0.1:11434"
	}
	u, _ := url.Parse(ollamaURL)

	// Use a client with a 240s timeout to prevent worker starvation
	httpClient := &http.Client{Timeout: 240 * time.Second}
	client := api.NewClient(u, httpClient)

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen2.5:1.5b"
	}

	options := getOllamaOptions(intent)

	req := &api.ChatRequest{
		Model:    model,
		Messages: messages,
		Options:  options,
		Tools:    tools,
		Stream:   new(bool),
	}

	fmt.Printf("[OLLAMA-NATIVE] Calling %s (intent=%s, messages=%d, tools=%d)\n", model, intent, len(messages), len(tools))

	// Context timeout: never block the worker goroutine more than 240s
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	var finalMsg *api.Message
	err := client.Chat(ctx, req, func(resp api.ChatResponse) error {
		finalMsg = &resp.Message
		return nil
	})

	if err != nil {
		errStr := err.Error()
		// FALLBACK: If model doesn't support tools, retry without tools
		if len(tools) > 0 && (strings.Contains(errStr, "does not support tools") ||
			strings.Contains(errStr, "400")) {
			fmt.Printf("[OLLAMA-NATIVE] Model %s doesn't support tools, retrying without tools...\n", model)

			req.Tools = nil
			ctx2, cancel2 := context.WithTimeout(context.Background(), 240*time.Second)
			defer cancel2()

			err2 := client.Chat(ctx2, req, func(resp api.ChatResponse) error {
				finalMsg = &resp.Message
				return nil
			})
			if err2 != nil {
				fmt.Printf("[OLLAMA-NATIVE] Retry without tools also failed: %v\n", err2)
				return nil, err2
			}
			fmt.Printf("[OLLAMA-NATIVE] Success without tools (response=%d chars)\n", len(finalMsg.Content))
			return finalMsg, nil
		}

		fmt.Printf("[OLLAMA-NATIVE] Error: %v\n", err)
		return nil, err
	}

	if finalMsg != nil {
		fmt.Printf("[OLLAMA-NATIVE] Success (response=%d chars, toolCalls=%d)\n", len(finalMsg.Content), len(finalMsg.ToolCalls))
	}
	return finalMsg, nil
}

func getOllamaOptions(intent Intent) map[string]interface{} {
	base := map[string]interface{}{
		"num_thread":     3,
		"num_ctx":        4096,
		"temperature":    0.4,
		"num_predict":    512,
		"top_p":          0.9,
		"repeat_penalty": 1.1,
		"numa":           true,
	}

	switch intent {
	case IntentCode:
		base["temperature"] = 0.2
		base["num_predict"] = 1024
	case IntentSummary:
		base["temperature"] = 0.3
		base["num_predict"] = 400
	case IntentStory:
		base["temperature"] = 0.75
		base["num_predict"] = 800
	}

	return base
}

func callOllamaWithIntent(prompt string, intent Intent, images []string) (string, error) {
	messages := []api.Message{
		{Role: "user", Content: prompt},
	}
	msg, err := ChatWithOllama(messages, intent, nil)
	if err != nil {
		return "", err
	}
	return msg.Content, nil
}

func cleanResponse(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "..."
	}

	unwantedPrefixes := []string{
		"Bonjour ! Je suis Poulga",
		"Je suis Poulga,",
		"Bien sûr !",
	}
	for _, prefix := range unwantedPrefixes {
		if strings.HasPrefix(text, prefix) {
			text = strings.TrimSpace(text[len(prefix):])
		}
	}

	return text
}
