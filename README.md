# MorningStar WhatsApp Assistant

MorningStar est un assistant intelligent pour WhatsApp conçu pour les groupes professionnels et les interactions structurées. Il combine la puissance de l'IA locale (Ollama), une connectivité robuste (Evolution API), et une automatisation flexible (n8n).

## 🚀 Fonctionnalités

- **Intelligence Contextuelle** : Utilise Gemma 3 (4.3B) via Ollama pour des réponses précises et professionnelles.
- **Mémoire Long Terme** : Stockage des faits et du contexte dans PostgreSQL pour des conversations cohérentes dans le temps.
- **Connectivité Stable** : Propulsé par Evolution API (Baileys) pour une intégration transparente avec WhatsApp.
- **Automation n8n** : Workflow prêt pour l'automatisation de tâches complexes.
- **Analyse de Sentiment & Extraction de Faits** : Extrait automatiquement les informations importantes des conversations pour enrichir sa base de connaissances.

## 🛠 Architecture

- **Brain (Go)** : Le coeur de l'assistant. Gère les webhooks, la logique métier, et l'orchestration LLM.
- **Evolution API** : Interface de connexion WhatsApp.
- **Ollama** : Moteur d'inférence LLM local.
- **n8n** : Outil d'automatisation de workflows.
- **PostgreSQL** : Base de données pour les faits et l'historique.
- **Redis** : Cache pour la gestion d'état en temps réel.

## 📦 Installation

### Prérequis
- Docker & Docker Compose
- Ollama installé localement (ou accessible via réseau)

### Configuration
1. Clonez le repository :
   ```bash
   git clone https://github.com/Cabrel10/morningstar-whatsapp.git
   cd morningstar-whatsapp
   ```

2. Configurez le fichier `.env` :
   ```bash
   cp .env.example .env
   # Modifiez les variables selon votre environnement
   ```

3. Lancez les services :
   ```bash
   docker compose up -d
   ```

### Connexion WhatsApp
Accédez à l'interface de gestion :
`http://votre-ip:8081/manager/`
Utilisez la clé API configurée dans votre `.env` pour scanner le QR Code.

## 📄 Licence

Ce projet est sous licence **Apache License 2.0**. Cette licence permet une utilisation commerciale, la modification et la distribution, tout en offrant une protection par brevet. Idéal pour bâtir des solutions SaaS ou des services professionnels au-dessus de cette base.

---
Développé avec ❤️ par [Cabrel10](https://github.com/Cabrel10)
