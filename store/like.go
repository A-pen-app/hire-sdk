package store

import "strings"

// likeEscaper 讓 % 和 _ 以字面比對，否則使用者輸入的那些字元會被當成萬用字元。
// Postgres 的 LIKE/ILIKE 預設就以反斜線跳脫，不需要 ESCAPE 子句。
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// containsPattern 把關鍵字包成前後萬用字元的 ILIKE 樣板。
func containsPattern(keyword string) string {
	return "%" + likeEscaper.Replace(keyword) + "%"
}
