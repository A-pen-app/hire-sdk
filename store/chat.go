package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// chat_thread's hidden_at and cleared_at are added by hand, per environment, and this
// library ships ahead of the DDL — see docs/chat_visibility.md. So archive and delete
// ask the database whether they can be done at all instead of assuming it.
//
// This is what makes one build deployable to an environment that has the columns and to
// one that does not, in either order and with no coordination. With no columns the SQL
// never names them, every room reads as never-archived and never-cleared — exactly the
// behaviour of the build that came before the feature — and the two write methods
// return models.ErrorChatArchiveUnavailable, which callers answer as 501. The DDL can
// then be applied underneath a running process, with no deploy and no restart, and
// archive starts working within a minute of the ALTER.
type columnProbe struct {
	mu      sync.Mutex
	present bool
	lastTry time.Time
}

// columnRecheckInterval bounds how often a column-less environment re-asks. Once the
// answer is yes it is never asked again: columns do not disappear from under a running
// process.
const columnRecheckInterval = time.Minute

// has reports whether public.chat_thread carries every one of the named columns.
//
// A false answer is a real answer, not an error: the caller drops the columns out of the
// SQL entirely. A probe that fails for any other reason also answers false, so a hiccup
// in the catalog degrades the feature rather than the chat list.
//
// It reads pg_catalog rather than information_schema on purpose. information_schema only
// shows columns the connecting role holds a privilege on, so a role that reads the table
// through its owner's grants would be told the column does not exist and the feature
// would stay silently off forever. pg_catalog answers what the database actually has.
func (p *columnProbe) has(ctx context.Context, db *sqlx.DB, columns ...string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.present {
		return true
	}
	if !p.lastTry.IsZero() && time.Since(p.lastTry) < columnRecheckInterval {
		return false
	}
	p.lastTry = time.Now()

	query := db.Rebind(`
	SELECT count(*)
	FROM pg_attribute a
	JOIN pg_class c ON c.oid=a.attrelid
	JOIN pg_namespace n ON n.oid=c.relnamespace
	WHERE n.nspname='public' AND c.relname='chat_thread'
		AND a.attname=ANY(?) AND a.attnum>0 AND NOT a.attisdropped`)

	found := 0
	if err := db.GetContext(ctx, &found, query, pq.Array(columns)); err != nil {
		logging.Errorw(ctx, "probe chat_thread columns failed", "err", err, "columns", columns)
		return false
	}

	// All or nothing: a half-applied ALTER is not a schema this code can run against.
	p.present = found == len(columns)
	return p.present
}

type chatStore struct {
	db *sqlx.DB

	// archive caches whether this environment can do archive and delete at all.
	archive columnProbe
}

// NewChat returns an implementation of store.Chat
func NewChat(db *sqlx.DB) Chat {
	return &chatStore{db: db}
}

// archivable reports whether this environment's chat_thread has the archive columns.
//
// Both are required together: cleared_at without hidden_at would let a room be deleted
// but never leave the list, which is not a state the rules describe.
func (s *chatStore) archivable(ctx context.Context) bool {
	return s.archive.has(ctx, s.db, "hidden_at", "cleared_at")
}

// chatBaseColumns and chatFrom build every room-shaped query in this file. Sharing them
// means a room can never mean two different things depending on which query returned it,
// and chatFrom is reused on its own by the chat list's cursor subquery.
const chatBaseColumns = `
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
		CT.hire_contact,
		CT.name`

// chatColumns adds the archive columns only where they exist. Selecting a column that
// is not there fails the whole query, and Get is the membership check every other chat
// operation runs first, so naming them unconditionally would take the entire hiring chat
// down on an environment that has not had the ALTER applied yet.
//
// Left out, HiddenAt and ClearedAt stay nil on every room, which every rule already
// reads as "never archived" and "never deleted".
func chatColumns(archivable bool) string {
	if !archivable {
		return chatBaseColumns
	}
	return chatBaseColumns + `,
		CT.hidden_at,
		CT.cleared_at`
}

// The trailing WHERE is deliberate: every caller appends its own conditions.
const chatFrom = `
	FROM public.chat_thread AS CT
	JOIN public.chat AS C
	ON CT.chat_id=C.id
	WHERE `

// Get returns one room as this user sees it.
//
// Deliberately unfiltered on hidden_at: archiving only removes a room from the list.
// Opening it by direct link or from a notification has to keep working and keep showing
// every message (CHAT-102). It is also the membership check the service layer relies on
// — chat_thread is per-user, so no row means not a participant.
func (s *chatStore) Get(ctx context.Context, appID, chatID, userID string) (*models.ChatRoom, error) {
	chat := models.ChatRoom{}
	query := chatColumns(s.archivable(ctx)) + chatFrom + `C.id=? AND C.app_id=? AND CT.sender_id=?`
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

// SetHidden archives or un-archives a chat room for one user (rule R1, 封存).
//
// Archiving writes hidden_at and drops the room's unread count to zero (rule R3): the
// room leaves the list and there is no archived tab to open it from, so a surviving
// unread count could never be cleared. Un-archiving only clears the flag — it does not
// restore the count, because those messages have been marked read.
//
// Nothing else is touched. The messages stay, and Get deliberately does not filter on
// hidden_at, so opening the room by direct link or from a notification still shows the
// full history (CHAT-102).
func (s *chatStore) SetHidden(ctx context.Context, chatID, userID string, hidden bool) error {
	// Writing the column is the one thing that cannot degrade: without it there is
	// nowhere to record the archive, so say so instead of letting Postgres answer with a
	// 500-shaped "column does not exist".
	if !s.archivable(ctx) {
		return models.ErrorChatArchiveUnavailable
	}

	query := `
	UPDATE public.chat_thread
	SET hidden_at=NULL
	WHERE chat_id=? AND sender_id=?
	`
	if hidden {
		query = `
	UPDATE public.chat_thread
	SET hidden_at=now(), unread_count=0
	WHERE chat_id=? AND sender_id=?
	`
	}
	query = s.db.Rebind(query)
	if _, err := s.db.ExecContext(ctx, query, chatID, userID); err != nil {
		logging.Errorw(ctx, "set chat thread hidden failed", "err", err, "chatID", chatID, "userID", userID, "hidden", hidden)
		return err
	}

	return nil
}

// SetCleared deletes the conversation for one user (rule R2, 刪除對話): every message
// already in the room stops being visible to them, while the other participant keeps
// the lot.
//
// Delete is archive plus a cutoff, so hidden_at and cleared_at are written in the same
// statement and therefore carry the *same* timestamp — now() is the transaction's
// start time, not the statement's. The room leaves the list exactly as archiving would
// and comes back the same way when a new message arrives, carrying only what arrived
// after the cutoff.
//
// The cutoff is never reset; deleting again only moves it forward. The unread count is
// zeroed for the same reason as in SetHidden (rule R3): deleting a room counts as
// having read it.
func (s *chatStore) SetCleared(ctx context.Context, chatID, userID string) error {
	if !s.archivable(ctx) {
		return models.ErrorChatArchiveUnavailable
	}

	query := `
	UPDATE public.chat_thread
	SET hidden_at=now(), cleared_at=now(), unread_count=0
	WHERE chat_id=? AND sender_id=?
	`
	query = s.db.Rebind(query)
	if _, err := s.db.ExecContext(ctx, query, chatID, userID); err != nil {
		logging.Errorw(ctx, "clear chat thread failed", "err", err, "chatID", chatID, "userID", userID)
		return err
	}

	return nil
}

// chatConditions is shared by GetChats and CountChats, so a count never
// disagrees with the list it labels. Paging stays out of it.
func chatConditions(appID, userID string, opt models.GetOption, archivable bool) ([]string, []interface{}) {
	// CT.status!=Deleted is the legacy "hidden forever" mark and stays as it is; the
	// hidden_at clause next to it is rule R1 — an archived room is listed again as soon
	// as a message arrives, and C.updated_at is written by the send path alone, so the
	// comparison needs no extra write anywhere. Strictly greater: activity exactly at
	// hidden_at leaves the room archived (CHAT-306).
	//
	// This is shared by GetChats, CountChats and CountByPostIDs on purpose: an archived
	// room must be absent from every list query and from the counts that label them
	// (CHAT-107).
	//
	// With no hidden_at column there is nothing to hide and the clause is left out
	// entirely, which is the list this repo produced before the feature existed.
	conditions := []string{
		"C.app_id=?",
		"CT.sender_id=?",
		"CT.status!=?",
	}
	if archivable {
		conditions = append(conditions, "(CT.hidden_at IS NULL OR C.updated_at>CT.hidden_at)")
	}
	values := []interface{}{appID, userID, models.Deleted}

	if opt.IsOfficialRole {
		conditions = append(conditions, "((C.post_id IS NULL AND CT.control_flag = ?) OR (C.post_id IS NOT NULL AND CT.control_flag IN (?, ?)))")
		values = append(values, models.Pass, models.Pass, models.NeverGotMessages)
	} else {
		conditions = append(conditions, "CT.control_flag IN (?, ?)")
		values = append(values, models.Pass, models.NeverGotMessages)
	}
	if opt.Status != models.None {
		conditions = append(conditions, "CT.status=?")
		values = append(values, opt.Status)
	}
	if opt.UnreadOnly {
		if opt.AwaitingReply {
			conditions = append(conditions, "(CT.unread_count>0 OR C.last_message_id IS NULL)")
		} else {
			conditions = append(conditions, "CT.unread_count>0")
		}
	}
	if opt.HasPost {
		conditions = append(conditions, "C.post_id IS NOT NULL")
	}
	if opt.PostID != nil {
		conditions = append(conditions, "C.post_id=?")
		values = append(values, *opt.PostID)
	}
	if opt.ExcludeOwnApplications {
		// Both belong to the job seeker, so either one marks the viewer as one.
		conditions = append(conditions, `NOT EXISTS (
			SELECT 1 FROM public.resume_relation RR
			WHERE RR.chat_id=C.id AND RR.user_id=?)`)
		values = append(values, userID)
		conditions = append(conditions, `NOT EXISTS (
			SELECT 1 FROM public.business_card_snapshot BCS
			JOIN public.business_card BC ON BC.id=BCS.business_card_id
			WHERE BCS.id=C.business_card_snapshot_id AND BC.user_id=?)`)
		values = append(values, userID)
	}
	if opt.Keyword != nil {
		// 履歷、名片、自訂名稱三個都比；CT 已被 sender_id 圈住，所以比到的是自己
		// 取的名字。用 EXISTS 而非 join：join 得改動每個分支共用的 FROM 子句。
		conditions = append(conditions, `(
			EXISTS (
				SELECT 1 FROM public.resume_relation RR
				JOIN public.resume_snapshot RS ON RS.id=RR.snapshot_id
				WHERE RR.chat_id=C.id AND RS.content->>'real_name' ILIKE ?)
			OR EXISTS (
				SELECT 1 FROM public.business_card_snapshot BS
				WHERE BS.id=C.business_card_snapshot_id AND BS.content->>'real_name' ILIKE ?)
			OR CT.name ILIKE ?
		)`)
		pattern := containsPattern(*opt.Keyword)
		values = append(values, pattern, pattern, pattern)
	}
	return conditions, values
}

// pinnedChatQuery returns every pinned room, unpaged. Pinning is a display flag, not a
// page of results: the whole set is served on the first page and never again, which is
// what keeps the cursor honest — see pagedChatQuery.
//
// "First page" cannot be spelled "the caller sent no cursor": GetChats defaults an empty
// one to now+2s, so this layer never sees one. What identifies the first page is the
// cursor sitting above the whole list — no listable room of this user has updated_at >=
// it. A genuine page-2 cursor is the updated_at of a row the client was just served, so
// that row satisfies >= and the pinned set is correctly left out. The comparison is
// strict for the same reason the paging one is: >= would re-serve the pinned rooms on
// every later page.
//
// The subquery that measures this reuses the aliases CT / C. The inner scope shadows the
// outer one and references nothing from it, so the same condition strings serve both —
// and they have to be the same conditions: measuring the cursor against a wider set
// would let a room this caller cannot even see push the pinned rooms off their own first
// page.
func pinnedChatQuery(columns string, conditions []string, values []interface{}, next string) (string, []interface{}) {
	firstPage := "TO_TIMESTAMP(?)>(SELECT COALESCE(MAX(C.updated_at), TO_TIMESTAMP(0))" +
		chatFrom + strings.Join(conditions, " AND ") + ")"

	pinned := append(append([]string{}, conditions...), "CT.is_pinned=true", firstPage)
	args := append(append([]interface{}{}, values...), next)
	args = append(args, values...)

	return columns + chatFrom + strings.Join(pinned, " AND ") + " ORDER BY C.updated_at DESC", args
}

// pagedChatQuery pages the unpinned rooms, most recently active first, with `next` as a
// keyset cursor on updated_at.
//
// The cursor carries updated_at alone, so it can only address a single-column sort key.
// Pinned rooms are therefore excluded here and handled by pinnedChatQuery: every row this
// query can return is unpinned, which makes the last row's updated_at an exact
// description of how far the client has read. Sorting pinned rooms into these pages
// instead — which is what `ORDER BY CT.is_pinned DESC, C.updated_at DESC` over a single
// paged query used to do — breaks that in both directions: the cursor only moves
// backwards, so a pinned-but-quiet room reappears at the top of page 1, 2, 3..., and once
// the pinned rooms fill a page the cursor jumps back to an old timestamp and swallows
// every unpinned room newer than it.
//
// The client still gets pinned-first ordering: GetChats concatenates the two.
func pagedChatQuery(columns string, conditions []string, values []interface{}, next string, count int) (string, []interface{}) {
	paged := append(append([]string{}, conditions...), "CT.is_pinned=false", "C.updated_at<TO_TIMESTAMP(?)")
	args := append(append([]interface{}{}, values...), next, count)

	return columns + chatFrom + strings.Join(paged, " AND ") + " ORDER BY C.updated_at DESC LIMIT ?", args
}

// GetChats returns one page of the user's chat list: every pinned room first, then the
// unpinned ones by last activity, newest first.
//
// count sizes the unpinned page only, so the first response holds count rows plus
// however many rooms the user has pinned.
func (s *chatStore) GetChats(ctx context.Context, appID, userID string, next string, count int, opt models.GetOption) ([]*models.ChatRoom, error) {
	if next == "" {
		// +2 seconds to prevent the last chat is created at almost the same time with getting chats
		next = strconv.FormatInt(time.Now().Unix()+2, 10)
	}
	archivable := s.archivable(ctx)
	columns := chatColumns(archivable)
	conditions, values := chatConditions(appID, userID, opt, archivable)

	chats := []*models.ChatRoom{}
	query, args := pinnedChatQuery(columns, conditions, values, next)
	if err := s.db.Select(&chats, s.db.Rebind(query), args...); err != nil {
		logging.Errorw(ctx, "get pinned chat thread list failed", "err", err, "appID", appID, "userID", userID)
		return nil, err
	}

	unpinned := []*models.ChatRoom{}
	query, args = pagedChatQuery(columns, conditions, values, next, count)
	if err := s.db.Select(&unpinned, s.db.Rebind(query), args...); err != nil {
		logging.Errorw(ctx, "get chat thread list failed", "err", err, "appID", appID, "userID", userID, "count", count)
		return nil, err
	}

	return append(chats, unpinned...), nil
}

func (s *chatStore) CountByPostIDs(ctx context.Context, appID, userID string, postIDs []string) (map[string]int, error) {
	counts := map[string]int{}
	if len(postIDs) == 0 {
		return counts, nil
	}

	conditions, values := chatConditions(appID, userID, models.GetOption{}, s.archivable(ctx))
	conditions = append(conditions, "C.post_id=ANY(?)")
	values = append(values, pq.Array(postIDs))

	query := `
	SELECT
		C.post_id,
		COUNT(*) AS count
	FROM public.chat_thread AS CT
	JOIN public.chat AS C
	ON CT.chat_id=C.id
	WHERE ` + strings.Join(conditions, " AND ") + `
	GROUP BY C.post_id
	`
	query = s.db.Rebind(query)

	rows := []struct {
		PostID string `db:"post_id"`
		Count  int    `db:"count"`
	}{}
	if err := s.db.SelectContext(ctx, &rows, query, values...); err != nil {
		logging.Errorw(ctx, "count chats by post ids failed", "err", err, "appID", appID, "userID", userID)
		return nil, err
	}

	for _, r := range rows {
		counts[r.PostID] = r.Count
	}
	return counts, nil
}

// CountChats counts what GetChats would list under the same options.
func (s *chatStore) CountChats(ctx context.Context, appID, userID string, opt models.GetOption) (int, error) {
	conditions, values := chatConditions(appID, userID, opt, s.archivable(ctx))

	query := `
	SELECT COUNT(*)
	FROM public.chat_thread AS CT
	JOIN public.chat AS C
	ON CT.chat_id=C.id
	WHERE ` + strings.Join(conditions, " AND ")
	query = s.db.Rebind(query)

	count := 0
	if err := s.db.GetContext(ctx, &count, query, values...); err != nil {
		logging.Errorw(ctx, "count chats failed", "err", err, "appID", appID, "userID", userID)
		return 0, err
	}
	return count, nil
}

// Deprecated: use CountChats with models.RecruitingRooms() and models.Unread().
func (s *chatStore) CountUnreadChats(ctx context.Context, appID, userID string) (int, error) {
	opt := models.GetOption{}
	for _, f := range []models.GetOptionFunc{models.RecruitingRooms(), models.Unread()} {
		if err := f(&opt); err != nil {
			return 0, err
		}
	}
	return s.CountChats(ctx, appID, userID, opt)
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

// GetNewMessages returns the messages of a room newer than `after`, as the owner of
// clearedAt sees them.
//
// clearedAt is that caller's delete cutoff (rule R2) and nil when they never deleted
// the room. It is applied here rather than after the query on purpose — see GetMessages.
func (s *chatStore) GetNewMessages(ctx context.Context, chatID string, after time.Time, clearedAt *time.Time) ([]*models.Message, error) {

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
	WHERE chat_id=? AND created_at>?`
	values := []interface{}{
		chatID,
		after,
	}
	if clearedAt != nil {
		query += ` AND created_at>?`
		values = append(values, *clearedAt)
	}
	query += `
	ORDER BY created_at DESC
	`
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

// GetMessages returns one page of a room's messages as the owner of clearedAt sees
// them, newest first.
//
// clearedAt is that caller's delete cutoff (rule R2) and nil when they never deleted
// the room. It belongs in the SQL rather than in a filter over the returned page: the
// mobile clients decide whether older history exists by asking "did I get `count` rows
// back?", so dropping rows after the fact makes a full page come back short and the
// client silently stops paging. Filtered in the WHERE clause, a page of `count` rows is
// always `count` rows the caller can see.
func (s *chatStore) GetMessages(ctx context.Context, chatID string, next string, count int, clearedAt *time.Time) ([]*models.Message, error) {

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
	WHERE chat_id=? AND created_at<TO_TIMESTAMP(?)`
	values := []interface{}{
		chatID,
		next,
	}
	if clearedAt != nil {
		query += ` AND created_at>?`
		values = append(values, *clearedAt)
	}
	query += `
	ORDER BY created_at DESC
	LIMIT ?
	`
	values = append(values, count)
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

func (s *chatStore) UpdateName(ctx context.Context, chatID string, userID string, name *string) error {
	query := `
	UPDATE public.chat_thread SET name=?
	WHERE chat_id=? AND sender_id=?
	`
	query = s.db.Rebind(query)
	if _, err := s.db.ExecContext(ctx, query, name, chatID, userID); err != nil {
		logging.Errorw(ctx, "update chat name failed", "err", err, "chatID", chatID, "userID", userID)
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

// UpdateAccessStatusByPosts is the counterpart of UpdateRelationListStatus;
// the two are always used together.
func (s *chatStore) UpdateAccessStatusByPosts(ctx context.Context, postIDs []string, status models.AccessStatus) error {
	query := s.db.Rebind(`
	UPDATE public.chat
	SET access_status=?
	WHERE post_id = ANY(?)
	`)
	if _, err := s.db.Exec(query, status, pq.Array(postIDs)); err != nil {
		logging.Errorw(ctx, "failed to update chat access status by posts", "err", err, "postIDs", postIDs)
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
	SELECT C.id AS chat_id, C.post_id, C.business_card_snapshot_id AS snapshot_id
	FROM public.chat C
	WHERE C.app_id = ?
	  AND C.business_card_snapshot_id IS NOT NULL
	  AND C.post_id IS NOT NULL
	  AND C.created_at < ?
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
