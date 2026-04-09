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
		CT.is_pinned,
		C.business_card_snapshot_id,
		C.access_status,
		CT.hire_contact
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

func (s *chatStore) GetChats(ctx context.Context, appID, userID string, next string, count int, status models.ChatAnnotation, unreadOnly bool, isOfficialRole bool) ([]*models.ChatRoom, error) {
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
		CT.is_pinned,
		C.business_card_snapshot_id,
		C.access_status,
		CT.hire_contact
	FROM public.chat_thread AS CT
	JOIN public.chat AS C
	ON CT.chat_id=C.id
	WHERE `
	conditions := []string{
		"C.app_id=?",
		"CT.sender_id=?",
		"C.updated_at<TO_TIMESTAMP(?)",
		"CT.status!=?",
	}
	values := []interface{}{
		appID,
		userID,
		next,
		models.Deleted,
	}

	if isOfficialRole {
		conditions = append(conditions, "((C.post_id IS NULL AND CT.control_flag = ?) OR (C.post_id IS NOT NULL AND CT.control_flag IN (?, ?)))")
		values = append(values, models.Pass, models.Pass, models.NeverGotMessages)
	} else {
		conditions = append(conditions, "CT.control_flag IN (?, ?)")
		values = append(values, models.Pass, models.NeverGotMessages)
	}
	if status != models.None {
		conditions = append(conditions, "CT.status=?")
		values = append(values, status)
	}
	if unreadOnly {
		conditions = append(conditions, "CT.unread_count>0")
	}

	query = query + strings.Join(conditions, " AND ") + " ORDER BY CT.is_pinned DESC, C.updated_at DESC LIMIT ?"
	values = append(values, count)

	query = s.db.Rebind(query)
	if err := s.db.Select(&chats, query, values...); err != nil {
		logging.Errorw(ctx, "get chat thread list failed", "err", err, "appID", appID, "userID", userID, "count", count)
		return nil, err
	}
	return chats, nil
}

func (s *chatStore) GetChatID(ctx context.Context, appID, senderID, receiverID string, postID *string, opts ...models.GetChatIDOptionFunc) (string, bool, error) {
	opt := models.GetChatIDOption{}
	for _, f := range opts {
		f(&opt)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		logging.Errorw(ctx, "begin tx failed", "err", err)
		return "", false, err
	}
	defer tx.Rollback()

	chatID := ""
	created := false
	// step 1: check if chat_id already exists
	var query string
	var args []interface{}

	if postID == nil {
		// 查詢一般聊天室 (post_id IS NULL)
		query = `
		SELECT ct.chat_id FROM public.chat_thread ct
		JOIN public.chat c ON ct.chat_id = c.id
		WHERE c.app_id=? AND ct.sender_id=? AND ct.receiver_id=? AND c.post_id IS NULL`
		args = []interface{}{appID, senderID, receiverID}
	} else {
		// 查詢特定貼文聊天室 (post_id = ?)
		query = `
		SELECT ct.chat_id FROM public.chat_thread ct
		JOIN public.chat c ON ct.chat_id = c.id
		WHERE c.app_id=? AND ct.sender_id=? AND ct.receiver_id=? AND c.post_id=?`
		args = []interface{}{appID, senderID, receiverID, *postID}
	}

	query = s.db.Rebind(query)
	if err := tx.QueryRow(query, args...).Scan(&chatID); err == sql.ErrNoRows {

		created = true
		chatID = uuid.New().String()

		accessStatus := models.AccessStatusLocked
		if opt.AccessStatus != nil {
			accessStatus = *opt.AccessStatus
		}

		// step 2: create a new chat
		var query2 string
		var args2 []interface{}
		if postID == nil {
			query2 = `
			INSERT INTO public.chat (id, app_id, access_status, created_at, updated_at)
			VALUES (?, ?, ?, now(), now())`
			args2 = []interface{}{chatID, appID, accessStatus}
		} else {
			query2 = `
			INSERT INTO public.chat (id, app_id, post_id, access_status, created_at, updated_at)
			VALUES (?, ?, ?, ?, now(), now())`
			args2 = []interface{}{chatID, appID, *postID, accessStatus}
		}
		query2 = s.db.Rebind(query2)
		if _, err := tx.Exec(query2, args2...); err != nil {
			logging.Errorw(ctx, "insert new chat failed", "err", err, "senderID", senderID, "receiverID", receiverID)
			return "", false, err
		}

		// step 3: create new chat threads for both sender and receiver
		pinned := postID == nil
		query = `
		INSERT INTO public.chat_thread (chat_id, sender_id, receiver_id, unread_count, control_flag,is_pinned)
		VALUES (?, ?, ?, 0, ?, ?)
		`
		query = s.db.Rebind(query)
		if _, err := tx.Exec(query, chatID, senderID, receiverID, models.NeverGotMessages, pinned); err != nil {
			logging.Errorw(ctx, "insert new chat thread failed", "err", err, "senderID", senderID, "receiverID", receiverID)
			return "", false, err
		}

		query = `
		INSERT INTO public.chat_thread (chat_id, sender_id, receiver_id, unread_count, control_flag, hire_contact)
		VALUES (?, ?, ?, 0, ?, ?)
		`
		query = s.db.Rebind(query)
		if _, err := tx.Exec(query, chatID, receiverID, senderID, models.NeverGotMessages, opt.RecruiterContact); err != nil {
			logging.Errorw(ctx, "insert new chat thread (reversed) failed", "err", err, "senderID", receiverID, "receiverID", senderID)
			return "", false, err
		}

	} else if err != nil {
		logging.Errorw(ctx, "get existing chat ID failed", "err", err, "appID", appID, "senderID", senderID, "receiverID", receiverID)
		return "", false, err
	} else {
		// chat already exists, update contact on receiver's thread and access_status on chat
		if opt.RecruiterContact != nil {
			query = s.db.Rebind(`UPDATE public.chat_thread SET hire_contact=? WHERE chat_id=? AND sender_id=?`)
			if _, err := tx.Exec(query, opt.RecruiterContact, chatID, receiverID); err != nil {
				logging.Errorw(ctx, "update receiver hire_contact failed", "err", err, "chatID", chatID)
				return "", false, err
			}
		}
		if opt.AccessStatus != nil {
			query = s.db.Rebind(`UPDATE public.chat SET access_status=? WHERE id=?`)
			if _, err := tx.Exec(query, *opt.AccessStatus, chatID); err != nil {
				logging.Errorw(ctx, "update chat access_status failed", "err", err, "chatID", chatID)
				return "", false, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		logging.Errorw(ctx, "commit tx failed", "err", err)
		return "", false, err
	}

	return chatID, created, nil

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

func (s *chatStore) GetFirstMessages(ctx context.Context, opt []models.FirstMessageOption) (map[string]*models.Message, error) {
	if len(opt) == 0 {
		return nil, nil
	}

	chatIDs := make([]string, len(opt))
	excludedSenderIDs := make([]*string, len(opt))
	for i, opt := range opt {
		chatIDs[i] = opt.ChatID
		excludedSenderIDs[i] = opt.ExcludedSenderID
	}

	// Use LATERAL join to get the first employer message for each chat_id
	query := `
	SELECT
		m.id,
		m.type,
		m.body,
		m.chat_id,
		m.sender_id,
		m.created_at,
		m.reply_to_message_id,
		m.status,
		m.media_ids,
		m.reference_id
	FROM unnest(?::text[], ?::text[]) AS input(chat_id, job_seeker_id)
	CROSS JOIN LATERAL (
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
		WHERE chat_id = input.chat_id::uuid AND sender_id != input.job_seeker_id::uuid
		ORDER BY created_at ASC
		LIMIT 1
	) m
	`
	query = s.db.Rebind(query)

	rows, err := s.db.Queryx(query, pq.Array(chatIDs), pq.Array(excludedSenderIDs))
	if err != nil {
		logging.Errorw(ctx, "failed to get first employer messages", "err", err, "opt", opt)
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*models.Message)
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
			pq.Array(&msg.MediaIDs),
			&msg.RefID,
		); err != nil {
			logging.Errorw(ctx, "failed to scan message", "err", err)
			return nil, err
		}
		result[msg.ChatID] = &msg
	}

	return result, nil
}

func (s *chatStore) UpdateHireContact(ctx context.Context, chatID string, userID string, contact *models.HireContact) error {
	query := `
	UPDATE public.chat_thread SET hire_contact=?
	WHERE chat_id=? AND sender_id=?
	`
	query = s.db.Rebind(query)
	if _, err := s.db.Exec(query, contact, chatID, userID); err != nil {
		logging.Errorw(ctx, "update hire contact failed", "err", err, "chatID", chatID, "userID", userID)
		return err
	}

	return nil
}

func (s *chatStore) UpdateBusinessCardSnapshotID(ctx context.Context, chatID, snapshotID string) error {
	query := `
	UPDATE public.chat SET business_card_snapshot_id=?
	WHERE id=?
	`
	query = s.db.Rebind(query)
	if _, err := s.db.Exec(query, snapshotID, chatID); err != nil {
		logging.Errorw(ctx, "update business card snapshot id failed", "err", err, "chatID", chatID, "snapshotID", snapshotID)
		return err
	}
	return nil
}

func (s *chatStore) UpdateAccessStatus(ctx context.Context, chatID string, status models.AccessStatus) error {
	query := `
	UPDATE public.chat SET access_status=?
	WHERE id=?
	`
	query = s.db.Rebind(query)
	if _, err := s.db.Exec(query, status, chatID); err != nil {
		logging.Errorw(ctx, "update access status failed", "err", err, "chatID", chatID, "status", status)
		return err
	}
	return nil
}

func (s *chatStore) GetBusinessCardChats(ctx context.Context, appID string, before time.Duration) ([]*models.BusinessCardChat, error) {
	query := `
	SELECT C.id AS chat_id, CT.sender_id, CT.receiver_id, C.post_id
	FROM public.chat C
	JOIN public.chat_thread CT ON C.id = CT.chat_id
	WHERE C.app_id = ?
	  AND C.business_card_snapshot_id IS NOT NULL
	  AND C.post_id IS NOT NULL
	  AND C.created_at < ?
	  AND CT.sender_id != CT.receiver_id
	`
	query = s.db.Rebind(query)

	threshold := time.Now().Add(-before)
	var chats []*models.BusinessCardChat
	if err := s.db.SelectContext(ctx, &chats, query, appID, threshold); err != nil {
		logging.Errorw(ctx, "failed to get business card chats", "err", err, "appID", appID)
		return nil, err
	}
	return chats, nil
}

func (s *chatStore) GetUserChattingPostIDs(ctx context.Context, appID, userID string) ([]string, error) {
	query := `
	SELECT C.post_id
	FROM public.chat C
	JOIN public.chat_thread CT ON C.id = CT.chat_id
	WHERE C.app_id = ? AND CT.sender_id = ?
	  AND C.business_card_snapshot_id IS NOT NULL
	  AND C.post_id IS NOT NULL
	`
	query = s.db.Rebind(query)

	var postIDs []string
	if err := s.db.SelectContext(ctx, &postIDs, query, appID, userID); err != nil {
		logging.Errorw(ctx, "failed to get chatting post ids", "err", err, "appID", appID, "userID", userID)
		return nil, err
	}
	return postIDs, nil
}

func (s *chatStore) GetBusinessCardChatInfos(ctx context.Context, chatIDs []string) (map[string]*models.BusinessCardChatInfo, error) {
	if len(chatIDs) == 0 {
		return map[string]*models.BusinessCardChatInfo{}, nil
	}

	type row struct {
		ChatID     string  `db:"id"`
		SnapshotID *string `db:"business_card_snapshot_id"`
		PostID     *string `db:"post_id"`
	}

	query := `
	SELECT id, business_card_snapshot_id, post_id
	FROM public.chat
	WHERE id = ANY(?) AND business_card_snapshot_id IS NOT NULL
	`
	query = s.db.Rebind(query)

	var rows []row
	if err := s.db.SelectContext(ctx, &rows, query, pq.Array(chatIDs)); err != nil {
		logging.Errorw(ctx, "failed to get business card chat infos", "err", err)
		return nil, err
	}

	m := make(map[string]*models.BusinessCardChatInfo, len(rows))
	for _, r := range rows {
		if r.SnapshotID != nil {
			info := &models.BusinessCardChatInfo{SnapshotID: *r.SnapshotID}
			if r.PostID != nil {
				info.PostID = *r.PostID
			}
			m[r.ChatID] = info
		}
	}
	return m, nil
}
