# Poulga - Archiviste & Analyste WhatsApp

Poulga est une intelligence artificielle avancée pour WhatsApp, conçue pour agir comme la mémoire vivante et l'analyste d'un groupe. Elle combine la puissance de l'IA locale (Ollama), une connectivité robuste (Evolution API), et une architecture de mémoire structurée pour transformer les conversations en connaissances exploitables.

## 🚀 Fonctionnalités (v2)

- **Identité d'Archiviste** : Poulga n'est pas qu'un chatbot ; elle observe, mémorise et synthétise les échanges du groupe avec un ton naturel et professionnel.
- **Cartographie Sociale & Profils** : Analyse en temps réel de l'activité des membres, détection des experts et suivi des centres d'intérêt via une base de données structurée.
- **Performance Optimisée** : 
  - Modèle **Gemma 3 (4.3B)** maintenu "chaud" en RAM pour des réponses instantanées.
  - Utilisation multi-cœurs (4 vCPU) pour une inférence rapide.
  - Découplage des tâches lourdes (extraction de faits) en arrière-plan.
- **Mémoire Hybride** : Combine l'historique récent (20 messages), les faits marquants persistants et une cartographie d'activité sur 14 jours.
- **Prêt pour l'Automation** : Endpoint dédié pour générer des résumés hebdomadaires automatiques via **n8n**.

## 🛠 Architecture

- **Brain (Go)** : Orchestrateur haute performance. Gère les profils de membres, la logique de mémoire et la communication LLM.
- **Evolution API (v2.3.7)** : Interface WhatsApp stable et riche en fonctionnalités.
- **Ollama** : Moteur d'inférence LLM local.
- **PostgreSQL** : Stockage structuré (Faits, Profils, Cartographie).
- **n8n** : Automatisation des résumés et tâches planifiées.

## 📦 Installation

### Prérequis
- Docker & Docker Compose
- Ollama installé (avec le modèle `gemma3:4b`)

### Configuration
1. Clonez le repository :
   ```bash
   git clone https://github.com/Cabrel10/morningstar-whatsapp.git
   cd morningstar-whatsapp
   ```

2. Configurez le fichier `.env` :
   ```bash
   cp .env.example .env
   # Modifiez les variables (URL Ollama, API Key Evolution, etc.)
   ```

3. Lancez les services :
   ```bash
   docker compose up -d
   ```

### Intégration n8n
Poulga expose un endpoint pour vos workflows :
`GET http://brain:3000/summary/weekly?remoteJid=[ID_DU_GROUPE]`

## 📄 Licence

Ce projet est sous licence **Apache License 2.0**.
---
Développé avec ❤️ par [Cabrel10](https://github.com/Cabrel10)
