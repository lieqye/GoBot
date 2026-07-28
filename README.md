
# ⚡ GoBot

<p align="center">
  <b>A blazing-fast Telegram MTProto Userbot written in Go.</b><br>
  Powerful moderation • AI assistant • High-precision scheduler • Zero database
</p>

<p align="center">

![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?style=for-the-badge&logo=go)
![Telegram](https://img.shields.io/badge/Telegram-MTProto-26A5E4?style=for-the-badge&logo=telegram)
![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)
![Status](https://img.shields.io/badge/Status-Active-success?style=for-the-badge)

</p>

---

## 🚀 Why GoBot?

GoBot is a **self-hosted Telegram userbot** powered by **MTProto (gotd/td)**.
Unlike Bot API bots, it runs directly from **your own Telegram account**, unlocking native Telegram capabilities with the performance of Go.

### ✨ Highlights

- ⚡ Concurrent goroutine-based command execution
- 🛡️ 28+ moderation commands
- 🤖 Built-in AI assistant
- 🕒 Hidden high-precision scheduler
- 👑 Owner / Sudo / Approved permissions
- 💾 Zero database (`users.json`)
- 🌍 DMs, Groups, Supergroups & Channels
- 🎨 Automatic small-caps styling
- 🔥 Optimized for speed and low latency

---

## 📚 Table of Contents

- Features
- Installation
- Configuration
- Commands
- AI
- Hidden Scheduler
- Permissions
- Project Structure
- Changelog
- License

---

## 🛡️ Features

### Moderation
- Ban / Unban
- Kick
- Mute / Unmute
- Warn System
- Lock / Unlock
- Pin / Unpin
- Purge
- Promote / Demote

### AI
- Natural language replies
- Emoji-enhanced responses
- Action chaining (sudo only)
- Non-blocking execution

### Scheduler (.sh)
- High-precision scheduling
- DM scheduling
- Channel discussion comments
- Persistent jobs
- Cancel/List support

---

## 📦 Installation

```bash
git clone <repo>
cd gobot
go mod tidy
```

```bash
export TG_API_ID=123456
export TG_API_HASH=your_hash
export TG_PHONE=+911234567890

go run .
```

---

## 🕹️ Commands

| Category | Examples |
|----------|----------|
| General | `.help` `.ping` `.id` `.info` `.stats` |
| Moderation | `.ban` `.kick` `.mute` `.warn` `.purge` |
| Admin | `.approve` `.sudo` `.promote` `.demote` |
| AI | `.ai <prompt>` |
| Scheduler | `.sh` *(hidden, sudo only)* |

---

## 🤖 AI

`.ai` provides conversational responses for approved users.

Sudo users can chain moderation actions **only when replying to the target message**, preventing accidental or abusive execution.

---

## 🕒 Hidden Scheduler

```
.sh @username 07:00:00 Good morning!
.sh https://t.me/channel/123 09:00:00 Nice update!
.sh list
.sh cancel <id>
```

- IST timezone
- Sub-millisecond timer precision
- Persistent schedules
- Survives restarts

---

## 📁 Project Structure

```text
.
├── main.go
├── handlers.go
├── ai.go
├── scheduler.go
├── config.go
├── go.mod
└── .gitignore
```

---

## 🩹 Recent Improvements

- Fixed DM handling
- Added channel update support
- Preserved multiline formatting
- Centralized font styling
- Improved `.ping`
- Reused AI HTTP client
- Safer AI action chaining

---

## ⭐ Support

If GoBot helped you, consider giving the repository a **⭐ Star**.

It helps more people discover the project and motivates future development.

---

## 📜 License

Released under the **MIT License**.
