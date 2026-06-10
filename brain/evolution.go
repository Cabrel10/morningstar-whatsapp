package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/go-resty/resty/v2"
)

func sendWhatsAppMessage(instance, remoteJid, text string) error {
	// ... (Existing code)
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	number := strings.Split(remoteJid, "@")[0]

	client := resty.New()
	resp, err := client.R().
		SetHeader("apikey", apiKey).
		SetBody(EvolutionSendMessageRequest{
			Number:      number,
			Text:        text,
			Delay:       1000,
			LinkPreview: true,
		}).
		Post(fmt.Sprintf("%s/message/sendText/%s", evoURL, instance))

	if err != nil {
		return err
	}

	if resp.IsError() {
		return fmt.Errorf("evolution error: %s", resp.String())
	}

	return nil
}

func sendWhatsAppAudio(instance, remoteJid string, audioBase64 string) error {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	number := strings.Split(remoteJid, "@")[0]

	client := resty.New()
	resp, err := client.R().
		SetHeader("apikey", apiKey).
		SetBody(EvolutionSendAudioRequest{
			Number: number,
			Audio:  audioBase64,
			Delay:  1000,
		}).
		Post(fmt.Sprintf("%s/message/sendWhatsAppAudio/%s", evoURL, instance))

	if err != nil {
		return err
	}

	if resp.IsError() {
		return fmt.Errorf("evolution audio error: %s", resp.String())
	}

	return nil
}

func generateTTS(text string) (string, error) {
	ttsURL := "http://kokoro-tts:8887/v1/audio/speech"
	
	client := resty.New()
	resp, err := client.R().
		SetBody(TTSRequest{
			Model: "kokoro",
			Input: text,
			Voice: "fr_sarah",
		}).
		Post(ttsURL)

	if err != nil {
		return "", err
	}

	if resp.IsError() {
		return "", fmt.Errorf("tts error: %s", resp.String())
	}

	// Kokoro returns raw audio (wav/mp3), need to base64 encode it for Evolution API
	return base64.StdEncoding.EncodeToString(resp.Body()), nil
}

func sendTypingStatus(instance, remoteJid string) error {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	number := strings.Split(remoteJid, "@")[0]

	client := resty.New()
	_, err := client.R().
		SetHeader("apikey", apiKey).
		SetBody(EvolutionPresenceRequest{
			Number:   number,
			Presence: "composing",
		}).
		Post(fmt.Sprintf("%s/chat/presenceUpdate/%s", evoURL, instance))

	return err
}
