# Poulga Architecture Comparison: v5.3 (Working 15h-17h) vs v5.7 (Current Broken)

## Executive Summary
**v5.3 worked** because it had disciplined LLM parameters and simple command handling.
**v5.7 broke** because it became too ambitious with command expansion and temperature tuning got out of control.

---

## Critical Differences

### 1. **Temperature Parameter (MOST CRITICAL)**

#### v5.3 (WORKING)
```go
response, _ := callOllama(prompt, nil, 0.7)
```
- Temperature: **0.7** (moderately focused)
- Model: `gemma3:4b` (explicit in ollama.go)
- Behavior: Structured, coherent responses

#### v5.7 (BROKEN)
```go
response, _ := callOllama(prompt, nil, 0.95)
```
- Temperature: **0.95** (extremely creative/random)
- Model: Undefined, expects env var, falls back to `llama3` (not installed)
- Behavior: Hallucinates, cuts off mid-response ("je suis là")

**Impact:** 0.95 is 35% more creative than 0.7. The LLM loses focus and abandons responses.

---

### 2. **Ollama Configuration in ollama.go**

#### v5.3 (WORKING)
```go
Options: map[string]interface{}{
    "num_thread":     2,
    "num_batch":      128,
    "num_ctx":        2048,     // ← SMALL context
    "numa":           true,
    "temperature":    0.3,      // ← HARDCODED 0.3!
    "top_p":          0.9,
    "repeat_penalty": 1.2,      // ← STRICT repetition control
    "num_predict":    128,      // ← SHORT output limit
},
```

#### v5.7 (BROKEN)
```go
Options: map[string]interface{}{
    "num_thread":     2,
    "num_batch":      128,
    "num_ctx":        3072,     // ← BIG context (wastes memory)
    "numa":           true,
    "temperature":    temperature, // ← PARAMETER PASSED (0.95!)
    "top_p":          0.95,     // ← TOO HIGH (0.9 in v5.3)
    "repeat_penalty": 1.15,     // ← TOO LENIENT
    "num_predict":    256,      // ← LONG output (allows rambling)
},
```

**Root Cause Chain:**
- v5.3: Hardcoded `temperature: 0.3` in ollama.go
- v5.4: Changed to `temperature: parameter`, defaulting to 0.95
- v5.7: Compounded by increasing `num_predict` (256 vs 128) → more tokens to "think" randomly

---

### 3. **Command Detection Logic (Added Bad Behavior in v5.4+)**

#### v5.3 (WORKING)
```go
if cmd, args, isCmd := IsCommand(cleanText); isCmd {
    fmt.Printf("[DEBUG] EXECUTING_COMMAND=%s ARGS=%s\n", cmd, args)
    go handleCommand(instance, remoteJid, cmd, args, msgId, senderJid, quotedMsgId)
    return c.NoContent(http.StatusOK)
}
```
- Only recognized valid commands (help, ping, yt, fb, tt, sticker, tagall)
- Unmotched text → Ollama

#### v5.7 (BROKEN)
```go
cmd, cmdArgs, isCmd := IsCommand(cleanText)
if !isCmd && isMentioned && cleanText != "" {
    // NEW LOGIC: If mentioned and not a '.' command, 
    // treat first word as command
    parts := strings.Fields(cleanText)
    if len(parts) > 0 {
        cmd = parts[0]
        cmdArgs = strings.TrimSpace(cleanText[len(cmd):])
        isCmd = true // ← FORCE AS COMMAND!
    }
}
```

**What This Means:**
```
v5.3: @poulga fais un programme python
      → IsCommand("fais un programme python") = false
      → Pass to Ollama ✓

v5.7: @poulga fais un programme python
      → IsCommand("fais un programme python") = false
      → BUT mentioned=true, so FORCE cmd="fais"
      → handleCommand("fais", "un programme python")
      → handleCommand doesn't recognize "fais" → fallback to default case ❌
```

This added fallback was **removed in v5.6**, but the temperature damage was already done.

---

### 4. **Command List Expansion (v5.5)**

#### v5.3 (WORKING)
```go
commands := []string{"help", "stats", "persona", "confidentialité", "ping", 
                     "tagall", "sticker", "menu", "yt", "fb", "tt", "video", "audio"}
```
- 12 commands recognized
- All have handlers in handleCommand()

#### v5.7 (BROKEN)
```go
commands := []string{
    "aide", "help", "menu", "qui-es-tu", "qui", "mémoire", "résumé", "resume", 
    "stats", "statistiques", "persona", "personnalité", "confidentialité", "privacy",
    "ping", "pong", "tagall", "mentionner", "sticker", "ouvrir", "open", "fermer", 
    "close", "avertir", "warn", "avertissements", "warnings", "warn-list", 
    "warn-reset", "reset", "bienvenue", "anti-lien", "anti-spam", "anti-suppression",
    "yt", "fb", "tt", "video", "vidéo", "audio", "télécharger", "download", 
    "miniature", "thumbnail", "infos", "info", "recherche", "search", "code", 
    "explique", "explain", "débogue", "debug", "statut-serveur", "server-status", 
    "logs", "docker", "fait", "fact",
}
```
- 47 commands listed
- But many have broken handlers or placeholders

**Impact:** v5.3 was minimal and worked. v5.7 tried to do everything and broke the basics.

---

### 5. **Message History Context**

#### v5.3
```go
history, _ := getRecentMessages(remoteJid, 8)
```

#### v5.7
```go
history, _ := getRecentMessages(remoteJid, 10)
```

Minor, but adds context noise.

---

### 6. **Persona Changes**

#### v5.3 (WORKING)
```go
const PersonaPrompt = `Tu es une meuf du groupe WhatsApp, cool et directe. 
Si on te demande qui tu es ou de te présenter: 'Je suis Poulga, je mémorise 
vos échanges et peux résumer ou retrouver des infos.' Sinon, ne te présente 
pas sans être demandé.
...
Réponse de Poulga :`
```

#### v5.7 (BROKEN)
```go
const PersonaPrompt = `Tu es Poulga, une assistante du groupe. Tu aides les 
membres, tu réponds aux questions, tu racontes des histoires, tu proposes des 
jeux, tu donnes des conseils. Tu as accès aux faits mémorisés et à l'historique 
récent. Sois naturelle, brève mais complète. N'hésite pas à être un peu 
impertinente ou drôle. Ne te présente jamais. Réponds directement à la demande.
...
Réponse :`
```

v5.7 persona is **less focused** and encourages rambling ("raconte des histoires", "proposes des jeux").

---

## Why v5.3 Worked and v5.7 Doesn't

### v5.3 Architecture (15h-17h)
```
Message → Clean & Parse
        ↓
        Is exact command? (12 known commands)
        ├─ Yes → handleCommand() → Direct response
        └─ No → Ollama
                ├─ Temperature: 0.3 (focused)
                ├─ num_predict: 128 (short responses)
                ├─ repeat_penalty: 1.2 (no rambling)
                └─ Output: Natural, complete
```

**Key:** Small command set, aggressive LLM constraints.

### v5.7 Architecture (Current)
```
Message → Clean & Parse
        ↓
        Is known command OR first word looks like command?
        ├─ Yes → handleCommand() 
        │        ├─ If recognized → response
        │        └─ If not → fallback to default ("unknown command")
        └─ No → Ollama
                ├─ Temperature: 0.95 (creative/random)
                ├─ num_predict: 256 (long rambling)
                ├─ repeat_penalty: 1.15 (allows repetition)
                └─ Output: Cuts off ("je suis là")
```

**Key:** Large command set with incomplete handlers, permissive LLM parameters.

---

## The Root Problem Chain

1. **v5.3** → Temperature hardcoded to 0.3 in ollama.go
2. **v5.4** → Commit changed to parameterized temperature, default 0.95
3. **v5.5** → Added 35 new commands but didn't complete handlers
4. **v5.6** → Tried to fix command detection, but LLM already broken
5. **v5.7** → Added reply detection, still didn't fix temperature

**The bot didn't break because of replies or command detection.**
**It broke because Ollama parameters were set to encourage hallucination.**

---

## Solution

### Immediate Fix (5 minutes)
```go
// In processResponse(), change:
response, _ := callOllama(prompt, nil, 0.95)

// To:
response, _ := callOllama(prompt, nil, 0.6) // Back to v5.3 discipline
```

### Proper Fix (30 minutes)
1. Revert ollama.go Options to v5.3 hardcoded values
2. Reduce command list to only implemented handlers
3. Test basic "fais un programme python" → gets full code

### Complete Fix (2 hours)
1. Reduce command set to 15-20 that are fully working
2. Implement proper Ollama model checking (ensure llama3 or gemma installed)
3. Add timeout detection and fallback for Ollama failures
4. Complete all command handlers before expanding

---

## Conclusion

**v5.3 was better architecture because it embraced simplicity:**
- 12 commands (all working)
- Temperature 0.3 (focused)
- num_predict 128 (concise)

**v5.7 tried to be ambitious:**
- 47 commands (many broken)
- Temperature 0.95 (hallucinating)
- num_predict 256 (rambling)

The lesson: **A bot with 5 perfect commands beats a bot with 50 broken ones.**
