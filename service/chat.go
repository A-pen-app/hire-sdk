package service

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
	"github.com/A-pen-app/logging"
)

type chatService struct {
	c  store.Chat
	r  store.Resume
	a  store.App
	m  store.Media
	s  store.Subscription
	bc store.BusinessCard
}

func NewChat(c store.Chat, r store.Resume, a store.App, m store.Media, s store.Subscription, bc store.BusinessCard) Chat {
	return &chatService{
		c:  c,
		r:  r,
		a:  a,
		m:  m,
		s:  s,
		bc: bc,
	}
}

func (s *chatService) New(ctx context.Context, bundleID, senderID, receiverID string, postID *string, options ...models.NewChatOptionFunc) (string, error) {
	opt := models.NewChatOption{}
	for _, f := range options {
		if err := f(&opt); err != nil {
			logging.Errorw(ctx, "failed to apply new chat option", "err", err)
			return "", err
		}
	}

	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return "", err
	}

	var chatOpts []models.GetChatIDOptionFunc
	if opt.RecruiterContact != nil {
		chatOpts = append(chatOpts, models.WithChatRecruiterContact(opt.RecruiterContact))
	}
	if opt.AccessStatus != nil {
		chatOpts = append(chatOpts, models.WithChatAccessStatus(*opt.AccessStatus))
	}

	chatID, created, err := s.c.GetChatID(ctx, app.ID, senderID, receiverID, postID, chatOpts...)
	if err != nil {
		logging.Errorw(ctx, "failed to get chat ID", "err", err, "appID", app.ID, "senderID", senderID, "receiverID", receiverID)
		return "", err
	}

	// MsgPost is sent when the chat is newly created and has a postID
	if created && postID != nil {
		if _, err := s.c.AddMessage(ctx, senderID, chatID, receiverID, models.MsgPost, nil, nil, nil, postID); err != nil {
			logging.Errorw(ctx, "failed to add post message", "err", err, "chatID", chatID, "senderID", senderID, "receiverID", receiverID)
			return "", err
		}
	}

	if opt.Resume != nil {
		if postID == nil {
			return "", models.ErrorWrongParams
		}

		relation, err := s.r.GetRelation(ctx, models.ByChat(chatID))
		if err != nil && err != sql.ErrNoRows {
			logging.Errorw(ctx, "failed to get resume relation", "err", err, "chatID", chatID)
			return "", err
		}
		if relation != nil {
			return chatID, models.ErrorNotAllowed
		}

		// Update the user's resume
		if err := s.r.Update(ctx, app.ID, senderID, opt.Resume); err != nil {
			logging.Errorw(ctx, "failed to update resume", "err", err, "appID", app.ID, "senderID", senderID)
			return "", err
		}

		// Create a snapshot of the updated resume
		snapshot, err := s.r.CreateSnapshot(ctx, app.ID, senderID)
		if err != nil {
			logging.Errorw(ctx, "failed to create resume snapshot", "err", err, "appID", app.ID, "senderID", senderID)
			return "", err
		}

		// Create a relation: derive ResumeStatus from AccessStatus
		resumeStatus := models.ResumeStatusLocked
		if opt.AccessStatus != nil && *opt.AccessStatus == models.AccessStatusUnlocked {
			resumeStatus = models.ResumeStatusUnlocked
		}

		if _, err := s.r.CreateRelation(ctx, app.ID, senderID, snapshot.ID, chatID, *postID, resumeStatus); err != nil {
			logging.Errorw(ctx, "failed to create resume relation", "err", err, "snapshotID", snapshot.ID, "chatID", chatID, "postID", *postID)
			return "", err
		}

		// Add a resume message with reference_id pointing to the snapshot
		if _, err := s.c.AddMessage(ctx, senderID, chatID, receiverID, models.MsgResume, nil, nil, nil, &snapshot.ID); err != nil {
			logging.Errorw(ctx, "failed to add resume message", "err", err, "chatID", chatID, "senderID", senderID, "receiverID", receiverID)
			return "", err
		}
	}

	if opt.Card != nil {
		infos, err := s.c.GetBusinessCardChatInfos(ctx, []string{chatID})
		if err != nil {
			logging.Errorw(ctx, "failed to get business card chat infos", "err", err, "chatID", chatID)
			return "", err
		}
		if _, ok := infos[chatID]; ok {
			return chatID, models.ErrorNotAllowed
		}

		// Create a business card bcSnapshot
		bcSnapshot, err := s.bc.CreateSnapshot(ctx, app.ID, senderID, opt.Card)
		if err != nil {
			logging.Errorw(ctx, "failed to create business card snapshot", "err", err, "appID", app.ID, "senderID", senderID)
			return "", err
		}

		// Link snapshot to chat
		if err := s.c.UpdateBusinessCardSnapshotID(ctx, chatID, bcSnapshot.ID); err != nil {
			logging.Errorw(ctx, "failed to update business card snapshot id", "err", err, "chatID", chatID, "snapshotID", bcSnapshot.ID)
			return "", err
		}

		// Add a business card message with reference_id pointing to the snapshot
		if _, err := s.c.AddMessage(ctx, senderID, chatID, receiverID, models.MsgBusinessCard, nil, nil, nil, &bcSnapshot.ID); err != nil {
			logging.Errorw(ctx, "failed to add business card message", "err", err, "chatID", chatID, "senderID", senderID)
			return "", err
		}
	}

	return chatID, nil
}

func (s *chatService) Get(ctx context.Context, bundleID, chatID, userID string) (*models.ChatRoom, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	chat, err := s.c.Get(ctx, app.ID, chatID, userID)
	if err != nil {
		logging.Errorw(ctx, "failed to get chat", "err", err, "appID", app.ID, "chatID", chatID, "userID", userID)
		return nil, err
	}

	hireStatus := models.HireStatusInactive
	chat.HireStatus = &hireStatus

	if msgID := chat.LastMessageID; msgID != nil {
		msg, err := s.aggregateLastMessage(ctx, userID, *msgID, false, chat.ClearedAt)
		if err != nil {
			logging.Errorw(ctx, "aggregate last message failed", "err", err, "msgID", *msgID)
		} else {
			chat.LastMessage = msg
		}
	}

	if chat.PostID != nil {
		// Resume relation (used for both resume snapshot and job seeker detection)
		relation, err := s.r.GetRelation(ctx, models.ByChat(chatID))
		if err != nil && err != sql.ErrNoRows {
			logging.Errorw(ctx, "failed to get resume relation", "err", err, "chatID", chatID)
			return nil, err
		}

		// Determine job seeker: prefer resume relation, fall back to business card owner
		jobSeekerID := ""
		if relation != nil {
			jobSeekerID = relation.UserID
		} else if chat.BusinessCardSnapshotID != nil {
			ownerMap, err := s.bc.GetSnapshotOwners(ctx, []string{*chat.BusinessCardSnapshotID})
			if err != nil {
				logging.Errorw(ctx, "failed to get business card snapshot owner", "err", err, "snapshotID", *chat.BusinessCardSnapshotID)
				return nil, err
			}
			jobSeekerID = ownerMap[*chat.BusinessCardSnapshotID]
		}

		if userID == jobSeekerID {
			chat.AccessStatus = models.AccessStatusUnlocked
		} else if chat.AccessStatus != models.AccessStatusUnlocked &&
			subscribed(ctx, s.s, app.ID, userID) {
			chat.AccessStatus = models.AccessStatusUnlocked
		}

		// Resume snapshot
		if relation != nil {
			snapshot, err := s.r.GetSnapshot(ctx, relation.SnapshotID)
			if err != nil {
				logging.Errorw(ctx, "failed to get resume snapshot", "err", err, "snapshotID", relation.SnapshotID)
				return nil, err
			}

			chat.ResumeSnapshot = &models.ChatResumeSnapshot{
				ID:      snapshot.ID,
				Content: snapshot.Content,
				IsRead:  relation.IsRead,
				Status:  toResumeStatus(chat.AccessStatus),
			}
		}

		// Business card
		if chat.BusinessCardSnapshotID != nil {
			bcSnapshot, err := s.bc.GetSnapshot(ctx, *chat.BusinessCardSnapshotID)
			if err != nil {
				logging.Errorw(ctx, "failed to get business card snapshot", "err", err, "snapshotID", *chat.BusinessCardSnapshotID)
				return nil, err
			}
			chat.BusinessCardSnapshot = bcSnapshot
		}
	}

	return chat, nil
}

// CountChats counts the rooms matching options; a badge adds models.Unread().
func (s *chatService) CountChats(ctx context.Context, bundleID, userID string, options ...models.GetOptionFunc) (int, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return 0, err
	}

	opt := models.GetOption{}
	for _, f := range options {
		if err := f(&opt); err != nil {
			return 0, err
		}
	}
	return s.c.CountChats(ctx, app.ID, userID, opt)
}

func (s *chatService) GetChats(ctx context.Context, bundleID, userID string, next string, count int, options ...models.GetOptionFunc) ([]*models.ChatRoom, string, error) {
	if count == 0 {
		return []*models.ChatRoom{}, next, nil
	}

	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, "", err
	}

	opt := models.GetOption{}
	for _, f := range options {
		if err := f(&opt); err != nil {
			logging.Errorw(ctx, "failed to apply get option", "err", err)
			return nil, "", err
		}
	}

	chats, err := s.c.GetChats(ctx, app.ID, userID, next, count+1, opt)
	if err != nil {
		logging.Errorw(ctx, "failed to get chats", "err", err, "appID", app.ID, "userID", userID)
		return nil, "", err
	}

	// Batch: collect hire chat IDs
	var hireChatIDs []string
	for _, chat := range chats {
		if chat.PostID != nil {
			hireChatIDs = append(hireChatIDs, chat.ChatID)
		}
	}

	// Batch fetch resume relations
	resumeRelationMap := map[string]*models.ResumeRelation{}
	if len(hireChatIDs) > 0 {
		resumeRelations, err := s.r.ListRelations(ctx, app.ID, models.ByChatIDs(hireChatIDs))
		if err != nil {
			logging.Errorw(ctx, "failed to list resume relations", "err", err, "appID", app.ID)
		} else {
			for _, rel := range resumeRelations {
				resumeRelationMap[rel.ChatID] = rel
			}
		}
	}

	// Batch fetch business card snapshot owners (fallback for job seeker detection when no resume relation)
	bcOwnerMap := map[string]string{}
	var bcSnapshotIDs []string
	for _, chat := range chats {
		if chat.PostID != nil && chat.BusinessCardSnapshotID != nil {
			if _, ok := resumeRelationMap[chat.ChatID]; !ok {
				bcSnapshotIDs = append(bcSnapshotIDs, *chat.BusinessCardSnapshotID)
			}
		}
	}
	if len(bcSnapshotIDs) > 0 {
		owners, err := s.bc.GetSnapshotOwners(ctx, bcSnapshotIDs)
		if err != nil {
			logging.Errorw(ctx, "failed to get business card snapshot owners", "err", err)
		} else {
			bcOwnerMap = owners
		}
	}

	// Subscription: query once
	var subscription *models.UserSubscription
	if len(hireChatIDs) > 0 {
		subscription, err = s.s.Get(ctx, app.ID, userID)
		if err != nil && err != sql.ErrNoRows {
			logging.Errorw(ctx, "failed to get subscription", "err", err, "appID", app.ID, "userID", userID)
		}
	}
	isSubscribed := subscription != nil && subscription.Status.HasOneOf(models.SubscriptionSubscribed)

	for i := range chats {
		hireStatus := models.HireStatusInactive
		chats[i].HireStatus = &hireStatus

		if msgID := chats[i].LastMessageID; msgID != nil {
			msg, err := s.aggregateLastMessage(ctx, userID, *msgID, true, chats[i].ClearedAt)
			if err != nil {
				logging.Errorw(ctx, "aggregate last message failed", "err", err, "msgID", *msgID)
			} else {
				chats[i].LastMessage = msg
			}
		}

		if chats[i].PostID != nil {
			// Determine job seeker: prefer resume relation, fall back to business card owner
			jobSeekerID := ""
			if rel, ok := resumeRelationMap[chats[i].ChatID]; ok {
				jobSeekerID = rel.UserID
			} else if chats[i].BusinessCardSnapshotID != nil {
				jobSeekerID = bcOwnerMap[*chats[i].BusinessCardSnapshotID]
			}

			if userID == jobSeekerID {
				chats[i].AccessStatus = models.AccessStatusUnlocked
			} else if chats[i].AccessStatus != models.AccessStatusUnlocked && isSubscribed {
				chats[i].AccessStatus = models.AccessStatusUnlocked
			}

			// Resume
			if relation, ok := resumeRelationMap[chats[i].ChatID]; ok {
				snapshot, err := s.r.GetSnapshot(ctx, relation.SnapshotID)
				if err != nil {
					logging.Errorw(ctx, "get resume snapshot failed", "err", err, "snapshotID", relation.SnapshotID)
					continue
				}

				chats[i].ResumeSnapshot = &models.ChatResumeSnapshot{
					ID:      snapshot.ID,
					Content: snapshot.Content,
					IsRead:  relation.IsRead,
					Status:  toResumeStatus(chats[i].AccessStatus),
				}
			}

			// Business card
			if chats[i].BusinessCardSnapshotID != nil {
				bcSnapshot, err := s.bc.GetSnapshot(ctx, *chats[i].BusinessCardSnapshotID)
				if err != nil {
					logging.Errorw(ctx, "get business card snapshot failed", "err", err, "snapshotID", *chats[i].BusinessCardSnapshotID)
					continue
				}
				chats[i].BusinessCardSnapshot = bcSnapshot
			}
		}
	}

	// The store serves every pinned room on the first page and pages only the unpinned
	// ones, so `count` sizes the unpinned tail and the cursor has to be read off that
	// tail alone. Counting the pinned rooms into it would move the cursor backwards past
	// unpinned rooms the client has never been served. Pinned rooms only ever come from
	// the store's unpaged query, so they are exactly the IsPinned rows and they all sit
	// at the front.
	pinned := 0
	for _, chat := range chats {
		if chat.IsPinned {
			pinned++
		}
	}

	next = ""
	n := len(chats)
	if n-pinned > count {
		next = strconv.FormatInt(chats[pinned+count-1].UpdatedAt.Unix(), 10)
		n = pinned + count
	}
	return chats[:n], next, nil
}

func (s *chatService) FetchNewMessages(ctx context.Context, bundleID, userID, chatID string, messageID string) ([]*models.Message, error) {

	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	// The ownership check doubles as the source of the caller's delete cutoff: their own
	// chat_thread row is both "is a participant" and "how far back their history goes".
	chat, err := s.c.Get(ctx, app.ID, chatID, userID)
	if err != nil {
		logging.Errorw(ctx, "failed to verify chat ownership", "err", err, "appID", app.ID, "chatID", chatID, "userID", userID)
		return nil, err
	}
	// check last message
	lastMsg, err := s.c.GetMessage(ctx, messageID)
	if err != nil {
		return nil, err
	} else if lastMsg.ChatID != chatID {
		return nil, models.ErrorNotAllowed
	}

	nonFilteredMsgs, err := s.c.GetNewMessages(ctx, chatID, lastMsg.CreatedAt, chat.ClearedAt)
	if err != nil {
		return nil, err
	}

	return s.aggregateMessages(ctx, userID, nonFilteredMsgs), nil
}

func (s *chatService) GetChatMessages(ctx context.Context, bundleID, userID, chatID string, next string, count int) ([]*models.Message, string, error) {

	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, "", err
	}

	// Same as FetchNewMessages: this is the membership check and the source of the
	// caller's delete cutoff at once.
	chat, err := s.c.Get(ctx, app.ID, chatID, userID)
	if err != nil {
		logging.Errorw(ctx, "failed to verify chat ownership", "err", err, "appID", app.ID, "chatID", chatID, "userID", userID)
		return nil, "", err
	}
	if count == 0 {
		return []*models.Message{}, next, nil
	}

	// get one more element for determining next cursor
	nonFilteredMsgs, err := s.c.GetMessages(ctx, chatID, next, count+1, chat.ClearedAt)
	if err != nil {
		logging.Errorw(ctx, "failed to get messages", "err", err, "chatID", chatID, "count", count+1)
		return nil, "", err
	}

	msgs := s.aggregateMessages(ctx, userID, nonFilteredMsgs)

	// prepare next cursor
	next = ""
	n := len(nonFilteredMsgs)
	if n > count { // more elements available
		next = strconv.FormatInt(nonFilteredMsgs[count-1].CreatedAt.Unix(), 10)
		n = count
	}
	if n > len(msgs) {
		n = len(msgs)
	}

	return msgs[:n], next, nil
}

func (s *chatService) SendMessage(ctx context.Context, bundleID, userID, chatID string, options ...models.SendOptionFunc) (*models.Message, error) {
	params := models.SendOption{}
	for _, optionFunc := range options {
		if err := optionFunc(&params); err != nil {
			logging.Errorw(ctx, "build send option failed", "err", err)
			return nil, err
		}
	}

	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	chat, err := s.c.Get(ctx, app.ID, chatID, userID)
	if err != nil {
		logging.Errorw(ctx, "get chat failed", "err", err, "user_id", userID, "chat_id", chatID)
		return nil, err
	}

	msgID, err := s.c.AddMessage(ctx, userID, chatID, chat.ReceiverID, params.Type, params.Body, params.MediaIDs, params.ReplyToMessageID, nil)
	if err != nil {
		logging.Errorw(ctx, "create new message failed", "err", err, "user_id", userID, "chat_id", chatID)
		return nil, err
	}

	msg, err := s.c.GetMessage(ctx, msgID)
	if err != nil {
		logging.Errorw(ctx, "get message failed", "err", err, "message_id", msgID)
		return nil, err
	}
	s.injectContent(ctx, userID, msg, true)

	return msg, nil
}

func (s *chatService) UnsendMessage(ctx context.Context, bundleID, userID, messageID string) error {

	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return err
	}

	msg, err := s.c.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}

	chat, err := s.c.Get(ctx, app.ID, msg.ChatID, userID)
	if err != nil {
		logging.Errorw(ctx, "get chat failed", "err", err, "user_id", userID, "chat_id", msg.ChatID)
		return err
	}

	// check ownership
	if chat.AppID != app.ID || msg.SenderID != userID {
		return models.ErrorNotAllowed
	}
	// make this function idempotent
	if msg.Status.HasOneOf(models.Unsent | models.DeletedBySender) {
		return nil
	}

	if msg.Status != models.Normal {
		return models.ErrorNotAllowed
	}

	if err := s.c.EditMessage(ctx, messageID, models.Unsent); err != nil {
		logging.Errorw(ctx, "edit message failed", "err", err, "message_id", messageID)
		return err
	}

	return nil
}

// requireMembership resolves the app and returns the caller's own chat_thread row.
//
// chat_thread is per-user, so "has a row" is exactly "is a participant": a caller with
// none is not in the room and the store's sql.ErrNoRows is the right answer, before any
// visibility rule is considered. Every room-scoped write below goes through here rather
// than writing by chat id alone, which would let any signed-in user archive, delete or
// pin a room whose id they can guess.
func (s *chatService) requireMembership(ctx context.Context, bundleID, userID, chatID string) (*models.ChatRoom, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	chat, err := s.c.Get(ctx, app.ID, chatID, userID)
	if err != nil {
		logging.Errorw(ctx, "failed to verify chat ownership", "err", err, "appID", app.ID, "chatID", chatID, "userID", userID)
		return nil, err
	}
	return chat, nil
}

// Archive hides a chat room from this user's list, or brings it back (封存).
//
// Archiving touches no message: the room simply stops being listed until a new message
// from either side arrives, and opening it directly still shows the full history. The
// room is also marked read, because there is no archived tab from which a surviving
// unread count could ever be cleared. One-sided — the other participant is unaffected.
// See docs/chat_visibility.md.
func (s *chatService) Archive(ctx context.Context, bundleID, userID, chatID string, archived bool) error {
	if _, err := s.requireMembership(ctx, bundleID, userID, chatID); err != nil {
		return err
	}
	return s.c.SetHidden(ctx, chatID, userID, archived)
}

// Clear deletes the conversation for this user only (刪除對話).
//
// Every message already in the room stops being visible to the caller; the other
// participant keeps every one. The room also leaves the caller's list exactly as
// archiving would, and comes back when a new message arrives — carrying only what
// arrived after the cutoff. Deleting again only moves the cutoff forward; it is never
// reset. See docs/chat_visibility.md.
func (s *chatService) Clear(ctx context.Context, bundleID, userID, chatID string) error {
	if _, err := s.requireMembership(ctx, bundleID, userID, chatID); err != nil {
		return err
	}
	return s.c.SetCleared(ctx, chatID, userID)
}

// Pin pins a chat room to the top of this user's list, or unpins it (置頂).
//
// Display only: no message, unread count or visibility rule is touched. GetChats
// already orders on CT.is_pinned before C.updated_at, so the caller gets the list
// pinned-first, and the flag is returned on every room as is_pinned. One-sided, like
// Archive. Idempotent: setting the same value twice is a no-op.
func (s *chatService) Pin(ctx context.Context, bundleID, userID, chatID string, pinned bool) error {
	if _, err := s.requireMembership(ctx, bundleID, userID, chatID); err != nil {
		return err
	}
	return s.c.Pin(ctx, chatID, userID, pinned)
}

func (s *chatService) GetBusinessCardOnly(ctx context.Context, bundleID string, before time.Duration) ([]*models.BusinessCardChat, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, err
	}

	bcChats, err := s.c.GetBusinessCardChats(ctx, app.ID, before)
	if err != nil {
		return nil, err
	}
	if len(bcChats) == 0 {
		return nil, nil
	}

	chatIDs := make([]string, len(bcChats))
	for i, c := range bcChats {
		chatIDs[i] = c.ChatID
	}

	resumeRelations, err := s.r.ListRelations(ctx, app.ID, models.ByChatIDs(chatIDs))
	if err != nil {
		return nil, err
	}

	hasResumeMap := map[string]struct{}{}
	for _, rel := range resumeRelations {
		hasResumeMap[rel.ChatID] = struct{}{}
	}

	var filtered []*models.BusinessCardChat
	for _, c := range bcChats {
		if _, ok := hasResumeMap[c.ChatID]; !ok {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == 0 {
		return nil, nil
	}

	// Populate UserID (job seeker) from business card
	snapshotIDs := make([]string, len(filtered))
	for i, c := range filtered {
		snapshotIDs[i] = c.SnapshotID
	}

	userIDMap, err := s.bc.GetSnapshotOwners(ctx, snapshotIDs)
	if err != nil {
		return nil, err
	}

	for _, c := range filtered {
		c.UserID = userIDMap[c.SnapshotID]
	}

	return filtered, nil
}

func toResumeStatus(status models.AccessStatus) models.ResumeStatus {
	if status == models.AccessStatusUnlocked {
		return models.ResumeStatusUnlocked
	}
	return models.ResumeStatusLocked
}

// aggregateLastMessage processes the last message with business logic (without user info)
//
// clearedAt is the viewer's delete cutoff (rule R2), nil when they never deleted the
// room. The room's last_message_id points straight at a row, so this is the one read
// path with no room-scoped query to hang the cutoff off — it has to be applied here.
// A preview from before the cutoff is not "an empty preview", it is a message this
// viewer no longer has, so it comes back as nil exactly like a deleted or unsent one.
func (s *chatService) aggregateLastMessage(ctx context.Context, userID string, msgID string, isInjectContent bool, clearedAt *time.Time) (*models.Message, error) {
	msg, err := s.c.GetMessage(ctx, msgID)
	if err != nil {
		logging.Errorw(ctx, "get last message failed", "err", err, "msgID", msgID)
		return nil, err
	}

	if !models.IsLastMessageVisibleFor(msg, userID, clearedAt) {
		return nil, nil
	}
	msg.Status = models.Normal

	if isInjectContent {
		if err := s.injectContent(ctx, userID, msg, false); err != nil {
			logging.Errorw(ctx, "inject content to message failed", "err", err, "msgID", msgID, "userID", userID)
			return nil, err
		}
	}

	return msg, nil
}

// injectContent processes message content based on type and handles reply messages (without user info)
func (s *chatService) injectContent(ctx context.Context, userID string, msg *models.Message, injectReplyTo bool) error {
	switch msg.Type {
	case models.MsgText:
		if msg.Body == nil {
			emptyString := ""
			msg.Body = &emptyString
		}
	case models.MsgImage, models.MsgFile:
		media, err := s.m.Get(ctx, msg.MediaIDs)
		if err != nil {
			return err
		}
		msg.Medias = media
	case models.MsgForm:
		//TODO: inject form
	case models.MsgMeetup:
		//TODO: inject meetup
	case models.MsgBusinessCard:
		if msg.RefID != nil {
			snapshot, err := s.bc.GetSnapshot(ctx, *msg.RefID)
			if err != nil {
				return err
			}
			msg.BusinessCard = snapshot.Content
		}
	case models.MsgResume:
		if msg.RefID != nil {
			snapshot, err := s.r.GetSnapshot(ctx, *msg.RefID)
			if err != nil {
				return err
			}
			msg.Resume = snapshot.Content
		}
	}

	if injectReplyTo && msg.ReplyToMessageID != nil {
		replyMsg, err := s.c.GetMessage(ctx, *msg.ReplyToMessageID)
		if err != nil {
			return err
		}
		status := replyMsg.Status
		switch {
		case status.HasOneOf(models.DeletedBySender) && userID == replyMsg.SenderID,
			status.HasOneOf(models.DeletedByReceiver) && userID != replyMsg.SenderID,
			status.HasOneOf(models.Unsent):

			// user deleted/unsent this message, mark it as unavailable
			replyMsg.Status = models.Unavailable

			// wipe out message content for unsent
			replyMsg.Body = nil
			replyMsg.MediaIDs = nil
			// replyMsg.Type = models.MsgEmpty
		default:
			replyMsg.Status = models.Normal
		}

		if err := s.injectContent(ctx, userID, replyMsg, false); err != nil {
			return err
		}

		msg.ReplyTo = replyMsg
	}
	return nil
}

func (s *chatService) aggregateMessages(ctx context.Context, userID string, nonFilteredMsgs []*models.Message) []*models.Message {
	msgs := []*models.Message{}
	for i := range nonFilteredMsgs {
		msg := nonFilteredMsgs[i]
		status := msg.Status
		switch {
		case status.HasOneOf(models.DeletedBySender) && userID == msg.SenderID,
			status.HasOneOf(models.DeletedByReceiver) && userID != msg.SenderID:
			// user deleted this message, skip it
			continue
		case status.HasOneOf(models.Unsent):
			// wipe out message content for unsent
			msg.Body = nil
			msg.MediaIDs = nil
			// msg.Type = models.MsgEmpty
			msg.Status = models.Unsent
		default:
			msg.Status = models.Normal
		}

		s.injectContent(ctx, userID, msg, true)

		msgs = append(msgs, msg)
	}
	return msgs
}
