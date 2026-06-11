package main

import (
	"encoding/json"
	"time"
)

type WebhookPayload struct {
	Event    string          `json:"event"`
	Instance string          `json:"instance"`
	Data     json.RawMessage `json:"data"`
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
	Instance     string
	RemoteJid    string
	UserText     string
	QuotedText   string
	QuotedSender string
	QuotedMsgId  string
	MsgId        string
	SenderJid    string
	Image        string // Base64 image data
	Type         string // 'text', 'image', 'audio'
}

type EmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type EmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

type TTSRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
	Voice string `json:"voice"`
}

type MessageContent struct {
	Conversation string               `json:"conversation"`
	ExtendedText *ExtendedTextMessage `json:"extendedTextMessage"`
	ImageMessage *ImageMessage        `json:"imageMessage"`
	VideoMessage *VideoMessage        `json:"videoMessage"`
	StickerMessage *StickerMessage    `json:"stickerMessage"`
	ReactionMessage *ReactionMessage  `json:"reactionMessage"`
	AudioMessage   *AudioMessage      `json:"audioMessage"`
}

type AudioMessage struct {
	Url      string `json:"url"`
	Mimetype string `json:"mimetype"`
	Seconds  int    `json:"seconds"`
}

type EvolutionSendAudioRequest struct {
	Number string `json:"number"`
	Audio  string `json:"audio"` // Base64 or URL
	Delay  int    `json:"delay"`
}

type ExtendedTextMessage struct {
	Text        string       `json:"text"`
	ContextInfo *ContextInfo `json:"contextInfo"`
}

type ImageMessage struct {
	Caption     string       `json:"caption"`
	Url         string       `json:"url"`
	Mimetype    string       `json:"mimetype"`
	ContextInfo *ContextInfo `json:"contextInfo"`
}

type VideoMessage struct {
	Caption     string       `json:"caption"`
	Url         string       `json:"url"`
	Mimetype    string       `json:"mimetype"`
	ContextInfo *ContextInfo `json:"contextInfo"`
}

type StickerMessage struct {
	Url           string `json:"url"`
	FileSha256    string `json:"fileSha256"`
}

type ReactionMessage struct {
	Key  MessageKey `json:"key"`
	Text string     `json:"text"`
}

type ContextInfo struct {
	QuotedMessage *QuotedMessage `json:"quotedMessage"`
	Participant   string         `json:"participant"`
	StanzaId      string         `json:"stanzaId"`
}

type QuotedMessage struct {
	Conversation string `json:"conversation"`
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
		if m.ImageMessage != nil {
			return m.ImageMessage.Caption
		}
	}

	return ""
}

type OllamaRequest struct {
	Model     string                 `json:"model"`
	Prompt    string                 `json:"prompt"`
	Stream    bool                   `json:"stream"`
	KeepAlive string                 `json:"keep_alive"`
	Options   map[string]interface{} `json:"options"`
	Images    []string               `json:"images,omitempty"`
}

type OllamaResponse struct {
	Response string `json:"response"`
}

type EvolutionSendMessageRequest struct {
	Number      string                 `json:"number"`
	Text        string                 `json:"text"`
	Delay       int                    `json:"delay"`
	LinkPreview bool                   `json:"linkPreview"`
	Quoted      map[string]interface{} `json:"quoted,omitempty"`
	Mentioned   []string               `json:"mentioned,omitempty"`
}

type EvolutionPresenceRequest struct {
	Number   string `json:"number"`
	Presence string `json:"presence"`
}

type EvolutionSendMediaRequest struct {
	Number    string `json:"number"`
	Media     string `json:"media"` // Base64 or URL
	FileName  string `json:"fileName"`
	Caption   string `json:"caption"`
	MediaType string `json:"mediaType"` // 'image', 'video', 'audio', 'document'
	Delay     int    `json:"delay"`
}

type Fact struct {
	Id        int
	RemoteJid string
	Content   string
	CreatedAt time.Time
}
