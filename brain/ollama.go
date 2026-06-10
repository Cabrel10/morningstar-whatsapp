package main

import (
	"fmt"
	"os"

	"github.com/go-resty/resty/v2"
)

func callOllama(prompt string) (string, error) {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	client := resty.New()
	fmt.Printf("Ollama Call: %s/api/generate (Model: gemma3:4b)\n", ollamaURL)
	var result OllamaResponse
	resp, err := client.R().
		SetBody(OllamaRequest{
			Model:     "gemma3:4b",
			Prompt:    prompt,
			Stream:    false,
			KeepAlive: "-1",
			Options: map[string]interface{}{
				"num_thread":     4,
				"temperature":    0.7,
				"top_p":          0.9,
				"repeat_penalty": 1.2,
			},
		}).
		SetResult(&result).
		Post(fmt.Sprintf("%s/api/generate", ollamaURL))

	if err != nil {
		return "", err
	}

	if resp.IsError() {
		return "", fmt.Errorf("ollama error: %s", resp.String())
	}

	return result.Response, nil
}
