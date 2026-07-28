package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf16"

	"github.com/gotd/td/tg"
)

// Bot wires together the raw MTProto client, our permission config, and
// the userbot's own identity.
type Bot struct {
	api       *tg.Client
	cfg       *Config
	self      *tg.User
	startedAt time.Time
	cmdCount  int64 // atomic

	// sched powers the hidden ".sh" command. Deliberately not wired into
	// commandList — see HandleMessage's ".sh" case, which checks sudo and
	// dispatches directly instead of going through the generic gate/help
	// listing, so it never shows up in .help and non-sudo users don't
	// even get told it exists.
	sched *Scheduler
}

func NewBot(api *tg.Client, cfg *Config, self *tg.User) *Bot {
	return &Bot{api: api, cfg: cfg, self: self, startedAt: time.Now()}
}

// commandList is shown by .help. Keep in sync with the switch in HandleMessage.
// Base is the exact token typed (e.g. ".mute"); Cmd is the display form
// shown in .help (e.g. ".mute [minutes]").
var commandList = []struct {
	Base, Cmd, Desc string
	SudoOnly        bool
}{
	// ---- info / utility ----
	{".help", ".help", "📜 show this menu", false},
	{".ping", ".ping", "🏓 check response speed", false},
	{".id", ".id", "🆔 get user/chat id (reply optional)", false},
	{".info", ".info", "ℹ️ info about a replied-to user", false},
	{".stats", ".stats", "📊 uptime & commands handled", false},

	// ---- moderation (supergroups) ----
	{".ban", ".ban", "🔨 reply to ban a user", false},
	{".unban", ".unban", "✅ reply to unban a user", false},
	{".mute", ".mute [minutes]", "🔇 reply to mute a user", false},
	{".unmute", ".unmute", "🔊 reply to unmute a user", false},
	{".kick", ".kick", "👢 reply to remove a user (no ban)", false},
	{".warn", ".warn", "⚠️ reply to warn a user", false},
	{".warnings", ".warnings", "📋 reply to check a user's warnings", false},
	{".resetwarn", ".resetwarn", "🧹 reply to clear a user's warnings", false},
	{".lock", ".lock", "🔒 stop non-admins from messaging", false},
	{".unlock", ".unlock", "🔓 allow everyone to message again", false},

	// ---- messages ----
	{".pin", ".pin", "📌 pin the replied-to message", false},
	{".unpin", ".unpin", "📍 unpin the replied-to message", false},
	{".del", ".del", "🗑️ delete the replied-to message", false},
	{".purge", ".purge <n>", "🧹 delete the last n messages", false},

	// ---- fun ----
	{".ai", ".ai <question>", "🤖 ask the ai anything, with attitude", false},

	// ---- admin (sudo only) ----
	{".promote", ".promote [title]", "⭐ reply to promote a user to admin", true},
	{".demote", ".demote", "⬇️ reply to demote an admin", true},
	{".approve", ".approve", "🟢 reply to grant a user command access", true},
	{".approverm", ".approve rm", "🔴 reply to revoke a user's access", true},
	{".approved", ".approved", "📋 list all approved users", true},
	{".sudo", ".sudo", "👑 reply to grant full sudo", true},
	{".sudorm", ".sudo rm", "⛔ reply to revoke sudo", true},
	{".sudolist", ".sudolist", "📋 list all sudo users", true},
}

// HandleMessage is invoked for every new message the account can see, in
// any chat type — DMs, basic groups, supergroups, and channels (see
// main.go, which wires both UpdateNewMessage and UpdateNewChannelMessage
// into this same entry point). It only reacts to messages starting with
// "." (the command prefix).
func (b *Bot) HandleMessage(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	text := strings.TrimSpace(msg.Message)
	if !strings.HasPrefix(text, ".") || len(text) < 2 {
		return nil
	}

	senderID, ok := b.senderID(msg)
	if !ok {
		return nil
	}

	// Anyone not approved (and not sudo) is invisible to the bot entirely —
	// no reply, no trace, so randoms spamming commands get nothing back.
	if !b.cfg.IsApproved(senderID) {
		return nil
	}

	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]
	isSudo := b.cfg.IsSudo(senderID)

	// ".approve rm" / ".sudo rm" are subcommands — fold the "rm" arg into
	// a distinct internal command name so the switch below stays flat.
	internalCmd := cmd
	if (cmd == ".approve" || cmd == ".sudo") && len(args) > 0 && strings.ToLower(args[0]) == "rm" {
		internalCmd = cmd + "rm"
		args = args[1:]
	}

	// Gate sudo-only commands up front so every handler below can assume
	// the caller is allowed to be there.
	for _, c := range commandList {
		if c.Base == internalCmd && c.SudoOnly && !isSudo {
			return b.reply(ctx, e, msg, "🚫 only sudo users can do that")
		}
	}

	atomic.AddInt64(&b.cmdCount, 1)

	switch internalCmd {
	case ".help":
		return b.cmdHelp(ctx, e, msg, isSudo)
	case ".ping":
		return b.cmdPing(ctx, e, msg)
	case ".id":
		return b.cmdID(ctx, e, msg)
	case ".info":
		return b.cmdInfo(ctx, e, msg)
	case ".stats":
		return b.cmdStats(ctx, e, msg)
	case ".ban":
		return b.cmdBan(ctx, e, msg, true)
	case ".unban":
		return b.cmdBan(ctx, e, msg, false)
	case ".mute":
		return b.cmdMute(ctx, e, msg, args, true)
	case ".unmute":
		return b.cmdMute(ctx, e, msg, args, false)
	case ".kick":
		return b.cmdKick(ctx, e, msg)
	case ".warn":
		return b.cmdWarn(ctx, e, msg)
	case ".warnings":
		return b.cmdWarnings(ctx, e, msg)
	case ".resetwarn":
		return b.cmdResetWarn(ctx, e, msg)
	case ".lock":
		return b.cmdLock(ctx, e, msg, true)
	case ".unlock":
		return b.cmdLock(ctx, e, msg, false)
	case ".pin":
		return b.cmdPin(ctx, e, msg, true)
	case ".unpin":
		return b.cmdPin(ctx, e, msg, false)
	case ".del":
		return b.cmdDel(ctx, e, msg)
	case ".purge":
		return b.cmdPurge(ctx, e, msg, args)
	case ".ai":
		return b.cmdAI(ctx, e, msg, strings.Join(args, " "), isSudo)
	case ".promote":
		return b.cmdPromote(ctx, e, msg, strings.Join(args, " "), true)
	case ".demote":
		return b.cmdPromote(ctx, e, msg, "", false)
	case ".approve":
		return b.cmdApprove(ctx, e, msg, true)
	case ".approverm":
		return b.cmdApprove(ctx, e, msg, false)
	case ".approved":
		return b.cmdListApproved(ctx, e, msg)
	case ".sudo":
		return b.cmdAddSudo(ctx, e, msg)
	case ".sudorm":
		return b.cmdRmSudo(ctx, e, msg)
	case ".sudolist":
		return b.cmdSudoList(ctx, e, msg)

	// ".sh" is intentionally NOT in commandList: it never appears in
	// .help, and non-sudo users get total silence instead of even a
	// "you can't use that" hint that it exists.
	case ".sh":
		if !isSudo {
			return nil
		}
		return b.cmdSchedule(ctx, e, msg, args)
	}
	return nil
}

// ---- helpers ----------------------------------------------------------

// senderID extracts the Telegram user ID of whoever sent msg. Works the
// same way regardless of chat type: in groups/channels FromID is always
// populated; in private chats, your own outgoing messages have FromID
// unset (Out=true implies "you"), everything else carries FromID directly.
// senderID extracts the Telegram user ID of whoever sent msg. Groups and
// channels always populate FromID explicitly, so that's checked first. In
// private chats (DMs), Telegram often leaves FromID unset entirely —
// the sender is implied by the chat itself instead: Out=true means you
// sent it, Out=false means it came from the other side of the DM (the
// chat's peer). Without this fallback, every incoming DM was silently
// dropped, which is why the bot worked in groups/channels but not DMs.
func (b *Bot) senderID(msg *tg.Message) (int64, bool) {
	if peerUser, ok := msg.FromID.(*tg.PeerUser); ok {
		return peerUser.UserID, true
	}
	if msg.Out {
		return b.self.ID, true
	}
	if pu, ok := msg.GetPeerID().(*tg.PeerUser); ok {
		return pu.UserID, true
	}
	return 0, false
}

// replyTarget resolves the user whose message is being replied to.
func (b *Bot) replyTarget(ctx context.Context, e tg.Entities, msg *tg.Message) (*tg.User, error) {
	if msg.ReplyTo == nil {
		return nil, fmt.Errorf("reply to the user's message first")
	}
	replyHeader, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok {
		return nil, fmt.Errorf("reply to the user's message first")
	}

	peer := msg.GetPeerID()
	history, err := b.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:     inputPeerFromPeer(e, peer),
		OffsetID: replyHeader.ReplyToMsgID + 1,
		Limit:    1,
	})
	if err != nil {
		return nil, fmt.Errorf("could not resolve replied-to user: %w", err)
	}

	var targetMsg *tg.Message
	switch h := history.(type) {
	case *tg.MessagesChannelMessages:
		if len(h.Messages) > 0 {
			targetMsg, _ = h.Messages[0].(*tg.Message)
		}
		for _, u := range h.Users {
			if user, ok := u.(*tg.User); ok && targetMsg != nil {
				if fu, ok2 := targetMsg.FromID.(*tg.PeerUser); ok2 && fu.UserID == user.ID {
					return user, nil
				}
			}
		}
	case *tg.MessagesMessages:
		if len(h.Messages) > 0 {
			targetMsg, _ = h.Messages[0].(*tg.Message)
		}
		for _, u := range h.Users {
			if user, ok := u.(*tg.User); ok && targetMsg != nil {
				if fu, ok2 := targetMsg.FromID.(*tg.PeerUser); ok2 && fu.UserID == user.ID {
					return user, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("could not resolve replied-to user")
}

func inputPeerFromPeer(e tg.Entities, peer tg.PeerClass) tg.InputPeerClass {
	switch p := peer.(type) {
	case *tg.PeerChannel:
		if ch, ok := e.Channels[p.ChannelID]; ok {
			return &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
		}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: p.ChatID}
	case *tg.PeerUser:
		if u, ok := e.Users[p.UserID]; ok {
			return &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}
		}
	}
	return &tg.InputPeerEmpty{}
}

// ---- font + formatting (applied to EVERY outgoing reply, centrally) -------

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
// Non-letters (numbers, punctuation, emoji, @mentions, backtick code) pass
// through untouched. Critically, this preserves whitespace EXACTLY as
// given — newlines, multiple spaces, tabs — instead of collapsing it all
// to single spaces, which is what made multi-line replies like .help turn
// into one giant wall of text before.
func StyleFont(s string) string {
	var out strings.Builder
	var word strings.Builder

	flushWord := func() {
		if word.Len() == 0 {
			return
		}
		runes := []rune(word.String())
		for j, r := range runes {
			lower := toLowerRune(r)
			if j == 0 && isLetter(r) {
				out.WriteRune(toUpperRune(r)) // first letter of the word: normal capital
				continue
			}
			if mapped, ok := smallCaps[lower]; ok {
				out.WriteRune(mapped)
			} else {
				out.WriteRune(r)
			}
		}
		word.Reset()
	}

	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			flushWord()
			out.WriteRune(r) // whitespace passes through untouched, verbatim
			continue
		}
		word.WriteRune(r)
	}
	flushWord()
	return out.String()
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

// buildCodeEntities strips `backtick` markers from styled text and turns
// them into real Telegram monospace entities, so IDs/numbers actually
// render as code instead of showing literal backticks. Offsets are
// computed in UTF-16 code units (what Telegram's entity API requires),
// correctly accounting for emoji that need surrogate pairs.
func buildCodeEntities(text string) (string, []tg.MessageEntityClass) {
	var out strings.Builder
	var entities []tg.MessageEntityClass
	utf16Offset := 0
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '`' {
			j := i + 1
			for j < len(runes) && runes[j] != '`' {
				j++
			}
			if j < len(runes) {
				code := string(runes[i+1 : j])
				out.WriteString(code)
				length := utf16RuneLen(code)
				entities = append(entities, &tg.MessageEntityCode{Offset: utf16Offset, Length: length})
				utf16Offset += length
				i = j
				continue
			}
		}
		out.WriteRune(runes[i])
		utf16Offset += utf16RuneLen(string(runes[i]))
	}
	return out.String(), entities
}

func utf16RuneLen(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// sendFormatted is the single choke point every reply goes through: it
// applies the small-caps font, converts `code` markers into real entities,
// and sends. Central application here is what guarantees every reply —
// .help, .ban, errors, everything — uses the font, without needing to
// remember to wrap every string manually.
func (b *Bot) sendFormatted(ctx context.Context, peer tg.InputPeerClass, text string, replyToID int) (tg.UpdatesClass, error) {
	styled := StyleFont(text)
	cleaned, entities := buildCodeEntities(styled)
	req := &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  cleaned,
		RandomID: randomID(),
	}
	if replyToID != 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyToID}
	}
	if len(entities) > 0 {
		req.Entities = entities
	}
	return b.api.MessagesSendMessage(ctx, req)
}

func (b *Bot) editFormatted(ctx context.Context, peer tg.InputPeerClass, msgID int, text string) error {
	styled := StyleFont(text)
	cleaned, entities := buildCodeEntities(styled)
	req := &tg.MessagesEditMessageRequest{
		Peer:    peer,
		ID:      msgID,
		Message: cleaned,
	}
	if len(entities) > 0 {
		req.Entities = entities
	}
	_, err := b.api.MessagesEditMessage(ctx, req)
	return err
}

func (b *Bot) reply(ctx context.Context, e tg.Entities, msg *tg.Message, text string) error {
	_, err := b.sendFormatted(ctx, inputPeerFromPeer(e, msg.GetPeerID()), text, msg.ID)
	return err
}

// extractSentMessageID pulls the new message ID out of whatever update
// shape Telegram happened to return — this varies (Updates, UpdateShort,
// UpdateShortSentMessage) depending on chat type, so .ping checks all of them.
func extractSentMessageID(u tg.UpdatesClass) int {
	switch v := u.(type) {
	case *tg.Updates:
		for _, upd := range v.Updates {
			if m, ok := upd.(*tg.UpdateMessageID); ok {
				return m.ID
			}
		}
	case *tg.UpdateShortSentMessage:
		return v.ID
	case *tg.UpdateShort:
		if m, ok := v.Update.(*tg.UpdateMessageID); ok {
			return m.ID
		}
	}
	return 0
}

// ---- info / utility commands -------------------------------------------

func (b *Bot) cmdHelp(ctx context.Context, e tg.Entities, msg *tg.Message, isSudo bool) error {
	var sb strings.Builder
	sb.WriteString("🤖 here's what i can do\n\n")
	lastSudo := false
	for _, c := range commandList {
		if c.SudoOnly && !isSudo {
			continue
		}
		if c.SudoOnly && !lastSudo {
			sb.WriteString("\n👑 sudo only\n")
			lastSudo = true
		}
		fmt.Fprintf(&sb, "%s — %s\n", c.Cmd, c.Desc)
	}
	return b.reply(ctx, e, msg, sb.String())
}

func (b *Bot) cmdPing(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	peer := inputPeerFromPeer(e, msg.GetPeerID())
	start := time.Now()
	sent, err := b.sendFormatted(ctx, peer, "🏓 pinging", msg.ID)
	if err != nil {
		return err
	}
	elapsed := time.Since(start)

	sentID := extractSentMessageID(sent)
	if sentID == 0 {
		return nil
	}
	return b.editFormatted(ctx, peer, sentID, fmt.Sprintf("🏓 pong: %dms", elapsed.Milliseconds()))
}

func (b *Bot) cmdID(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		chatID := peerChatID(msg.GetPeerID())
		return b.reply(ctx, e, msg, fmt.Sprintf("🆔 chat id: `%d`", chatID))
	}
	return b.reply(ctx, e, msg, fmt.Sprintf("🆔 user id: `%d`", target.ID))
}

func (b *Bot) cmdInfo(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	var sb strings.Builder
	sb.WriteString("ℹ️ user info\n")
	fmt.Fprintf(&sb, "👤 %s\n", displayName(target))
	fmt.Fprintf(&sb, "🆔 `%d`\n", target.ID)
	if target.Username != "" {
		fmt.Fprintf(&sb, "🔗 @%s\n", target.Username)
	}
	if target.Bot {
		sb.WriteString("🤖 bot account\n")
	}
	if target.Premium {
		sb.WriteString("⭐ premium user\n")
	}
	if target.Scam {
		sb.WriteString("🚩 flagged as scam\n")
	}
	if target.Fake {
		sb.WriteString("🚩 flagged as fake\n")
	}
	warns := b.cfg.WarnCount(target.ID)
	fmt.Fprintf(&sb, "⚠️ warnings: %d\n", warns)
	return b.reply(ctx, e, msg, sb.String())
}

func (b *Bot) cmdStats(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	uptime := time.Since(b.startedAt).Round(time.Second)
	count := atomic.LoadInt64(&b.cmdCount)
	text := fmt.Sprintf("📊 bot stats\n⏱️ uptime: %s\n⚡ commands run: %d", uptime, count)
	return b.reply(ctx, e, msg, text)
}

// ---- moderation commands (supergroups only — see channelFromPeer) ---------

func (b *Bot) cmdBan(ctx context.Context, e tg.Entities, msg *tg.Message, ban bool) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	channel, ok := channelFromPeer(e, msg.GetPeerID())
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ this only works in supergroups")
	}

	rights := tg.ChatBannedRights{}
	if ban {
		rights.ViewMessages = true
		rights.SendMessages = true
		rights.SendMedia = true
	}
	// ban=false with all rights left unset lifts the restriction.

	_, err = b.api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel:      channel,
		Participant:  &tg.InputPeerUser{UserID: target.ID, AccessHash: target.AccessHash},
		BannedRights: rights,
	})
	if err != nil {
		return b.reply(ctx, e, msg, "❌ failed: "+err.Error())
	}
	if ban {
		return b.reply(ctx, e, msg, "🔨 banned "+displayName(target))
	}
	return b.reply(ctx, e, msg, "✅ unbanned "+displayName(target))
}

// cmdKick bans then immediately un-bans, removing the member without a
// permanent ban staying in place.
func (b *Bot) cmdKick(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	channel, ok := channelFromPeer(e, msg.GetPeerID())
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ this only works in supergroups")
	}
	participant := &tg.InputPeerUser{UserID: target.ID, AccessHash: target.AccessHash}

	if _, err := b.api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel: channel, Participant: participant,
		BannedRights: tg.ChatBannedRights{ViewMessages: true},
	}); err != nil {
		return b.reply(ctx, e, msg, "❌ failed: "+err.Error())
	}
	time.Sleep(300 * time.Millisecond)
	_, _ = b.api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel: channel, Participant: participant,
		BannedRights: tg.ChatBannedRights{},
	})
	return b.reply(ctx, e, msg, "👢 kicked "+displayName(target))
}

func (b *Bot) cmdMute(ctx context.Context, e tg.Entities, msg *tg.Message, args []string, mute bool) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	channel, ok := channelFromPeer(e, msg.GetPeerID())
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ this only works in supergroups")
	}

	rights := tg.ChatBannedRights{}
	if mute {
		rights.SendMessages = true
		if len(args) > 0 {
			if mins, err := parseMinutes(args[0]); err == nil {
				rights.UntilDate = int(time.Now().Add(mins).Unix())
			}
		}
	}

	_, err = b.api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel:      channel,
		Participant:  &tg.InputPeerUser{UserID: target.ID, AccessHash: target.AccessHash},
		BannedRights: rights,
	})
	if err != nil {
		return b.reply(ctx, e, msg, "❌ failed: "+err.Error())
	}
	if mute {
		return b.reply(ctx, e, msg, "🔇 muted "+displayName(target))
	}
	return b.reply(ctx, e, msg, "🔊 unmuted "+displayName(target))
}

func (b *Bot) cmdWarn(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	n := b.cfg.Warn(target.ID)
	text := fmt.Sprintf("⚠️ warned %s (%d/3)", displayName(target), n)
	if n >= 3 {
		text += "\n🔨 3 warnings reached — consider a ban"
	}
	return b.reply(ctx, e, msg, text)
}

func (b *Bot) cmdWarnings(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	n := b.cfg.WarnCount(target.ID)
	return b.reply(ctx, e, msg, fmt.Sprintf("📋 %s: %d/3", displayName(target), n))
}

func (b *Bot) cmdResetWarn(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	b.cfg.ResetWarn(target.ID)
	return b.reply(ctx, e, msg, "🧹 cleared warnings for "+displayName(target))
}

// cmdLock toggles whether non-admins can send messages in the chat at all.
func (b *Bot) cmdLock(ctx context.Context, e tg.Entities, msg *tg.Message, lock bool) error {
	channel, ok := channelFromPeer(e, msg.GetPeerID())
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ this only works in supergroups")
	}
	_, err := b.api.MessagesEditChatDefaultBannedRights(ctx, &tg.MessagesEditChatDefaultBannedRightsRequest{
		Peer:         &tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash},
		BannedRights: tg.ChatBannedRights{SendMessages: lock},
	})
	if err != nil {
		return b.reply(ctx, e, msg, "❌ failed: "+err.Error())
	}
	if lock {
		return b.reply(ctx, e, msg, "🔒 chat locked — only admins can message")
	}
	return b.reply(ctx, e, msg, "🔓 chat unlocked — everyone can message")
}

// ---- message commands (any chat type) --------------------------------------

func (b *Bot) cmdPin(ctx context.Context, e tg.Entities, msg *tg.Message, pin bool) error {
	if msg.ReplyTo == nil {
		return b.reply(ctx, e, msg, "⚠️ reply to the message first")
	}
	replyHeader, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ reply to the message first")
	}
	_, err := b.api.MessagesUpdatePinnedMessage(ctx, &tg.MessagesUpdatePinnedMessageRequest{
		Peer:   inputPeerFromPeer(e, msg.GetPeerID()),
		ID:     replyHeader.ReplyToMsgID,
		Unpin:  !pin,
		Silent: true,
	})
	if err != nil {
		return b.reply(ctx, e, msg, "❌ failed: "+err.Error())
	}
	if pin {
		return b.reply(ctx, e, msg, "📌 pinned")
	}
	return b.reply(ctx, e, msg, "📍 unpinned")
}

func (b *Bot) cmdDel(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	if msg.ReplyTo == nil {
		return b.reply(ctx, e, msg, "⚠️ reply to the message first")
	}
	replyHeader, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ reply to the message first")
	}
	if err := b.deleteMessages(ctx, e, msg, []int{replyHeader.ReplyToMsgID, msg.ID}); err != nil {
		return b.reply(ctx, e, msg, "❌ failed: "+err.Error())
	}
	return nil // the command message itself is deleted too — nothing left to reply to
}

// cmdPurge deletes the last n messages counting back from this command.
func (b *Bot) cmdPurge(ctx context.Context, e tg.Entities, msg *tg.Message, args []string) error {
	n := 10
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	if n > 100 {
		n = 100 // safety cap
	}
	ids := make([]int, 0, n+1)
	for i := 0; i <= n; i++ {
		ids = append(ids, msg.ID-i)
	}
	if err := b.deleteMessages(ctx, e, msg, ids); err != nil {
		return b.reply(ctx, e, msg, "❌ failed: "+err.Error())
	}
	return nil
}

// deleteMessages picks the right API depending on chat type: channels
// (supergroups/broadcast channels) use one endpoint, everything else
// (private chats, basic groups) uses another.
func (b *Bot) deleteMessages(ctx context.Context, e tg.Entities, msg *tg.Message, ids []int) error {
	if channel, ok := channelFromPeer(e, msg.GetPeerID()); ok {
		_, err := b.api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: channel,
			ID:      ids,
		})
		return err
	}
	_, err := b.api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
		ID:     ids,
		Revoke: true,
	})
	return err
}

// ---- admin commands (sudo only) --------------------------------------------

func (b *Bot) cmdPromote(ctx context.Context, e tg.Entities, msg *tg.Message, title string, promote bool) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	channel, ok := channelFromPeer(e, msg.GetPeerID())
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ this only works in supergroups")
	}

	rights := tg.ChatAdminRights{}
	if promote {
		rights = tg.ChatAdminRights{
			ChangeInfo: true, DeleteMessages: true, BanUsers: true,
			InviteUsers: true, PinMessages: true, ManageCall: true,
		}
	}
	if title == "" {
		title = "admin"
	}

	_, err = b.api.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
		Channel:     channel,
		UserID:      &tg.InputUser{UserID: target.ID, AccessHash: target.AccessHash},
		AdminRights: rights,
		Rank:        title,
	})
	if err != nil {
		return b.reply(ctx, e, msg, "❌ failed: "+err.Error())
	}
	if promote {
		return b.reply(ctx, e, msg, "⭐ promoted "+displayName(target))
	}
	return b.reply(ctx, e, msg, "⬇️ demoted "+displayName(target))
}

func (b *Bot) cmdApprove(ctx context.Context, e tg.Entities, msg *tg.Message, approve bool) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	if approve {
		if b.cfg.Approve(target.ID) {
			return b.reply(ctx, e, msg, "🟢 approved "+displayName(target))
		}
		return b.reply(ctx, e, msg, "🟡 already approved")
	}
	if b.cfg.Unapprove(target.ID) {
		return b.reply(ctx, e, msg, "🔴 removed "+displayName(target))
	}
	return b.reply(ctx, e, msg, "🟡 that user wasn't approved")
}

func (b *Bot) cmdAddSudo(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	if b.cfg.AddSudo(target.ID) {
		return b.reply(ctx, e, msg, "👑 granted sudo to "+displayName(target))
	}
	return b.reply(ctx, e, msg, "🟡 already sudo")
}

func (b *Bot) cmdRmSudo(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	if b.cfg.RmSudo(target.ID) {
		return b.reply(ctx, e, msg, "⛔ revoked sudo from "+displayName(target))
	}
	return b.reply(ctx, e, msg, "🟡 can't remove that (owner or not sudo)")
}

func (b *Bot) cmdListApproved(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	if len(b.cfg.ApprovedIDs) == 0 {
		return b.reply(ctx, e, msg, "🟡 nobody's approved yet")
	}
	var sb strings.Builder
	sb.WriteString("📋 approved users\n")
	for _, id := range b.cfg.ApprovedIDs {
		fmt.Fprintf(&sb, "• `%d`\n", id)
	}
	return b.reply(ctx, e, msg, sb.String())
}

func (b *Bot) cmdSudoList(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	var sb strings.Builder
	sb.WriteString("👑 sudo users\n")
	fmt.Fprintf(&sb, "• `%d` (owner)\n", b.cfg.OwnerID)
	for _, id := range b.cfg.SudoIDs {
		fmt.Fprintf(&sb, "• `%d`\n", id)
	}
	return b.reply(ctx, e, msg, sb.String())
}

// ---- ai command -------------------------------------------------------

// cmdAI handles ".ai <question>". Everyone approved+ gets a witty text
// reply. Actually *executing* an action (ban/mute/kick) via natural
// language is sudo-only, and only when the sudo user is replying to a
// target message — an approved (non-sudo) user asking ".ai ban this guy"
// only ever gets the text reply, nothing is executed.
func (b *Bot) cmdAI(ctx context.Context, e tg.Entities, msg *tg.Message, query string, isSudo bool) error {
	if strings.TrimSpace(query) == "" {
		return b.reply(ctx, e, msg, "⚠️ ask me something")
	}

	reply, err := AskAI(ctx, query)
	if err != nil {
		return b.reply(ctx, e, msg, "❌ ai's down: "+err.Error())
	}
	if err := b.reply(ctx, e, msg, reply); err != nil {
		return err
	}

	if isSudo && msg.ReplyTo != nil && looksLikeActionRequest(query) {
		lower := strings.ToLower(query)
		switch {
		case strings.Contains(lower, "unban"):
			return b.cmdBan(ctx, e, msg, false)
		case strings.Contains(lower, "ban"):
			return b.cmdBan(ctx, e, msg, true)
		case strings.Contains(lower, "unmute"):
			return b.cmdMute(ctx, e, msg, nil, false)
		case strings.Contains(lower, "mute"):
			return b.cmdMute(ctx, e, msg, nil, true)
		case strings.Contains(lower, "kick"):
			return b.cmdKick(ctx, e, msg)
		}
	}
	return nil
}

func looksLikeActionRequest(query string) bool {
	lower := strings.ToLower(query)
	for _, kw := range []string{"ban", "mute", "kick"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// ---- small utilities -------------------------------------------------------

// channelFromPeer succeeds only for supergroups/broadcast channels — the
// only chat types where Telegram's admin API (ban/mute/promote/lock)
// applies. Private chats and legacy basic groups correctly fall through to
// the "only works in supergroups" message from each handler.
func channelFromPeer(e tg.Entities, peer tg.PeerClass) (*tg.InputChannel, bool) {
	pc, ok := peer.(*tg.PeerChannel)
	if !ok {
		return nil, false
	}
	ch, ok := e.Channels[pc.ChannelID]
	if !ok {
		return nil, false
	}
	return &tg.InputChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}, true
}

func peerChatID(peer tg.PeerClass) int64 {
	switch p := peer.(type) {
	case *tg.PeerChannel:
		return p.ChannelID
	case *tg.PeerChat:
		return p.ChatID
	case *tg.PeerUser:
		return p.UserID
	}
	return 0
}

func displayName(u *tg.User) string {
	name := u.FirstName
	if u.LastName != "" {
		name += " " + u.LastName
	}
	if name == "" {
		name = "user"
	}
	if u.Username != "" {
		return fmt.Sprintf("%s (@%s)", name, u.Username)
	}
	return name
}

func parseMinutes(s string) (time.Duration, error) {
	s = strings.TrimSuffix(s, "m")
	mins, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return time.Duration(mins) * time.Minute, nil
}

var randomCounter int64 = time.Now().UnixNano()

func randomID() int64 {
	return atomic.AddInt64(&randomCounter, 1)
}
