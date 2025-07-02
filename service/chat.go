package service

import (
	"context"
	"strconv"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/hire-sdk/store"
)

type chatService struct {
	c store.Chat
	r store.Resume
	a store.App
}

func NewChat(c store.Chat, r store.Resume, a store.App) Chat {
	return &chatService{
		c: c,
		r: r,
		a: a,
	}
}

func (s *chatService) New(ctx context.Context, bundleID, senderID, receiverID string, postID *string, resume *models.ResumeContent) (string, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		return "", err
	}

	chatID, err := s.c.GetChatID(ctx, app.ID, senderID, receiverID, postID)
	if err != nil {
		return "", err
	}

	if resume != nil {
		if postID == nil {
			return "", models.ErrorWrongParams
		}

		if err := s.r.Update(ctx, app.ID, senderID, resume); err != nil {
			return "", err
		}

		snapshot, err := s.r.CreateSnapshot(ctx, app.ID, senderID)
		if err != nil {
			return "", err
		}

		if _, err := s.r.CreateRelation(ctx, app.ID, senderID, snapshot.ID, chatID, *postID); err != nil {
			return "", err
		}

		if _, err := s.c.AddMessage(ctx, senderID, chatID, receiverID, models.MsgPost, nil, nil, nil, postID); err != nil {
			return "", err
		}

	}

	return chatID, nil
}

func (s *chatService) Get(ctx context.Context, bundleID, chatID, userID string) (*models.ChatRoom, error) {
	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		return nil, err
	}

	chat, err := s.c.Get(ctx, app.ID, chatID, userID)
	if err != nil {
		return nil, err
	}

	relation, err := s.r.GetRelation(ctx, chatID)
	if err != nil {
		return nil, err
	}

	snapshot, err := s.r.GetSnapshot(ctx, relation.SnapshotID)
	if err != nil {
		return nil, err
	}

	chat.ResumeSnapshot = *snapshot
	chat.IsResumeRead = relation.IsRead

	return chat, nil
}

func (s *chatService) GetChats(ctx context.Context, bundleID, userID string, next string, count int, options ...models.GetOptionFunc) ([]*models.ChatRoom, string, error) {
	if count == 0 {
		return []*models.ChatRoom{}, next, nil
	}

	app, err := s.a.GetByBundleID(ctx, bundleID)
	if err != nil {
		return nil, "", err
	}

	opt := models.GetOption{}
	for _, f := range options {
		if err := f(&opt); err != nil {
			return nil, "", err
		}
	}

	chats, err := s.c.GetChats(ctx, app.ID, userID, next, count+1, opt.Status, opt.UnreadOnly, false)
	if err != nil {
		return nil, "", err
	}

	for i := range chats {
		relation, err := s.r.GetRelation(ctx, chats[i].ChatID)
		if err != nil {
			continue
		}

		snapshot, err := s.r.GetSnapshot(ctx, relation.SnapshotID)
		if err != nil {
			continue
		}

		chats[i].ResumeSnapshot = *snapshot
		chats[i].IsResumeRead = relation.IsRead
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
		return nil, "", err
	}

	// check ownership
	if _, err := s.c.Get(ctx, app.ID, chatID, userID); err != nil {
		return nil, "", err
	}
	if count == 0 {
		return []*models.Message{}, next, nil
	}

	// get one more element for determining next cursor
	nonFilteredMsgs, err := s.c.GetMessages(ctx, chatID, next, count+1)
	if err != nil {
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
