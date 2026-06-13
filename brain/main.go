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

	// DEBUG: Log raw message for replies analysis
	if data.PushName != "" && strings.Contains(strings.ToLower(text), "poulga") == false {
		b, _ := json.MarshalIndent(m, "", "  ")
		fmt.Printf("[DEBUG] RAW_MESSAGE from %s: %s\n", data.PushName, string(b))
	}

	// Extract context info (quoted messages)
	var ctxInfo *ContextInfo
	if m.ContextInfo != nil { // Check root ContextInfo first
		ctxInfo = m.ContextInfo
	} else if m.ExtendedTextMessage != nil && m.ExtendedTextMessage.ContextInfo != nil {
		ctxInfo = m.ExtendedTextMessage.ContextInfo
	} else if m.ImageMessage != nil && m.ImageMessage.ContextInfo != nil {
		ctxInfo = m.ImageMessage.ContextInfo
	} else if m.VideoMessage != nil && m.VideoMessage.ContextInfo != nil {
		ctxInfo = m.VideoMessage.ContextInfo
	} else if m.AudioMessage != nil && m.AudioMessage.ContextInfo != nil {
		ctxInfo = m.AudioMessage.ContextInfo
	} else if m.DocumentMessage != nil && m.DocumentMessage.ContextInfo != nil {
		ctxInfo = m.DocumentMessage.ContextInfo
	}

	// DEBUG: Log contextInfo details
	if ctxInfo != nil {
		fmt.Printf("[DEBUG] CONTEXT_INFO: Participant=%q StanzaId=%q QuotedMsg=%v MentionedJids=%v\n",
			ctxInfo.Participant, ctxInfo.StanzaId, ctxInfo.QuotedMessage != nil, len(ctxInfo.MentionedJid))
	}

	// Build quoted info
	quotedText := ""
	quotedSender := ""
	quotedMsgId := ""
	var mentionedJids []string

	if ctxInfo != nil {
		quotedSender = ctxInfo.Participant
		quotedMsgId = ctxInfo.StanzaId
		mentionedJids = ctxInfo.MentionedJid

		// Extract quotedText from ctxInfo.QuotedMessage (which is 'any')
		if ctxInfo.QuotedMessage != nil {
			quotedMsgJSON, err := json.Marshal(ctxInfo.QuotedMessage)
			if err == nil {
				// The quotedMessage is a full MessageContent structure
				var qm struct {
					Conversation        string `json:"conversation"`
					ExtendedTextMessage struct {
						Text string `json:"text"`
					} `json:"extendedTextMessage"`
					ImageMessage struct {
						Caption string `json:"caption"`
					} `json:"imageMessage"`
					VideoMessage struct {
						Caption string `json:"caption"`
					} `json:"videoMessage"`
				}
				if json.Unmarshal(quotedMsgJSON, &qm) == nil {
					if qm.Conversation != "" {
						quotedText = qm.Conversation
					} else if qm.ExtendedTextMessage.Text != "" {
						quotedText = qm.ExtendedTextMessage.Text
					} else if qm.ImageMessage.Caption != "" {
						quotedText = "[Image] " + qm.ImageMessage.Caption
					} else if qm.VideoMessage.Caption != "" {
						quotedText = "[Video] " + qm.VideoMessage.Caption
					}
				}
			}
		}
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
	go SaveMessage(msgId, remoteJid, senderJid, data.PushName, text, false, quotedMsgId)

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
	hasReply := (ctxInfo != nil && ctxInfo.QuotedMessage != nil)

	fmt.Printf("[DEBUG] REPLY_CHECK: hasReply=%v ctxInfo=%v quotedMsg=%v\n",
		hasReply, ctxInfo != nil, ctxInfo != nil && ctxInfo.QuotedMessage != nil)

	if ctxInfo != nil && hasReply {
		// Method 1: Check participant field (standard)
		participant := ctxInfo.Participant
		
		// If participant is empty (happens in some Evolution API versions for private chats), 
		// fallback to checking the quoted message metadata if available
		
		isBotParticipant := strings.Contains(participant, "237620864894") || (botJid != "" && participant == botJid)
		
		if isBotParticipant {
			isReplyToBot = true
			fmt.Printf("[DEBUG] REPLY_METHOD1_MATCHED\n")
		}

		// Method 2: Check stanzaId in our database
		if !isReplyToBot && ctxInfo.StanzaId != "" {
			fmt.Printf("[DEBUG] REPLY_METHOD2: checking stanzaId=%q in DB\n", ctxInfo.StanzaId)
			if IsMessageFromBot(remoteJid, ctxInfo.StanzaId) {
				isReplyToBot = true
				fmt.Printf("[DEBUG] REPLY_METHOD2_MATCHED\n")
			}
		}

		// Method 3: Text content matching (last resort)
		if !isReplyToBot && quotedText != "" {
			debugQuotedText := quotedText
			if len(debugQuotedText) > 50 {
				debugQuotedText = debugQuotedText[:50]
			}
			fmt.Printf("[DEBUG] REPLY_METHOD3: checking quotedText=%q\n", debugQuotedText)
			if IsRecentBotMessage(remoteJid, quotedText) {
				isReplyToBot = true
				fmt.Printf("[DEBUG] REPLY_METHOD3_MATCHED\n")
			}
		}

		if isReplyToBot {
			fmt.Printf("[REPLY] Reply to Poulga detected from %s (participant=%q, stanzaId=%s)\n",
				data.PushName, participant, ctxInfo.StanzaId)
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
		botMsgId, _ := sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, fastReply, ctx.MsgId, ctx.SenderJid)
		if botMsgId != "" {
			go saveBotResponse(ctx, fastReply, botMsgId)
		}
		return
	}

	// ======================================================================
	// STEP 1: PRE-LLM TOOL DETECTION (keyword-based, instant)
	// ======================================================================
	if tc, ok := DetectToolFromKeywords(ctx.Text); ok {
		fmt.Printf("[TOOL] PRE-LLM keyword match: tool=%q args=%q\n", tc.Tool, tc.Args)
		result := ExecuteTool(tc, ctx)
		if result.Response != "" {
			botMsgId, _ := sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, result.Response, ctx.MsgId, ctx.SenderJid)
			if botMsgId != "" {
				go saveBotResponse(ctx, result.Response, botMsgId)
			}
		}
		elapsed := time.Since(start)
		fmt.Printf("[TOOL] Completed in %.1fs (pre-LLM)\n", elapsed.Seconds())
		return
	}

	// ======================================================================
	// STEP 2: INTENT DETECTION + CONTEXT GATHERING
	// ======================================================================
	intent := DetectIntent(ctx.Text, ctx.IsMentioned, ctx.IsReplyToBot)
	fmt.Printf("[LLM] Intent: %s (%.0fms)\n", intent, float64(time.Since(start).Microseconds())/1000)

	// Adaptive history: more context when user replies to bot (thread continuation)
	historyLimit := 10
	if ctx.IsReplyToBot {
		historyLimit = 15
	}
	history, _ := GetConversationContext(ctx.RemoteJid, historyLimit)
	userMem, _ := GetUserMemory(ctx.SenderJid, ctx.RemoteJid)
	groupMem, _ := GetGroupMemory(ctx.RemoteJid)
	groupFacts, err := GetGroupFacts(ctx.RemoteJid)
	if err != nil {
		fmt.Printf("[DB] Error getting group facts: %v\n", err)
	}
	summary, _ := GetLatestSummary(ctx.RemoteJid)
	customPersona := GetGroupPersona(ctx.RemoteJid)

	// Convert GroupMemoryEntry slice to string slice for legacy Builders if needed
	var factsLegacy []string
	for _, f := range groupFacts {
		factsLegacy = append(factsLegacy, fmt.Sprintf("%s: %s", f.Key, f.Value))
	}

	// ======================================================================
	// STEP 3: BUILD PROMPT (with Tool Catalog for chat/question intents)
	// ======================================================================
	var prompt string
	switch intent {
	case IntentGreeting:
		prompt = BuildGreetingPrompt(ctx)
	case IntentQuestion:
		prompt = BuildQuestionPrompt(ctx, history, factsLegacy)
	case IntentStory:
		prompt = BuildStoryPrompt(ctx)
	case IntentCode:
		prompt = BuildCodePrompt(ctx)
	case IntentGame:
		prompt = BuildGamePrompt(ctx, history)
	case IntentSearch:
		prompt = BuildSearchPrompt(ctx, "")
	case IntentSummary:
		prompt = BuildSummaryPrompt(ctx, "", history)
	default: // IntentChat — inject tool catalog
		prompt = BuildChatPrompt(ctx, history, userMem, groupMem, factsLegacy, summary, customPersona)
		// Inject tool catalog so LLM can output structured tool calls
		prompt = prompt + "\n\n" + ToolCatalog
	}

	// ======================================================================
	// STEP 4: CALL LLM
	// ======================================================================
	response, err := callOllamaWithIntent(prompt, intent, nil)
	if err != nil {
		fmt.Printf("[LLM] ALL backends error: %v\n", err)
		response = "Hmm, je n'ai pas pu reflechir a ca. Reessaie."
	}

	fmt.Printf("[RAW OLLAMA] %q\n", response)

	// ======================================================================
	// STEP 5: POST-LLM TOOL DETECTION (parse LLM structured output)
	// ======================================================================
	if tc, ok := ParseToolCall(response); ok {
		fmt.Printf("[TOOL] POST-LLM tool call detected: tool=%q args=%q\n", tc.Tool, tc.Args)
		result := ExecuteTool(tc, ctx)
		if result.Response != "" {
			botMsgId, _ := sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, result.Response, ctx.MsgId, ctx.SenderJid)
			if botMsgId != "" {
				go saveBotResponse(ctx, result.Response, botMsgId)
			}
		}
		elapsed := time.Since(start)
		fmt.Printf("[TOOL] Completed in %.1fs (post-LLM)\n", elapsed.Seconds())
		return
	}

	// ======================================================================
	// STEP 6: CLEAN + SEND (normal chat response, no tool call)
	// ======================================================================
	fmt.Printf("[BEFORE CLEAN] %q\n", response)
	response = cleanResponse(response)
	fmt.Printf("[AFTER CLEAN] %q\n", response)

	elapsed := time.Since(start)
	fmt.Printf("[LLM] Response sent (intent=%s, %.1fs, %d chars)\n", intent, elapsed.Seconds(), len(response))

	fmt.Printf("[SEND] %q\n", response)
	botMsgId, _ := sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, response, ctx.MsgId, ctx.SenderJid)

	if botMsgId != "" {
		go saveBotResponse(ctx, response, botMsgId)
	}
}

// saveBotResponse saves the bot's response to conversation history
func saveBotResponse(ctx MessageContext, response string, botMsgId string) {
	botJid := os.Getenv("BOT_JID")
	if botJid == "" {
		botJid = "237620864894@s.whatsapp.net"
	}
	SaveMessage(botMsgId, ctx.RemoteJid, botJid, "Poulga", response, true, ctx.MsgId)
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

	// Provide a dummy context for the summary builder
	dummyCtx := MessageContext{RemoteJid: remoteJid}

	prompt := BuildSummaryPrompt(dummyCtx, profiles, history)
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
