# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-07-28

### Added
- **Core MTProto Client** — Native Telegram userbot built on [`gotd/td`](https://github.com/gotd/td), bypassing Bot API limitations.
- **28+ Moderation Commands** — Comprehensive group management: ban, unban, kick, mute, unmute, warn, warnings, resetwarn, lock, unlock, pin, unpin, delete, purge, promote, demote.
- **AI Assistant (`.ai`)** — Natural-language replies with emoji flair and small-caps styling. Sudo users can chain moderation actions when replying to target messages.
- **Hidden Scheduler (`.sh`)** — High-precision message/comment scheduling with sub-millisecond accuracy. Supports DM scheduling and channel discussion comments. Persistent across restarts via `schedules.json`.
- **Tiered Permission System** — Owner (permanent), Sudo (full access), Approved (general commands), and Silent Ignore for unrecognized users. Managed via `users.json`.
- **Concurrent Goroutine Execution** — Every incoming message handled in its own goroutine; slow operations never block urgent moderation.
- **Universal Chat Support** — Full compatibility with DMs, basic groups, supergroups, and channels.
- **Centralized Font Styling** — All outgoing messages automatically rendered in small-caps with consistent emoji decoration.
- **Zero-Database Architecture** — All state persisted in plain JSON files (`users.json`, `schedules.json`).
- **Session Persistence** — Secure session storage in `session.json` with automatic re-authentication.
- **UTF-16 Aware Entity Offsets** — Proper monospace/code formatting that correctly handles multi-unit emoji characters.
- **Connection Reuse** — Shared HTTP client for AI API calls with connection keep-alive for reduced latency.

### Fixed
- **DM Handling** — Resolved silent dropping of direct messages by falling back to chat peer when `FromID` is unset.
- **Channel Message Support** — Added `UpdateNewChannelMessage` handler alongside `UpdateNewMessage` for supergroup/channel compatibility.
- **Multiline Formatting** — Font styler now preserves whitespace (line breaks, tabs) instead of collapsing to single spaces.
- **Consistent Styling** — All replies, errors, and help text now pass through a centralized styling pipeline.
- **Robust Ping** — `.ping` command now handles multiple Telegram update shapes across different chat types.
- **Safer AI Action Chaining** — Action execution now requires an explicit reply target, preventing misfires from vague phrasing.

### Security
- **Action Guardrails** — AI-triggered moderation actions are restricted to sudo users with explicit reply targets only.
- **Silent Failure** — Unrecognized users receive zero response, preventing information leakage about bot capabilities.
- **Owner Immutability** — The initial owner account cannot be demoted or removed from the sudo list.

---

## Release Checklist

- [x] All core features implemented and tested
- [x] DM, group, supergroup, and channel support verified
- [x] Permission system (owner/sudo/approved) functional
- [x] AI command with safe action chaining implemented
- [x] Hidden scheduler with persistence completed
- [x] README documentation complete
- [x] `.gitignore` excludes sensitive files (`session.json`, `users.json`, `schedules.json`)
- [x] MIT License included

---

[1.0.0]: https://github.com/lieqye/GoBot/releases/tag/v1.0.0
