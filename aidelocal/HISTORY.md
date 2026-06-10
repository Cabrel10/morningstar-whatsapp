# Historique des actions

## [2026-06-10] Phase 1 : Initialisation et Recherche
- Analyse de l'environnement : Python 3.11 et Docker sont présents. Go et Ollama sont absents.
- Création du dossier `aidelocal` pour la communication des besoins d'installation système.
- Rédaction de `aidelocal/INSTALL.md` avec les commandes `sudo` nécessaires pour Go et Ollama.
- [OK] Installation de Go et Ollama terminée par l'utilisateur.

## [2026-06-10] Phase 2 : Mise en place de l'Infrastructure
- Préparation du fichier `docker-compose.yml` pour PostgreSQL et Redis.
- Lancement du téléchargement du modèle Qwen2.5-VL-3B via Ollama.
