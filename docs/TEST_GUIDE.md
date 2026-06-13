# Guide de Test et Évaluation de l'Architecture Poulga

Ce guide explique comment tester les nouvelles fonctionnalités de navigation web et évaluer la robustesse de l'architecture actuelle.

## 1. Test des Capacités Web

### Lecture Directe (.lire)
- **Test basique :** `.lire https://go.dev`
  - *Attendu :* Poulga doit extraire le contenu principal et en faire une synthèse.
- **Test de sécurité (SSRF) :** `.lire http://localhost:11434`
  - *Attendu :* Message d'erreur indiquant que l'accès aux réseaux locaux est interdit.
- **Test de résilience :** `.lire [URL d'un site protégé par Cloudflare]`
  - *Attendu :* Si CAMOUFOX_API_URL est configuré, cela doit passer. Sinon, une erreur ou un contenu limité.

### Recherche Google (.google)
- **Test :** `.google actualités IA aujourd'hui`
  - *Attendu :* Résumé des premiers résultats de recherche.

### Navigation Autonome (LLM Tool)
- **Test :** "Peux-tu me dire ce qu'il y a sur le site https://github.com/PuerkitoBio/goquery ?"
  - *Attendu :* Poulga doit détecter l'URL, utiliser l'outil `web_read` et répondre avec les informations du site.

## 2. Évaluation de l'Architecture

### Points de Robustesse
- **Gestion du Contexte :** Vérifiez que Poulga ne "perd pas le fil" après plusieurs échanges. Utilisez `.clear` pour repartir à zéro.
- **Vitesse de Réponse :** Le scraping + synthèse IA peut prendre 10-20 secondes sur CPU. C'est le prix de l'autonomie locale.
- **Consommation RAM :** Surveillez l'usage mémoire avec `.statut-serveur` pendant un scraping intensif.

### Angles Morts Identifiés
- **JavaScript :** Le scraper natif ne supporte pas le JS. Les sites full-React/Vue peuvent apparaître vides.
- **Citations :** Si vous répondez à un message de Poulga, vérifiez qu'elle prend bien en compte le texte cité dans sa réponse.

## 3. Commandes Utiles pour l'Évaluation
- `.aide` : Voir toutes les commandes disponibles.
- `.statut-serveur` : Surveiller les ressources du VPS.
- `.stats` : Voir votre réputation et usage.
- `.memoire` : Vérifier ce que Poulga a retenu de vous.

---
*Poulga Agentic System - v5.8 (Autonomous Browsing Edition)*
