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
			Model:  "gemma3:4b",
			Prompt: prompt,
			Stream: false,
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
