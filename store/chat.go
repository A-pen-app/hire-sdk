package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type chatStore struct {
	db *sqlx.DB
}

// NewChat returns an implementation of store.Chat
func NewChat(db *sqlx.DB) Chat {
	return &chatStore{db: db}
}

func (s *chatStore) Get(ctx context.Context, appID, chatID, userID string) (*models.ChatRoom, error) {
	chat := models.ChatRoom{}
	query := `
	SELECT 
		CT.chat_id, 
		CT.sender_id, 
		CT.receiver_id,
		C.app_id,
		C.last_message_id,
		CT.unread_count,
		CT.last_seen_at,
		C.updated_at,
		CT.status,
		CT.control_flag,
		C.created_at,
		C.post_id,
		CT.is_pinned
	FROM public.chat_thread AS CT
	JOIN public.chat AS C
	ON CT.chat_id=C.id
	WHERE C.id=? AND C.app_id=? AND CT.sender_id=?
	`
	values := []interface{}{
		chatID,
		appID,
		userID,
	}
	query = s.db.Rebind(query)
	if err := s.db.QueryRowx(query, values...).StructScan(&chat); err != nil {
		logging.Errorw(ctx, "get chat thread failed", "err", err, "chatID", chatID, "appID", appID, "userID", userID)
		return nil, err
	}
	return &chat, nil

}

func (s *chatStore) Read(ctx context.Context, userID, chatID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		logging.Errorw(ctx, "begin tx failed", "err", err)
		return err
	}
	defer tx.Rollback()

	// step 1: update last_seed_at in receiver chat thread
	query := `
	UPDATE public.chat_thread
	SET last_seen_at=now()
	WHERE chat_id=? AND receiver_id=?
	`
	query = s.db.Rebind(query)
	if _, err := tx.Exec(query, chatID, userID); err != nil {
		logging.Errorw(ctx, "read chat thread failed", "err", err, "chatID", chatID, "userID", userID)
		return err
	}

	// step 2: update unread count in sender chat thread
	query = `
	UPDATE public.chat_thread
	SET unread_count=0
	WHERE chat_id=? AND sender_id=?
	RETURNING (SELECT unread_count FROM public.chat_thread
		WHERE chat_id=? AND sender_id=?
	)
	`
	query = s.db.Rebind(query)
	unreadCountInChat := 0
	if err := tx.QueryRow(query, chatID, userID, chatID, userID).Scan(&unreadCountInChat); err != nil {
		logging.Errorw(ctx, "update unread_count failed", "err", err, "chatID", chatID, "userID", userID)
		return err
	}

	if err := tx.Commit(); err != nil {
		logging.Errorw(ctx, "commit tx failed", "err", err)
		return err
	}

	return nil
}

func (s *chatStore) Annotate(ctx context.Context, chatID, userID string, status models.ChatAnnotation) error {
	query := `
	UPDATE public.chat_thread
	SET status=?
	WHERE chat_id=? AND sender_id=?
	`
	query = s.db.Rebind(query)
	if _, err := s.db.Exec(query, status, chatID, userID); err != nil {
		logging.Errorw(ctx, "annotate chat thread failed", "err", err, "chatID", chatID, "userID", userID)
		return err
	}

	return nil
}

func (s *chatStore) Pin(ctx context.Context, chatID, userID string, isPinned bool) error {
	query := `
	UPDATE public.chat_thread
	SET is_pinned=?
	WHERE chat_id=? AND sender_id=?
	`
	query = s.db.Rebind(query)
	if _, err := s.db.Exec(query, isPinned, chatID, userID); err != nil {
		logging.Errorw(ctx, "pin chat thread failed", "err", err, "chatID", chatID, "userID", userID, "isPinned", isPinned)
		return err
	}

	return nil
}

func (s *chatStore) GetChats(ctx context.Context, appID, userID string, next string, count int, status models.ChatAnnotation, unreadOnly bool) ([]*models.ChatRoom, error) {
	chats := []*models.ChatRoom{}
	if next == "" {
		// +2 seconds to prevent the last chat is created at almost the same time with getting chats
		next = strconv.FormatInt(time.Now().Unix()+2, 10)
	}
	query := `
	SELECT 
		CT.chat_id, 
		CT.sender_id, 
		CT.receiver_id,
		C.app_id,
		C.last_message_id,
		CT.unread_count,
		CT.last_seen_at,
		C.updated_at,
		CT.status,
		CT.control_flag,
		C.created_at,
		C.post_id,
		CT.is_pinned
	FROM public.chat_thread AS CT
	JOIN public.chat AS C
	ON CT.chat_id=C.id
	WHERE `
	conditions := []string{
		"C.app_id=?",
		"CT.sender_id=?",
		"C.updated_at<TO_TIMESTAMP(?)",
		"CT.status!=?",
		"CT.control_flag IN (?, ?)",
	}
	values := []interface{}{
		appID,
		userID,
		next,
		models.Deleted,
		models.Pass,
		models.NeverGotMessages,
	}
	if status != models.None {
		conditions = append(conditions, "CT.status=?")
		values = append(values, status)
	}
	if unreadOnly {
		conditions = append(conditions, "CT.unread_count>0")
	}

	query = query + strings.Join(conditions, " AND ") + " ORDER BY C.post_id IS NULL DESC, CT.is_pinned DESC, C.updated_at DESC LIMIT ?"
	values = append(values, count)

	query = s.db.Rebind(query)
	if err := s.db.Select(&chats, query, values...); err != nil {
		logging.Errorw(ctx, "get chat thread list failed", "err", err, "appID", appID, "userID", userID, "count", count)
		return nil, err
	}
	return chats, nil
}

func (s *chatStore) GetChatID(ctx context.Context, appID, senderID, receiverID string, postID *string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		logging.Errorw(ctx, "begin tx failed", "err", err)
		return "", err
	}
	defer tx.Rollback()

	chatID := ""
	// step 1: check if chat_id already exists
	query := `
	SELECT ct.chat_id FROM public.chat_thread ct
	JOIN public.chat c ON ct.chat_id = c.id
	WHERE c.app_id=? AND ct.sender_id=? AND ct.receiver_id=?
	  AND ((c.post_id IS NULL AND ? IS NULL) OR c.post_id=?)
	`
	query = s.db.Rebind(query)
	if err := tx.QueryRow(query, appID, senderID, receiverID, postID, postID).Scan(&chatID); err == sql.ErrNoRows {

		chatID = uuid.New().String()
		// step 2: create a new chat
		var query2 string
		var args []interface{}
		if postID == nil {
			query2 = `
			INSERT INTO public.chat (id, app_id, created_at, updated_at)
			VALUES (?, ?, now(), now())`
			args = []interface{}{chatID, appID}
		} else {
			query2 = `
			INSERT INTO public.chat (id, app_id, post_id, created_at, updated_at)
			VALUES (?, ?, ?, now(), now())`
			args = []interface{}{chatID, appID, *postID}
		}
		query2 = s.db.Rebind(query2)
		if _, err := tx.Exec(query2, args...); err != nil {
			logging.Errorw(ctx, "insert new chat failed", "err", err, "senderID", senderID, "receiverID", receiverID)
			return "", err
		}

		// step 3: create new chat threads for both sender and receiver
		query = `
		INSERT INTO public.chat_thread (chat_id, sender_id, receiver_id, unread_count, control_flag)
		VALUES (?, ?, ?, 0, ?)
		`
		query = s.db.Rebind(query)
		if _, err := tx.Exec(query, chatID, senderID, receiverID, models.NeverGotMessages); err != nil {
			logging.Errorw(ctx, "insert new chat thread failed", "err", err, "senderID", senderID, "receiverID", receiverID)
			return "", err
		}

		query = `
		INSERT INTO public.chat_thread (chat_id, sender_id, receiver_id, unread_count, control_flag)
		VALUES (?, ?, ?, 0, ?)
		`
		query = s.db.Rebind(query)
		if _, err := tx.Exec(query, chatID, receiverID, senderID, models.NeverGotMessages); err != nil {
			logging.Errorw(ctx, "insert new chat thread (reversed) failed", "err", err, "senderID", receiverID, "receiverID", senderID)
			return "", err
		}

	} else if err != nil {
		logging.Errorw(ctx, "get existing chat ID failed", "err", err, "appID", appID, "senderID", senderID, "receiverID", receiverID)
		return "", err
	}

	if err := tx.Commit(); err != nil {
		logging.Errorw(ctx, "commit tx failed", "err", err)
		return "", err
	}

	return chatID, nil

}

func (s *chatStore) AddMessages(ctx context.Context, userID, chatID, receiverID string, msgs []*models.Message) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		logging.Errorw(ctx, "begin tx failed", "err", err)
		return err
	}
	defer tx.Rollback()

	// step 1: add new message
	query := `
	INSERT INTO public.message (
		id,
		type,
		body,
		chat_id,
		sender_id,
		created_at,
		reply_to_message_id,
		status,
		media_ids
	)
	VALUES (
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		?,
		?
	)`
	var msgID string
	for i := range msgs {
		msgID = uuid.New().String()
		query = s.db.Rebind(query)
		if _, err = tx.Exec(query,
			msgID,
			msgs[i].Type,
			msgs[i].Body,
			chatID,
			userID,
			time.Now().UTC(),
			msgs[i].ReplyToMessageID,
			models.Normal,
			pq.Array(msgs[i].MediaIDs),
		); err != nil {
			logging.Errorw(ctx, "insert new message failed", "err", err, "chatID", chatID, "userID", userID)
			return err
		}
	}

	// step 2: update my chat table
	n := len(msgs)
	query = `
	UPDATE public.chat SET 
		updated_at=now(), 
		last_message_id=?
	WHERE id=?`
	query = s.db.Rebind(query)
	_, err = tx.Exec(query, msgID, chatID)
	if err != nil {
		logging.Errorw(ctx, "update last message failed", "err", err, "chat_id", chatID)
		return err
	}

	// step 3: update other's chat thread
	query = `
	UPDATE public.chat_thread SET 
		unread_count=unread_count+?,
		control_flag=control_flag&(~?::smallint)
	WHERE chat_id=? AND sender_id=?`
	query = s.db.Rebind(query)
	_, err = tx.Exec(query, n, models.NeverGotMessages, chatID, receiverID)
	if err != nil {
		logging.Errorw(ctx, "update unread_count failed", "err", err, "chat_id", chatID, "receiver_id", receiverID, "count", n)
		return err
	}

	if err := tx.Commit(); err != nil {
		logging.Errorw(ctx, "commit tx failed", "err", err)
		return err
	}

	return nil
}

func (s *chatStore) AddMessage(ctx context.Context, userID, chatID, receiverID string, typ models.MessageType, body *string, mediaIDs []string, replyToMessageID *string, referenceID *string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		logging.Errorw(ctx, "begin tx failed", "err", err)
		return "", err
	}
	defer tx.Rollback()

	// step 1: add new message
	msgID := uuid.New().String()
	query := `
	INSERT INTO public.message (
		id,
		type,
		body,
		chat_id,
		sender_id,
		created_at,
		reply_to_message_id,
		status,
		media_ids,
		reference_id
	)
	VALUES (
		?,
		?,
		?,
		?,
		?,
		now(),
		?,
		?,
		?,
		?
	)`
	query = s.db.Rebind(query)
	_, err = tx.Exec(query,
		msgID,
		typ,
		body,
		chatID,
		userID,
		// now()
		replyToMessageID,
		models.Normal,
		pq.Array(mediaIDs),
		referenceID,
	)
	if err != nil {
		logging.Errorw(ctx, "insert new message failed", "err", err, "chat_id", chatID)
		return "", err
	}

	// step 2: update my chat table
	query = `
	UPDATE public.chat SET 
		updated_at=now(), 
		last_message_id=?
	WHERE id=?`
	query = s.db.Rebind(query)
	_, err = tx.Exec(query, msgID, chatID)
	if err != nil {
		logging.Errorw(ctx, "update last message failed", "err", err, "chat_id", chatID)
		return "", err
	}

	// step 3: update other's chat thread
	query = `
	UPDATE public.chat_thread SET 
		unread_count=unread_count+1,
		control_flag=control_flag&(~?::smallint)
	WHERE chat_id=? AND sender_id=?`
	query = s.db.Rebind(query)
	_, err = tx.Exec(query, models.NeverGotMessages, chatID, receiverID)
	if err != nil {
		logging.Errorw(ctx, "update unread_count failed", "err", err, "chat_id", chatID, "receiver_id", receiverID)
		return "", err
	}

	if err := tx.Commit(); err != nil {
		logging.Errorw(ctx, "commit tx failed", "err", err)
		return "", err
	}
	return msgID, nil
}

func (s *chatStore) EditMessage(ctx context.Context, messageID string, newStatus models.MessageStatus) error {
	query := `
	UPDATE public.message SET status=status|? WHERE id=?
	`
	query = s.db.Rebind(query)
	if _, err := s.db.Exec(query, newStatus, messageID); err != nil {
		logging.Errorw(ctx, "update message status failed", "err", err, "messageID", messageID, "status", newStatus.String())
		return err
	}
	return nil
}

func (s *chatStore) GetMessage(ctx context.Context, messageID string) (*models.Message, error) {
	msg := models.Message{}
	query := `
	SELECT 
		id, 
		type, 
		body, 
		chat_id, 
		sender_id, 
		created_at, 
		reply_to_message_id, 
		status, 
		media_ids,
		reference_id
	FROM public.message WHERE id=?`
	query = s.db.Rebind(query)
	if err := s.db.QueryRowx(query, messageID).Scan(
		&msg.ID,
		&msg.Type,
		&msg.Body,
		&msg.ChatID,
		&msg.SenderID,
		&msg.CreatedAt,
		&msg.ReplyToMessageID,
		&msg.Status,
		pq.Array(&msg.MediaIDs), // workaround for postgres array type
		&msg.RefID,
	); err != nil {
		logging.Errorw(ctx, "get message failed", "err", err, "messageID", messageID)
		return nil, err
	}

	return &msg, nil
}

func (s *chatStore) GetNewMessages(ctx context.Context, chatID string, after time.Time) ([]*models.Message, error) {

	query := `
	SELECT
		id,
		type,
		body,
		chat_id,
		sender_id,
		created_at,
		reply_to_message_id,
		status,
		media_ids,
		reference_id	
	FROM public.message
	WHERE chat_id=? AND created_at>?
	ORDER BY created_at DESC
	`
	values := []interface{}{
		chatID,
		after,
	}
	query = s.db.Rebind(query)
	rows, err := s.db.Queryx(query, values...)
	if err != nil {
		logging.Errorw(ctx, "get new messages failed", "err", err, "chatID", chatID, "after", after)
		return nil, err
	}
	defer rows.Close()

	msgs := []*models.Message{}
	for rows.Next() {
		msg := models.Message{}
		if err := rows.Scan(
			&msg.ID,
			&msg.Type,
			&msg.Body,
			&msg.ChatID,
			&msg.SenderID,
			&msg.CreatedAt,
			&msg.ReplyToMessageID,
			&msg.Status,
			pq.Array(&msg.MediaIDs), // workaround for postgres array type
			&msg.RefID,
		); err != nil {
			logging.Errorw(ctx, "scan message failed", "err", err, "chatID", chatID)
			continue
		}
		msgs = append(msgs, &msg)
	}

	return msgs, nil
}

func (s *chatStore) GetMessages(ctx context.Context, chatID string, next string, count int) ([]*models.Message, error) {

	if next == "" {
		// +2 seconds to prevent the last message is created at almost the same time with getting messages
		next = strconv.FormatInt(time.Now().Unix()+2, 10)
	}
	query := `
	SELECT
		id,
		type,
		body,
		chat_id,
		sender_id,
		created_at,
		reply_to_message_id,
		status,
		media_ids,
		reference_id
	FROM public.message
	WHERE chat_id=? AND created_at<TO_TIMESTAMP(?)
	ORDER BY created_at DESC
	LIMIT ?
	`
	values := []interface{}{
		chatID,
		next,
		count,
	}
	query = s.db.Rebind(query)
	rows, err := s.db.Queryx(query, values...)
	if err != nil {
		logging.Errorw(ctx, "get messages failed", "err", err, "chatID", chatID)
		return nil, err
	}
	defer rows.Close()

	msgs := []*models.Message{}
	for rows.Next() {
		msg := models.Message{}
		if err := rows.Scan(
			&msg.ID,
			&msg.Type,
			&msg.Body,
			&msg.ChatID,
			&msg.SenderID,
			&msg.CreatedAt,
			&msg.ReplyToMessageID,
			&msg.Status,
			pq.Array(&msg.MediaIDs), // workaround for postgres array type
			&msg.RefID,
		); err != nil {
			logging.Errorw(ctx, "scan message failed", "err", err, "chatID", chatID)
			continue
		}
		msgs = append(msgs, &msg)
	}

	return msgs, nil
}
