package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

func sendWhatsAppMessage(instance, remoteJid, text, quotedMsgId, participant string) error {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	number := strings.Split(remoteJid, "@")[0]

	body := EvolutionSendMessageRequest{
		Number:      number,
		Text:        text,
		Delay:       1000,
		LinkPreview: true,
	}

	// Add quoting if msgId provided
	if quotedMsgId != "" {
		body.Quoted = map[string]interface{}{
			"key": map[string]interface{}{
				"id": quotedMsgId,
			},
		}
	}

	// Add mentions for groups
	if participant != "" && strings.HasSuffix(remoteJid, "@g.us") {
		body.Mentioned = []string{participant}
	}

	client := resty.New()
	client.SetTimeout(15 * time.Second)
	resp, err := client.R().
		SetHeader("apikey", apiKey).
		SetBody(body).
		Post(fmt.Sprintf("%s/message/sendText/%s", evoURL, instance))

	if err != nil {
		return err
	}

	if resp.IsError() {
		return fmt.Errorf("evolution error: %s", resp.String())
	}

	return nil
}

func getGroupMetadata(instance, groupJid string) ([]string, error) {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	client := resty.New()
	client.SetTimeout(15 * time.Second)
	resp, err := client.R().
		SetHeader("apikey", apiKey).
		SetQueryParam("groupJid", groupJid).
		Get(fmt.Sprintf("%s/group/findGroupInfos/%s", evoURL, instance))

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, fmt.Errorf("evolution group error: %s", resp.String())
	}

	var groupInfo struct {
		Participants []struct {
			Id    string `json:"id"`
			Admin string `json:"admin"`
		} `json:"participants"`
	}

	if err := json.Unmarshal(resp.Body(), &groupInfo); err != nil {
		return nil, err
	}

	var participants []string
	for _, p := range groupInfo.Participants {
		participants = append(participants, p.Id)
	}

	return participants, nil
}

func isUserAdmin(instance, groupJid, userJid string) (bool, error) {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	client := resty.New()
	client.SetTimeout(15 * time.Second)
	resp, err := client.R().
		SetHeader("apikey", apiKey).
		SetQueryParam("groupJid", groupJid).
		Get(fmt.Sprintf("%s/group/findGroupInfos/%s", evoURL, instance))

	if err != nil {
		return false, err
	}

	var groupInfo struct {
		Participants []struct {
			Id    string `json:"id"`
			Admin string `json:"admin"`
		} `json:"participants"`
	}

	if err := json.Unmarshal(resp.Body(), &groupInfo); err != nil {
		return false, err
	}

	for _, p := range groupInfo.Participants {
		if p.Id == userJid {
			return p.Admin != "", nil
		}
	}

	return false, nil
}

func kickUser(instance, groupJid, userJid string) error {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	client := resty.New()
	client.SetTimeout(15 * time.Second)
	_, err := client.R().
		SetHeader("apikey", apiKey).
		SetBody(map[string]interface{}{
			"groupJid": groupJid,
			"participants": []string{userJid},
		}).
		Post(fmt.Sprintf("%s/group/updateParticipant/%s?action=remove", evoURL, instance))

	return err
}

func sendSticker(instance, remoteJid, stickerBase64 string) error {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	number := strings.Split(remoteJid, "@")[0]

	body := map[string]interface{}{
		"number":  number,
		"sticker": stickerBase64,
	}

	client := resty.New()
	client.SetTimeout(15 * time.Second)
	resp, err := client.R().
		SetHeader("apikey", apiKey).
		SetBody(body).
		Post(fmt.Sprintf("%s/message/sendSticker/%s", evoURL, instance))

	if err != nil {
		return err
	}

	if resp.IsError() {
		return fmt.Errorf("evolution sticker error: %s", resp.String())
	}

	return nil
}

func getMediaBase64(instance, msgId string) (string, error) {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	client := resty.New()
	client.SetTimeout(30 * time.Second)
	resp, err := client.R().
		SetHeader("apikey", apiKey).
		SetQueryParam("keyId", msgId).
		Get(fmt.Sprintf("%s/message/media-base64/%s", evoURL, instance))

	if err != nil {
		return "", err
	}

	if resp.IsError() {
		return "", fmt.Errorf("evolution media fetch error: %s", resp.String())
	}

	var result struct {
		Base64 string `json:"base64"`
	}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		// Sometimes it's directly the string or in another field
		return string(resp.Body()), nil
	}

	return result.Base64, nil
}

func sendWhatsAppMedia(instance, remoteJid, mediaBase64, fileName, caption, mediaType string) error {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	number := strings.Split(remoteJid, "@")[0]

	client := resty.New()
	client.SetTimeout(15 * time.Second)
	resp, err := client.R().
		SetHeader("apikey", apiKey).
		SetBody(EvolutionSendMediaRequest{
			Number:    number,
			Media:     mediaBase64,
			FileName:  fileName,
			Caption:   caption,
			MediaType: mediaType,
			Delay:     1000,
		}).
		Post(fmt.Sprintf("%s/message/sendMedia/%s", evoURL, instance))

	if err != nil {
		return err
	}

	if resp.IsError() {
		return fmt.Errorf("evolution media error: %s", resp.String())
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
	client.SetTimeout(15 * time.Second)
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
	client.SetTimeout(15 * time.Second)
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
