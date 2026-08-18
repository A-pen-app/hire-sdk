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
	conn := &recordingConnector{rows: [][]driver.Value{{int64(3)}}}
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

// 這層沒有 DB 測試基礎建設，與其為了一支查詢引進 mock 套件，不如記下真正送出去的
// SQL 與參數。
type recordingConnector struct {
	rows [][]driver.Value

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
	return &recordingRows{values: c.c.rows}, nil
}

func (c *recordingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (c *recordingConn) Close() error { return nil }

func (c *recordingConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type recordingRows struct {
	values [][]driver.Value
	next   int
}

func (r *recordingRows) Columns() []string { return []string{"count"} }

func (r *recordingRows) Close() error { return nil }

func (r *recordingRows) Next(dest []driver.Value) error {
	if r.next >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.next])
	r.next++
	return nil
}
