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

	number := remoteJid

	body := EvolutionSendMessageRequest{
		Number:      number,
		Text:        text,
		Delay:       800,
		LinkPreview: true,
	}

	if quotedMsgId != "" {
		body.Quoted = map[string]interface{}{
			"key": map[string]interface{}{
				"id":        quotedMsgId,
				"fromMe":    false,
				"remoteJid": remoteJid,
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

func sendWhatsAppMessageWithMentions(instance, remoteJid, text string, mentions []string) (string, error) {
	client, evoURL, _ := evoClient()

	number := remoteJid

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
	number := remoteJid

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

func sendWhatsAppAudio(instance, remoteJid string, audioBase64 string) (string, error) {
	client, evoURL, _ := evoClient()
	number := remoteJid

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

func getMediaBase64(instance, msgId string) (string, error) {
	client, evoURL, _ := evoClient()
	client.SetTimeout(30 * time.Second)

	payloadData := map[string]interface{}{
		"message": map[string]interface{}{
			"key": map[string]interface{}{"id": msgId},
		},
		"convertToMp4": false,
	}

	resp, err := client.R().
		SetBody(payloadData).
		Post(fmt.Sprintf("%s/chat/getBase64FromMediaMessage/%s", evoURL, instance))

	if err != nil {
		return "", err
	}
	if resp.IsError() {
		return "", fmt.Errorf("fetch error: %s", resp.String())
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)
	base64Data, _ := result["base64"].(string)
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}
	return base64Data, nil
}

func sendSticker(instance, remoteJid, stickerBase64 string) error {
	client, evoURL, _ := evoClient()
	number := remoteJid

	body := map[string]interface{}{
		"number":  number,
		"sticker": stickerBase64,
	}

	resp, err := client.R().
		SetBody(body).
		Post(fmt.Sprintf("%s/message/sendSticker/%s", evoURL, instance))

	if err != nil { return err }
	if resp.IsError() { return fmt.Errorf("error: %s", resp.String()) }
	return nil
}

// ============================================================================
// GROUP MANAGEMENT
// ============================================================================

func getGroupMetadata(instance, groupJid string) ([]string, error) {
	client, evoURL, _ := evoClient()

	// Robust parsing for Evolution v2
	resp, err := client.R().
		SetQueryParam("groupJid", groupJid).
		Get(fmt.Sprintf("%s/group/findGroupInfos/%s", evoURL, instance))

	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("evolution group error: %s", resp.String())
	}

	// Structure flexible for Evolution v2 (handles both lowercase and capital cases)
	var groupInfo struct {
		Participants []struct {
			Id          string `json:"id"`
			JID         string `json:"JID"`
			PhoneNumber string `json:"PhoneNumber"`
		} `json:"participants"`
		Data struct {
			Participants []struct {
				Id          string `json:"id"`
				JID         string `json:"JID"`
				PhoneNumber string `json:"PhoneNumber"`
			} `json:"Participants"`
		} `json:"data"`
	}

	// DEBUG: Log raw response for diagnosing .tagall issues
	rawBody := string(resp.Body())
	if len(rawBody) < 3000 {
		fmt.Printf("[DEBUG] GROUP_METADATA raw: %s\n", rawBody)
	} else {
		fmt.Printf("[DEBUG] GROUP_METADATA raw (truncated): %s...\n", rawBody[:3000])
	}

	if err := json.Unmarshal(resp.Body(), &groupInfo); err != nil {
		return nil, err
	}

	var participants []string
	
	// Check both potential locations for participants
	source := groupInfo.Participants
	if len(groupInfo.Data.Participants) > 0 {
		source = groupInfo.Data.Participants
	}

	for _, p := range source {
		jid := p.JID
		if jid == "" { jid = p.Id }
		if jid == "" { jid = p.PhoneNumber }
		
		if jid != "" {
			if !strings.Contains(jid, "@") {
				jid += "@s.whatsapp.net"
			}
			participants = append(participants, jid)
		}
	}
	
	return participants, nil
}

func isUserAdmin(instance, groupJid, userJid string) (bool, error) {
	client, evoURL, _ := evoClient()
	resp, err := client.R().
		SetQueryParam("groupJid", groupJid).
		Get(fmt.Sprintf("%s/group/findGroupInfos/%s", evoURL, instance))

	if err != nil { return false, err }

	var groupInfo struct {
		Participants []struct {
			Id    string `json:"id"`
			JID   string `json:"JID"`
			Admin string `json:"admin"`
		} `json:"participants"`
		Data struct {
			Participants []struct {
				Id    string `json:"id"`
				JID   string `json:"JID"`
				Admin string `json:"admin"`
			} `json:"Participants"`
		} `json:"data"`
	}

	json.Unmarshal(resp.Body(), &groupInfo)
	
	source := groupInfo.Participants
	if len(groupInfo.Data.Participants) > 0 { source = groupInfo.Data.Participants }

	for _, p := range source {
		jid := p.JID
		if jid == "" { jid = p.Id }
		if jid == userJid || strings.Split(jid, "@")[0] == strings.Split(userJid, "@")[0] {
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
	if close { action = "announcement" }
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
	if err != nil { return "", err }
	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)
	if code, ok := result["inviteCode"].(string); ok { return "https://chat.whatsapp.com/" + code, nil }
	if data, ok := result["data"].(map[string]interface{}); ok {
		if code, ok := data["inviteCode"].(string); ok { return "https://chat.whatsapp.com/" + code, nil }
	}
	return "", fmt.Errorf("invite code not found")
}

func sendTypingStatus(instance, remoteJid string) error {
	client, evoURL, _ := evoClient()
	number := remoteJid
	_, err := client.R().
		SetBody(EvolutionPresenceRequest{
			Number:   number,
			Presence: "composing",
		}).
		Post(fmt.Sprintf("%s/chat/presenceUpdate/%s", evoURL, instance))
	return err
}

func generateTTS(text string) (string, error) {
	ttsURL := "http://kokoro-tts:8887/v1/audio/speech"
	client := resty.New().SetTimeout(15 * time.Second)
	resp, err := client.R().
		SetBody(TTSRequest{
			Model: "kokoro",
			Input: text,
			Voice: "fr_sarah",
		}).
		Post(ttsURL)

	if err != nil { return "", err }
	if resp.IsError() { return "", fmt.Errorf("tts error: %s", resp.String()) }
	return base64.StdEncoding.EncodeToString(resp.Body()), nil
}
