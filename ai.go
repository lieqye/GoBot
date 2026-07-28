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

// smallCaps maps lowercase ascii letters to their small-caps unicode
// equivalent, matching the "I Kᴇᴇᴘ Tʀᴀᴄᴋ Oғ Oᴜʀ Cᴏɴᴠᴇʀsᴀᴛɪᴏɴ" style:
// first letter of each word stays a normal capital, the rest of the word
// is lower-cased and run through this table (letters with no small-caps
// glyph, like q/s/x, are left as-is).
var smallCaps = map[rune]rune{
	'a': 'ᴀ', 'b': 'ʙ', 'c': 'ᴄ', 'd': 'ᴅ', 'e': 'ᴇ', 'f': 'ғ',
	'g': 'ɢ', 'h': 'ʜ', 'i': 'ɪ', 'j': 'ᴊ', 'k': 'ᴋ', 'l': 'ʟ',
	'm': 'ᴍ', 'n': 'ɴ', 'o': 'ᴏ', 'p': 'ᴘ', 'q': 'q', 'r': 'ʀ',
	's': 's', 't': 'ᴛ', 'u': 'ᴜ', 'v': 'ᴠ', 'w': 'ᴡ', 'x': 'x',
	'y': 'ʏ', 'z': 'ᴢ',
}

// StyleFont converts plain text into the bot's signature small-caps look.
// Non-letters (numbers, punctuation, emoji, @mentions) pass through untouched.
func StyleFont(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		var b strings.Builder
		runes := []rune(w)
		for j, r := range runes {
			lower := toLowerRune(r)
			if j == 0 && isLetter(r) {
				// first letter of the word: keep as a normal capital
				b.WriteRune(toUpperRune(r))
				continue
			}
			if mapped, ok := smallCaps[lower]; ok {
				b.WriteRune(mapped)
			} else {
				b.WriteRune(r)
			}
		}
		words[i] = b.String()
	}
	return strings.Join(words, " ")
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

// httpClient is reused across calls so TCP/TLS connections to the AI API
// get pooled instead of renegotiated every time — noticeably faster on
// back-to-back .ai calls. The tuned transport keeps a warm connection
// ready instead of tearing it down between requests.
var httpClient = &http.Client{
	Timeout: 6 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	},
}

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// AskAI calls the AI backend and returns an emoji-flavored reply. Font
// styling is applied once, centrally, when the reply is actually sent —
// not here — so callers get plain text back.
func AskAI(ctx context.Context, query string) (string, error) {
	u := aiAPIBase + "?message=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Do(req)
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

	lead := flavorEmojis[rng.Intn(len(flavorEmojis))]
	tail := flavorEmojis[rng.Intn(len(flavorEmojis))]
	return fmt.Sprintf("%s %s %s", lead, parsed.Response, tail), nil
}
