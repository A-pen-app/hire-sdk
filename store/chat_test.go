package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/A-pen-app/hire-sdk/models"
	"github.com/A-pen-app/logging"
	"github.com/jmoiron/sqlx"
)

// The store logs on error paths, and logging panics on a nil zap logger.
func TestMain(m *testing.M) {
	if err := logging.Initialize(nil); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestCountChatsUnreadRooms(t *testing.T) {
	conn := &recordingConnector{columns: []string{"count"}, rows: [][]driver.Value{{int64(3)}}}
	db := sqlx.NewDb(sql.OpenDB(conn), "postgres")
	defer db.Close()

	opt := models.GetOption{}
	for _, f := range []models.GetOptionFunc{models.RecruitingRooms(), models.Unread()} {
		if err := f(&opt); err != nil {
			t.Fatalf("option failed: %v", err)
		}
	}
	count, err := NewChat(db).CountChats(context.Background(), "app-1", "user-1", opt)
	if err != nil {
		t.Fatalf("CountChats failed: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	// 未讀的兩種來源都要在；官方房（沒有 post_id）與他自己去應徵的房都不能算進去
	for _, want := range []string{
		"CT.unread_count>0 OR C.last_message_id IS NULL",
		"C.post_id IS NOT NULL",
		"public.resume_relation RR",
		"public.business_card_snapshot BCS",
	} {
		if !strings.Contains(conn.query, want) {
			t.Errorf("query missing %q:\n%s", want, conn.query)
		}
	}

	// Rebind 之後靠位置對應，順序錯了就會用別人的值去篩
	wantArgs := []driver.Value{
		"app-1", "user-1",
		int64(models.Deleted), int64(models.Pass), int64(models.NeverGotMessages),
		// 排除自己是求職方的房：履歷一次、名片一次
		"user-1", "user-1",
	}
	if len(conn.args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", conn.args, wantArgs)
	}
	for i := range wantArgs {
		if conn.args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %v, want %v", i, conn.args[i], wantArgs[i])
		}
	}
}

// R1: an archived room must be absent from every list query, and from the counts that
// label them (CHAT-107). The clause lives in chatConditions, which GetChats, CountChats
// and CountByPostIDs all share, so asserting it here asserts it on all three.
func TestChatConditionsHideArchivedRooms(t *testing.T) {
	conditions, values := chatConditions("app-1", "user-1", models.GetOption{}, true)
	joined := strings.Join(conditions, " AND ")

	// Strictly greater, so a room whose last activity lands exactly on hidden_at stays
	// archived (CHAT-306).
	const want = "(CT.hidden_at IS NULL OR C.updated_at>CT.hidden_at)"
	if !strings.Contains(joined, want) {
		t.Errorf("conditions missing %q: %s", want, joined)
	}
	// The legacy "hidden forever" mark keeps working alongside it (CHAT-401).
	if !strings.Contains(joined, "CT.status!=?") {
		t.Errorf("conditions dropped status!=Deleted: %s", joined)
	}
	// R1 carries no placeholder, so it must not disturb the argument list either way.
	assertQueryArgs(t, "conditions", values, []driver.Value{
		"app-1", "user-1", int64(models.Deleted), int64(models.Pass), int64(models.NeverGotMessages),
	})
}

// An environment whose chat_thread has no hidden_at column must produce exactly the
// list this repo produced before the feature existed — naming the column would fail the
// whole query, and the chat list is not something that may fail.
func TestChatConditionsWithoutArchiveColumns(t *testing.T) {
	conditions, values := chatConditions("app-1", "user-1", models.GetOption{}, false)
	joined := strings.Join(conditions, " AND ")

	if strings.Contains(joined, "hidden_at") {
		t.Errorf("conditions name hidden_at on a schema that has no such column: %s", joined)
	}
	if !strings.Contains(joined, "CT.status!=?") {
		t.Errorf("conditions dropped status!=Deleted: %s", joined)
	}
	assertQueryArgs(t, "conditions", values, []driver.Value{
		"app-1", "user-1", int64(models.Deleted), int64(models.Pass), int64(models.NeverGotMessages),
	})
}

// Get is the membership check every other chat operation runs first, so a SELECT that
// names a missing column takes the whole hiring chat down rather than just archive.
func TestChatColumns(t *testing.T) {
	with := chatColumns(true)
	if !strings.Contains(with, "CT.hidden_at") || !strings.Contains(with, "CT.cleared_at") {
		t.Errorf("archive columns missing from the select list:\n%s", with)
	}

	without := chatColumns(false)
	if strings.Contains(without, "hidden_at") || strings.Contains(without, "cleared_at") {
		t.Errorf("select list names the archive columns on a schema that has none:\n%s", without)
	}
	// Everything else has to survive, or the degraded build returns a different room.
	for _, want := range []string{"CT.chat_id", "CT.is_pinned", "CT.hire_contact", "CT.name", "C.updated_at"} {
		if !strings.Contains(without, want) {
			t.Errorf("degraded select list dropped %q:\n%s", want, without)
		}
	}
}

// The build ships ahead of the DDL, so the write side has to say "this environment
// cannot do that" rather than let Postgres answer with a 500-shaped "column does not
// exist". The read side degrades instead — see TestChatColumns.
func TestArchiveWritesRefuseWithoutColumns(t *testing.T) {
	// The probe counts the columns it found; one short of what it asked for is a schema
	// this code cannot run against.
	conn := &recordingConnector{columns: []string{"count"}, rows: [][]driver.Value{{int64(1)}}}
	db := sqlx.NewDb(sql.OpenDB(conn), "postgres")
	defer db.Close()

	c := NewChat(db)
	if err := c.SetHidden(context.Background(), "chat-1", "user-1", true); !errors.Is(err, models.ErrorChatArchiveUnavailable) {
		t.Errorf("SetHidden err = %v, want ErrorChatArchiveUnavailable", err)
	}
	if err := c.SetCleared(context.Background(), "chat-1", "user-1"); !errors.Is(err, models.ErrorChatArchiveUnavailable) {
		t.Errorf("SetCleared err = %v, want ErrorChatArchiveUnavailable", err)
	}

	// information_schema only shows columns the connecting role holds a privilege on, so
	// a role reading the table through its owner's grants would be told the column does
	// not exist and archive would stay silently off forever.
	if !strings.Contains(conn.query, "pg_attribute") {
		t.Errorf("probe does not read pg_catalog:\n%s", conn.query)
	}
	if strings.Contains(conn.query, "information_schema") {
		t.Errorf("probe reads information_schema, which hides columns from an unprivileged role:\n%s", conn.query)
	}
}

func TestArchiveWritesWhenColumnsExist(t *testing.T) {
	conn := &recordingConnector{columns: []string{"count"}, rows: [][]driver.Value{{int64(2)}}}
	db := sqlx.NewDb(sql.OpenDB(conn), "postgres")
	defer db.Close()

	c := NewChat(db)
	if err := c.SetCleared(context.Background(), "chat-1", "user-1"); err != nil {
		t.Fatalf("SetCleared failed: %v", err)
	}

	// Delete is archive plus a cutoff, and both have to be written by the same statement:
	// now() is the transaction's start time, so one statement is what makes the two
	// timestamps identical rather than a microsecond apart.
	for _, want := range []string{"hidden_at=now()", "cleared_at=now()", "unread_count=0"} {
		if !strings.Contains(conn.query, want) {
			t.Errorf("clear statement missing %q:\n%s", want, conn.query)
		}
	}
	// One user's row only — the other participant keeps everything.
	if !strings.Contains(conn.query, "chat_id=? AND sender_id=?") && !strings.Contains(conn.query, "chat_id=$1 AND sender_id=$2") {
		t.Errorf("clear statement is not scoped to one participant's row:\n%s", conn.query)
	}
}

// The chat list is two queries: every pinned room, unpaged, plus a page of the unpinned
// ones. Both are built from chatConditions, so what they ask for can be asserted without
// a database.
func TestPinnedChatQuery(t *testing.T) {
	conditions, values := chatConditions("app-1", "user-1", models.GetOption{}, true)
	query, args := pinnedChatQuery(chatColumns(true), conditions, values, "1757505600")

	if !strings.Contains(query, "CT.is_pinned=true") {
		t.Errorf("pinned query is not restricted to pinned rooms:\n%s", query)
	}
	if !strings.Contains(query, "TO_TIMESTAMP(?)>(SELECT COALESCE(MAX(C.updated_at)") {
		t.Errorf("pinned query does not test for the first page:\n%s", query)
	}
	// Unpaged: the whole pinned set is served, or the cursor stops describing the list.
	if strings.Contains(query, "LIMIT") {
		t.Errorf("pinned query is paged, it must serve the whole set:\n%s", query)
	}
	// R1 applies to pinned rooms too — pinning an archived room must not resurrect it.
	if !strings.Contains(query, "CT.hidden_at IS NULL") {
		t.Errorf("pinned query missing the archive condition:\n%s", query)
	}

	// Rebind is positional: the conditions appear twice, once on the outer query and
	// once inside the subquery that measures the cursor, with the cursor between them.
	wantArgs := []driver.Value{
		"app-1", "user-1", int64(models.Deleted), int64(models.Pass), int64(models.NeverGotMessages),
		"1757505600",
		"app-1", "user-1", int64(models.Deleted), int64(models.Pass), int64(models.NeverGotMessages),
	}
	assertQueryArgs(t, "pinned", args, wantArgs)
}

func TestPagedChatQuery(t *testing.T) {
	conditions, values := chatConditions("app-1", "user-1", models.GetOption{}, true)
	query, args := pagedChatQuery(chatColumns(true), conditions, values, "1757505600", 20)

	// Pinned rooms are held out of the paged query; sorting them in instead would make a
	// pinned-but-quiet room reappear at the top of every page.
	if !strings.Contains(query, "CT.is_pinned=false") {
		t.Errorf("paged query does not exclude pinned rooms:\n%s", query)
	}
	if !strings.Contains(query, "ORDER BY C.updated_at DESC LIMIT ?") {
		t.Errorf("paged query is not ordered and limited on the cursor column alone:\n%s", query)
	}
	if strings.Contains(query, "CT.is_pinned DESC") {
		t.Errorf("paged query still sorts pinned rooms into the pages:\n%s", query)
	}
	if !strings.Contains(query, "CT.hidden_at IS NULL") {
		t.Errorf("paged query missing the archive condition:\n%s", query)
	}

	wantArgs := []driver.Value{
		"app-1", "user-1", int64(models.Deleted), int64(models.Pass), int64(models.NeverGotMessages),
		"1757505600", int64(20),
	}
	assertQueryArgs(t, "paged", args, wantArgs)
}

// Both queries are built from the same slices, so a builder that appended in place would
// leave the second one carrying the first one's conditions.
func TestChatQueryBuildersDoNotShareState(t *testing.T) {
	conditions, values := chatConditions("app-1", "user-1", models.GetOption{}, true)
	before := strings.Join(conditions, " AND ")

	columns := chatColumns(true)
	pinnedQuery, pinnedArgs := pinnedChatQuery(columns, conditions, values, "1757505600")
	pagedQuery, pagedArgs := pagedChatQuery(columns, conditions, values, "1757505600", 20)

	if got := strings.Join(conditions, " AND "); got != before {
		t.Errorf("a builder mutated the shared conditions: %s", got)
	}
	if len(values) != 5 {
		t.Errorf("values = %v, want the five base arguments untouched", values)
	}
	if strings.Contains(pagedQuery, "CT.is_pinned=true") {
		t.Errorf("paged query inherited the pinned query's condition:\n%s", pagedQuery)
	}
	if strings.Contains(pinnedQuery, "CT.is_pinned=false") {
		t.Errorf("pinned query inherited the paged query's condition:\n%s", pinnedQuery)
	}
	if len(pinnedArgs) == len(pagedArgs) {
		t.Errorf("both queries take %d arguments; one is reusing the other's slice", len(pinnedArgs))
	}
}

func assertQueryArgs(t *testing.T, label string, got []interface{}, want []driver.Value) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s args = %v, want %v", label, got, want)
	}
	for i := range want {
		converted, err := driver.DefaultParameterConverter.ConvertValue(got[i])
		if err != nil {
			t.Fatalf("%s args[%d] = %v is not a valid driver value: %v", label, i, got[i], err)
		}
		if converted != want[i] {
			t.Errorf("%s args[%d] = %v, want %v", label, i, converted, want[i])
		}
	}
}

// R2: the delete cutoff belongs in the WHERE clause, not in a filter over the page —
// dropping rows after the query makes a full page come back short and the client stops
// paging. A caller who never deleted the room must not pay for a condition at all.
func TestGetMessagesAppliesClearedAt(t *testing.T) {
	columns := []string{"id", "type", "body", "chat_id", "sender_id", "created_at", "reply_to_message_id", "status", "media_ids", "reference_id"}

	t.Run("never deleted", func(t *testing.T) {
		conn := &recordingConnector{columns: columns}
		db := sqlx.NewDb(sql.OpenDB(conn), "postgres")
		defer db.Close()

		if _, err := NewChat(db).GetMessages(context.Background(), "chat-1", "1757505600", 20, nil); err != nil {
			t.Fatalf("GetMessages failed: %v", err)
		}
		if strings.Contains(conn.query, "created_at>") {
			t.Errorf("query added a cutoff for a caller who never deleted the room:\n%s", conn.query)
		}
		if len(conn.args) != 3 {
			t.Fatalf("args = %v, want chat_id, next, limit", conn.args)
		}
	})

	t.Run("deleted", func(t *testing.T) {
		conn := &recordingConnector{columns: columns}
		db := sqlx.NewDb(sql.OpenDB(conn), "postgres")
		defer db.Close()

		clearedAt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
		if _, err := NewChat(db).GetMessages(context.Background(), "chat-1", "1757505600", 20, &clearedAt); err != nil {
			t.Fatalf("GetMessages failed: %v", err)
		}
		if !strings.Contains(conn.query, "created_at>") {
			t.Errorf("query missing the delete cutoff:\n%s", conn.query)
		}
		// Rebind is positional: the cutoff has to sit between the paging cursor and the
		// limit, or the query filters on the wrong value.
		if len(conn.args) != 4 {
			t.Fatalf("args = %v, want chat_id, next, cleared_at, limit", conn.args)
		}
		if conn.args[2] != clearedAt {
			t.Errorf("args[2] = %v, want %v", conn.args[2], clearedAt)
		}
	})
}

// 這層沒有 DB 測試基礎建設，與其為了一支查詢引進 mock 套件，不如記下真正送出去的
// SQL 與參數。
type recordingConnector struct {
	columns []string
	rows    [][]driver.Value

	query string
	args  []driver.Value
}

func (c *recordingConnector) Connect(context.Context) (driver.Conn, error) {
	return &recordingConn{c: c}, nil
}

func (c *recordingConnector) Driver() driver.Driver { return nil }

type recordingConn struct {
	c *recordingConnector
}

func (c *recordingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.c.query = query
	c.c.args = nil
	for _, arg := range args {
		c.c.args = append(c.c.args, arg.Value)
	}
	return &recordingRows{columns: c.c.columns, values: c.c.rows}, nil
}

func (c *recordingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.c.query = query
	c.c.args = nil
	for _, arg := range args {
		c.c.args = append(c.c.args, arg.Value)
	}
	return driver.RowsAffected(1), nil
}

func (c *recordingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (c *recordingConn) Close() error { return nil }

func (c *recordingConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type recordingRows struct {
	columns []string
	values  [][]driver.Value
	next    int
}

func (r *recordingRows) Columns() []string { return r.columns }

func (r *recordingRows) Close() error { return nil }

func (r *recordingRows) Next(dest []driver.Value) error {
	if r.next >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.next])
	r.next++
	return nil
}
