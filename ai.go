package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// smallCaps maps ASCII letters to the requested compact small-caps look.
// The first letter of each word stays normal, while the rest are converted.
var smallCaps = map[rune]rune{
	'a': 'ᴀ', 'b': 'ʙ', 'c': 'ᴄ', 'd': 'ᴅ', 'e': 'ᴇ', 'f': 'ꜰ',
	'g': 'ɢ', 'h': 'ʜ', 'i': 'ɪ', 'j': 'ᴊ', 'k': 'ᴋ', 'l': 'ʟ',
	'm': 'ᴍ', 'n': 'ɴ', 'o': 'ᴏ', 'p': 'ᴘ', 'q': 'ꞯ', 'r': 'ʀ',
	's': 'ꜱ', 't': 'ᴛ', 'u': 'ᴜ', 'v': 'ᴠ', 'w': 'ᴡ', 'x': 'x',
	'y': 'ʏ', 'z': 'ᴢ',
}

// StyleFont converts plain text into the bot's signature small-caps look.
// It preserves punctuation and emoji while keeping the output visually tight.
func StyleFont(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	words := strings.Fields(s)
	for i, w := range words {
		runes := []rune(w)
		if i > 0 {
			b.WriteByte(' ')
		}
		for j, r := range runes {
			if j == 0 && isLetter(r) {
				b.WriteRune(toUpperRune(r))
				continue
			}
			if mapped, ok := smallCaps[toLowerRune(r)]; ok {
				b.WriteRune(mapped)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func toLowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

func toUpperRune(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - ('a' - 'A')
	}
	return r
}

// flavorEmojis get sprinkled onto AI replies so every message has a bit of
// attitude, since the upstream API returns plain text with no emoji of its own.
var flavorEmojis = []string{"😂", "💀", "🔥", "😎", "🤡", "🙃", "😤", "🫡", "😹", "⚡"}

const aiAPIBase = "https://error-ai-api.vercel.app/chat"

type aiAPIResponse struct {
	Response string `json:"response"`
}

var aiHTTPClient = &http.Client{
	Timeout: 3 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
	},
}

// AskAI calls the AI backend and returns a styled, emoji-flavored reply
// ready to send to Telegram.
func AskAI(ctx context.Context, query string) (string, error) {
	u := aiAPIBase + "?message=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai api unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed aiAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("ai api bad response: %w", err)
	}
	if strings.TrimSpace(parsed.Response) == "" {
		return "", fmt.Errorf("ai api returned empty response")
	}

	lead := flavorEmojis[rand.Intn(len(flavorEmojis))]
	tail := flavorEmojis[rand.Intn(len(flavorEmojis))]
	styled := StyleFont(parsed.Response)
	return fmt.Sprintf("%s %s %s", lead, styled, tail), nil
}
