package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
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

func (s *chatStore) Get(ctx context.Context, chatID, userID string) (*models.ChatRoom, error) {
	chat := models.ChatRoom{}
	query := `
	SELECT 
		CT.chat_id, 
		CT.sender_id, 
		CT.receiver_id,
		C.last_message_id,
		CT.unread_count,
		CT.last_seen_at,
		C.updated_at,
		CT.status,
		CT.control_flag,
		C.created_at,
		C.post_id,
		CT.is_pinned,
		C.is_resume_read
	FROM public.chat_thread AS CT
	JOIN public.chat AS C
	ON CT.chat_id=C.id
	WHERE C.id=? AND CT.sender_id=?
	`
	values := []interface{}{
		chatID,
		userID,
	}
	query = s.db.Rebind(query)
	if err := s.db.QueryRowx(query, values...).StructScan(&chat); err != nil {
		return nil, err
	}
	return &chat, nil

}

func (s *chatStore) Read(ctx context.Context, userID, chatID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
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
		return err
	}

	// step 3: update user's total unread count
	query = `
	UPDATE public.user SET 
		unread_count=GREATEST(unread_count-?, 0)
	WHERE id=?
	RETURNING unread_count AS new_value, (SELECT unread_count FROM public.user WHERE id=?) AS old_value`
	query = s.db.Rebind(query)
	oldUnreads := 0
	newUnreads := 0
	if err = tx.QueryRow(query, unreadCountInChat, userID, userID).Scan(&newUnreads, &oldUnreads); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
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
		return err
	}

	return nil
}

func (s *chatStore) GetChats(ctx context.Context, userID string, next string, count int, status models.ChatAnnotation, unreadOnly bool) ([]*models.ChatRoom, error) {
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
		C.last_message_id,
		CT.unread_count,
		CT.last_seen_at,
		C.updated_at,
		CT.status,
		CT.control_flag,
		C.created_at,
		C.post_id,
		CT.is_pinned,
		C.is_resume_read
	FROM public.chat_thread AS CT
	JOIN public.chat AS C
	ON CT.chat_id=C.id
	JOIN public.user AS U
	ON CT.receiver_id=U.id
	WHERE `
	conditions := []string{
		"CT.sender_id=?",
		"C.updated_at<TO_TIMESTAMP(?)",
		"CT.status!=?",
		"CT.control_flag=?",
	}
	values := []interface{}{
		userID,
		next,
		models.Deleted,
		models.Pass,
	}
	if status != models.None {
		conditions = append(conditions, "CT.status=?")
		values = append(values, status)
	}
	if unreadOnly {
		conditions = append(conditions, "CT.unread_count>0")
	}
	query = query + strings.Join(conditions, " AND ") + " ORDER BY C.updated_at DESC LIMIT ?"
	values = append(values, count)

	query = s.db.Rebind(query)
	if err := s.db.Select(&chats, query, values...); err != nil {
		return nil, err
	}
	return chats, nil
}

func (s *chatStore) GetChatID(ctx context.Context, postID, senderID, receiverID string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	chatID := ""
	// step 1: check if chat_id already exists
	query := `
	SELECT chat_id FROM public.chat_thread 
	WHERE sender_id=? AND receiver_id=?
	`
	query = s.db.Rebind(query)
	if err := tx.QueryRow(query, senderID, receiverID).Scan(&chatID); err == sql.ErrNoRows {

		chatID = uuid.New().String()
		// step 2: create a new chat
		query = `
		INSERT INTO public.chat (id, post_id, created_at, updated_at)
		VALUES (?, ?, now(), now())`
		query = s.db.Rebind(query)
		if _, err := tx.Exec(query, chatID, postID); err != nil {
			return "", err
		}

		// step 3: create new chat threads for both sender and receiver
		query = `
		INSERT INTO public.chat_thread (chat_id, sender_id, receiver_id, unread_count, control_flag)
		VALUES (?, ?, ?, 0, ?)
		`
		query = s.db.Rebind(query)
		if _, err := tx.Exec(query, chatID, senderID, receiverID, models.NeverGotMessages); err != nil {
			return "", err
		}

		query = `
		INSERT INTO public.chat_thread (chat_id, sender_id, receiver_id, unread_count, control_flag)
		VALUES (?, ?, ?, 0, ?)
		`
		query = s.db.Rebind(query)
		if _, err := tx.Exec(query, chatID, receiverID, senderID, models.NeverGotMessages); err != nil {
			return "", err
		}

	} else if err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return chatID, nil

}

func (s *chatStore) AddMessages(ctx context.Context, userID, chatID, receiverID string, msgs []*models.Message) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
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
		return err
	}

	// step 4: update other's total unread count
	query = `
	UPDATE public.user SET 
		unread_count=unread_count+?
	WHERE id=?
	RETURNING unread_count AS new_value, (SELECT unread_count FROM public.user WHERE id=?) AS old_value
	`
	query = s.db.Rebind(query)
	oldUnreads := 0
	newUnreads := 0
	if err = tx.QueryRow(query, n, receiverID, receiverID).Scan(&newUnreads, &oldUnreads); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (s *chatStore) AddMessage(ctx context.Context, userID, chatID, receiverID string, typ models.MessageType, body *string, mediaIDs []string, replyToMessageID *string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
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
		media_ids
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
	)
	if err != nil {
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
		return "", err
	}

	// step 4: update other's total unread count
	query = `
	UPDATE public.user SET 
		unread_count=unread_count+1 
	WHERE id=?
	RETURNING unread_count AS new_value, (SELECT unread_count FROM public.user WHERE id=?) AS old_value
	`
	query = s.db.Rebind(query)
	oldUnreads := 0
	newUnreads := 0
	if err = tx.QueryRow(query, receiverID, receiverID).Scan(&newUnreads, &oldUnreads); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
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
		media_ids
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
	); err != nil {
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
		media_ids
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
		); err != nil {
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
		media_ids
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
		); err != nil {
			continue
		}
		msgs = append(msgs, &msg)
	}

	return msgs, nil
}
