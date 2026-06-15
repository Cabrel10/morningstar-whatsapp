#!/bin/bash
# Script pour configurer une cle API LLM pour le bot WhatsApp
# Usage: ./setup_llm_key.sh groq <votre-cle>
#    ou: ./setup_llm_key.sh gemini <votre-cle>

PROVIDER=$1
KEY=$2

if [ -z "$PROVIDER" ] || [ -z "$KEY" ]; then
    echo "Usage:"
    echo "  ./setup_llm_key.sh groq <votre-cle-groq>"
    echo "  ./setup_llm_key.sh gemini <votre-cle-gemini>"
    echo ""
    echo "Pour obtenir une cle gratuite:"
    echo "  Groq (recommande): https://console.groq.com/keys"
    echo "  Gemini: https://aistudio.google.com/apikey"
    exit 1
fi

if [ "$PROVIDER" == "groq" ]; then
    sed -i "s|^GROQ_API_KEY=.*|GROQ_API_KEY=$KEY|" .env
    echo "[OK] Cle Groq configuree"
elif [ "$PROVIDER" == "gemini" ]; then
    sed -i "s|^GEMINI_API_KEY=.*|GEMINI_API_KEY=$KEY|" .env
    echo "[OK] Cle Gemini configuree"
else
    echo "Provider invalide. Utilise 'groq' ou 'gemini'"
    exit 1
fi

echo "[INFO] Redemarrage du brain..."
docker compose up -d brain
sleep 5
docker compose logs brain --tail=5
echo ""
echo "[OK] Bot reconfigure. Teste avec un message WhatsApp."
