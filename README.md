<div align="center">

# ⚡ GoBot

**A high-performance Telegram MTProto Userbot written in Go**

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![MTProto](https://img.shields.io/badge/Protocol-MTProto-blue)](https://core.telegram.org/mtproto)
[![Maintenance](https://img.shields.io/badge/Maintained-yes-brightgreen.svg)](https://github.com/lieqye/GoBot/graphs/commit-activity)

**Native moderation • AI assistant • Precision scheduler • Zero database**

[Features](#-features) • [Installation](#-installation) • [Configuration](#-configuration) • [Commands](#-commands) • [Scheduler](#-hidden-scheduler) • [License](#-license)

</div>

---

## 🚀 Overview

GoBot is a **self-hosted Telegram userbot** built on the native **MTProto protocol** ([`gotd/td`](https://github.com/gotd/td)), not the Bot API. It runs directly from your own Telegram account, unlocking full native capabilities — group moderation, AI-powered responses, and precision message scheduling — with the speed and efficiency of Go's concurrent architecture.

> ⚠️ **Disclaimer:** This tool automates actions from a personal Telegram account. Use a secondary/throwaway account if possible. Normal group moderation is generally acceptable, but any account risk is yours alone.

---

## ✨ Features

| Feature | Description |
|---------|-------------|
| **⚡ Concurrent Execution** | Every message spawns its own goroutine — slow AI calls never block urgent moderation commands |
| **🛡️ 28+ Moderation Commands** | Ban, mute, kick, warn, purge, promote, demote, pin, lock, and more |
| **🤖 AI Assistant** | Natural-language replies with emoji flair; sudo users can chain moderation actions via `.ai` |
| **🕒 Precision Scheduler** | Sub-millisecond accuracy for scheduled DMs and channel comments (hidden `.sh` command) |
| **👑 Tiered Permissions** | Owner → Sudo → Approved → Silent ignore; granular access control via `users.json` |
| **💾 Zero Database** | All state (permissions, warnings, schedules) persisted in plain JSON files |
| **🌍 Universal Chat Support** | DMs, basic groups, supergroups, and channels — all fully supported |
| **🎨 Consistent Styling** | Centralized small-caps font rendering on every outgoing message |

---

## 📋 Prerequisites

- [Go](https://golang.org/dl/) 1.22 or later
- A Telegram account (preferably secondary)
- API credentials from [my.telegram.org](https://my.telegram.org)

---

## 📦 Installation

```bash
# Clone the repository
git clone https://github.com/lieqye/GoBot.git
cd GoBot

# Download dependencies
go mod tidy

# Build (optional but recommended)
go build .
```

---

## 🔧 Configuration

Set the following environment variables before running:

```bash
export TG_API_ID=12345678          # Your API ID from my.telegram.org
export TG_API_HASH=your_api_hash   # Your API Hash from my.telegram.org
export TG_PHONE=+919876543210      # Phone number of the account
```

**First run** will prompt for the Telegram login code (and 2FA password if enabled) in the terminal. Your session is then saved to `session.json` — **never commit or share this file**.

Your account is automatically registered as the **owner** (permanent sudo) in `users.json` on first login.

---

## 🕹️ Commands

All commands use a `.` prefix. Reply to a target user's message for commands that require one.

### General Commands (Approved + Sudo)

| Command | Target | Description |
|---------|--------|-------------|
| `.help` | — | Display the command menu |
| `.ping` | — | Check bot response latency |
| `.id` | Optional reply | Get user or chat ID |
| `.info` | Reply | Show info about a user |
| `.stats` | — | Uptime and commands handled |
| `.ban` / `.unban` | Reply | Ban or unban a user |
| `.mute [minutes]` / `.unmute` | Reply | Mute or unmute a user |
| `.kick` | Reply | Remove a user (no permanent ban) |
| `.warn` | Reply | Issue a warning (3-strike tracking) |
| `.warnings` | Reply | Check a user's warning count |
| `.resetwarn` | Reply | Clear all warnings for a user |
| `.lock` / `.unlock` | — | Restrict/allow non-admin messaging |
| `.pin` / `.unpin` | Reply | Pin or unpin a message |
| `.del` | Reply | Delete a message |
| `.purge <n>` | — | Delete last `n` messages (max 100) |
| `.ai <question>` | Optional reply | AI-generated witty response |

### Admin Commands (Sudo Only)

| Command | Target | Description |
|---------|--------|-------------|
| `.approve` | Reply | Grant a user general command access |
| `.approve rm` | Reply | Revoke a user's approved status |
| `.approved` | — | List all approved users |
| `.sudo` | Reply | Grant full sudo privileges |
| `.sudo rm` | Reply | Revoke sudo (cannot remove owner) |
| `.sudolist` | — | List all sudo users |
| `.promote [title]` | Reply | Promote user to group admin |
| `.demote` | Reply | Demote an admin |

> 🔇 **Unrecognized users receive no response.** Commands from anyone not in `users.json` are silently ignored.

---

## 🤖 AI Command

`.ai <question>` generates a conversational, emoji-enhanced, small-caps reply for any approved user.

**Action Chaining (Sudo Only):** When a sudo user replies to a target message and their prompt implies an action (e.g., `.ai ban this guy`), the bot executes the moderation action *in addition* to the text reply. Non-sudo users receive only the text response, regardless of phrasing — this prevents accidental abuse.

---

## 🕐 Hidden Scheduler

The `.sh` command is **intentionally hidden** from `.help` and silently ignores non-sudo users.

```bash
.sh help                                    # Show scheduler-specific help
.sh @username 07:00:00 Good morning!        # Schedule a DM
.sh https://t.me/channel/123 09:00:00 Nice! # Schedule a channel comment
.sh list                                    # View pending jobs
.sh cancel <id>                             # Cancel a scheduled job
```

- **Timezone:** IST (`HH:MM[:SS]`, optional AM/PM)
- **Precision:** Sub-millisecond via Go's `time.AfterFunc`
- **Persistence:** Jobs survive restarts via `schedules.json`
- **Overdue handling:** Missed jobs fire immediately on startup

---

## 📁 Project Structure

```
GoBot/
├── main.go          # MTProto client, session handling, update dispatcher
├── handlers.go      # Command parsing, permission checks, 28+ commands
├── config.go        # users.json read/write (sudo/approved/warnings)
├── ai.go            # AI API integration + font styling + emoji flair
├── scheduler.go     # Hidden .sh command: scheduling + persistence
├── go.mod           # Go module definition
└── .gitignore       # Excludes session.json, users.json, schedules.json
```

---

## 🛡️ Security & Permissions

- The running account must be a **group admin** with relevant rights for moderation commands.
- **Owner** (first login) cannot be demoted.
- **Sudo** users have full access. **Approved** users have general commands only.
- **Everyone else** is invisible — no errors, no replies.

---

## 🩹 Changelog

### Recent Fixes
- **DM Support:** Fixed silent dropping of direct messages by falling back to chat peer when `FromID` is unset.
- **Channel Support:** Added handler for `UpdateNewChannelMessage` alongside `UpdateNewMessage`.
- **Multiline Formatting:** Preserved whitespace in font styler — line breaks and tabs are now retained.
- **Centralized Styling:** All outgoing messages pass through a single styling pipeline for consistent rendering.
- **Monospace Entities:** Added UTF-16-aware entity offsets for proper code formatting with emoji.
- **Robust Ping:** Handles multiple Telegram update shapes across chat types.
- **AI Optimization:** Reused HTTP client for connection keep-alive, reducing latency on repeated calls.
- **Safer Action Chaining:** AI-triggered actions now require an explicit reply target before executing.

---

## ⚠️ Important Notes

- Ensure `go build .` compiles cleanly before deployment — `gotd/td` field names may shift between versions.
- Key areas to verify: `tg.MessagesGetHistoryRequest`, `tg.ChannelsEditBannedRequest`, `tg.ChannelsEditAdminRequest`, and `auth.UserAuthenticator` interfaces.
- Channel comment scheduling requires resolving discussion groups; private channel links require prior membership.

---

## 📜 License

Released under the [MIT License](LICENSE).

---

<div align="center">

**[⭐ Star this repo](https://github.com/lieqye/GoBot)** if you found it useful!

</div>
