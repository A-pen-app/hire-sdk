package service

import (
	"context"
	"strconv"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
	"github.com/A-pen-app/logging"
)

type chatService struct {
	c store.Chat
	r store.Resume
	a store.App
	m store.Media
}

func NewChat(c store.Chat, r store.Resume, a store.App, m store.Media) Chat {
	return &chatService{
		c: c,
		r: r,
		a: a,
		m: m,
	}
}

func (s *chatService) New(ctx context.Context, bundleID, senderID, receiverID string, postID *string, resume *models.ResumeContent) (string, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return "", err
	}

	chatID, err := s.c.GetChatID(ctx, app.ID, senderID, receiverID, postID)
	if err != nil {
		logging.Errorw(ctx, "failed to get chat ID", "err", err, "appID", app.ID, "senderID", senderID, "receiverID", receiverID)
		return "", err
	}

	if resume != nil {
		if postID == nil {
			return "", models.ErrorWrongParams
		}

		// Update the user's resume
		if err := s.r.Update(ctx, app.ID, senderID, resume); err != nil {
			logging.Errorw(ctx, "failed to update resume", "err", err, "appID", app.ID, "senderID", senderID)
			return "", err
		}

		// Create a snapshot of the updated resume
		snapshot, err := s.r.CreateSnapshot(ctx, app.ID, senderID)
		if err != nil {
			logging.Errorw(ctx, "failed to create resume snapshot", "err", err, "appID", app.ID, "senderID", senderID)
			return "", err
		}

		// Create a relation between the resume snapshot and the chat room
		if _, err := s.r.CreateRelation(ctx, app.ID, senderID, snapshot.ID, chatID, *postID); err != nil {
			logging.Errorw(ctx, "failed to create resume relation", "err", err, "snapshotID", snapshot.ID, "chatID", chatID, "postID", *postID)
			return "", err
		}

		// Add a message indicating a post has been sent
		if _, err := s.c.AddMessage(ctx, senderID, chatID, receiverID, models.MsgPost, nil, nil, nil, postID); err != nil {
			logging.Errorw(ctx, "failed to add post message", "err", err, "chatID", chatID, "senderID", senderID, "receiverID", receiverID)
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

	if msgID := chat.LastMessageID; msgID != nil {
		msg, err := s.aggregateLastMessage(ctx, userID, *msgID, false)
		if err != nil {
			logging.Errorw(ctx, "aggregate last message failed", "err", err, "msgID", *msgID)
		} else {
			chat.LastMessage = msg
		}
	}

	if chat.PostID != nil {
		relation, err := s.r.GetRelation(ctx, models.ByChat(chatID))
		if err != nil {
			logging.Errorw(ctx, "failed to get resume relation", "err", err, "chatID", chatID)
			return nil, err
		}

		snapshot, err := s.r.GetSnapshot(ctx, relation.SnapshotID)
		if err != nil {
			logging.Errorw(ctx, "failed to get resume snapshot", "err", err, "snapshotID", relation.SnapshotID)
			return nil, err
		}

		chat.ResumeSnapshot = &models.ChatResumeSnapshot{
			ID:      snapshot.ID,
			Content: snapshot.Content,
			IsRead:  relation.IsRead,
			Status:  models.ResumeStatusUnlocked,
		}
	}

	return chat, nil
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

	chats, err := s.c.GetChats(ctx, app.ID, userID, next, count+1, opt.Status, opt.UnreadOnly)
	if err != nil {
		logging.Errorw(ctx, "failed to get chats", "err", err, "appID", app.ID, "userID", userID)
		return nil, "", err
	}

	for i := range chats {
		if msgID := chats[i].LastMessageID; msgID != nil {
			msg, err := s.aggregateLastMessage(ctx, userID, *msgID, true)
			if err != nil {
				logging.Errorw(ctx, "aggregate last message failed", "err", err, "msgID", *msgID)
			} else {
				chats[i].LastMessage = msg
			}
		}

		if chats[i].PostID != nil {
			relation, err := s.r.GetRelation(ctx, models.ByChat(chats[i].ChatID))
			if err != nil {
				logging.Errorw(ctx, "get resume relation  failed", "err", err, "chatID", chats[i].ChatID)
				continue
			}

			snapshot, err := s.r.GetSnapshot(ctx, relation.SnapshotID)
			if err != nil {
				logging.Errorw(ctx, "get resume snapshot  failed", "err", err, "snapshotID", relation.SnapshotID)
				continue
			}

			chats[i].ResumeSnapshot = &models.ChatResumeSnapshot{
				ID:      snapshot.ID,
				Content: snapshot.Content,
				IsRead:  relation.IsRead,
				Status:  models.ResumeStatusUnlocked,
			}
		}
	}

	next = ""
	n := len(chats)
	if n > count {
		next = strconv.FormatInt(chats[count-1].UpdatedAt.Unix(), 10)
		n = count
	}
	return chats[:n], next, nil
}

func (s *chatService) GetChatMessages(ctx context.Context, bundleID, userID, chatID string, next string, count int) ([]*models.Message, string, error) {

	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		logging.Errorw(ctx, "failed to get app by bundle ID", "err", err, "bundleID", bundleID)
		return nil, "", err
	}

	// check ownership
	if _, err := s.c.Get(ctx, app.ID, chatID, userID); err != nil {
		logging.Errorw(ctx, "failed to verify chat ownership", "err", err, "appID", app.ID, "chatID", chatID, "userID", userID)
		return nil, "", err
	}
	if count == 0 {
		return []*models.Message{}, next, nil
	}

	// get one more element for determining next cursor
	nonFilteredMsgs, err := s.c.GetMessages(ctx, chatID, next, count+1)
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

// aggregateLastMessage processes the last message with business logic (without user info)
func (s *chatService) aggregateLastMessage(ctx context.Context, userID string, msgID string, isInjectContent bool) (*models.Message, error) {
	msg, err := s.c.GetMessage(ctx, msgID)
	if err != nil {
		logging.Errorw(ctx, "get last message failed", "err", err, "msgID", msgID)
		return nil, err
	}

	status := msg.Status
	switch {
	case status.HasOneOf(models.DeletedBySender) && userID == msg.SenderID,
		status.HasOneOf(models.DeletedByReceiver) && userID != msg.SenderID,
		status.HasOneOf(models.Unsent):
		return nil, nil
	default:
		msg.Status = models.Normal
	}

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
			replyMsg.Type = models.MsgEmpty
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
			msg.Type = models.MsgEmpty
			msg.Status = models.Unsent
		default:
			msg.Status = models.Normal
		}
		msgs = append(msgs, msg)
	}
	return msgs
}
