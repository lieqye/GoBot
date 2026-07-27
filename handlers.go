package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

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

	// ---- moderation ----
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

// HandleMessage is invoked for every new message the account can see.
// It only reacts to messages that start with "." (the command prefix).
// Every reply this and its helpers send goes out with an emoji up front.
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
			return b.reply(ctx, e, msg, "🚫 "+StyleFont("only sudo users can do that"))
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
	}
	return nil
}

// ---- helpers ----------------------------------------------------------

// senderID extracts the Telegram user ID of whoever sent msg.
func (b *Bot) senderID(msg *tg.Message) (int64, bool) {
	if msg.Out && msg.FromID == nil {
		return b.self.ID, true
	}
	peerUser, ok := msg.FromID.(*tg.PeerUser)
	if !ok {
		return 0, false
	}
	return peerUser.UserID, true
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

func (b *Bot) reply(ctx context.Context, e tg.Entities, msg *tg.Message, text string) error {
	_, err := b.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     inputPeerFromPeer(e, msg.GetPeerID()),
		Message:  text,
		ReplyTo:  &tg.InputReplyToMessage{ReplyToMsgID: msg.ID},
		RandomID: randomID(),
	})
	return err
}

// ---- info / utility commands -------------------------------------------

func (b *Bot) cmdHelp(ctx context.Context, e tg.Entities, msg *tg.Message, isSudo bool) error {
	var sb strings.Builder
	sb.WriteString("🤖 " + StyleFont("here's what i can do") + "\n\n")
	lastSudo := false
	for _, c := range commandList {
		if c.SudoOnly && !isSudo {
			continue
		}
		if c.SudoOnly && !lastSudo {
			sb.WriteString("\n👑 " + StyleFont("sudo only") + "\n")
			lastSudo = true
		}
		fmt.Fprintf(&sb, "%s — %s\n", c.Cmd, c.Desc)
	}
	return b.reply(ctx, e, msg, sb.String())
}

func (b *Bot) cmdPing(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	start := time.Now()
	sent, err := b.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     inputPeerFromPeer(e, msg.GetPeerID()),
		Message:  "🏓 " + StyleFont("pinging"),
		ReplyTo:  &tg.InputReplyToMessage{ReplyToMsgID: msg.ID},
		RandomID: randomID(),
	})
	if err != nil {
		return err
	}
	elapsed := time.Since(start)

	// Find the message ID we just sent so we can edit it in place.
	var sentID int
	if u, ok := sent.(*tg.Updates); ok {
		for _, upd := range u.Updates {
			if m, ok := upd.(*tg.UpdateMessageID); ok {
				sentID = m.ID
			}
		}
	}
	if sentID == 0 {
		return nil
	}
	_, err = b.api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:    inputPeerFromPeer(e, msg.GetPeerID()),
		ID:      sentID,
		Message: fmt.Sprintf("🏓 %s: %dms", StyleFont("pong"), elapsed.Milliseconds()),
	})
	return err
}

func (b *Bot) cmdID(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		chatID := peerChatID(msg.GetPeerID())
		return b.reply(ctx, e, msg, fmt.Sprintf("🆔 %s: `%d`", StyleFont("chat id"), chatID))
	}
	return b.reply(ctx, e, msg, fmt.Sprintf("🆔 %s: `%d`", StyleFont("user id"), target.ID))
}

func (b *Bot) cmdInfo(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	var sb strings.Builder
	sb.WriteString("ℹ️ " + StyleFont("user info") + "\n")
	fmt.Fprintf(&sb, "👤 %s\n", displayName(target))
	fmt.Fprintf(&sb, "🆔 %d\n", target.ID)
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
	text := fmt.Sprintf("📊 %s\n⏱️ %s: %s\n⚡ %s: %d",
		StyleFont("bot stats"), StyleFont("uptime"), uptime, StyleFont("commands run"), count)
	return b.reply(ctx, e, msg, text)
}

// ---- moderation commands -------------------------------------------------

func (b *Bot) cmdBan(ctx context.Context, e tg.Entities, msg *tg.Message, ban bool) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	channel, ok := channelFromPeer(e, msg.GetPeerID())
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ "+StyleFont("this only works in supergroups"))
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
		return b.reply(ctx, e, msg, "❌ "+StyleFont("failed")+": "+err.Error())
	}
	if ban {
		return b.reply(ctx, e, msg, "🔨 "+StyleFont("banned")+" "+displayName(target))
	}
	return b.reply(ctx, e, msg, "✅ "+StyleFont("unbanned")+" "+displayName(target))
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
		return b.reply(ctx, e, msg, "⚠️ "+StyleFont("this only works in supergroups"))
	}
	participant := &tg.InputPeerUser{UserID: target.ID, AccessHash: target.AccessHash}

	if _, err := b.api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel: channel, Participant: participant,
		BannedRights: tg.ChatBannedRights{ViewMessages: true},
	}); err != nil {
		return b.reply(ctx, e, msg, "❌ "+StyleFont("failed")+": "+err.Error())
	}
	time.Sleep(300 * time.Millisecond)
	_, _ = b.api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel: channel, Participant: participant,
		BannedRights: tg.ChatBannedRights{},
	})
	return b.reply(ctx, e, msg, "👢 "+StyleFont("kicked")+" "+displayName(target))
}

func (b *Bot) cmdMute(ctx context.Context, e tg.Entities, msg *tg.Message, args []string, mute bool) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	channel, ok := channelFromPeer(e, msg.GetPeerID())
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ "+StyleFont("this only works in supergroups"))
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
		return b.reply(ctx, e, msg, "❌ "+StyleFont("failed")+": "+err.Error())
	}
	if mute {
		return b.reply(ctx, e, msg, "🔇 "+StyleFont("muted")+" "+displayName(target))
	}
	return b.reply(ctx, e, msg, "🔊 "+StyleFont("unmuted")+" "+displayName(target))
}

func (b *Bot) cmdWarn(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	n := b.cfg.Warn(target.ID)
	text := fmt.Sprintf("⚠️ %s %s (%d/3)", StyleFont("warned"), displayName(target), n)
	if n >= 3 {
		text += "\n🔨 " + StyleFont("3 warnings reached — consider a ban")
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
	return b.reply(ctx, e, msg, "🧹 "+StyleFont("cleared warnings for")+" "+displayName(target))
}

// cmdLock toggles whether non-admins can send messages in the chat at all.
func (b *Bot) cmdLock(ctx context.Context, e tg.Entities, msg *tg.Message, lock bool) error {
	channel, ok := channelFromPeer(e, msg.GetPeerID())
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ "+StyleFont("this only works in supergroups"))
	}
	_, err := b.api.MessagesEditChatDefaultBannedRights(ctx, &tg.MessagesEditChatDefaultBannedRightsRequest{
		Peer:         &tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash},
		BannedRights: tg.ChatBannedRights{SendMessages: lock},
	})
	if err != nil {
		return b.reply(ctx, e, msg, "❌ "+StyleFont("failed")+": "+err.Error())
	}
	if lock {
		return b.reply(ctx, e, msg, "🔒 "+StyleFont("chat locked — only admins can message"))
	}
	return b.reply(ctx, e, msg, "🔓 "+StyleFont("chat unlocked — everyone can message"))
}

// ---- message commands -----------------------------------------------------

func (b *Bot) cmdPin(ctx context.Context, e tg.Entities, msg *tg.Message, pin bool) error {
	if msg.ReplyTo == nil {
		return b.reply(ctx, e, msg, "⚠️ "+StyleFont("reply to the message first"))
	}
	replyHeader, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ "+StyleFont("reply to the message first"))
	}
	_, err := b.api.MessagesUpdatePinnedMessage(ctx, &tg.MessagesUpdatePinnedMessageRequest{
		Peer:   inputPeerFromPeer(e, msg.GetPeerID()),
		ID:     replyHeader.ReplyToMsgID,
		Unpin:  !pin,
		Silent: true,
	})
	if err != nil {
		return b.reply(ctx, e, msg, "❌ "+StyleFont("failed")+": "+err.Error())
	}
	if pin {
		return b.reply(ctx, e, msg, "📌 "+StyleFont("pinned"))
	}
	return b.reply(ctx, e, msg, "📍 "+StyleFont("unpinned"))
}

func (b *Bot) cmdDel(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	if msg.ReplyTo == nil {
		return b.reply(ctx, e, msg, "⚠️ "+StyleFont("reply to the message first"))
	}
	replyHeader, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok {
		return b.reply(ctx, e, msg, "⚠️ "+StyleFont("reply to the message first"))
	}
	if err := b.deleteMessages(ctx, e, msg, []int{replyHeader.ReplyToMsgID, msg.ID}); err != nil {
		return b.reply(ctx, e, msg, "❌ "+StyleFont("failed")+": "+err.Error())
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
		return b.reply(ctx, e, msg, "❌ "+StyleFont("failed")+": "+err.Error())
	}
	return nil
}

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
		return b.reply(ctx, e, msg, "⚠️ "+StyleFont("this only works in supergroups"))
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
		return b.reply(ctx, e, msg, "❌ "+StyleFont("failed")+": "+err.Error())
	}
	if promote {
		return b.reply(ctx, e, msg, "⭐ "+StyleFont("promoted")+" "+displayName(target))
	}
	return b.reply(ctx, e, msg, "⬇️ "+StyleFont("demoted")+" "+displayName(target))
}

func (b *Bot) cmdApprove(ctx context.Context, e tg.Entities, msg *tg.Message, approve bool) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	if approve {
		if b.cfg.Approve(target.ID) {
			return b.reply(ctx, e, msg, "🟢 "+StyleFont("approved")+" "+displayName(target))
		}
		return b.reply(ctx, e, msg, "🟡 "+StyleFont("already approved"))
	}
	if b.cfg.Unapprove(target.ID) {
		return b.reply(ctx, e, msg, "🔴 "+StyleFont("removed")+" "+displayName(target))
	}
	return b.reply(ctx, e, msg, "🟡 "+StyleFont("that user wasn't approved"))
}

func (b *Bot) cmdAddSudo(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	if b.cfg.AddSudo(target.ID) {
		return b.reply(ctx, e, msg, "👑 "+StyleFont("granted sudo to")+" "+displayName(target))
	}
	return b.reply(ctx, e, msg, "🟡 "+StyleFont("already sudo"))
}

func (b *Bot) cmdRmSudo(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	target, err := b.replyTarget(ctx, e, msg)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ "+err.Error())
	}
	if b.cfg.RmSudo(target.ID) {
		return b.reply(ctx, e, msg, "⛔ "+StyleFont("revoked sudo from")+" "+displayName(target))
	}
	return b.reply(ctx, e, msg, "🟡 "+StyleFont("can't remove that (owner or not sudo)"))
}

func (b *Bot) cmdListApproved(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	if len(b.cfg.ApprovedIDs) == 0 {
		return b.reply(ctx, e, msg, "🟡 "+StyleFont("nobody's approved yet"))
	}
	var sb strings.Builder
	sb.WriteString("📋 " + StyleFont("approved users") + "\n")
	for _, id := range b.cfg.ApprovedIDs {
		fmt.Fprintf(&sb, "• `%d`\n", id)
	}
	return b.reply(ctx, e, msg, sb.String())
}

func (b *Bot) cmdSudoList(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	var sb strings.Builder
	sb.WriteString("👑 " + StyleFont("sudo users") + "\n")
	fmt.Fprintf(&sb, "• `%d` (owner)\n", b.cfg.OwnerID)
	for _, id := range b.cfg.SudoIDs {
		fmt.Fprintf(&sb, "• `%d`\n", id)
	}
	return b.reply(ctx, e, msg, sb.String())
}

// ---- ai command -------------------------------------------------------

// cmdAI handles ".ai <question>". Everyone approved+ gets a witty text
// reply. Actually *executing* an action (ban/mute/kick) via natural
// language is sudo-only — an approved (non-sudo) user asking ".ai ban this
// guy" only gets a reply, nothing happens.
func (b *Bot) cmdAI(ctx context.Context, e tg.Entities, msg *tg.Message, query string, isSudo bool) error {
	if strings.TrimSpace(query) == "" {
		return b.reply(ctx, e, msg, "⚠️ "+StyleFont("ask me something"))
	}

	reply, err := AskAI(ctx, query)
	if err != nil {
		return b.reply(ctx, e, msg, "❌ "+StyleFont("ai's down")+": "+err.Error())
	}
	if err := b.reply(ctx, e, msg, reply); err != nil {
		return err
	}

	if isSudo && looksLikeActionRequest(query) {
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
