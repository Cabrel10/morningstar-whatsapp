package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var (
	jobQueue = make(chan Job, 100)
)

func main() {
	fmt.Println("=== MorningStar Brain v2.0 starting ===")

	// Initialize DB with retry
	for {
		err := initDB()
		if err == nil {
			fmt.Println("[INIT] Database connected (pool)")
			break
		}
		fmt.Printf("[INIT] DB retry: %v\n", err)
		time.Sleep(2 * time.Second)
	}

	// Initialize Redis with retry
	for {
		err := initRedis()
		if err == nil {
			fmt.Println("[INIT] Redis connected")
			break
		}
		fmt.Printf("[INIT] Redis retry: %v\n", err)
		time.Sleep(2 * time.Second)
	}

	// Start workers
	go ollamaWorker()
	go runDailyCleanup()
	go runPeriodicCompression()

	// HTTP Server
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())

	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, "MorningStar Brain v2.0 OK")
	})

	e.POST("/webhook", handleWebhook)
	e.GET("/summary/weekly", handleWeeklySummary)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	fmt.Printf("[INIT] Listening on :%s\n", port)
	e.Logger.Fatal(e.Start(":" + port))
}

// ============================================================================
// WORKERS
// ============================================================================

func ollamaWorker() {
	fmt.Println("[WORKER] Ollama worker started")
	for job := range jobQueue {
		processLLMResponse(job.Ctx)
	}
}

func runDailyCleanup() {
	for {
		time.Sleep(24 * time.Hour)
		fmt.Println("[CLEANUP] Running daily cleanup...")
		cleanupOldMessages(30)
	}
}

// runPeriodicCompression compresses old conversations into summaries (Level 4)
func runPeriodicCompression() {
	for {
		time.Sleep(6 * time.Hour) // Every 6 hours
		// This could iterate over active groups and compress conversations
		// For now, this is a placeholder that will be enhanced
		fmt.Println("[COMPRESSION] Periodic compression check...")
	}
}

// ============================================================================
// WEBHOOK HANDLER
// ============================================================================

func handleWebhook(c echo.Context) error {
	var payload WebhookPayload
	if err := c.Bind(&payload); err != nil {
		return c.NoContent(http.StatusOK)
	}

	instance := payload.Instance
	if instance == "" {
		instance = os.Getenv("INSTANCE_NAME")
	}

	// ---- GROUP EVENTS ----
	if payload.Event == "group-participants.update" {
		handleGroupParticipantUpdate(instance, payload.Data)
		return c.NoContent(http.StatusOK)
	}

	// ---- ONLY process messages ----
	if payload.Event != "messages.upsert" {
		return c.NoContent(http.StatusOK)
	}

	// Parse message data (handles both object and array formats)
	var data MessageData
	if err := json.Unmarshal(payload.Data, &data); err != nil {
		var dataArray []MessageData
		if err := json.Unmarshal(payload.Data, &dataArray); err == nil && len(dataArray) > 0 {
			data = dataArray[0]
		} else {
			return c.NoContent(http.StatusOK)
		}
	}

	// Skip our own messages
	if data.Key.FromMe {
		return c.NoContent(http.StatusOK)
	}

	// Extract identifiers
	remoteJid := data.Key.RemoteJid
	senderJid := data.Key.Participant
	msgId := data.Key.Id
	if senderJid == "" {
		senderJid = remoteJid
	}

	// Dedup
	if IsDuplicateMessage(msgId) {
		return c.NoContent(http.StatusOK)
	}

	// Parse message content
	var m MessageContent
	_ = json.Unmarshal(data.Message, &m)
	text := GetMessageText(data.Message)

	// Extract context info (quoted messages)
	var ctxInfo *ContextInfo
	if m.ContextInfo != nil {
		ctxInfo = m.ContextInfo
	} else if m.ExtendedText != nil && m.ExtendedText.ContextInfo != nil {
		ctxInfo = m.ExtendedText.ContextInfo
	} else if m.ImageMessage != nil && m.ImageMessage.ContextInfo != nil {
		ctxInfo = m.ImageMessage.ContextInfo
	} else if m.VideoMessage != nil && m.VideoMessage.ContextInfo != nil {
		ctxInfo = m.VideoMessage.ContextInfo
	} else if m.AudioMessage != nil && m.AudioMessage.ContextInfo != nil {
		ctxInfo = m.AudioMessage.ContextInfo
	}

	// Build quoted info
	quotedText := ""
	quotedSender := ""
	quotedMsgId := ""
	var mentionedJids []string

	if ctxInfo != nil {
		quotedSender = ctxInfo.Participant
		quotedMsgId = ctxInfo.StanzaId
		quotedText = GetQuotedText(ctxInfo.QuotedMessage)
		mentionedJids = ctxInfo.MentionedJid
	}

	// ---- HANDLE REACTIONS (don't process further) ----
	if m.ReactionMessage != nil {
		targetParticipant := m.ReactionMessage.Key.Participant
		if targetParticipant == "" {
			targetParticipant = m.ReactionMessage.Key.RemoteJid
		}
		go recordInteraction(remoteJid, senderJid, targetParticipant, "reaction")
		return c.NoContent(http.StatusOK)
	}

	// ---- HANDLE STICKERS (record usage, don't process) ----
	if m.StickerMessage != nil {
		go recordStickerUsage(senderJid, m.StickerMessage.FileSha256)
		return c.NoContent(http.StatusOK)
	}

	// ---- TRACK MEMBER ----
	go upsertMember(senderJid, remoteJid, data.PushName)

	// ---- TRACK SOCIAL GRAPH (replies) ----
	if quotedSender != "" {
		go recordInteraction(remoteJid, senderJid, quotedSender, "reply")
	}

	// ---- SAVE MESSAGE TO CONVERSATION HISTORY ----
	go SaveMessage(remoteJid, senderJid, data.PushName, text, false, quotedMsgId)

	// ---- ANTI-LINK CHECK ----
	settings, _ := getGroupSettings(remoteJid)
	if settings.AntiLinkEnabled && containsLink(text) {
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			go deleteMessage(instance, remoteJid, msgId)
			warnMsg := fmt.Sprintf("@%s, les liens ne sont pas autorises ici.", strings.Split(senderJid, "@")[0])
			go sendWhatsAppMessage(instance, remoteJid, warnMsg, "", senderJid)
			return c.NoContent(http.StatusOK)
		}
	}

	// ---- DETECT MENTION ----
	cleanText := strings.TrimSpace(text)
	isMentioned := false
	lowerText := strings.ToLower(cleanText)
	if strings.Contains(lowerText, "@poulga") {
		isMentioned = true
		// Remove @poulga from text (case insensitive)
		idx := strings.Index(lowerText, "@poulga")
		cleanText = strings.TrimSpace(cleanText[:idx] + cleanText[idx+len("@poulga"):])
	}
	// Also check mentionedJids
	botJid := os.Getenv("BOT_JID")
	if botJid == "" {
		botJid = "237620864894@s.whatsapp.net"
	}
	for _, jid := range mentionedJids {
		if jid == botJid || strings.Contains(jid, "237620864894") {
			isMentioned = true
		}
	}

	// ---- DETECT REPLY TO BOT ----
	isReplyToBot := false
	
	// Check if this message is a reply (has contextInfo with quotedMessage)
	hasReply := (m.ExtendedText != nil && m.ExtendedText.ContextInfo != nil && 
		m.ExtendedText.ContextInfo.QuotedMessage != nil) ||
		(m.ImageMessage != nil && m.ImageMessage.ContextInfo != nil && 
		m.ImageMessage.ContextInfo.QuotedMessage != nil) ||
		(ctxInfo != nil && ctxInfo.QuotedMessage != nil)
	
	if ctxInfo != nil && hasReply {
		// Check if the quoted message sender is the bot
		participant := ctxInfo.Participant
		if strings.Contains(participant, "237620864894") || participant == botJid {
			isReplyToBot = true
			fmt.Printf("[REPLY] Detected reply to Poulga from %s\n", data.PushName)
		}
	}

	isPrivateChat := !strings.HasSuffix(remoteJid, "@g.us")

	// ---- DETERMINE MEDIA TYPE ----
	mediaType := "text"
	if m.AudioMessage != nil {
		mediaType = "audio"
	} else if m.ImageMessage != nil {
		mediaType = "image"
	} else if m.VideoMessage != nil {
		mediaType = "video"
	} else if m.DocumentMessage != nil {
		mediaType = "document"
	}

	// ---- BUILD MESSAGE CONTEXT ----
	ctx := MessageContext{
		Instance:      instance,
		MsgId:         msgId,
		RemoteJid:     remoteJid,
		SenderJid:     senderJid,
		PushName:      data.PushName,
		Text:          cleanText,
		RawText:       text,
		MediaType:     mediaType,
		IsPrivateChat: isPrivateChat,
		IsGroupChat:   !isPrivateChat,
		IsMentioned:   isMentioned,
		IsReplyToBot:  isReplyToBot,
		QuotedText:    quotedText,
		QuotedSender:  quotedSender,
		QuotedMsgId:   quotedMsgId,
		MentionedJids: mentionedJids,
		Timestamp:     time.Now(),
	}

	// ======================================================================
	// ROUTING DECISION
	// ======================================================================

	// STEP 1: Check for commands (strict prefix . or !)
	cmd, cmdArgs, isCmd := ParseCommand(cleanText)
	if isCmd {
		fmt.Printf("[ROUTER] COMMAND: .%s args=%q from=%s\n", cmd, cmdArgs, data.PushName)
		go handleCommand(ctx, cmd, cmdArgs)
		return c.NoContent(http.StatusOK)
	}

	// STEP 2: Empty text after cleaning (just a mention with no text)
	if isMentioned && cleanText == "" {
		go sendWhatsAppMessage(instance, remoteJid, "Oui ? Dis-moi.", msgId, senderJid)
		return c.NoContent(http.StatusOK)
	}

	// STEP 3: Should the LLM respond?
	shouldRespond := isPrivateChat || isMentioned || isReplyToBot
	if !shouldRespond {
		return c.NoContent(http.StatusOK)
	}

	// STEP 4: Rate limiting
	if CheckRateLimit(senderJid, 10) { // Max 10 msgs/minute
		go sendWhatsAppMessage(instance, remoteJid, "Doucement, tu vas trop vite !", msgId, senderJid)
		return c.NoContent(http.StatusOK)
	}

	// STEP 5: Queue for LLM processing
	fmt.Printf("[ROUTER] LLM: text=%q from=%s mentioned=%v reply=%v private=%v\n",
		cleanText, data.PushName, isMentioned, isReplyToBot, isPrivateChat)

	jobQueue <- Job{Ctx: ctx}

	return c.NoContent(http.StatusOK)
}

// ============================================================================
// LLM RESPONSE PROCESSOR
// ============================================================================

func processLLMResponse(ctx MessageContext) {
	start := time.Now()

	// Acquire typing lock to prevent double processing
	if !AcquireTypingLock(ctx.RemoteJid, ctx.SenderJid) {
		fmt.Printf("[LLM] Typing lock active, skipping for %s\n", ctx.SenderJid)
		return
	}
	defer ReleaseTypingLock(ctx.RemoteJid, ctx.SenderJid)

	// Show typing indicator
	go sendTypingStatus(ctx.Instance, ctx.RemoteJid)

	// Fast reply check
	if fastReply, ok := GetFastReply(ctx.Text); ok {
		_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, fastReply, ctx.MsgId, ctx.SenderJid)
		return
	}

	// Detect intent
	intent := DetectIntent(ctx.Text, ctx.IsMentioned, ctx.IsReplyToBot)
	fmt.Printf("[LLM] Intent: %s (%.0fms)\n", intent, float64(time.Since(start).Microseconds())/1000)

	// Gather context from all memory levels
	history, _ := GetConversationContext(ctx.RemoteJid, 8)
	userMem, _ := GetUserMemory(ctx.SenderJid, ctx.RemoteJid)
	groupMem, _ := GetGroupMemory(ctx.RemoteJid)
	facts, _ := getFacts(ctx.RemoteJid)
	summary, _ := GetLatestSummary(ctx.RemoteJid)
	customPersona := GetGroupPersona(ctx.RemoteJid)

	// Limit facts to avoid token overflow
	if len(facts) > 3 {
		facts = facts[:3]
	}

	// Build prompt based on intent
	var prompt string
	switch intent {
	case IntentGreeting:
		prompt = BuildGreetingPrompt(ctx)
	case IntentQuestion:
		prompt = BuildQuestionPrompt(ctx, history, facts)
	case IntentStory:
		prompt = BuildStoryPrompt(ctx)
	case IntentCode:
		prompt = BuildCodePrompt(ctx)
	case IntentGame:
		prompt = BuildGamePrompt(ctx, history)
	case IntentSearch:
		prompt = BuildSearchPrompt(ctx, facts, "")
	case IntentSummary:
		prompt = BuildSummaryPrompt("", history)
	default: // IntentChat
		prompt = BuildChatPrompt(ctx, history, userMem, groupMem, facts, summary, customPersona)
	}

	// Call Ollama with intent-tuned parameters
	response, err := callOllamaWithIntent(prompt, intent, nil)
	if err != nil {
		fmt.Printf("[LLM] Ollama error: %v\n", err)
		response = "Hmm, je n'ai pas pu reflechir a ca. Reessaie."
	}

	// Clean response
	response = cleanResponse(response)

	elapsed := time.Since(start)
	fmt.Printf("[LLM] Response sent (intent=%s, %.1fs, %d chars)\n", intent, elapsed.Seconds(), len(response))

	// Send response
	_ = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, response, ctx.MsgId, ctx.SenderJid)

	// Save bot response to conversation history
	botJid := os.Getenv("BOT_JID")
	if botJid == "" {
		botJid = "237620864894@s.whatsapp.net"
	}
	go SaveMessage(ctx.RemoteJid, botJid, "Poulga", response, true, ctx.MsgId)
}

// ============================================================================
// GROUP PARTICIPANT EVENTS
// ============================================================================

func handleGroupParticipantUpdate(instance string, data json.RawMessage) {
	var groupUpdate struct {
		Id           string   `json:"id"`
		Participants []string `json:"participants"`
		Action       string   `json:"action"`
	}
	if err := json.Unmarshal(data, &groupUpdate); err != nil {
		return
	}

	settings, _ := getGroupSettings(groupUpdate.Id)

	for _, p := range groupUpdate.Participants {
		number := strings.Split(p, "@")[0]
		if groupUpdate.Action == "add" && settings.WelcomeEnabled {
			msg := fmt.Sprintf("Bienvenue @%s ! Tape .aide pour voir ce que je peux faire.", number)
			go sendWhatsAppMessage(instance, groupUpdate.Id, msg, "", p)
		} else if groupUpdate.Action == "remove" {
			msg := fmt.Sprintf("Au revoir @%s. A bientot !", number)
			go sendWhatsAppMessage(instance, groupUpdate.Id, msg, "", "")
		}
	}
}

// ============================================================================
// WEEKLY SUMMARY ENDPOINT (for n8n)
// ============================================================================

func handleWeeklySummary(c echo.Context) error {
	remoteJid := c.QueryParam("remoteJid")
	if remoteJid == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "remoteJid required"})
	}

	profiles, _ := getMemberProfiles(remoteJid)
	history, _ := GetConversationContext(remoteJid, 100)

	prompt := BuildSummaryPrompt(profiles, history)
	response, err := callOllamaWithIntent(prompt, IntentSummary, nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"summary": response})
}

// ============================================================================
// UTILITIES
// ============================================================================

func containsLink(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "www.")
}
