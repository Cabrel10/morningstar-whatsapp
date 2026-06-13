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

// ============================================================================
// HELPER - Get Evolution API config
// ============================================================================

func evoClient() (*resty.Client, string, string) {
	evoURL := os.Getenv("EVOLUTION_URL")
	if evoURL == "" {
		evoURL = "http://evolution-api:8080"
	}
	apiKey := os.Getenv("AUTHENTICATION_API_KEY")

	client := resty.New()
	client.SetTimeout(20 * time.Second)
	client.SetHeader("apikey", apiKey)
	client.SetHeader("Content-Type", "application/json")

	return client, evoURL, apiKey
}

// ============================================================================
// SEND TEXT MESSAGE
// ============================================================================

func sendWhatsAppMessage(instance, remoteJid, text, quotedMsgId, participant string) (string, error) {
	client, evoURL, _ := evoClient()

	number := strings.Split(remoteJid, "@")[0]

	body := EvolutionSendMessageRequest{
		Number:      number,
		Text:        text,
		Delay:       800,
		LinkPreview: true,
	}

	if quotedMsgId != "" {
		body.Quoted = map[string]interface{}{
			"key": map[string]interface{}{
				"id": quotedMsgId,
			},
		}
	}

	if participant != "" && strings.HasSuffix(remoteJid, "@g.us") {
		body.Mentioned = []string{participant}
	}

	var result EvolutionResponse
	resp, err := client.R().
		SetBody(body).
		SetResult(&result).
		Post(fmt.Sprintf("%s/message/sendText/%s", evoURL, instance))

	if err != nil {
		fmt.Printf("[EVO] sendText error: %v\n", err)
		return "", err
	}
	if resp.IsError() {
		fmt.Printf("[EVO] sendText API error: %s\n", resp.String())
		return "", fmt.Errorf("evolution error: %s", resp.String())
	}

	return result.Key.Id, nil
}

// sendWhatsAppMessageWithMentions sends a message with multiple mentions
func sendWhatsAppMessageWithMentions(instance, remoteJid, text string, mentions []string) (string, error) {
	client, evoURL, _ := evoClient()

	number := strings.Split(remoteJid, "@")[0]

	body := EvolutionSendMessageRequest{
		Number:    number,
		Text:      text,
		Mentioned: mentions,
	}

	var result EvolutionResponse
	resp, err := client.R().
		SetBody(body).
		SetResult(&result).
		Post(fmt.Sprintf("%s/message/sendText/%s", evoURL, instance))

	if err != nil {
		return "", err
	}
	if resp.IsError() {
		return "", fmt.Errorf("evolution error: %s", resp.String())
	}
	return result.Key.Id, nil
}

// ============================================================================
// SEND MEDIA (image, video, audio, document)
// ============================================================================

func sendWhatsAppMedia(instance, remoteJid, mediaBase64, fileName, caption, mediaType string) (string, error) {
	client, evoURL, _ := evoClient()

	number := strings.Split(remoteJid, "@")[0]

	var result EvolutionResponse
	resp, err := client.R().
		SetBody(EvolutionSendMediaRequest{
			Number:    number,
			Media:     mediaBase64,
			FileName:  fileName,
			Caption:   caption,
			MediaType: mediaType,
			Delay:     800,
		}).
		SetResult(&result).
		Post(fmt.Sprintf("%s/message/sendMedia/%s", evoURL, instance))

	if err != nil {
		return "", err
	}
	if resp.IsError() {
		return "", fmt.Errorf("evolution media error: %s", resp.String())
	}
	return result.Key.Id, nil
}

// sendWhatsAppAudio sends a voice message
func sendWhatsAppAudio(instance, remoteJid string, audioBase64 string) (string, error) {
	client, evoURL, _ := evoClient()

	number := strings.Split(remoteJid, "@")[0]

	var result EvolutionResponse
	resp, err := client.R().
		SetBody(EvolutionSendAudioRequest{
			Number: number,
			Audio:  audioBase64,
			Delay:  800,
		}).
		SetResult(&result).
		Post(fmt.Sprintf("%s/message/sendWhatsAppAudio/%s", evoURL, instance))

	if err != nil {
		return "", err
	}
	if resp.IsError() {
		return "", fmt.Errorf("evolution audio error: %s", resp.String())
	}
	return result.Key.Id, nil
}

// ============================================================================
// STICKER PIPELINE
// ============================================================================

// getMediaBase64 fetches base64 data from a media message via Evolution API
func getMediaBase64(instance, msgId string) (string, error) {
	client, evoURL, _ := evoClient()
	client.SetTimeout(30 * time.Second)

	payloadData := map[string]interface{}{
		"message": map[string]interface{}{
			"key": map[string]interface{}{
				"id": msgId,
			},
		},
		"convertToMp4": false,
	}

	resp, err := client.R().
		SetBody(payloadData).
		Post(fmt.Sprintf("%s/chat/getBase64FromMediaMessage/%s", evoURL, instance))

	if err != nil {
		fmt.Printf("[MEDIA] getBase64 request error: %v\n", err)
		return "", err
	}

	if resp.IsError() {
		fmt.Printf("[MEDIA] getBase64 API error: %s\n", resp.String())
		return "", fmt.Errorf("evolution media fetch error: %s", resp.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return "", err
	}

	base64Data, ok := result["base64"].(string)
	if !ok || base64Data == "" {
		return "", fmt.Errorf("base64 not found in response")
	}

	// Clean data URI prefix if present
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}

	return base64Data, nil
}

// sendSticker sends a sticker via Evolution API
func sendSticker(instance, remoteJid, stickerBase64 string) error {
	client, evoURL, _ := evoClient()

	number := strings.Split(remoteJid, "@")[0]

	body := map[string]interface{}{
		"number":  number,
		"sticker": stickerBase64,
	}

	resp, err := client.R().
		SetBody(body).
		Post(fmt.Sprintf("%s/message/sendSticker/%s", evoURL, instance))

	if err != nil {
		fmt.Printf("[STICKER] send error: %v\n", err)
		return err
	}
	if resp.IsError() {
		fmt.Printf("[STICKER] send API error: %s\n", resp.String())
		return fmt.Errorf("evolution sticker error: %s", resp.String())
	}
	return nil
}

// ============================================================================
// GROUP MANAGEMENT
// ============================================================================

func getGroupMetadata(instance, groupJid string) ([]string, error) {
	client, evoURL, _ := evoClient()

	resp, err := client.R().
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
	client, evoURL, _ := evoClient()

	resp, err := client.R().
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
	client, evoURL, _ := evoClient()

	_, err := client.R().
		SetBody(map[string]interface{}{
			"groupJid":     groupJid,
			"participants": []string{userJid},
		}).
		Post(fmt.Sprintf("%s/group/updateParticipant/%s?action=remove", evoURL, instance))

	return err
}

func promoteUser(instance, groupJid, userJid string) error {
	client, evoURL, _ := evoClient()

	_, err := client.R().
		SetBody(map[string]interface{}{
			"groupJid":     groupJid,
			"participants": []string{userJid},
		}).
		Post(fmt.Sprintf("%s/group/updateParticipant/%s?action=promote", evoURL, instance))
	return err
}

func demoteUser(instance, groupJid, userJid string) error {
	client, evoURL, _ := evoClient()

	_, err := client.R().
		SetBody(map[string]interface{}{
			"groupJid":     groupJid,
			"participants": []string{userJid},
		}).
		Post(fmt.Sprintf("%s/group/updateParticipant/%s?action=demote", evoURL, instance))
	return err
}

func setGroupAnnouncement(instance, groupJid string, close bool) error {
	client, evoURL, _ := evoClient()

	action := "not_announcement"
	if close {
		action = "announcement"
	}

	_, err := client.R().
		Post(fmt.Sprintf("%s/group/updateSetting/%s?groupJid=%s&action=%s", evoURL, instance, groupJid, action))

	return err
}

func deleteMessage(instance, remoteJid, msgId string) error {
	client, evoURL, _ := evoClient()

	_, err := client.R().
		SetBody(map[string]interface{}{
			"key": map[string]interface{}{
				"remoteJid": remoteJid,
				"fromMe":    false,
				"id":        msgId,
			},
		}).
		Post(fmt.Sprintf("%s/message/delete/%s", evoURL, instance))

	return err
}

func getGroupInviteLink(instance, groupJid string) (string, error) {
	client, evoURL, _ := evoClient()

	resp, err := client.R().
		SetQueryParam("groupJid", groupJid).
		Get(fmt.Sprintf("%s/group/inviteCode/%s", evoURL, instance))

	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return "", err
	}

	if code, ok := result["inviteCode"].(string); ok {
		return "https://chat.whatsapp.com/" + code, nil
	}
	if code, ok := result["invite"].(string); ok {
		return "https://chat.whatsapp.com/" + code, nil
	}

	return "", fmt.Errorf("invite code not found")
}

// ============================================================================
// PRESENCE / TYPING
// ============================================================================

func sendTypingStatus(instance, remoteJid string) error {
	client, evoURL, _ := evoClient()

	number := strings.Split(remoteJid, "@")[0]

	_, err := client.R().
		SetBody(EvolutionPresenceRequest{
			Number:   number,
			Presence: "composing",
		}).
		Post(fmt.Sprintf("%s/chat/presenceUpdate/%s", evoURL, instance))

	return err
}

// ============================================================================
// TTS (optional)
// ============================================================================

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

	return base64.StdEncoding.EncodeToString(resp.Body()), nil
}
