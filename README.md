# ⚡ Telegram Userbot (Go)

A fast, self-hosted Telegram **userbot** — runs from your own account via
MTProto ([`gotd/td`](https://github.com/gotd/td)), not the Bot API. 28+
group-moderation commands, a sudo/approved permission system, an AI chat
command, and every reply dressed up with emojis + a small-caps font.

Each incoming message is handled in its own goroutine, so a slow `.ai`
call never blocks a `.ban` from firing instantly.

---

## ✨ Features

- 🔐 **Two-tier permissions** — sudo (full access) and approved (general
  commands only). Everyone else is invisible to the bot.
- 🛡️ **28+ commands** — bans, mutes, kicks, warnings, pins, purges, admin
  promotion, and more.
- 🤖 **`.ai`** — witty AI replies via a free API, styled in small-caps with
  emoji flair. Sudo users can chain an action off a natural-language ask.
- ⚡ **Fast** — concurrent message handling, no blocking on slow API calls.
- 💾 **Zero database** — sudo/approved lists live in a plain `users.json`.

---

## 📦 Setup

### 1. Get API credentials

1. Go to [my.telegram.org](https://my.telegram.org) and log in with the
   phone number this userbot will run as.
2. **API development tools** → create an app → copy your `api_id` and
   `api_hash`.

> ⚠️ **Use a secondary/throwaway account if you can.** Automating actions
> from a personal account can violate Telegram's ToS if used for spam or
> mass messaging. Normal moderation in groups you already run is generally
> fine, but any ban risk lands on your account, not Telegram's bot infra.

### 2. Clone & install

```bash
git clone https://github.com/lieqye/GoBot
cd myuserbot
go mod tidy
```

### 3. Configure & run

```bash
export TG_API_ID=12345678
export TG_API_HASH=your_api_hash_here
export TG_PHONE=+919876543210

go run .
```

First run asks for the Telegram login code (and 2FA password, if set)
right in the terminal. After that your session is saved to
`session.json` — **never commit or share this file**, it's equivalent to
being logged into your account.

Your own account is automatically saved as the **owner** (permanent sudo)
on first login, in `users.json` — also gitignored by default.

---

## 🕹️ Commands

All commands are typed as a normal message with a `.` prefix, in any chat
your account is in. Reply to someone's message for commands that need a
target user.

### General (approved + sudo)

| Command | Description |
|---|---|
| `.help` | 📜 show the command menu |
| `.ping` | 🏓 check response speed |
| `.id` | 🆔 get a user's or the chat's id |
| `.info` | ℹ️ info about a replied-to user |
| `.stats` | 📊 uptime & commands handled |
| `.ban` / `.unban` | 🔨 / ✅ reply to ban/unban a user |
| `.mute [minutes]` / `.unmute` | 🔇 / 🔊 reply to mute/unmute a user |
| `.kick` | 👢 reply to remove a user (no permanent ban) |
| `.warn` | ⚠️ reply to warn a user (tracked, 3-strike hint) |
| `.warnings` | 📋 reply to check a user's warning count |
| `.resetwarn` | 🧹 reply to clear a user's warnings |
| `.lock` / `.unlock` | 🔒 / 🔓 stop/allow non-admins from messaging |
| `.pin` / `.unpin` | 📌 / 📍 reply to pin/unpin a message |
| `.del` | 🗑️ reply to delete a message |
| `.purge <n>` | 🧹 delete the last `n` messages (max 100) |
| `.ai <question>` | 🤖 witty AI reply with attitude & emojis |

### Sudo only 👑

| Command | Description |
|---|---|
| `.approve` | 🟢 reply to grant a user command access |
| `.approve rm` | 🔴 reply to revoke a user's access |
| `.approved` | 📋 list everyone currently approved |
| `.sudo` | 👑 reply to grant a user full sudo |
| `.sudo rm` | ⛔ reply to revoke sudo (can't remove the owner) |
| `.sudolist` | 📋 list everyone with sudo |
| `.promote [title]` | ⭐ reply to promote a user to admin |
| `.demote` | ⬇️ reply to demote an admin |

Anyone not in `users.json` gets **no response at all** — commands from
randoms are silently ignored.

### `.ai` and actions

`.ai <anything>` always replies with an AI-generated, emoji'd, small-caps
response — available to any approved user. If a **sudo** user's question
also implies an action (e.g. `.ai ban this guy` while replying to their
message), the bot performs it too. Approved (non-sudo) users only ever get
the text reply — asking `.ai` to ban someone does nothing beyond the joke.

---

## 🔧 Requirements for moderation commands

The account running this userbot must itself be an **admin** in the group
with the relevant rights (ban/pin/promote), exactly as if you were doing
it by hand.

---

## 📁 Project structure

```
.
├── main.go       # login flow, session handling, update dispatcher
├── handlers.go   # command parsing, permissions, all 28+ commands
├── config.go     # users.json (sudo/approved/warnings) read & write
├── ai.go         # AI API call + small-caps font styling + emoji flair
├── go.mod
└── .gitignore    # keeps session.json and users.json out of git
```

---

## ⚠️ Before you deploy

This was written and reviewed without a live Go toolchain to compile
against (no network access in the authoring environment), so please run:

```bash
go build .
```

and skim any compiler errors before running it for real — `gotd/td`'s
exact field/method names can shift slightly between versions. The spots
most likely to need a small tweak:

- `tg.MessagesGetHistoryRequest` / `tg.ChannelsEditBannedRequest` /
  `tg.ChannelsEditAdminRequest` field names
- `auth.UserAuthenticator` interface method signatures, if you're on a
  notably older/newer `gotd/td` release

---

## 📜 License

MIT — see [LICENSE](LICENSE).
