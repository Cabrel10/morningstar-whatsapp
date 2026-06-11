# Historique des actions

## [2026-06-10] Phase 1 : Initialisation et Recherche
- Analyse de l'environnement : Python 3.11 et Docker sont présents. Go et Ollama sont absents.
- Création du dossier `aidelocal` pour la communication des besoins d'installation système.
- Rédaction de `aidelocal/INSTALL.md` avec les commandes `sudo` nécessaires pour Go et Ollama.
- [OK] Installation de Go et Ollama terminée par l'utilisateur.

## [2026-06-11] Phase 2 : Correction et Finalisation de l'Infrastructure
- [FIX] Pull du modèle `nomic-embed-text` pour la mémoire vectorielle (RAG).
- [FIX] Application manuelle de `init.sql` pour créer les tables manquantes (`group_members`, `message_embeddings`, etc.).
- [INFO] Vérification de l'état des conteneurs : Evolution API, Redis, DB et Brain sont fonctionnels.
- [INFO] Le Brain est opérationnel sur le port 3001 (externe) / 3000 (interne) et reçoit les webhooks d'Evolution API.
