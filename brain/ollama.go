package main

import (
	"fmt"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
)

var (
	semaphore = make(chan struct{}, 2) // Capacité de 2 slots simultanés
)

func callOllama(prompt string, images []string, temperature float64) (string, error) {
	semaphore <- struct{}{}        // On prend une place
	defer func() { <-semaphore }() // On libère la place

	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	client := resty.New()
	client.SetTimeout(60 * time.Second)
	fmt.Printf("Ollama Call: %s/api/generate (Model: gemma3:4b, Temp: %.1f)\n", ollamaURL, temperature)
	var result OllamaResponse
	resp, err := client.R().
		SetBody(OllamaRequest{
			Model:     "gemma3:4b",
			Prompt:    prompt,
			Stream:    false,
			KeepAlive: "24h",
			Options: map[string]interface{}{
				"num_thread":     2,   // 2 threads par requête (2*2 = 4 coeurs vCPU)
				"num_batch":      128, // Optimisation CPU
				"num_ctx":        2048, // Réduit pour libérer de la RAM et accélérer
				"numa":           true, // Optimisation accès mémoire
				"temperature":    0.3, // Réponses focalisées et cohérentes
				"top_p":          0.9,
				"repeat_penalty": 1.2, // Évite les répétitions
				"num_predict":    128, // Limite la longueur pour forcer la brièveté
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
