package main

import (
	"strings"
)

// KeyValue représente une ligne clé-valeur avec un émoji optionnel
type KeyValue struct {
	Key   string
	Value string
	Emoji string
}

// ResponseStyle encapsule la structure et le style complet d'un message WhatsApp
type ResponseStyle struct {
	Title      string
	TitleEmoji string
	Sections   []Section
	Footer     string // Champ Footer indispensable pour les signatures de Poulga
}

// Section représente une partie distincte du message
type Section struct {
	Title      string
	TitleEmoji string
	Content    string
	Items      []string
	KeyValues  []KeyValue // Tableau clé-valeur indispensable pour les profils et métriques
}

// RenderWhatsApp convertit l'objet ResponseStyle en texte formaté pour WhatsApp
func RenderWhatsApp(style ResponseStyle) string {
	var sb strings.Builder

	// 1. Rendu du Titre Global
	if style.Title != "" {
		if style.TitleEmoji != "" {
			sb.WriteString(style.TitleEmoji)
			sb.WriteString(" *")
			sb.WriteString(strings.ToUpper(style.Title))
			sb.WriteString("*")
			sb.WriteByte('\n')
			sb.WriteByte('\n')
		} else {
			sb.WriteString("*")
			sb.WriteString(strings.ToUpper(style.Title))
			sb.WriteString("*")
			sb.WriteByte('\n')
			sb.WriteByte('\n')
		}
	}

	// 2. Rendu des Sections
	for _, section := range style.Sections {
		// Titre de section
		if section.Title != "" {
			if section.TitleEmoji != "" {
				sb.WriteString(section.TitleEmoji)
				sb.WriteString(" *")
				sb.WriteString(section.Title)
				sb.WriteString("*")
				sb.WriteByte('\n')
			} else {
				sb.WriteString("*")
				sb.WriteString(section.Title)
				sb.WriteString("*")
				sb.WriteByte('\n')
			}
		}

		// Contenu paragraphe
		if section.Content != "" {
			sb.WriteString(section.Content)
			sb.WriteByte('\n')
		}

		// Liste à puces (Items)
		if len(section.Items) > 0 {
			for _, item := range section.Items {
				sb.WriteString("• ")
				sb.WriteString(item)
				sb.WriteByte('\n')
			}
		}

		// Tableau Clé-Valeur (KeyValues)
		if len(section.KeyValues) > 0 {
			for _, kv := range section.KeyValues {
				if kv.Emoji != "" {
					sb.WriteString(kv.Emoji)
					sb.WriteByte(' ')
				}
				sb.WriteString("*")
				sb.WriteString(kv.Key)
				sb.WriteString(" :* ")
				sb.WriteString(kv.Value)
				sb.WriteByte('\n')
			}
		}

		sb.WriteByte('\n')
	}

	// 3. Rendu du Footer (en italique)
	if style.Footer != "" {
		sb.WriteString("_\n")
		sb.WriteString(style.Footer)
		sb.WriteString("_")
	}

	return sb.String()
}

// Fonctions utilitaires de rendu rapide
func RenderSuccess(title, message string) string {
	return RenderWhatsApp(ResponseStyle{
		Title:      title,
		TitleEmoji: "✅",
		Sections:   []Section{{Content: message}},
	})
}

func RenderError(title, message string) string {
	return RenderWhatsApp(ResponseStyle{
		Title:      title,
		TitleEmoji: "❌",
		Sections:   []Section{{Content: message}},
	})
}

func RenderInfo(title, message string) string {
	return RenderWhatsApp(ResponseStyle{
		Title:      title,
		TitleEmoji: "ℹ️",
		Sections:   []Section{{Content: message}},
	})
}
