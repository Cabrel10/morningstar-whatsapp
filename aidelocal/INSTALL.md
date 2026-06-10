# Instructions d'installation

Pour continuer la mise en place du bot WhatsApp, veuillez exécuter les commandes suivantes sur votre serveur :

```bash
# Mise à jour des paquets et installation de Go (Golang)
sudo apt-get update && sudo apt-get install -y golang-go

# Installation de Ollama pour l'IA
curl -fsSL https://ollama.com/install.sh | sh

# Donner les permissions Docker à votre utilisateur actuel
sudo usermod -aG docker $USER
newgrp docker
```

Une fois ces installations terminées, prévenez-moi pour que je puisse passer à la configuration de la base de données, du client WhatsApp (Whatsmeow) et du service IA.
