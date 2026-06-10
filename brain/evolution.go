package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-resty/resty/v2"
)

func sendWhatsAppMessage(instance, remoteJid, text string) error {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	// remoteJid might be 123456789@s.whatsapp.net or 123456789@g.us
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
