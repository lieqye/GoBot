package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// ---- config from environment ----------------------------------------------
// Get api_id / api_hash from https://my.telegram.org (log in with the
// account you want this userbot to run as).
//
//   export TG_API_ID=12345678
//   export TG_API_HASH=abcdef0123456789abcdef0123456789
//   export TG_PHONE=+919876543210
//
// First run will ask for the login code sent to that number (and your 2FA
// password, if you have one) directly in the terminal. After that, the
// session is saved to session.json so you won't be asked again.

func main() {
	apiID, err := strconv.Atoi(os.Getenv("TG_API_ID"))
	if err != nil {
		fmt.Println("set TG_API_ID (int) — get it from https://my.telegram.org")
		os.Exit(1)
	}
	apiHash := os.Getenv("TG_API_HASH")
	if apiHash == "" {
		fmt.Println("set TG_API_HASH — get it from https://my.telegram.org")
		os.Exit(1)
	}
	phone := os.Getenv("TG_PHONE")
	if phone == "" {
		fmt.Println("set TG_PHONE (with country code, e.g. +919876543210)")
		os.Exit(1)
	}

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Println("failed to load users.json:", err)
		os.Exit(1)
	}

	dispatcher := tg.NewUpdateDispatcher()

	client := telegram.NewClient(apiID, apiHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: "session.json"},
		UpdateHandler:  dispatcher,
	})

	ctx := context.Background()

	err = client.Run(ctx, func(ctx context.Context) error {
		flow := auth.NewFlow(termAuth{phone: phone}, auth.SendCodeOptions{})
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return fmt.Errorf("auth failed: %w", err)
		}

		self, err := client.Self(ctx)
		if err != nil {
			return fmt.Errorf("could not fetch self: %w", err)
		}
		if cfg.OwnerID == 0 {
			cfg.OwnerID = self.ID
			if err := cfg.Save(); err != nil {
				return err
			}
		}

		bot := NewBot(client.API(), cfg, self)

		sched, err := NewScheduler(bot)
		if err != nil {
			return fmt.Errorf("failed to load schedules.json: %w", err)
		}
		bot.sched = sched

		// Telegram delivers messages from private chats/basic groups via
		// UpdateNewMessage, but supergroups AND channels use a *separate*
		// update type, UpdateNewChannelMessage. Registering only the first
		// one (the easy mistake) means the bot silently never sees a single
		// message posted in a supergroup or channel. Both are wired here so
		// it works everywhere: DMs, basic groups, supergroups, channels.
		dispatch := func(ctx context.Context, e tg.Entities, msg *tg.Message) {
			// Each message is handled in its own goroutine so a slow
			// command (like .ai hitting the network) never delays the
			// bot's response to anything else happening concurrently.
			go func() {
				if err := bot.HandleMessage(ctx, e, msg); err != nil {
					fmt.Println("handler error:", err)
				}
			}()
		}

		dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
			if msg, ok := u.Message.(*tg.Message); ok {
				dispatch(ctx, e, msg)
			}
			return nil
		})
		dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
			if msg, ok := u.Message.(*tg.Message); ok {
				dispatch(ctx, e, msg)
			}
			return nil
		})

		fmt.Printf("✅ logged in as %s (id=%d) — userbot is live\n", displayName(self), self.ID)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
}

// termAuth implements auth.UserAuthenticator by prompting on stdin. Used
// only for the one-time login; after that, session.json is reused and none
// of this runs again.
type termAuth struct {
	phone string
}

func (a termAuth) Phone(_ context.Context) (string, error) {
	return a.phone, nil
}

func (a termAuth) Password(_ context.Context) (string, error) {
	return prompt("2FA password: ")
}

func (a termAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	return prompt("login code (check Telegram): ")
}

func (a termAuth) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

func (a termAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("account does not exist — sign up manually in the Telegram app first")
}

func prompt(label string) (string, error) {
	fmt.Print(label)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
