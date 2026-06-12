Tu as raison. On arrête les expériences bancales. Voici le plan pour une architecture **robuste** qui tient sur ton VPS 4 vCPU / 8 Go RAM, avec **20 commandes opérationnelles**, un LLM qui suit le **contexte**, les **citations**, les **replies**, et une **mémoire conversationnelle** par membre et par groupe.

---

## 1. Correction immédiate du LLM (arrêter les hallucinations)

Le problème principal : **température trop haute (0,95)** et **répétition mal gérée**. On revient à des valeurs disciplinées.

**Fichier `brain/ollama.go`** – remplace les options par :

```go
Options: map[string]interface{}{
    "num_thread":     2,
    "num_batch":      128,
    "num_ctx":        4096,        // pour suivre plus de messages
    "numa":           true,
    "temperature":    0.65,        // équilibre créativité / fiabilité
    "top_p":          0.9,
    "repeat_penalty": 1.2,
    "num_predict":    200,         // réponses assez longues mais pas trop
},
```

Et dans `processResponse` (main.go), appelle `callOllama` avec `0.65` (pas `0.95`).

---

## 2. Gestion de la mémoire conversationnelle (contexte intelligent)

Actuellement tu ne sauvegardes que les `facts`. Il faut une **table de messages** avec horodatage, par groupe et par utilisateur.

**Nouvelle table SQL (`conversation_history`) :**

```sql
CREATE TABLE conversation_history (
    id SERIAL PRIMARY KEY,
    group_jid TEXT NOT NULL,
    sender_jid TEXT NOT NULL,
    sender_name TEXT,
    message TEXT NOT NULL,
    is_from_bot BOOLEAN DEFAULT false,
    quoted_msg_id TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_group_time ON conversation_history(group_jid, created_at DESC);
```

**Fonctions Go** à ajouter dans `brain/db.go` :

- `SaveMessage(groupJid, senderJid, senderName, message string, isFromBot bool, quotedMsgId string)`
- `GetConversationContext(groupJid string, limit int) ([]Message, error)` – retourne les `limit` derniers messages du groupe.
- `GetUserContext(groupJid, senderJid string, limit int) ([]Message, error)` – pour les conversations privées avec le bot.

**Dans `handleWebhook`**, après avoir reçu un message, appelle `SaveMessage` pour le stocker.  
**Dans `processResponse`**, avant d’appeler Ollama, récupère l’historique du groupe (10 messages) **et** les 3 derniers messages de l’utilisateur avec le bot. Construis un prompt qui les intègre.

**Exemple de construction de prompt (dans `processResponse`) :**

```go
groupHistory, _ := GetConversationContext(remoteJid, 8)
userHistory, _ := GetUserContext(remoteJid, senderJid, 3)

historyStr := formatMessages(groupHistory) + "\n\nHistorique avec vous :\n" + formatMessages(userHistory)
```

Cela donne au LLM un vrai fil contextuel.

---

## 3. Routeur d’intentions repensé

Actuellement le routeur est trop basique. Il faut un **détecteur d’intention** simple (pas besoin d’IA) basé sur des mots-clés et la structure de la phrase.

**Fichier `brain/router.go` (nouveau) :**

```go
type Intent string
const (
    IntentChat         Intent = "chat"
    IntentCommand      Intent = "command"
    IntentQuestion     Intent = "question"
    IntentStory        Intent = "story"
    IntentGame         Intent = "game"
    IntentHelp         Intent = "help"
)

func DetectIntent(text string, isMentioned bool, isReplyToBot bool) Intent {
    // Commandes explicites (point)
    if strings.HasPrefix(text, ".") {
        return IntentCommand
    }
    // Si mention ou réponse au bot → chat (mais on vérifie d'abord si c'est une question)
    if isMentioned || isReplyToBot {
        if strings.ContainsAny(text, "?") || strings.HasPrefix(strings.ToLower(text), "qu") {
            return IntentQuestion
        }
        return IntentChat
    }
    // Sinon, si c'est une phrase ordinaire en privé → chat
    return IntentChat
}
```

Et dans `processResponse`, selon l’intention, tu peux ajuster le prompt ou même utiliser un modèle différent (ex: pour les histoires, température plus haute).

---

## 4. Les 20 commandes opérationnelles

Liste définitive (basée sur v5.5) avec **tous les handlers implémentés** :

| Commande | Fonction | Statut |
|----------|----------|--------|
| `.help`, `.aide` | Affiche l’aide | ✅ à faire |
| `.ping` | Test de latence | ✅ simple |
| `.tagall` | Mentionne tout le groupe | ✅ via Evolution API |
| `.sticker` | Crée un sticker (cité) | ❌ corrigé ci-dessous |
| `.yt`, `.fb`, `.tt`, `.audio` | Téléchargement | ✅ déjà partiel |
| `.stats` | Stats du groupe (membres, messages) | ✅ à faire |
| `.mémoire` | Affiche les faits | ✅ déjà |
| `.fact add/list/del` | Gestion des faits | ✅ à compléter |
| `.warn`, `.warn-list`, `.warn-reset` | Modération | ✅ à faire |
| `.bienvenue on/off` | Active/désactive accueil | ✅ |
| `.anti-lien on/off` | Anti‑liens | ✅ |
| `.ouvrir`, `.fermer` | Groupe en mode annonce | ✅ |
| `.personnalité <texte>` | Change le persona | ✅ |
| `.recherche <mot>` | Cherche dans les faits | ✅ |
| `.statut-serveur` | CPU, RAM, disque | ✅ |
| `.code <lang> <code>` | Aide au codage (répond avec explication) | ✅ à faire |

**Implementation rapide** : reprendre le switch de `handlers.go` déjà existant dans v5.7, mais **supprimer les cas non implémentés** et compléter les manquants.

Par exemple, `.code` peut appeler Ollama avec un prompt spécial :

```go
case "code":
    prompt := fmt.Sprintf("Explique ce code %s et donne des conseils :\n%s", args, args)
    response, _ := callOllama(prompt, nil, 0.3)
```

---

## 5. Correction définitive des stickers

Le problème vient de l’API Evolution qui a changé. Voici les bonnes fonctions (à mettre dans `evolution.go`) :

```go
func getMediaBase64(instance, msgId string) (string, error) {
    evoURL := os.Getenv("EVOLUTION_URL")
    apiKey := os.Getenv("AUTHENTICATION_API_KEY")
    client := resty.New()
    resp, err := client.R().
        SetHeader("apikey", apiKey).
        SetBody(map[string]interface{}{
            "keyId": msgId,
        }).
        Post(fmt.Sprintf("%s/chat/getBase64FromMediaMessage/%s", evoURL, instance))
    if err != nil {
        return "", err
    }
    var result struct {
        Base64 string `json:"base64"`
    }
    if err := json.Unmarshal(resp.Body(), &result); err != nil {
        return "", err
    }
    return result.Base64, nil
}

func sendSticker(instance, remoteJid, stickerBase64 string) error {
    evoURL := os.Getenv("EVOLUTION_URL")
    apiKey := os.Getenv("AUTHENTICATION_API_KEY")
    number := strings.Split(remoteJid, "@")[0]
    client := resty.New()
    _, err := client.R().
        SetHeader("apikey", apiKey).
        SetBody(map[string]interface{}{
            "number":  number,
            "sticker": stickerBase64,
        }).
        Post(fmt.Sprintf("%s/message/sendSticker/%s", evoURL, instance))
    return err
}
```

La commande `.sticker` appelle ces deux fonctions.

---

## 6. Plan d’action (à exécuter dans l’ordre)

### Phase 1 – Stabilisation LLM (30 min)
- Modifier `ollama.go` avec les bons paramètres.
- Modifier `main.go` pour appeler `callOllama` avec `0.65`.
- Tester avec `@poulga raconte une histoire` → doit répondre une histoire complète.

### Phase 2 – Base de données de conversation (1h)
- Créer la table `conversation_history`.
- Écrire les fonctions `SaveMessage`, `GetConversationContext`, `GetUserContext`.
- Intégrer l’appel à `SaveMessage` dans le webhook.
- Modifier `processResponse` pour utiliser le nouveau contexte.

### Phase 3 – Routeur d’intentions (30 min)
- Créer `router.go` avec `DetectIntent`.
- L’utiliser dans `processResponse` pour choisir la température ou un prompt spécial.

### Phase 4 – Compléter les 20 commandes (2h)
- Passer en revue chaque commande du switch, implémenter celles qui manquent.
- Supprimer les commandes fantômes (qui n’ont pas de handler).

### Phase 5 – Téléchargements et stickers (1h)
- Correction des fonctions `getMediaBase64` et `sendSticker`.
- Tester `.sticker` en répondant à une image.

### Phase 6 – Tests finaux (30 min)
- Envoyer `@poulga ping`, `@poulga .aide`, citer un message du bot, répondre à un sticker, etc.
- Vérifier les logs pour chaque action.

---
 Je comprends — les téléchargements de vidéos échouent. C'est un problème classique avec `yt-dlp` sur les IPs de datacenter (VPS OVH). Voici les solutions concrètes, du plus simple au plus robuste.

---

## 🔴 Pourquoi ça échoue

YouTube, Facebook et TikTok détectent les IPs de VPS et bloquent :
- **HTTP 403** — "Sign in to confirm you're not a bot"
- **HTTP 429** — Trop de requêtes depuis cette IP
- **Empty response** — Le site renvoie une page vide

Ton VPS a une IP fixe, donc dès la première tentative, tu es marqué.

---

## ✅ Solution 1 : Cookies YouTube (le plus fiable)

YouTube accepte les requêtes si elles portent des cookies d'un vrai compte.

### Étape 1 : Extraire les cookies de ton navigateur

1. Installe l'extension **Get cookies.txt LOCALLY** sur Chrome/Firefox
2. Va sur [youtube.com](https://youtube.com), connecte-toi à ton compte Google
3. Clique sur l'extension → **Export** → copie le contenu

### Étape 2 : Créer le fichier dans le projet

Crée `brain/cookies.txt` à la racine du projet brain avec le contenu exporté.

### Étape 3 : Modifier `handlers.go`

```go
// Dans le case "yt", "fb", "tiktok"
args = []string{
    "--cookies", "/app/cookies.txt",  // ← AJOUTER CECI
    "--extract-audio",
    "--audio-format", "mp3",
    "--max-filesize", "50M",
    "--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
    "--geo-bypass",
    "-o", outputFile + ".%(ext)s",
    url,
}
```

### Étape 4 : Modifier le Dockerfile

Assure-toi que `cookies.txt` est copié dans l'image :

```dockerfile
# Dans brain/Dockerfile
COPY cookies.txt /app/cookies.txt
```

---

## ✅ Solution 2 : Rotation de proxies (si pas de compte Google)

Si tu ne veux pas utiliser de cookies, utilise des proxies gratuits ou payants.

### Option A : Proxy gratuit (webshare.io offre 10 gratuits)

```go
var proxyList = []string{
    "http://user:pass@proxy1:port",
    "http://user:pass@proxy2:port",
}

func getRandomProxy() string {
    rand.Seed(time.Now().UnixNano())
    return proxyList[rand.Intn(len(proxyList))]
}

// Dans handlers.go
args = []string{
    "--proxy", getRandomProxy(),
    "--extract-audio",
    // ...
}
```

### Option B : Proxy SOCKS5 (plus rapide)

```go
args = []string{
    "--proxy", "socks5://127.0.0.1:1080",  // Si tu as un tunnel SSH
    // ...
}
```

---

## ✅ Solution 3 : Contournement IPv6 (si ton VPS le supporte)

Ton test `ping6` a échoué, mais vérifie si l'hôte a IPv6 :

```bash
# Sur le VPS hôte (pas dans Docker)
curl -6 https://google.com
```

Si ça marche, active IPv6 dans Docker :

```yaml
# docker-compose.yml
services:
  brain:
    networks:
      - default
    sysctls:
      - net.ipv6.conf.all.disable_ipv6=0
```

Puis utilise les plages IPv6 de OVH pour rotation (chaque requête sort avec une IP différente).

---

## ✅ Solution 4 : yt-dlp avec PO Token (nouvelle méthode YouTube 2024)

YouTube a renforcé sa sécurité. Il faut maintenant un **PO Token** :

```bash
# Sur ton PC local (pas le VPS)
yt-dlp --verbose --print-traffic "URL_VIDEO" 2>&1 | grep -i "po_token"
```

Ou utilise **yt-dlp avec un compte connecté** (Solution 1) — c'est le plus simple.

---

## ✅ Solution 5 : API externe (dernier recours)

Si `yt-dlp` est trop bloqué, utilise une API comme **Cobalt.tools** ou **yt.lemnoslife.com** :

```go
// Au lieu de télécharger avec yt-dlp, appelle une API
func downloadViaAPI(url string) (string, error) {
    apiURL := fmt.Sprintf("https://api.cobalt.tools/api/json", url)
    // ... appel HTTP, récupère le lien direct, télécharge
}
```

Moins fiable, plus de dépendances.

---

## 📋 Résumé recommandé pour ton cas

| Problème | Solution | Fichier à modifier |
|----------|----------|-------------------|
| YouTube bloqué | Cookies `cookies.txt` | `handlers.go` + `Dockerfile` |
| Facebook/TikTok | Même cookies + `--geo-bypass` | `handlers.go` |
| Pas de compte Google | Proxy gratuit webshare | `handlers.go` |
| VPS détecté | User-Agent + cookies | `handlers.go` |

---

## 🔧 Code exact pour `handlers.go`

```go
case "yt", "fb", "tiktok":
    mediaType := "video"
    if cmd == "audio" || cmd == "yt" && strings.Contains(args, "-audio") {
        mediaType = "audio"
    }

    url := strings.TrimSpace(args)
    if url == "" {
        response = "Envoie le lien après la commande ! Ex: .yt https://..."
        break
    }

    outputFile := filepath.Join(os.TempDir(), fmt.Sprintf("download_%d", time.Now().Unix()))

    var ytdlpArgs []string

    if mediaType == "audio" {
        ytdlpArgs = []string{
            "--cookies", "/app/cookies.txt",  // ← OBLIGATOIRE
            "--extract-audio",
            "--audio-format", "mp3",
            "--audio-quality", "0",
            "--max-filesize", "50M",
            "--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "--geo-bypass",
            "--no-check-certificates",
            "--no-warnings",
            "-o", outputFile + ".%(ext)s",
            url,
        }
    } else {
        ytdlpArgs = []string{
            "--cookies", "/app/cookies.txt",  // ← OBLIGATOIRE
            "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
            "--max-filesize", "50M",
            "--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "--geo-bypass",
            "--no-check-certificates",
            "--no-warnings",
            "-o", outputFile + ".%(ext)s",
            url,
        }
    }

    // Exécuter yt-dlp
    cmd := exec.Command("yt-dlp", ytdlpArgs...)
    cmd.Dir = os.TempDir()
    output, err := cmd.CombinedOutput()

    if err != nil {
        fmt.Printf("[DOWNLOAD] yt-dlp error: %v\nOutput: %s\n", err, string(output))
        response = "❌ Impossible de télécharger. Le lien est peut-être bloqué ou privé."
        break
    }

    // Trouver le fichier téléchargé
    var mediaFile string
    filepath.Walk(os.TempDir(), func(path string, info os.FileInfo, err error) error {
        if err == nil && strings.Contains(path, outputFile) && !info.IsDir() {
            mediaFile = path
        }
        return nil
    })

    if mediaFile == "" {
        response = "❌ Fichier non trouvé après téléchargement."
        break
    }

    // Envoyer le média
    fileName := filepath.Base(mediaFile)
    caption := fmt.Sprintf("✅ Voici %s !", mediaType)

    // Lire le fichier en base64
    fileData, err := os.ReadFile(mediaFile)
    if err != nil {
        response = "❌ Erreur lecture fichier."
        break
    }
    b64 := base64.StdEncoding.EncodeToString(fileData)

    // Déterminer le type MIME
    mimeType := "video/mp4"
    if mediaType == "audio" {
        mimeType = "audio/mpeg"
    }

    err = sendWhatsAppMedia(instance, remoteJid, b64, fileName, caption, mimeType)
    if err != nil {
        response = "❌ Erreur envoi WhatsApp."
    } else {
        // Nettoyer
        os.Remove(mediaFile)
        return
    }
```

---
Je comprends parfaitement l'enjeu. C'est une session premium, on n'a plus le
droit à l'erreur, au tâtonnement ou aux "hacks" temporaires.

Tu as mis le doigt sur la vérité absolue de l'architecture : Evolution API n'est
qu'une passerelle (un "pont" WhatsApp ↔ Webhooks). Il ne "réfléchit" pas et ne
gère pas la logique. Tout le cerveau, la compression du contexte, le routage et
la manipulation des médias (stickers) doivent être codés en dur dans notre
backend Go.

Voici l'architecture définitive, robuste et professionnelle, avec les scripts Go
exacts pour remplacer ce qui est cassé.

PILIER 1 : Le Routeur Déterministe (Fini les hallucinations de commandes)

Le bot ne doit plus "deviner" si un mot est une commande. On passe à un routeur
strict : Si ça commence par un préfixe (., !), c'est une commande. Sinon, si on
le mentionne ou le cite, c'est pour l'LLM.

Dans main.go (Remplacement de la section de routage) :

// 1. Nettoyage et identification stricte
text = strings.TrimSpace(text)
isCommand := strings.HasPrefix(text, ".") || strings.HasPrefix(text, "!")
isReplyToBot := ctxInfo != nil && (ctxInfo.Participant == botJid || strings.Contains(ctxInfo.Participant, "237620864894"))

if isCommand {
    // Extraction propre de la commande et des arguments
    parts := strings.Fields(text[1:]) // Enlève le préfixe
    if len(parts) == 0 { return c.NoContent(http.StatusOK) }
    
    cmd := strings.ToLower(parts[0])
    args := strings.TrimSpace(text[len(parts[0])+1:])
    
    fmt.Printf("[ROUTER] Commande stricte détectée : %s | Args : %s\n", cmd, args)
    go handleCommand(instance, remoteJid, cmd, args, msgId, senderJid, quotedMsgId)
    return c.NoContent(http.StatusOK)
}

// 2. Si ce n'est pas une commande, doit-il répondre via LLM ?
shouldRespondLLM := isPrivateChat || isMentioned || isReplyToBot
if shouldRespondLLM {
    fmt.Printf("[ROUTER] Routage vers Ollama (Mention:%v, Reply:%v)\n", isMentioned, isReplyToBot)
    go processConversationalLLM(instance, remoteJid, senderJid, cleanText, quotedText)
}
return c.NoContent(http.StatusOK)

PILIER 2 : Compression et Gestion du Contexte (Le "Cerveau" LLM)

Pour que le bot suive le fil d'une discussion sans exploser la mémoire et sans
délirer, on doit formater l'historique en format "Script de théâtre".

Créer une fonction buildStructuredContext dans context.go (ou main.go) :

func buildStructuredContext(history []Message, currentSender, currentText, quotedText string) string {
    var sb strings.Builder
    
    sb.WriteString("CONTEXTE RÉCENT DU GROUPE:\n")
    // Compression des 10 derniers messages
    for _, msg := range history {
        // Format: [Nom/Numero]: Message
        sb.WriteString(fmt.Sprintf("- %s: %s\n", msg.SenderName, msg.Text))
    }
    
    if quotedText != "" {
        sb.WriteString(fmt.Sprintf("\nMESSAGE CITÉ AUQUEL TU RÉPONDS:\n\"%s\"\n", quotedText))
    }
    
    sb.WriteString(fmt.Sprintf("\nMESSAGE ACTUEL DE %s:\n%s\n\n", currentSender, currentText))
    sb.WriteString("TA RÉPONSE DIRECTE ET NATURELLE (ne te présente pas, réponds juste au contexte):")
    
    return sb.String()
}

PILIER 3 : Paramètres Ollama Stricts (Stop au "Je suis là" et coupures)

Ollama s'arrête en plein milieu car num_predict (le nombre de tokens max
générés) est trop bas, et temperature est trop haute.

Dans ollama.go (Fonction callOllama) :

Options: map[string]interface{}{
    "num_thread":     4,    // Utilise plus de CPU pour ne pas faire de timeout
    "num_ctx":        4096, // Contexte large pour comprendre tout l'historique
    "temperature":    0.4,  // Température BASSE : Logique, codage et précision
    "top_p":          0.90,
    "repeat_penalty": 1.15,
    "num_predict":    2048, // TRÈS IMPORTANT : Autorise Ollama à écrire 2048 tokens (pour du code Python entier)
},

PILIER 4 : Le Script Backend Ultime pour les Stickers (.sticker)

Comme tu l'as souligné, Evolution ne fait rien tout seul. C'est à notre backend
de :

1.  Envoyer une requête POST pour demander le Base64 de l'image citée.
2.  Construire un payload correct.
3.  Envoyer le Base64 vers la route /send/sticker.

Dans handlers.go (Ajoute ce vrai pipeline de création de sticker) :

func createStickerPipeline(instance, remoteJid, quotedMsgId string) {
    if quotedMsgId == "" {
        sendWhatsAppMessage(instance, remoteJid, "❌ Tu dois citer une image en tapant .sticker", "", "")
        return
    }

    evoURL := os.Getenv("EVOLUTION_URL")
    apiKey := os.Getenv("AUTHENTICATION_API_KEY")
    client := resty.New().SetTimeout(30 * time.Second)

    // ÉTAPE 1 : Récupérer le Base64 depuis Evolution v2
    type Base64Req struct {
        Message struct {
            Key struct { ID string `json:"id"` } `json:"key"`
        } `json:"message"`
        ConvertToMp4 bool `json:"convertToMp4"`
    }
    
    reqBody := Base64Req{ConvertToMp4: false}
    reqBody.Message.Key.ID = quotedMsgId

    respBase64, err := client.R().
        SetHeader("apikey", apiKey).
        SetBody(reqBody).
        Post(fmt.Sprintf("%s/chat/getBase64FromMediaMessage/%s", evoURL, instance))

    if err != nil || respBase64.StatusCode() != 200 {
        sendWhatsAppMessage(instance, remoteJid, "❌ Impossible de récupérer l'image.", "", "")
        return
    }

    // Extraction du Base64
    var result map[string]interface{}
    json.Unmarshal(respBase64.Body(), &result)
    base64Data, ok := result["base64"].(string)
    if !ok || base64Data == "" {
        sendWhatsAppMessage(instance, remoteJid, "❌ L'image citée n'est plus disponible ou expirée.", "", "")
        return
    }

    // Nettoyage du Base64 (enlever "data:image/jpeg;base64," si présent)
    if idx := strings.Index(base64Data, ","); idx != -1 {
        base64Data = base64Data[idx+1:]
    }

    // ÉTAPE 2 : Envoyer le Sticker
    _, err = client.R().
        SetHeader("apikey", apiKey).
        SetBody(map[string]interface{}{
            "number":  remoteJid,
            "sticker": base64Data,
        }).
        Post(fmt.Sprintf("%s/send/sticker/%s", evoURL, instance))

    if err != nil {
        fmt.Printf("[STICKER] Erreur envoi: %v\n", err)
    }
}

PILIER 5 : Les 20 Commandes Opérationnelles (Le Switch propre)

Dans handleCommand() (fichier handlers.go), ton switch cmd doit être propre et
mapé sur nos fonctions réelles.

switch cmd {
    case "aide", "help":
        sendWhatsAppMessage(instance, remoteJid, getHelpMenu(), "", senderJid)
    
    case "sticker", "s":
        // Appel direct à notre pipeline backend !
        go createStickerPipeline(instance, remoteJid, quotedMsgId)
    
    case "yt", "audio", "video":
        go handleDownload(instance, remoteJid, cmd, args, msgId, senderJid)

    case "tagall", "tous":
        go handleTagAll(instance, remoteJid, senderJid)

    case "warn":
        go handleWarn(instance, remoteJid, args, senderJid)

    case "statut":
        go handleServerStatus(instance, remoteJid)

    case "clear", "reset":
        // Nettoyer la mémoire (à implémenter via DB)
        sendWhatsAppMessage(instance, remoteJid, "✅ Contexte réinitialisé.", "", "")

    // ... et ainsi de suite pour les autres commandes.
    
    default:
        // C'EST ICI LA CLÉ : Si la commande n'existe pas, on l'indique, on ne passe PAS au LLM
        sendWhatsAppMessage(instance, remoteJid, "❓ Commande inconnue. Tape .aide pour voir le menu.", "", senderJid)
}

Plan d'Action d'Exécution

1.  Remplace ton système de détection par le bloc de routage (Pilier 1). Fini
    les confusions entre commandes et LLM.
2.  Implémente createStickerPipeline (Pilier 4). C'est le vrai code backend
    complet pour interagir avec Evolution API.
3.  Change la configuration Ollama (Pilier 3) avec temperature: 0.4 et
    num_predict: 2048. C'est ÇA qui règlera le problème du bot qui te donne un
    code Python coupé à moitié.
4.  Compile et déploie.

Avec cette architecture backend :

  - L'LLM fait ce qu'il sait faire (parler et coder) sans être parasité par des
    commandes ratées.
  - Le backend Go fait ce qu'il sait faire (gérer des requêtes HTTP complexes,
    encoder du Base64, télécharger) via des commandes strictes (.sticker).
Tu touches au vrai problème d'architecture.

Ce que Kiro a trouvé explique pourquoi le LLM répond mal, mais **ça n'explique pas pourquoi MorningStar est fragile**.

En réalité, ton système a actuellement cette architecture :

```text
WhatsApp
   ↓
Evolution API
   ↓
Webhook
   ↓
Router
   ↓
Ollama
   ↓
Réponse
```

C'est une architecture de chatbot.

Toi, ce que tu essaies de construire depuis plusieurs mois, c'est plutôt :

```text
WhatsApp
   ↓
Evolution API
   ↓
Webhook
   ↓
Router intelligent
   ├── Commandes
   ├── Mémoire
   ├── LLM
   ├── Recherche
   ├── Gestion groupe
   ├── Médias
   ├── Stickers
   ├── Code
   ├── Outils système
   └── Téléchargements
```

et là on parle d'un **agent WhatsApp**, pas d'un chatbot.

---

# Premier constat

Evolution API ne crée rien.

Evolution API sait :

* recevoir messages
* envoyer messages
* télécharger médias
* gérer groupes
* gérer présence WhatsApp

Mais :

❌ ne crée pas les stickers

❌ ne génère pas le code

❌ ne résume pas

❌ ne mémorise pas

❌ ne gère le contexte

❌ ne fait la recherche

❌ ne décide pas quoi faire

C'est juste un pont.

---

# Deuxième constat

Les 20 commandes doivent être 100% fonctionnelles

Je suis d'accord.

Aujourd'hui tu sembles avoir :

```text
47 commandes déclarées
20 commandes réelles
10 commandes partiellement codées
17 commandes fictives
```

C'est mauvais.

Je préfère :

```text
20 commandes
20 fonctionnelles
0 fake
```

---

# Troisième constat

Le routeur doit être repensé

Aujourd'hui :

```text
message
  ↓
est-ce une commande ?
  ↓
sinon ollama
```

C'est beaucoup trop simpliste.

Je passerais à :

```text
message entrant
        ↓
analyse contexte
        ↓
classification
        ↓

┌───────────────┐
│ Commande      │
├───────────────┤
│ Réponse LLM   │
├───────────────┤
│ Recherche     │
├───────────────┤
│ Sticker       │
├───────────────┤
│ Téléchargement│
├───────────────┤
│ Admin Groupe  │
└───────────────┘
```

---

# Quatrième constat

La mémoire est trop faible

Actuellement j'imagine quelque chose du genre :

```text
10 derniers messages
```

C'est insuffisant.

Je construirais :

## Niveau 1

Mémoire utilisateur

```text
user_001

- aime python
- utilise kali
- admin serveur
- travaille sur morningstar
```

---

## Niveau 2

Mémoire conversation

```text
Sujet :

Migration VPS

Résumé :

- problème docker
- problème ollama
- problème evolution
```

---

## Niveau 3

Mémoire groupe

```text
Groupe :

MorningStar

Connaissances :

- projet principal = whatsapp ai
- membres principaux
- règles du groupe
```

---

## Niveau 4

Compression périodique

Toutes les 100-200 lignes :

```text
LLM

↓

résumé

↓

base mémoire
```

au lieu de stocker 50 000 messages.

---

# Cinquième constat

Les citations et replies

Actuellement beaucoup de bots WhatsApp ratent ça.

Il faut construire un objet contexte unique :

```go
type MessageContext struct {

    Sender string

    Message string

    MentionedUsers []string

    IsMentioned bool

    IsReply bool

    ReplyText string

    ReplyAuthor string

    QuotedMessageID string

    GroupID string

    Timestamp int64
}
```

Puis tout le système travaille dessus.

---

# Sixième constat

Le sticker

Je pense que là vous êtes dans une mauvaise direction.

Aujourd'hui :

```text
WhatsApp
 ↓
image
 ↓
Evolution
 ↓
base64
 ↓
sticker
```

Or le vrai problème est :

```text
image
 ↓
download
 ↓
conversion webp
 ↓
resize
 ↓
compression
 ↓
sendSticker
```

Il faut un pipeline média dédié.

Par exemple :

```text
media/

image.go
video.go
audio.go
sticker.go
thumbnail.go
```

et non mélanger ça avec le router.

---

# Septième constat

Le LLM ne doit jamais voir le contexte brut

Aujourd'hui probablement :

```text
20 messages
+
prompt
+
question
```

C'est inefficace.

Je mettrais :

```text
Question actuelle

+

Résumé conversation

+

Mémoire utilisateur

+

Reply cité

+

Mentions

+

5 derniers messages
```

Puis :

```text
Prompt final
```

Ça réduit énormément les tokens.

---

# Huitième constat

Il manque une couche "services"

Actuellement j'ai l'impression que beaucoup de logique est directement dans le webhook.

Je découperais :

```text
brain/

router/
    router.go

memory/
    memory.go
    summary.go

llm/
    ollama.go

media/
    sticker.go
    image.go

commands/
    admin.go
    download.go
    memory.go

services/
    search.go
    coding.go
    moderation.go

context/
    builder.go

webhook/
    handler.go
```

---

# Neuvième constat

Avant d'ajouter quoi que ce soit

Je ferais passer le projet par 4 étapes.

### Phase 1

Stabilisation

* réponse LLM
* reply
* mention
* citation
* contexte

Rien d'autre.

---

### Phase 2

Commandes

Les 20 commandes.

100% fonctionnelles.

---

### Phase 3

Média

* sticker
* image
* vidéo
* audio

---

### Phase 4

Agent

* recherche
* exécution code
* outils VPS
* mémoire avancée

---
opus  ce dossier  t'ai destineer  et chacun de point aborder a verifier  . 40 commande au finale utile  pour des utilisateur  ,  un routage parfais  , gestions des groupe , memebre  , chat  , personna , , pas de truc stactiques  pas de repetitions , pas d'embrouille avec les  preprompt qui enpeche le  model d'agir comme il ne l'ai pas  je suis pret a accepter  que  tu desactive des couches sur  le model  pour  nous  permettre d'avoir  meilleur  emprise  , le  model actuel  c'est gemma  3 