# Ollama Debugging Log - Kiro

## Session Date: 2026-06-13

### Problem Statement
The `brain` service (Go application) fails to connect to the local Ollama instance, resulting in `context deadline exceeded` errors. The primary symptom is the bot not responding to LLM requests and logging `[LLM] ALL backends failed! Groq + Gemini + Ollama. Error: Post "http://host.docker.internal:11434/api/generate": context deadline exceeded`.

### Initial Observations & Actions
1.  **Error Type:** `context deadline exceeded` suggests a network/connectivity issue or an extremely slow response, rather than a model inference error itself.
2.  **Ollama Model:** Configured to `qwen2.5:0.5b` (a small, CPU-friendly model) in `.env` and `ollama.go`. User reported 8GB RAM available, which should be sufficient.
3.  **Timeout:** Increased `OllamaLocal` timeout in `brain/ollama.go` from 90s to 120s as a first step to rule out insufficient timeout.
    *   **File:** `brain/ollama.go`
    *   **Change:** `client.SetTimeout(90 * time.Second)` -> `client.SetTimeout(120 * time.Second)`
4.  **Ollama URL Discrepancy:**
    *   Initial logs showed connection attempts to `http://host.docker.internal:11434`.
    *   A manual `netstat` on the host confirmed Ollama was listening on `*:11434`.
    *   A manual `curl` from *inside* the `brain` container to `http://172.17.0.1:11434/api/tags` succeeded, indicating Ollama was reachable via `172.17.0.1` (Docker bridge IP) from the container.
    *   Checked `.env` and found `OLLAMA_URL=http://172.17.0.1:11434` was already set. This implies either the `brain` container was still running with an old `.env` or the provided logs were from an earlier state.

### Current Status & Next Steps
1.  **Confirmation of `.env`:** The `.env` file now correctly specifies `OLLAMA_URL=http://172.17.0.1:11434`.
2.  **Required Docker Action:** A full Docker rebuild and restart (`docker-compose down && docker-compose build --no-cache && docker-compose up`) is necessary to ensure the `brain` container is using this updated `OLLAMA_URL`.
3.  **Pending Diagnostic:** Still awaiting the output of the manual `curl` test from *within* the container to trigger a model generation and measure its direct response time:
    ```bash
    docker exec whatsapp-brain-1 timeout 30 curl -s -X POST http://172.17.0.1:11434/api/generate 
      -H "Content-Type: application/json" 
      -d '{"model":"qwen2.5:0.5b","prompt":"Hello","stream":false}' | head -50
    ```
    This output is crucial for determining if the model itself is taking too long to load or generate a response, even when the network path is clear.
4.  **Bot Testing:** After the rebuild and the `curl` test result, the bot needs to be tested again with an `@poulga` mention.