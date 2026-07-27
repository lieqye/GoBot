# GᴏBᴏᴛ

<div align="center">

## **Aɪ Cʜᴀᴛ • Mᴏᴅᴇʀᴀᴛɪᴏɴ • MᴛPʀᴏᴛᴏ • Gᴏ**

A powerful, self-hosted Telegram Userbot written in **Go**, built on **MTProto** instead of the Bot API.

<p>
<img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go"/>
<img src="https://img.shields.io/badge/Platform-Telegram-26A5E4?style=for-the-badge&logo=telegram"/>
<img src="https://img.shields.io/badge/License-MIT-success?style=for-the-badge"/>
</p>

*Fast • Lightweight • AI Powered • Concurrent • Self Hosted*

</div>

---

# ✨ Overview

GoBot is a modern Telegram Userbot that runs directly from **your Telegram account** using MTProto.

Instead of relying on the Bot API, GoBot behaves like a real Telegram client, making moderation faster and more capable while remaining lightweight and easy to deploy.

---

# 🚀 Highlights

- 🤖 Beautiful **Aɪ Cʜᴀᴛ**
- 🛡️ 28+ moderation commands
- 👑 Owner / Sudo / Approved permission system
- ⚡ Concurrent goroutines
- 💾 No database required
- 📊 Statistics & utilities
- 🔒 Secure local session storage
- 🧩 Clean Go codebase

---

# 📦 Installation

```bash
git clone https://github.com/lieqye/GoBot.git
cd GoBot
go mod tidy
go run .
```

---

# ⚙️ Configuration

```bash
export TG_API_ID=123456
export TG_API_HASH=YOUR_API_HASH
export TG_PHONE=+911234567890

go run .
```

---

# 🤖 Aɪ Cʜᴀᴛ

Use:

```text
.ai <question>
```

The bot generates styled AI replies with elegant formatting. Sudo users can also combine AI prompts with moderation actions.

---

# 🛡️ Moderation

```
.ban
.unban
.kick
.mute
.unmute
.warn
.resetwarn
.lock
.unlock
.pin
.unpin
.del
.purge
.promote
.demote
```

---

# 📂 Project Structure

```text
main.go
handlers.go
config.go
ai.go
go.mod
users.json
session.json
```

---

# 🛣️ Roadmap

- [ ] Plugin system
- [ ] Custom AI providers
- [ ] Better logging
- [ ] Dashboard
- [ ] Multi-language support

---

# 🤝 Contributing

Issues, pull requests and suggestions are always welcome.

If this project helps you, consider giving it a ⭐.

---

# 📜 License

MIT License.
