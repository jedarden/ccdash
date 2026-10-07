package ui

import (
	"fmt"
	"strings"
	"time"
)

// Utility functions

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func wrapText(text string, width int) string {
	if len(text) <= width {
		return text
	}

	var lines []string
	words := strings.Fields(text)
	currentLine := ""

	for _, word := range words {
		if len(currentLine)+len(word)+1 <= width {
			if currentLine == "" {
				currentLine = word
			} else {
				currentLine += " " + word
			}
		} else {
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return strings.Join(lines, "\n")
}

// wrapTextPreserveBreaks wraps text while preserving explicit line breaks
func wrapTextPreserveBreaks(text string, width int) string {
	// Split by newlines to preserve paragraph structure
	paragraphs := strings.Split(text, "\n")
	var wrappedParagraphs []string

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			// Preserve empty lines
			wrappedParagraphs = append(wrappedParagraphs, "")
			continue
		}

		// Wrap this paragraph
		if len(para) <= width {
			wrappedParagraphs = append(wrappedParagraphs, para)
			continue
		}

		// Word wrap this line
		words := strings.Fields(para)
		currentLine := ""
		for _, word := range words {
			if currentLine == "" {
				currentLine = word
			} else if len(currentLine)+len(word)+1 <= width {
				currentLine += " " + word
			} else {
				wrappedParagraphs = append(wrappedParagraphs, currentLine)
				currentLine = word
			}
		}
		if currentLine != "" {
			wrappedParagraphs = append(wrappedParagraphs, currentLine)
		}
	}

	return strings.Join(wrappedParagraphs, "\n")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}
