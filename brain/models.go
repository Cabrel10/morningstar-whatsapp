package main

import (
	"encoding/json"
	"time"
)

type WebhookPayload struct {
	Event    string      `json:"event"`
	Instance string      `json:"instance"`
	Data     MessageData `json:"data"`
}

type MessageData struct {
	Key      MessageKey      `json:"key"`
	Message  json.RawMessage `json:"message"`
	PushName string          `json:"pushName"`
}

type MessageKey struct {
	RemoteJid   string `json:"remoteJid"`
	FromMe      bool   `json:"fromMe"`
	Id          string `json:"id"`
	Participant string `json:"participant"` // Real sender ID in groups
}

type Job struct {
	Instance  string
	RemoteJid string
	UserText  string
}

type MessageContent struct {
	Conversation string `json:"conversation"`
	ExtendedText *ExtendedTextMessage `json:"extendedTextMessage"`
}

type ExtendedTextMessage struct {
	Text string `json:"text"`
}

func GetMessageText(raw json.RawMessage) string {
	var m MessageContent
	if err := json.Unmarshal(raw, &m); err == nil {
		if m.Conversation != "" {
			return m.Conversation
		}
		if m.ExtendedText != nil {
			return m.ExtendedText.Text
		}
	}

	// Fallback to simple string if it's not an object
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	return ""
}

type OllamaRequest struct {
	Model     string                 `json:"model"`
	Prompt    string                 `json:"prompt"`
	Stream    bool                   `json:"stream"`
	KeepAlive string                 `json:"keep_alive"`
	Options   map[string]interface{} `json:"options"`
}

type OllamaResponse struct {
	Response string `json:"response"`
}

type EvolutionSendMessageRequest struct {
	Number       string `json:"number"`
	Text         string `json:"text"`
	Delay        int    `json:"delay"`
	LinkPreview  bool   `json:"linkPreview"`
}

type EvolutionPresenceRequest struct {
	Number   string `json:"number"`
	Presence string `json:"presence"`
}

type Fact struct {
	Id        int
	RemoteJid string
	Content   string
	CreatedAt time.Time
}
