package main

import (
	"fmt"
	"os"

	"github.com/go-resty/resty/v2"
)

func callOllama(prompt string, images []string) (string, error) {
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
			KeepAlive: "24h",
			Options: map[string]interface{}{
				"num_thread":     4,
				"temperature":    0.7,
				"top_p":          0.9,
				"repeat_penalty": 1.2,
				"num_predict":    128, // Augmenté de 80 à 128 pour éviter les coupures
			},
			Images: images,
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

func generateEmbedding(text string) ([]float32, error) {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	client := resty.New()
	var result EmbeddingResponse
	resp, err := client.R().
		SetBody(EmbeddingRequest{
			Model:  "nomic-embed-text",
			Prompt: text,
		}).
		SetResult(&result).
		Post(fmt.Sprintf("%s/api/embeddings", ollamaURL))

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, fmt.Errorf("ollama embedding error: %s", resp.String())
	}

	return result.Embedding, nil
}
