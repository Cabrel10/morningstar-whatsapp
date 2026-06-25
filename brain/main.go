package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/ollama/ollama/api"
)

var (
	jobQueue = make(chan Job, 100)

	botJidsMutex sync.RWMutex
	botJidsMap   = map[string]bool{
		"237620864894@s.whatsapp.net": true,
	}
)

func registerBotJid(jid string) {
	if jid == "" {
		return
	}
	// Save to local map for speed
	botJidsMutex.Lock()
	botJidsMap[jid] = true
	botJidsMutex.Unlock()

	// Persist to Redis
	ctx := context.Background()
	_ = rdb.SAdd(ctx, "bot:jids", jid).Err()

	fmt.Printf("[INIT] Poulga a enregistré son propre ID/LID : %s\n", jid)
}

func isBotJid(jid string) bool {
	if jid == "" {
		return false
	}
	botJidsMutex.RLock()
	known := botJidsMap[jid]
	botJidsMutex.RUnlock()

	if known {
		return true
	}

	// Check Redis fallback
	ctx := context.Background()
	exists, _ := rdb.SIsMember(ctx, "bot:jids", jid).Result()
	if exists {
		// Update local cache
		botJidsMutex.Lock()
		botJidsMap[jid] = true
		botJidsMutex.Unlock()
		return true
	}

	// Pattern match for our known number or typical bot suffixes/prefixes
	return strings.Contains(jid, "237620864894") || strings.HasPrefix(jid, "lid_") || strings.Contains(jid, "bot")
}

func loadBotJids() {
	ctx := context.Background()
	jids, err := rdb.SMembers(ctx, "bot:jids").Result()
	if err == nil {
		botJidsMutex.Lock()
		for _, jid := range jids {
			botJidsMap[jid] = true
		}
		botJidsMutex.Unlock()
		fmt.Printf("[INIT] %d JIDs/LIDs bot chargés depuis Redis\n", len(jids))
	}
}

func main() {
	fmt.Println("=== MorningStar Brain v2.2 [ALPHA POSTURE] starting ===")

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

	// Load known bot JIDs
	loadBotJids()

	// Start workers
	go ollamaWorker()
	go runDailyCleanup()
	go runPeriodicCompression()

	// HTTP Server
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())

	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, "MorningStar Brain v2.1 OK")
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

func runPeriodicCompression() {
	for {
		time.Sleep(6 * time.Hour)
		fmt.Println("[COMPRESSION] Periodic compression check...")
	}
}

// ============================================================================
// WEBHOOK HANDLER
// ============================================================================

func handleWebhook(c echo.Context) error {
	// RAW DEBUG: Capture tout ce qui sort de Evolution
	var bodyBytes []byte
	if c.Request().Body != nil {
		bodyBytes, _ = io.ReadAll(c.Request().Body)
		// Remettre le body pour le bind ultérieur
		c.Request().Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if len(bodyBytes) > 0 {
		fmt.Printf("\n--- [RAW WEBHOOK START] ---\n%s\n--- [RAW WEBHOOK END] ---\n\n", string(bodyBytes))
	}

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
		botJid := data.Key.Participant
		if botJid == "" {
			botJid = data.Key.RemoteJid
		}
		// Fallback: learn from contextInfo if available
		if data.ContextInfo != nil && data.ContextInfo.Participant != "" {
			registerBotJid(data.ContextInfo.Participant)
		}
		
		if botJid != "" {
			registerBotJid(botJid)
		}
		return c.NoContent(http.StatusOK)
	}

	// Extract identifiers
	remoteJid := data.Key.RemoteJid
	senderJid := data.Key.Participant
	msgId := data.Key.Id
	if senderJid == "" {
		senderJid = remoteJid
	}

	fmt.Printf("[WEBHOOK] INCOMING: id=%s from=%s push=%s remote=%s\n", msgId, senderJid, data.PushName, remoteJid)

	// Dedup
	if IsDuplicateMessage(msgId) {
		return c.NoContent(http.StatusOK)
	}

	// Parse message content
	var m MessageContent
	_ = json.Unmarshal(data.Message, &m)
	text := GetMessageText(data.Message)

	// DEBUG: Log RAW JSON for ALL messages to diagnose reply detection
	rawJSON := string(data.Message)
	hasCtxInRaw := strings.Contains(rawJSON, "contextInfo")
	hasQuotedInRaw := strings.Contains(rawJSON, "quotedMessage")
	hasStanzaInRaw := strings.Contains(rawJSON, "stanzaId")

	if hasCtxInRaw || hasQuotedInRaw || hasStanzaInRaw {
		fmt.Printf("[DEBUG] RAW_REPLY_DETECTED from %s: contextInfo=%v quotedMsg=%v stanzaId=%v\n",
			data.PushName, hasCtxInRaw, hasQuotedInRaw, hasStanzaInRaw)
		if len(rawJSON) < 2000 {
			fmt.Printf("[DEBUG] RAW_JSON: %s\n", rawJSON)
		} else {
			fmt.Printf("[DEBUG] RAW_JSON (truncated): %s...\n", rawJSON[:2000])
		}
	}

	// Extract context info (quoted messages)
	// Evolution API v2.3.7 puts contextInfo inside extendedTextMessage for replies
	var ctxInfo *ContextInfo
	if m.ExtendedTextMessage != nil && m.ExtendedTextMessage.ContextInfo != nil {
		ctxInfo = m.ExtendedTextMessage.ContextInfo
		fmt.Printf("[DEBUG] CTX_SOURCE: extendedTextMessage.contextInfo\n")
	} else if m.ImageMessage != nil && m.ImageMessage.ContextInfo != nil {
		ctxInfo = m.ImageMessage.ContextInfo
		fmt.Printf("[DEBUG] CTX_SOURCE: imageMessage.contextInfo\n")
	} else if m.VideoMessage != nil && m.VideoMessage.ContextInfo != nil {
		ctxInfo = m.VideoMessage.ContextInfo
		fmt.Printf("[DEBUG] CTX_SOURCE: videoMessage.contextInfo\n")
	} else if m.AudioMessage != nil && m.AudioMessage.ContextInfo != nil {
		ctxInfo = m.AudioMessage.ContextInfo
		fmt.Printf("[DEBUG] CTX_SOURCE: audioMessage.contextInfo\n")
	} else if m.DocumentMessage != nil && m.DocumentMessage.ContextInfo != nil {
		ctxInfo = m.DocumentMessage.ContextInfo
		fmt.Printf("[DEBUG] CTX_SOURCE: documentMessage.contextInfo\n")
	} else if m.ContextInfo != nil {
		ctxInfo = m.ContextInfo
		fmt.Printf("[DEBUG] CTX_SOURCE: root contextInfo\n")
	}

	// FALLBACK: Evolution API v2.x puts contextInfo at the data level
	if ctxInfo == nil && data.ContextInfo != nil {
		ctxInfo = data.ContextInfo
		fmt.Printf("[DEBUG] CTX_SOURCE: data.contextInfo (Evolution v2.x)\n")
	}

	// FALLBACK 1.5: Evolution v2.3.7 flattened fields
	if ctxInfo == nil && data.StanzaId != "" {
		ctxInfo = &ContextInfo{
			StanzaId:      data.StanzaId,
			Participant:   data.Participant,
			MentionedJid:  data.MentionedJid,
			QuotedMessage: data.QuotedMessage,
		}
		fmt.Printf("[DEBUG] CTX_SOURCE: flattened data fields (Evolution v2.3.7)\n")
	}

	// FALLBACK 2: If no contextInfo found in struct but raw JSON has it
	if ctxInfo == nil && hasCtxInRaw {
		ctxInfo = extractContextInfoFromRaw(data.Message)
		if ctxInfo == nil {
			// Try parsing the whole data block if message block didn't have it
			ctxInfo = extractContextInfoFromRaw(payload.Data)
		}
		if ctxInfo != nil {
			fmt.Printf("[DEBUG] CTX_SOURCE: FALLBACK raw JSON extraction\n")
		}
	}

	// DEBUG: Log contextInfo details
	if ctxInfo != nil {
		fmt.Printf("[DEBUG] CONTEXT_INFO: Participant=%q StanzaId=%q HasQuotedMsg=%v MentionedJids=%v\n",
			ctxInfo.Participant, ctxInfo.StanzaId, ctxInfo.QuotedMessage != nil, ctxInfo.MentionedJid)
	} else if hasCtxInRaw {
		// We KNOW contextInfo is in the raw JSON but we couldn't parse it
		// Log the full raw JSON for diagnosis (only for replies we're missing)
		fmt.Printf("[BUG] CONTEXT_INFO_LOST: raw JSON has contextInfo but all 5 parsers failed!\n")
		if len(rawJSON) < 3000 {
			fmt.Printf("[BUG] FULL_RAW_MSG: %s\n", rawJSON)
		}
		// Also dump data-level raw
		dataRaw, _ := json.Marshal(payload.Data)
		if len(dataRaw) < 3000 {
			fmt.Printf("[BUG] FULL_RAW_DATA: %s\n", string(dataRaw))
		}
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

		// Extract quotedText from ctxInfo.QuotedMessage
		if ctxInfo.QuotedMessage != nil {
			quotedMsgJSON, err := json.Marshal(ctxInfo.QuotedMessage)
			if err == nil {
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

	// ---- DETECT MENTION ----
	cleanText := strings.TrimSpace(text)
	isMentioned := false
	lowerText := strings.ToLower(cleanText)

	// Check text for @poulga (case insensitive)
	if strings.Contains(lowerText, "@poulga") {
		isMentioned = true
		idx := strings.Index(lowerText, "@poulga")
		// Clean up the text by removing the mention and any immediate symbols/spaces
		before := cleanText[:idx]
		after := cleanText[idx+len("@poulga"):]
		after = strings.TrimLeft(after, " ,:;!?")
		cleanText = strings.TrimSpace(before + " " + after)
	}

	// Also check mentionedJids from Evolution API
	botJid := os.Getenv("BOT_JID")
	if botJid == "" {
		botJid = "237620864894@s.whatsapp.net"
	}
	
	// Check if bot is mentioned via JID or any known LID/bot ID
	for _, jid := range mentionedJids {
		if jid == botJid || strings.Contains(jid, "237620864894") || isBotJid(jid) {
			isMentioned = true
		}
	}

	// ---- DETECT REPLY TO BOT ----
	isReplyToBot := false

	hasReply := ctxInfo != nil && (ctxInfo.QuotedMessage != nil || ctxInfo.StanzaId != "")

	if hasReply {
		// Method 1: Check participant field directly (supporting LIDs)
		participant := quotedSender
		if isBotJid(participant) || strings.Contains(participant, "237620864894") {
			isReplyToBot = true
			fmt.Printf("[REPLY] METHOD1_MATCHED: participant=%q matches bot\n", participant)
		}

		// Method 2: Check stanzaId in our database
		if !isReplyToBot && ctxInfo.StanzaId != "" {
			if IsMessageFromBot(remoteJid, ctxInfo.StanzaId) {
				isReplyToBot = true
				fmt.Printf("[REPLY] METHOD2_MATCHED: stanzaId found in bot messages\n")
			}
		}

		// Method 3: Text content matching (last resort)
		if !isReplyToBot && quotedText != "" {
			if IsRecentBotMessage(remoteJid, quotedText) {
				isReplyToBot = true
				fmt.Printf("[REPLY] METHOD3_MATCHED: quotedText found in recent bot messages\n")
			}
		}

		// Method 4: Private chat fallback
		if !isReplyToBot && !strings.HasSuffix(remoteJid, "@g.us") && ctxInfo.StanzaId != "" {
			isReplyToBot = true
			fmt.Printf("[REPLY] METHOD4_MATCHED: private chat reply fallback\n")
		}
	}
	
	fmt.Printf("[REPLY] FINAL DETECTION: isMentioned=%v isReplyToBot=%v\n", isMentioned, isReplyToBot)

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

	// ---- ANTI-LINK CHECK ----
	settings, _ := getGroupSettings(remoteJid)
	if settings.AntiLinkEnabled && containsLink(text) {
		isAdmin, _ := isUserAdmin(instance, remoteJid, senderJid)
		if !isAdmin {
			go deleteMessage(instance, remoteJid, msgId)
			warnMsg := fmt.Sprintf("@%s, les liens ne sont pas autorisés ici.", strings.Split(senderJid, "@")[0])
			go sendAndSaveBotMessage(ctx, warnMsg, "")
			return c.NoContent(http.StatusOK)
		}
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

	// STEP 1.5: Automatic game move detection (no prefix)
	if isReplyToBot {
		// Morpion: "1,1"
		if regexp.MustCompile(`^[1-3],[1-3]$`).MatchString(cleanText) {
			fmt.Printf("[ROUTER] AUTO_MORPION_MOVE: %s from=%s\n", cleanText, data.PushName)
			go handleMorpionGame(ctx, cleanText)
			return c.NoContent(http.StatusOK)
		}
		// Pendu: single letter
		if regexp.MustCompile(`^[a-zA-Z]$`).MatchString(cleanText) {
			fmt.Printf("[ROUTER] AUTO_PENDU_GUESS: %s from=%s\n", cleanText, data.PushName)
			go handlePenduGame(ctx, cleanText)
			return c.NoContent(http.StatusOK)
		}
		// Quiz: single digit
		if regexp.MustCompile(`^[1-9]$`).MatchString(cleanText) {
			fmt.Printf("[ROUTER] AUTO_QUIZ_ANSWER: %s from=%s\n", cleanText, data.PushName)
			go handleQuizGame(ctx, cleanText)
			return c.NoContent(http.StatusOK)
		}
	}

	// STEP 2: Empty text after cleaning (just a mention with no text)
	if isMentioned && cleanText == "" {
		go sendAndSaveBotMessage(ctx, "Oui ? Dis-moi.", msgId)
		return c.NoContent(http.StatusOK)
	}

	// STEP 3: Should the LLM respond?
	// PERFORMANCE: Skip audio messages for now as we don't have a reliable transcriber
	if mediaType == "audio" && cleanText == "" {
		fmt.Printf("[ROUTER] SKIPPING audio message from %s (no transcription)\n", data.PushName)
		return c.NoContent(http.StatusOK)
	}

	shouldRespond := isPrivateChat || isMentioned || isReplyToBot
	if !shouldRespond {
		return c.NoContent(http.StatusOK)
	}

	// STEP 4: Rate limiting
	if CheckRateLimit(senderJid, 10) {
		go sendAndSaveBotMessage(ctx, "Doucement, tu vas trop vite !", msgId)
		return c.NoContent(http.StatusOK)
	}

	// STEP 5: Queue for LLM processing
	fmt.Printf("[ROUTER] → LLM QUEUE: text=%q from=%s mentioned=%v reply=%v private=%v\n",
		cleanText, data.PushName, isMentioned, isReplyToBot, isPrivateChat)

	jobQueue <- Job{Ctx: ctx}
	return c.NoContent(http.StatusOK)
}

// ============================================================================
// LLM RESPONSE PROCESSOR
// ============================================================================

func processLLMResponse(ctx MessageContext) {
	start := time.Now()

	// Acquire typing lock
	if !AcquireTypingLock(ctx.RemoteJid, ctx.SenderJid) {
		return
	}
	defer ReleaseTypingLock(ctx.RemoteJid, ctx.SenderJid)

	// Show typing indicator
	go sendTypingStatus(ctx.Instance, ctx.RemoteJid)

	// ======================================================================
	// STEP 1: CONTEXT GATHERING
	// ======================================================================
	intent := DetectIntent(ctx.Text, ctx.IsMentioned, ctx.IsReplyToBot)

	// Adaptive history: more context for ongoing conversations
	historyLimit := 15
	if ctx.IsReplyToBot {
		historyLimit = 20
	}
	history, _ := GetConversationContext(ctx.RemoteJid, historyLimit)
	userMem, _ := GetUserMemory(ctx.SenderJid, ctx.RemoteJid)
	groupFacts, _ := GetGroupFacts(ctx.RemoteJid)
	summary, _ := GetLatestSummary(ctx.RemoteJid)
	humeur := GetGroupHumeur(ctx.RemoteJid)

	var factsLegacy []string
	for _, f := range groupFacts {
		factsLegacy = append(factsLegacy, fmt.Sprintf("%s: %s", f.Key, f.Value))
	}

	// ======================================================================
	// STEP 2: BUILD STRUCTURED MESSAGES
	//
	// ARCHITECTURE (2026-06-25 fix):
	//   Message[0] = system prompt (personality + user profile + group facts)
	//   Message[1..N] = conversation history (proper role alternation)
	//   Message[N+1] = current user message
	//
	// PREVIOUS BUG: The system prompt via BuildChatPromptWithHumeur()
	// already contained the full conversation history as text, then
	// the same history was ALSO added as separate messages. This doubled
	// the history, wasting ~2000 tokens out of 4096 and drowning the
	// personality instructions. Now the system prompt ONLY contains the
	// personality + context (user profile, facts, summary), and history
	// is ONLY provided as structured messages with proper roles.
	// ======================================================================

	// Build the system prompt WITHOUT history (personality + context only)
	basePrompt := BuildChatPromptWithHumeur(ctx, nil, userMem, nil, factsLegacy, summary, "", humeur)

	messages := []api.Message{
		{Role: "system", Content: basePrompt},
	}

	// Add conversation history as properly structured messages
	// This gives the model real conversational context with correct roles
	for _, h := range history {
		role := "user"
		content := h.Message
		if h.IsFromBot {
			role = "assistant"
		} else {
			senderName := GetMemberName(h.SenderJid, h.GroupJid, h.SenderName)
			content = fmt.Sprintf("[%s]: %s", senderName, h.Message)
		}
		messages = append(messages, api.Message{Role: role, Content: content})
	}

	// Add the current user message with their name for identity
	currentUserName := GetMemberName(ctx.SenderJid, ctx.RemoteJid, ctx.PushName)
	currentMsg := fmt.Sprintf("[%s]: %s", currentUserName, ctx.Text)
	if ctx.QuotedText != "" {
		quotedAuthor := "quelqu\'un"
		if ctx.QuotedSender != "" {
			quotedAuthor = GetMemberName(ctx.QuotedSender, ctx.RemoteJid, strings.Split(ctx.QuotedSender, "@")[0])
			if strings.Contains(ctx.QuotedSender, "237620864894") || isBotJid(ctx.QuotedSender) {
				quotedAuthor = "Poulga (Toi)"
			}
		}
		currentMsg = fmt.Sprintf("[%s] (en réponse à %s: \"%s\"): %s", currentUserName, quotedAuthor, ctx.QuotedText, ctx.Text)
	}
	messages = append(messages, api.Message{Role: "user", Content: currentMsg})

	fmt.Printf("[DEBUG] AGENT LOOP: %d messages, intent=%s, model context budget well-managed\n", len(messages), intent)

	// ======================================================================
	// STEP 3: NATIVE AGENT LOOP
	//
	// The LLM can call tools (search, admin actions, etc.) for up to
	// maxIterations. Each tool result is fed back as a "tool" message
	// so the model can synthesize the final response.
	// ======================================================================
	maxIterations := 3
	tools := GetOllamaTools()
	var finalResponse string

	for i := 0; i < maxIterations; i++ {
		resp, err := ChatWithOllama(messages, intent, tools)
		if err != nil {
			fmt.Printf("[AGENT] Ollama error: %v\n", err)
			finalResponse = "Désolée, mon cerveau a eu un hoquet technique."
			break
		}

		// Add assistant's response to conversation
		messages = append(messages, *resp)

		if len(resp.ToolCalls) > 0 {
			fmt.Printf("[AGENT] Iteration %d: %d tool calls\n", i+1, len(resp.ToolCalls))

			for _, call := range resp.ToolCalls {
				fmt.Printf("[AGENT] Calling tool: %s with args: %v\n", call.Function.Name, call.Function.Arguments)

				// GENERIC arg extraction via buildToolCallArgs (2026-06-25 fix)
				// Previously: fragile per-tool if/else that missed many tools
				argBytes, _ := json.Marshal(call.Function.Arguments)
				var argsMap map[string]any
				json.Unmarshal(argBytes, &argsMap)

				tc := ToolCall{
					Tool: call.Function.Name,
					Args: buildToolCallArgs(call.Function.Name, argsMap),
				}

				result := ExecuteTool(tc, ctx)

				// Truncate oversized results to preserve context window
				resText := result.Response
				if len(resText) > 3000 {
					resText = resText[:3000] + "\n\n[TRONQUÉ...]"
				}

				// Feed tool result back to the model
				messages = append(messages, api.Message{
					Role:    "tool",
					Content: resText,
				})
			}
			// Continue loop so model can synthesize after tool results
		} else {
			// No tools called — this is the final text response
			finalResponse = resp.Content

			// FALLBACK: If the model didn't use native tool calling but wrote
			// a JSON tool call in its text, try to parse and execute it
			if tc, found := ParseToolCall(finalResponse); found {
				fmt.Printf("[AGENT] FALLBACK: Parsed text-based tool call: %s\n", tc.Tool)
				result := ExecuteTool(tc, ctx)
				if result.Success {
					// Re-run with tool result
					messages = append(messages, api.Message{
						Role:    "tool",
						Content: result.Response,
					})
					// One more iteration to synthesize
					resp2, err2 := ChatWithOllama(messages, intent, nil)
					if err2 == nil && resp2 != nil {
						finalResponse = resp2.Content
					}
				}
			}
			break
		}
	}

	// ======================================================================
	// STEP 4: CLEAN + RESOLVE MENTIONS + SEND
	// ======================================================================
	finalResponse = cleanResponse(finalResponse)
	finalResponse, mentions := sanitizeJidsInText(finalResponse, ctx.RemoteJid)

	// Convert friendly "@Prénom" tags written by the model into real mentions
	finalResponse, nameMentions := resolveNameMentions(finalResponse, ctx.RemoteJid)
	if len(nameMentions) > 0 {
		seen := make(map[string]bool)
		for _, m := range mentions {
			seen[m] = true
		}
		for _, m := range nameMentions {
			if !seen[m] {
				mentions = append(mentions, m)
				seen[m] = true
			}
		}
	}

	if finalResponse == "" || finalResponse == "..." {
		finalResponse = "Je n\'ai pas pu formuler une réponse claire. Peux-tu reformuler ?"
	}

	elapsed := time.Since(start)
	fmt.Printf("[LLM] Final response ready (intent=%s, %.1fs)\n", intent, elapsed.Seconds())

	var botMsgId string
	var err error
	if len(mentions) > 0 {
		botMsgId, err = sendWhatsAppMessageWithMentions(ctx.Instance, ctx.RemoteJid, finalResponse, mentions)
	} else {
		botMsgId, err = sendWhatsAppMessage(ctx.Instance, ctx.RemoteJid, finalResponse, ctx.MsgId, ctx.SenderJid)
	}

	if err == nil && botMsgId != "" {
		go saveBotResponse(ctx, finalResponse, botMsgId)
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
			style := ResponseStyle{
				Title:      "Nouveau Membre",
				TitleEmoji: "🎉",
				Sections: []Section{
					{Content: fmt.Sprintf("Bienvenue @%s dans le groupe !", number)},
					{
						Title:      "Pour commencer",
						TitleEmoji: "💡",
						Items: []string{
							"`.je-suis [Ton Prénom]` — Présente-toi",
							"`.aide` — Voir toutes mes commandes",
						},
					},
				},
				Footer: "— Poulga, ton assistante IA",
			}
			botMsgId, _ := sendWhatsAppMessage(instance, groupUpdate.Id, RenderWhatsApp(style), "", p)
			if botMsgId != "" {
				botJid := os.Getenv("BOT_JID")
				if botJid == "" { botJid = "237620864894@s.whatsapp.net" }
				_ = SaveMessage(botMsgId, groupUpdate.Id, botJid, "Poulga", "[Bienvenue @"+number+"]", true, "")
			}
		} else if groupUpdate.Action == "remove" {
			msg := fmt.Sprintf("👋 Au revoir @%s. À bientôt !", number)
			botMsgId, _ := sendWhatsAppMessage(instance, groupUpdate.Id, msg, "", "")
			if botMsgId != "" {
				botJid := os.Getenv("BOT_JID")
				if botJid == "" { botJid = "237620864894@s.whatsapp.net" }
				_ = SaveMessage(botMsgId, groupUpdate.Id, botJid, "Poulga", msg, true, "")
			}
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
	dummyCtx := MessageContext{RemoteJid: remoteJid}
	prompt := BuildSummaryPrompt(dummyCtx, profiles, history)
	response, err := callOllamaWithIntent(prompt, IntentSummary, nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"summary": response})
}

// ============================================================================
// JID/LID SANITIZER — Replace technical identifiers with human names
// ============================================================================

// resolveNameMentions scans the LLM output for "@Prénom" tokens and converts them
// into real WhatsApp mentions. WhatsApp requires the *number* in the text plus the
// full JID in the "mentioned" array; the client then displays the contact's name.
// So Poulga writes a friendly "@Morningstar" and we translate it to "@237..." + JID.
//
// Matching is case-insensitive and accent-insensitive on the first word of each
// member's display name (custom name > push name). The longest matching name wins,
// so "@Ken~v Sama" is preferred over "@Ken" when both exist.
func resolveNameMentions(text string, groupJid string) (string, []string) {
	if text == "" || !strings.Contains(text, "@") {
		return text, nil
	}

	members, err := GetGroupMembersDetailed(groupJid)
	if err != nil || len(members) == 0 {
		return text, nil
	}

	// Build name -> JID map. Use the resolved display name (custom > push).
	type cand struct {
		norm string // normalised name (lowercase, no accents, no spaces)
		jid  string
		num  string
	}
	var cands []cand
	seen := make(map[string]bool)
	for _, m := range members {
		display := GetMemberName(m.Jid, groupJid, m.PushName)
		if display == "" {
			continue
		}
		num := strings.Split(m.Jid, "@")[0]
		if strings.Contains(num, ":") {
			num = strings.Split(num, ":")[0]
		}
		// Full normalised name + first-token normalised name as fallbacks.
		full := normalizeName(display)
		first := normalizeName(strings.Fields(display)[0])
		for _, key := range []string{full, first} {
			if key == "" || seen[key+"|"+m.Jid] {
				continue
			}
			seen[key+"|"+m.Jid] = true
			cands = append(cands, cand{norm: key, jid: m.Jid, num: num})
		}
	}
	// Longest name first so multi-word names match before their first token.
	sort.Slice(cands, func(i, j int) bool { return len(cands[i].norm) > len(cands[j].norm) })

	mentionsMap := make(map[string]bool)
	// Match "@Word" or "@Word Word" (up to the candidate's token count).
	atRegex := regexp.MustCompile(`@([\p{L}][\p{L}0-9 _~.'-]{0,40})`)
	out := atRegex.ReplaceAllStringFunc(text, func(match string) string {
		raw := strings.TrimPrefix(match, "@")
		normCandidate := normalizeName(raw)
		if normCandidate == "" {
			return match
		}
		for _, c := range cands {
			// Match if the candidate name is a prefix of what the model wrote
			// (handles trailing punctuation/words after the name).
			if strings.HasPrefix(normCandidate, c.norm) {
				mentionsMap[c.jid] = true
				// Replace only the matched name part with @number, keep any trailing text.
				return "@" + c.num + strings.TrimPrefix(raw, raw[:matchedRuneLen(raw, c.norm)])
			}
		}
		return match
	})

	var mentions []string
	for jid := range mentionsMap {
		mentions = append(mentions, jid)
	}
	if len(mentions) > 0 {
		fmt.Printf("[DEBUG] resolveNameMentions: matched %d name tag(s): %v\n", len(mentions), mentions)
	}
	return out, mentions
}

// normalizeName lowercases, strips accents/diacritics and removes spaces &
// common punctuation so "Ken~v Sama" and "kenv sama" compare equal.
func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 0xC0 && r <= 0x17F: // accented latin range -> fold to base letter
			b.WriteRune(foldAccent(r))
		}
	}
	return b.String()
}

// foldAccent maps common accented latin letters to their base ASCII letter.
func foldAccent(r rune) rune {
	switch {
	case r >= 0xC0 && r <= 0xC5, r >= 0xE0 && r <= 0xE5:
		return 'a'
	case r == 0xC7 || r == 0xE7:
		return 'c'
	case r >= 0xC8 && r <= 0xCB, r >= 0xE8 && r <= 0xEB:
		return 'e'
	case r >= 0xCC && r <= 0xCF, r >= 0xEC && r <= 0xEF:
		return 'i'
	case r >= 0xD2 && r <= 0xD6, r >= 0xF2 && r <= 0xF6:
		return 'o'
	case r >= 0xD9 && r <= 0xDC, r >= 0xF9 && r <= 0xFC:
		return 'u'
	default:
		return r
	}
}

// matchedRuneLen returns how many runes of raw correspond to the normalised
// prefix `norm`, so we can keep the trailing portion after the matched name.
func matchedRuneLen(raw, norm string) int {
	count := 0
	matched := 0
	for i, r := range raw {
		nr := normalizeName(string(r))
		if nr != "" {
			matched += len(nr)
		}
		count = i + len(string(r))
		if matched >= len(norm) {
			break
		}
	}
	return count
}

// sanitizeJidsInText replaces any residual JID/LID patterns in LLM output with human names and collects mentions
func sanitizeJidsInText(text string, groupJid string) (string, []string) {
	if text == "" {
		return text, nil
	}

	mentionsMap := make(map[string]bool)

	// Pattern 1: Replace "237XXXXXXXXX@s.whatsapp.net" with @number and collect mention
	jidRegex := regexp.MustCompile(`(\d{10,20})@s\.whatsapp\.net`)
	text = jidRegex.ReplaceAllStringFunc(text, func(match string) string {
		mentionsMap[match] = true
		num := strings.Split(match, "@")[0]
		if strings.Contains(num, ":") {
			num = strings.Split(num, ":")[0]
		}
		fmt.Printf("[DEBUG] sanitizeJidsInText: found JID %s -> @%s\n", match, num)
		return "@" + num
	})

	// Pattern 2: Replace "XXXXX@lid" with @number
	lidRegex := regexp.MustCompile(`(\d+)@lid`)
	text = lidRegex.ReplaceAllStringFunc(text, func(match string) string {
		mentionsMap[match] = true
		num := strings.Split(match, "@")[0]
		fmt.Printf("[DEBUG] sanitizeJidsInText: found LID %s -> @%s\n", match, num)
		return "@" + num
	})

	// Pattern 3: Standalone phone numbers (12-15 digits) that are likely JIDs
	phoneRegex := regexp.MustCompile(`(?:^|[\s(])(\d{12,15})(?:[\s),.]|$)`)
	text = phoneRegex.ReplaceAllStringFunc(text, func(match string) string {
		// Extract just the digits
		digitRegex := regexp.MustCompile(`\d{12,15}`)
		digits := digitRegex.FindString(match)
		if digits == "" { return match }
		
		jid := digits + "@s.whatsapp.net"
		mentionsMap[jid] = true
		fmt.Printf("[DEBUG] sanitizeJidsInText: found phone %s -> @%s\n", digits, digits)
		return strings.Replace(match, digits, "@"+digits, 1)
	})

	// Pattern 4: Replace @g.us group JID references
	groupRegex := regexp.MustCompile(`\d+@g\.us`)
	text = groupRegex.ReplaceAllString(text, "ce groupe")

	var mentions []string
	for jid := range mentionsMap {
		mentions = append(mentions, jid)
	}

	if len(mentions) > 0 {
		fmt.Printf("[DEBUG] sanitizeJidsInText: total mentions collected: %v\n", mentions)
	}

	return text, mentions
}

// ============================================================================
// UTILITIES
// ============================================================================

func containsLink(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "www.")
}

// extractContextInfoFromRaw parses contextInfo from raw JSON when struct parsing fails.
func extractContextInfoFromRaw(raw json.RawMessage) *ContextInfo {
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil
	}

	// Helper to look for contextInfo recursively
	var findContextInfo func(any) *ContextInfo
	findContextInfo = func(val any) *ContextInfo {
		m, ok := val.(map[string]interface{})
		if !ok {
			return nil
		}

		// Look for contextInfo key
		ciVal, hasCI := m["contextInfo"]
		if !hasCI {
			// Some versions might have it at the top level directly
			if _, hasStanza := m["stanzaId"]; hasStanza {
				ciVal = m
				hasCI = true
			}
		}

		if hasCI {
			ci, ok := ciVal.(map[string]interface{})
			if ok {
				var ctx ContextInfo
				// Try various participant keys
				if p, ok := ci["participant"].(string); ok { ctx.Participant = p }
				if p, ok := ci["remoteJid"].(string); ok && ctx.Participant == "" { ctx.Participant = p }
				if p, ok := ci["sender"].(string); ok && ctx.Participant == "" { ctx.Participant = p }
				
				// Try various stanzaId keys
				if s, ok := ci["stanzaId"].(string); ok { ctx.StanzaId = s }
				if s, ok := ci["quotedMessageId"].(string); ok && ctx.StanzaId == "" { ctx.StanzaId = s }
				if s, ok := ci["id"].(string); ok && ctx.StanzaId == "" { ctx.StanzaId = s }
				
				if m, ok := ci["mentionedJid"].([]any); ok {
					for _, j := range m {
						if js, ok := j.(string); ok { ctx.MentionedJid = append(ctx.MentionedJid, js) }
					}
				}
				ctx.QuotedMessage = ci["quotedMessage"]
				
				// Only return if we found at least something useful
				if ctx.StanzaId != "" || ctx.Participant != "" || len(ctx.MentionedJid) > 0 {
					return &ctx
				}
			}
		}

		// Recurse into fields
		for _, v := range m {
			if res := findContextInfo(v); res != nil {
				return res
			}
		}
		return nil
	}

	return findContextInfo(generic)
}
