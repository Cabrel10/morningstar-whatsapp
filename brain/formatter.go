package main

import (
	"strings"
)

type ResponseStyle struct {
	Title      string
	TitleEmoji string
	Sections   []Section
}

type Section struct {
	Title      string
	TitleEmoji string
	Content    string
	Items      []string
}

func RenderWhatsApp(style ResponseStyle) string {
	var sb strings.Builder
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
	for _, section := range style.Sections {
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
		if section.Content != "" {
			sb.WriteString(section.Content)
			sb.WriteByte('\n')
		}
		if len(section.Items) > 0 {
			for _, item := range section.Items {
				sb.WriteString("• ")
				sb.WriteString(item)
				sb.WriteByte('\n')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
