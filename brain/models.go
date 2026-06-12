package main

import (
	"encoding/json"
	"time"
)

// ============================================================================
// WEBHOOK PAYLOAD (from Evolution API)
// ============================================================================

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
	Participant string `json:"participant"`
}

// ============================================================================
// MESSAGE CONTENT (WhatsApp message types)
// ============================================================================

type MessageContent struct {
	Conversation    string               `json:"conversation"`
	ContextInfo     *ContextInfo         `json:"contextInfo"`
	ExtendedText    *ExtendedTextMessage `json:"extendedTextMessage"`
	ImageMessage    *ImageMessage        `json:"imageMessage"`
	VideoMessage    *VideoMessage        `json:"videoMessage"`
	StickerMessage  *StickerMessage      `json:"stickerMessage"`
	ReactionMessage *ReactionMessage     `json:"reactionMessage"`
	AudioMessage    *AudioMessage        `json:"audioMessage"`
	DocumentMessage *DocumentMessage     `json:"documentMessage"`
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
	Url        string `json:"url"`
	FileSha256 string `json:"fileSha256"`
}

type ReactionMessage struct {
	Key  MessageKey `json:"key"`
	Text string     `json:"text"`
}

type AudioMessage struct {
	Url      string       `json:"url"`
	Mimetype string       `json:"mimetype"`
	Seconds  int          `json:"seconds"`
	ContextInfo *ContextInfo `json:"contextInfo"`
}

type DocumentMessage struct {
	Url      string       `json:"url"`
	Mimetype string       `json:"mimetype"`
	Title    string       `json:"title"`
	ContextInfo *ContextInfo `json:"contextInfo"`
}

type ContextInfo struct {
	QuotedMessage *QuotedMessage `json:"quotedMessage"`
	Participant   string         `json:"participant"`
	StanzaId      string         `json:"stanzaId"`
	MentionedJid  []string       `json:"mentionedJid"`
}

type QuotedMessage struct {
	Conversation string `json:"conversation"`
	ExtendedText *ExtendedTextMessage `json:"extendedTextMessage"`
}

// GetMessageText extracts text content from any message type
func GetMessageText(raw json.RawMessage) string {
	var m MessageContent
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if m.Conversation != "" {
		return m.Conversation
	}
	if m.ExtendedText != nil && m.ExtendedText.Text != "" {
		return m.ExtendedText.Text
	}
	if m.ImageMessage != nil && m.ImageMessage.Caption != "" {
		return m.ImageMessage.Caption
	}
	if m.VideoMessage != nil && m.VideoMessage.Caption != "" {
		return m.VideoMessage.Caption
	}
	if m.DocumentMessage != nil && m.DocumentMessage.Title != "" {
		return m.DocumentMessage.Title
	}
	return ""
}

// GetQuotedText extracts quoted message text, checking both conversation and extended text
func GetQuotedText(qm *QuotedMessage) string {
	if qm == nil {
		return ""
	}
	if qm.Conversation != "" {
		return qm.Conversation
	}
	if qm.ExtendedText != nil && qm.ExtendedText.Text != "" {
		return qm.ExtendedText.Text
	}
	return ""
}

// ============================================================================
// MESSAGE CONTEXT (the unified object every handler receives)
// ============================================================================

type MessageContext struct {
	// Identity
	Instance  string
	MsgId     string
	RemoteJid string // Group JID or private chat JID
	SenderJid string // Actual sender (in groups = participant)
	PushName  string // Display name

	// Content
	Text      string // Cleaned text (mention removed)
	RawText   string // Original text before cleaning
	MediaType string // "text", "image", "video", "audio", "sticker", "document"

	// Context flags
	IsPrivateChat bool
	IsGroupChat   bool
	IsMentioned   bool
	IsReplyToBot  bool

	// Quoted message info
	QuotedText    string
	QuotedSender  string
	QuotedMsgId   string
	MentionedJids []string

	// Timestamp
	Timestamp time.Time
}

// ============================================================================
// OLLAMA API
// ============================================================================

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

type EmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type EmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

// ============================================================================
// EVOLUTION API REQUESTS
// ============================================================================

type EvolutionSendMessageRequest struct {
	Number      string                 `json:"number"`
	Text        string                 `json:"text"`
	Delay       int                    `json:"delay"`
	LinkPreview bool                   `json:"linkPreview"`
	Quoted      map[string]interface{} `json:"quoted,omitempty"`
	Mentioned   []string               `json:"mentioned,omitempty"`
}

type EvolutionSendMediaRequest struct {
	Number    string `json:"number"`
	Media     string `json:"media"`
	FileName  string `json:"fileName"`
	Caption   string `json:"caption"`
	MediaType string `json:"mediaType"`
	Delay     int    `json:"delay"`
}

type EvolutionSendAudioRequest struct {
	Number string `json:"number"`
	Audio  string `json:"audio"`
	Delay  int    `json:"delay"`
}

type EvolutionPresenceRequest struct {
	Number   string `json:"number"`
	Presence string `json:"presence"`
}

// ============================================================================
// JOB QUEUE
// ============================================================================

type Job struct {
	Ctx MessageContext
}

// ============================================================================
// DATABASE MODELS
// ============================================================================

type ConversationMessage struct {
	ID         int
	GroupJid   string
	SenderJid  string
	SenderName string
	Message    string
	IsFromBot  bool
	QuotedMsgId string
	CreatedAt  time.Time
}

type UserMemory struct {
	ID        int
	UserJid   string
	GroupJid  string
	Key       string
	Value     string
	CreatedAt time.Time
}

type GroupMemoryEntry struct {
	ID        int
	GroupJid  string
	Key       string
	Value     string
	CreatedAt time.Time
}

type FactEntry struct {
	ID      int
	Content string
}

type GroupSettings struct {
	WelcomeEnabled         bool
	AntiLinkEnabled        bool
	AntiSpamEnabled        bool
	AntiSuppressionEnabled bool
	IsClosed               bool
}

// ============================================================================
// TTY / TTS (optional)
// ============================================================================

type TTSRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
	Voice string `json:"voice"`
}
