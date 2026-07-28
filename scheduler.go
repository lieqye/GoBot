package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/tg"
)

// istLocation is India Standard Time — a fixed UTC+5:30 offset with no DST,
// so a simple FixedZone is correct year-round.
var istLocation = time.FixedZone("IST", 5*3600+30*60)

const scheduleHelpText = `🕐 .sh — high-precision scheduler (sudo only)

Schedule a dm
.sh {username} {time ist} {message}
example: .sh @john 7:00:00 hello!

Schedule a channel post comment
.sh {post link} {time ist} {message}
example: .sh https://t.me/channelname/123 7:00:00 nice update!

Time format
HH:MM[:SS] [AM|PM] — AM/PM is optional. Without it, the nearest
upcoming occurrence is picked automatically. Example: at 9 PM,
"7:00:00" resolves to 7 AM tomorrow, not 7 PM today (already passed).

Other subcommands
.sh list — show pending scheduled jobs
.sh cancel <id> — cancel a pending job before it fires`

// ---- data model + persistence ---------------------------------------------

// ScheduledJob is one pending .sh action. Exactly one of TargetUsername or
// PostChannel+PostMsgID is set, depending on whether it's a DM or a
// channel-post comment.
type ScheduledJob struct {
	ID        int64     `json:"id"`
	CreatedBy int64     `json:"created_by"`
	RunAt     time.Time `json:"run_at"` // stored/compared in UTC
	Message   string    `json:"message"`

	TargetUsername string `json:"target_username,omitempty"`
	PostChannel    string `json:"post_channel,omitempty"` // public username, or "c/<internal_id>" for private channels
	PostMsgID      int    `json:"post_msg_id,omitempty"`
}

// Scheduler holds every pending job and its live timer, and persists to
// disk so a restart doesn't lose anything queued up.
type Scheduler struct {
	mu     sync.Mutex
	path   string
	bot    *Bot
	nextID int64
	jobs   map[int64]*ScheduledJob
	timers map[int64]*time.Timer
}

const schedulePath = "schedules.json"

// NewScheduler loads any previously-saved jobs and re-arms their timers.
// Anything that was already overdue when the bot was offline fires
// immediately rather than being silently dropped.
func NewScheduler(bot *Bot) (*Scheduler, error) {
	s := &Scheduler{
		path:   schedulePath,
		bot:    bot,
		jobs:   map[int64]*ScheduledJob{},
		timers: map[int64]*time.Timer{},
	}

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}

	var list []*ScheduledJob
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	for _, j := range list {
		s.jobs[j.ID] = j
		if j.ID >= s.nextID {
			s.nextID = j.ID + 1
		}
		s.arm(j)
	}
	return s, nil
}

// arm schedules (or re-schedules) the timer for a job. time.AfterFunc
// gives sub-millisecond firing precision from Go's runtime timer wheel —
// the dominant source of any real-world delay past that point is the
// network round-trip to Telegram's servers when the send actually happens,
// which no client-side scheduler can eliminate.
func (s *Scheduler) arm(j *ScheduledJob) {
	delay := time.Until(j.RunAt)
	if delay < 0 {
		delay = 0
	}
	s.timers[j.ID] = time.AfterFunc(delay, func() {
		s.execute(j)
	})
}

// Add saves a new job to disk and arms its timer, returning its ID.
func (s *Scheduler) Add(job ScheduledJob) (int64, error) {
	s.mu.Lock()
	job.ID = s.nextID
	s.nextID++
	s.jobs[job.ID] = &job
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return 0, err
	}
	s.arm(&job)
	return job.ID, nil
}

// Cancel stops and removes a pending job. Returns false if no such job exists.
func (s *Scheduler) Cancel(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.timers[id]; ok {
		t.Stop()
		delete(s.timers, id)
	}
	if _, ok := s.jobs[id]; ok {
		delete(s.jobs, id)
		_ = s.saveLocked()
		return true
	}
	return false
}

// List returns all pending jobs, soonest first.
func (s *Scheduler) List() []*ScheduledJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*ScheduledJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].RunAt.Before(out[k].RunAt) })
	return out
}

func (s *Scheduler) saveLocked() error {
	list := make([]*ScheduledJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		list = append(list, j)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// execute fires exactly once when a job's timer elapses: sends the DM or
// comment, then removes the job from the pending list either way (a
// failed send isn't retried — check the console log).
func (s *Scheduler) execute(j *ScheduledJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var err error
	if j.PostChannel != "" {
		err = s.bot.sendScheduledComment(ctx, j.PostChannel, j.PostMsgID, j.Message)
	} else {
		err = s.bot.sendScheduledDM(ctx, j.TargetUsername, j.Message)
	}
	if err != nil {
		fmt.Printf("scheduled job #%d failed: %v\n", j.ID, err)
	}

	s.mu.Lock()
	delete(s.jobs, j.ID)
	delete(s.timers, j.ID)
	_ = s.saveLocked()
	s.mu.Unlock()
}

// ---- time parsing -----------------------------------------------------

var scheduleTimeRe = regexp.MustCompile(`(?i)^(\d{1,2}):(\d{2})(?::(\d{2}))?\s*(AM|PM)?$`)

// parseScheduleTime resolves a "HH:MM[:SS] [AM|PM]" string (in IST) to a
// concrete future instant relative to now. When AM/PM is omitted, it picks
// whichever of the AM/PM (today/tomorrow) candidates is soonest but still
// in the future — matching "nearest upcoming occurrence".
func parseScheduleTime(input string, now time.Time) (time.Time, error) {
	m := scheduleTimeRe.FindStringSubmatch(strings.TrimSpace(input))
	if m == nil {
		return time.Time{}, fmt.Errorf("expected HH:MM[:SS] [AM|PM], got %q", input)
	}
	hour, _ := strconv.Atoi(m[1])
	minute, _ := strconv.Atoi(m[2])
	second := 0
	if m[3] != "" {
		second, _ = strconv.Atoi(m[3])
	}
	ampm := strings.ToUpper(m[4])

	if hour > 23 || minute > 59 || second > 59 {
		return time.Time{}, fmt.Errorf("time out of range")
	}

	loc := now.Location()
	mk := func(h, dayOffset int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), h, minute, second, 0, loc).AddDate(0, 0, dayOffset)
	}

	if ampm == "AM" || ampm == "PM" {
		h := hour % 12
		if ampm == "PM" {
			h += 12
		}
		candidate := mk(h, 0)
		if !candidate.After(now) {
			candidate = mk(h, 1)
		}
		return candidate, nil
	}

	if hour > 12 {
		// unambiguous 24-hour value, e.g. "19:00" — no AM/PM guessing needed.
		candidate := mk(hour, 0)
		if !candidate.After(now) {
			candidate = mk(hour, 1)
		}
		return candidate, nil
	}

	// Ambiguous 12-hour value with no AM/PM: pick whichever of the four
	// candidates (AM today, AM tomorrow, PM today, PM tomorrow) is the
	// soonest one still in the future.
	hAM := hour % 12
	hPM := hAM + 12
	var best time.Time
	for _, c := range []time.Time{mk(hAM, 0), mk(hAM, 1), mk(hPM, 0), mk(hPM, 1)} {
		if c.After(now) && (best.IsZero() || c.Before(best)) {
			best = c
		}
	}
	return best, nil
}

// ---- post link parsing -------------------------------------------------

// t.me/channelname/123 (public) or t.me/c/1234567890/123 (private, by
// internal numeric channel id).
var postLinkRe = regexp.MustCompile(`t\.me/(?:c/(\d+)|([A-Za-z0-9_]{4,}))/(\d+)`)

func parsePostLink(link string) (channelRef string, msgID int, ok bool) {
	m := postLinkRe.FindStringSubmatch(link)
	if m == nil {
		return "", 0, false
	}
	msgID, _ = strconv.Atoi(m[3])
	if m[1] != "" {
		return "c/" + m[1], msgID, true
	}
	return m[2], msgID, true
}

// ---- command handlers ---------------------------------------------------

func (b *Bot) cmdSchedule(ctx context.Context, e tg.Entities, msg *tg.Message, args []string) error {
	if len(args) == 0 {
		return b.reply(ctx, e, msg, scheduleHelpText)
	}
	switch strings.ToLower(args[0]) {
	case "help":
		return b.reply(ctx, e, msg, scheduleHelpText)
	case "list":
		return b.cmdScheduleList(ctx, e, msg)
	case "cancel":
		if len(args) < 2 {
			return b.reply(ctx, e, msg, "⚠️ usage: .sh cancel <id>")
		}
		return b.cmdScheduleCancel(ctx, e, msg, args[1])
	}

	if len(args) < 3 {
		return b.reply(ctx, e, msg, "⚠️ usage: .sh {target} {time} {message} — see .sh help")
	}
	target := args[0]
	timeStr := args[1]
	message := strings.Join(args[2:], " ")

	runAt, err := parseScheduleTime(timeStr, time.Now().In(istLocation))
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ couldn't parse that time: "+err.Error())
	}

	senderID, _ := b.senderID(msg)
	job := ScheduledJob{
		CreatedBy: senderID,
		RunAt:     runAt.UTC(),
		Message:   message,
	}
	var targetDesc string
	if channelRef, postID, ok := parsePostLink(target); ok {
		job.PostChannel = channelRef
		job.PostMsgID = postID
		targetDesc = fmt.Sprintf("post %d in %s", postID, channelRef)
	} else {
		job.TargetUsername = strings.TrimPrefix(target, "@")
		targetDesc = "@" + job.TargetUsername
	}

	id, err := b.sched.Add(job)
	if err != nil {
		return b.reply(ctx, e, msg, "❌ failed to schedule: "+err.Error())
	}
	return b.reply(ctx, e, msg, fmt.Sprintf(
		"✅ scheduled `#%d`\n🎯 %s\n🕐 %s ist",
		id, targetDesc, runAt.Format("2 Jan, 3:04:05 PM"),
	))
}

func (b *Bot) cmdScheduleList(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	jobs := b.sched.List()
	if len(jobs) == 0 {
		return b.reply(ctx, e, msg, "📋 no pending scheduled jobs")
	}
	var sb strings.Builder
	sb.WriteString("📋 pending scheduled jobs\n\n")
	for _, j := range jobs {
		var target string
		if j.TargetUsername != "" {
			target = "@" + j.TargetUsername
		} else {
			target = fmt.Sprintf("post %d in %s", j.PostMsgID, j.PostChannel)
		}
		fmt.Fprintf(&sb, "`#%d` — %s ist — %s: %q\n",
			j.ID, j.RunAt.In(istLocation).Format("2 Jan, 3:04:05 PM"), target, truncate(j.Message, 40))
	}
	return b.reply(ctx, e, msg, sb.String())
}

func (b *Bot) cmdScheduleCancel(ctx context.Context, e tg.Entities, msg *tg.Message, idStr string) error {
	id, err := strconv.ParseInt(strings.TrimPrefix(idStr, "#"), 10, 64)
	if err != nil {
		return b.reply(ctx, e, msg, "⚠️ give a valid job id, e.g. .sh cancel 3")
	}
	if b.sched.Cancel(id) {
		return b.reply(ctx, e, msg, fmt.Sprintf("🗑️ cancelled job `#%d`", id))
	}
	return b.reply(ctx, e, msg, "🟡 no pending job with that id")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ---- execution: actually sending the scheduled content ---------------------
//
// These send the RAW message the user typed (via sendRaw, no font styling,
// no entity parsing) — it's going out to a third party as real content,
// not a bot status reply, so it should look exactly as written.

func (b *Bot) sendRaw(ctx context.Context, peer tg.InputPeerClass, text string, replyToID int) (tg.UpdatesClass, error) {
	req := &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  text,
		RandomID: randomID(),
	}
	if replyToID != 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyToID}
	}
	return b.api.MessagesSendMessage(ctx, req)
}

func (b *Bot) sendScheduledDM(ctx context.Context, username, message string) error {
	res, err := b.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return fmt.Errorf("could not resolve @%s: %w", username, err)
	}
	var peer tg.InputPeerClass
	for _, u := range res.Users {
		if user, ok := u.(*tg.User); ok {
			peer = &tg.InputPeerUser{UserID: user.ID, AccessHash: user.AccessHash}
			break
		}
	}
	if peer == nil {
		return fmt.Errorf("@%s did not resolve to a user", username)
	}
	_, err = b.sendRaw(ctx, peer, message, 0)
	return err
}

// sendScheduledComment posts a comment on a channel post. Channel "comments"
// are actually replies inside the discussion group Telegram auto-links to
// the channel, where each post gets mirror-forwarded — so this looks up
// that mirrored message via getDiscussionMessage and replies to it there.
func (b *Bot) sendScheduledComment(ctx context.Context, channelRef string, postID int, message string) error {
	channel, err := b.resolveChannelRef(ctx, channelRef)
	if err != nil {
		return err
	}

	disc, err := b.api.MessagesGetDiscussionMessage(ctx, &tg.MessagesGetDiscussionMessageRequest{
		Peer:  &tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash},
		MsgID: postID,
	})
	if err != nil {
		return fmt.Errorf("could not find the discussion thread for that post (does the channel have comments enabled?): %w", err)
	}
	if len(disc.Messages) == 0 {
		return fmt.Errorf("no discussion messages found for that post")
	}
	lastMsg, ok := disc.Messages[len(disc.Messages)-1].(*tg.Message)
	if !ok {
		return fmt.Errorf("unexpected discussion message type")
	}

	var discPeer tg.InputPeerClass
	if pc, ok := lastMsg.PeerID.(*tg.PeerChannel); ok {
		for _, c := range disc.Chats {
			if ch, ok := c.(*tg.Channel); ok && ch.ID == pc.ChannelID {
				discPeer = &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
				break
			}
		}
	}
	if discPeer == nil {
		return fmt.Errorf("could not resolve the discussion group for that channel")
	}

	_, err = b.sendRaw(ctx, discPeer, message, lastMsg.ID)
	return err
}

// resolveChannelRef turns a public username or a "c/<internal_id>"
// reference (from a private channel link) into an InputChannel.
func (b *Bot) resolveChannelRef(ctx context.Context, ref string) (*tg.InputChannel, error) {
	if strings.HasPrefix(ref, "c/") {
		idStr := strings.TrimPrefix(ref, "c/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad private channel id %q: %w", idStr, err)
		}
		return b.findJoinedChannelByID(ctx, id)
	}

	res, err := b.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: ref})
	if err != nil {
		return nil, fmt.Errorf("could not resolve channel @%s: %w", ref, err)
	}
	for _, c := range res.Chats {
		if ch, ok := c.(*tg.Channel); ok {
			return &tg.InputChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}, nil
		}
	}
	return nil, fmt.Errorf("@%s did not resolve to a channel", ref)
}

// findJoinedChannelByID handles private channel links (t.me/c/<id>/<msg>),
// which carry no access hash — the only way to get one is to already be a
// member and look it up among your own joined chats.
func (b *Bot) findJoinedChannelByID(ctx context.Context, id int64) (*tg.InputChannel, error) {
	dialogs, err := b.api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      200,
	})
	if err != nil {
		return nil, err
	}
	var chats []tg.ChatClass
	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
	}
	for _, c := range chats {
		if ch, ok := c.(*tg.Channel); ok && ch.ID == id {
			return &tg.InputChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}, nil
		}
	}
	return nil, fmt.Errorf("channel %d not found among joined chats — the account must already be a member", id)
}
