# Chat Room Visibility — Archive and Delete (hire-sdk)

> Product spec: [product-handbook — Chat room](https://handbook.penpeer.co/docs/features/social/chatroom)
> Hiring context: [product-handbook — Hiring chat room](https://handbook.penpeer.co/docs/features/hiring/chatroom)
> Acceptance scenarios: [product-handbook — Chatroom Archive & Delete test cases](https://handbook.penpeer.co/engineering/social/chatroom-system)
>
> The "Visibility Rules" and "Scenario Matrix" sections below are kept
> **verbatim-identical** across `megaphone/docs/chat_visibility.md` and
> `{apen,nurse,phar}-api/docs/chat_visibility.md` — they are each other's consistency
> baseline. When you change one, change the other four.

## Why there are three implementations

There are three independent chat backends. They share no schema and no code:

| Implementation | Location | Database |
|---|---|---|
| Megaphone chat | `megaphone/` | megaphone's own Postgres |
| Hiring chat | `hire-sdk/` (this repo, a library, imported by all three API repos) | the `hiring` DB |
| Direct messages | `{apen,nurse,phar}-api/` | each app's main DB |

megaphone and hire-sdk share a common ancestor — the `ChatAnnotation`,
`ChatControlFlag` and `MessageStatus` enums are identical, because this repo was forked
from megaphone and then grew `app_id` / `post_id` / `is_pinned` / `hire_contact` /
`access_status`. Direct messages are a separate, older design with different column
names and types.

**All three must behave identically from the outside; how they get there depends on
each schema.**

---

## Visibility Rules

Each `chat_thread` row represents one user's view of one chat room. Both new concepts
live on that row, which is what makes archive and delete one-sided: writing one
participant's row never affects the other's.

### Two concepts

| Concept | Meaning |
|---|---|
| `hidden_at` | Archive timestamp. Once set, the room disappears from my list. Messages are untouched. |
| `cleared_at` | Delete cutoff. Once set, I only ever see messages with `created_at > cleared_at`. |

- **Archive** = set `hidden_at`
- **Delete** = set both `hidden_at` and `cleared_at`, to the same value

### Four rules

**R1 — List visibility**

A room appears in my list if and only if:

```
hidden_at IS NULL  OR  room's last message time > hidden_at
```

"Room's last message time" is `chat.updated_at` in megaphone and hire-sdk, and
`chat_thread.updated_at` in the three API repos — see "How a new message un-archives a
room" below for why the mechanism differs.

**R2 — Message visibility**

A message is visible to me if and only if both hold:

```
cleared_at IS NULL  OR  message.created_at > cleared_at
```
and the message is not hidden from me by one of the pre-existing per-message
mechanisms (unsend, per-message delete).

**Both comparisons are strictly greater.** A timestamp exactly equal to the cutoff
leaves the message hidden and the room archived (test cases CHAT-305 / CHAT-306).

**R3 — Unread count**

Setting `hidden_at` or `cleared_at` always zeroes that row's unread count and
recalculates the user-level total. Archive and delete both count as reading the room.

**R4 — A per-message delete renders as "unsent", it does not remove the row**

A message the viewer deleted stays in the list and is marked unsent for them. A later
message that quotes it still renders, with its quoted preview marked unavailable.

**Dropping the row is not an option, and this is not a style preference.** The mobile
clients page by asking "did I get `count` messages back?" to decide whether older
history exists. A dropped row makes a full page come back short, so the client
concludes there is nothing older and the user silently loses access to everything
before the first deleted message. apen-api hit exactly this and moved from dropping to
marking (apen-api#180); the remaining implementations that still drop carry the same
bug.

Product direction, agreed separately: per-message **delete is being retired in favour
of unsend**. Clients are removing the entry point, and the server renders the two
identically in the meantime, which is why R4 reads the way it does. Do not "fix" a
deleted message back into a dropped row.

Note this constraint applies to filtering done *after* the query. The room-level
`cleared_at` cutoff in R2 is applied in SQL, so a page of `count` rows is always
`count` rows the caller can see — it cannot cause the same problem.

### These rules describe what a participant sees

Every rule above is scoped to a `chat_thread` row, so reading one presupposes the
caller has such a row. **A caller who does not is not a participant and gets 404 —
before any of R1–R4 is considered.** This is not implied by the rules; it is the
precondition for them making sense.

Worth stating because the two are easy to conflate in code: the same lookup that
fetches the caller's `hidden_at` / `cleared_at` is also the membership check. Using
its result only for the cutoff, and querying messages by `chat_id` alone, hands any
signed-in user any room whose id they can guess — the message queries filter by room,
not by participant.

### How a new message un-archives a room

R1 deliberately compares timestamps instead of using a boolean flag, so a new message
restores the room with **no extra write on the send path**. That only works if the
"last message time" column is written by nothing but the send path.

| Implementation | Compared against | Written only by the send path | Mechanism used |
|---|---|---|---|
| megaphone | `chat.updated_at` | Yes — only `AddMessage` / `AddMessages` | Timestamp comparison, no extra write |
| hire-sdk | `chat.updated_at` | Yes — same | Timestamp comparison, no extra write |
| apen / nurse / phar | `chat_thread.updated_at` | No — `DeleteMessage` writes it too | Explicit reset of `hidden_at` to 0 in `NewMessageArrived` |

Both mechanisms are externally identical. Unit tests assert the semantics (R1–R4), not
the mechanism.

### The cutoff is never reset

Once set, `cleared_at` is never cleared by a new message. Deleting again only moves the
cutoff forward.

---

## Scenario Matrix

13 core scenarios. All three implementations must produce the same result. The IDs
refer to the [test case page](https://handbook.penpeer.co/engineering/social/chatroom-system).

| # | Scenario | Expected | Test case |
|---|---|---|---|
| 1 | A and B have 10 messages, A archives | Gone from A's list; A still sees 10 messages inside the room | CHAT-101 / 102 |
| 2 | Then B sends the 11th | Back in A's list; A and B both see 11 | CHAT-104 |
| 3 | A and B have 10 messages, A deletes | Gone from A's list; A sees 0; B sees 10 | CHAT-201 / 202 |
| 4 | Then B sends the 11th | A sees 1; B sees 11 | CHAT-203 |
| 5 | After 3, A sends a message | A sees 1 (their own); B sees 11 | CHAT-204 |
| 6 | A deletes with 3 unread | A's room unread and global badge both drop to zero; B unaffected | CHAT-206 |
| 7 | An archived room | Never appears in any list query, including paging and unread-only | CHAT-107 |
| 8 | Two more messages after a delete | Cutoff not reset; A sees only those two | CHAT-205 |
| 9 | Per-message delete stacked on the room cutoff | Both apply; visible = after cutoff, minus per-message deletes | CHAT-303 |
| 9b | How a per-message delete renders | Row kept and marked unsent; a reply's quote is marked unavailable. Dropping it breaks client paging | CHAT-311 / 312 |
| 10 | Message timestamp equals `hidden_at` / `cleared_at` | Not visible / not restored (strictly greater) | CHAT-305 / 306 |
| 11 | A archives with 3 unread | A's room unread and global badge both drop to zero; B unaffected | CHAT-106 |
| 12 | megaphone's legacy `status=Deleted` | A new message still does not bring the room back (legacy behaviour preserved) | CHAT-401 |

Scenario 12 applies to megaphone only; the other 12 must pass in all three
implementations.

---

## Implementation notes — hire-sdk

### This is a library, not a service

`hire-sdk` has no `main` and no HTTP layer. It is imported by `apen-api`, `nurse-api`
and `phar-api` (all three currently pin `v1.3.18`); the routes live in each API repo's
`api/hire.go`.

**Release flow for any change here:**

1. Change `hire-sdk` → run the tests → cut a new tag
2. Each API repo runs `go get github.com/A-pen-app/hire-sdk@<new tag>` and bumps `go.mod`
3. **All three must be upgraded together** — otherwise old code keeps running old logic
   against the same `hiring` DB

### The `hiring` DB is shared by all three apps

The `hiring` DB separates the three apps by `chat.app_id`; there is only one physical
database. So:

- DDL is applied once and takes effect for all three apps
- `hire-sdk` tracked no DDL of its own (this repo did not even contain a `.sql` file).
  New migrations will live in `hire-sdk/migrations/` — the directory and its
  `README.md` index are created by the implementation PR, not this one. They are
  deliberately *not* filed under the API repos' `migrations/`
  directories: those cover each app's own main database, and a row there would point
  at a file that lives in a different repo
- **There is no migration runner.** A DBA applies the DDL by hand before the code that
  depends on it is deployed, so the DDL must be idempotent

### Schema

`chat_thread` in the `hiring` DB. No DDL is tracked anywhere, so these columns are
reconstructed from the SQL in `store/chat.go`:

| Column | Meaning |
|---|---|
| `chat_id` | The room |
| `sender_id` | The owner of this row (= me) |
| `receiver_id` | The other party |
| `unread_count` | Unread count |
| `last_seen_at` | When the other party last saw my messages |
| `status` | `models.ChatAnnotation` |
| `control_flag` | `models.ChatControlFlag` bitmask (smallint) |
| `is_pinned` | Pinned |
| `hire_contact` | jsonb, this room's own contact details |

Two new columns:

```sql
ALTER TABLE public.chat_thread
    ADD COLUMN IF NOT EXISTS hidden_at  timestamptz,
    ADD COLUMN IF NOT EXISTS cleared_at timestamptz;
```

### Why not reuse `status`

`chat_thread.status` (`models.ChatAnnotation`) already has `None` / `Todo` / `Done` /
`Deleted`, and `GetChats` (`store/chat.go:139`) already filters `CT.status != Deleted`.
It is still not usable as an archive flag:

1. **It is a single-valued enum**, so "archived" would be mutually exclusive with
   "todo"/"done".
2. **It cannot express "restore on the next message"**: `AddMessage`
   (`store/chat.go:411`) only clears the `NeverGotMessages` bit of `control_flag`, it
   never touches `status`.
3. **In hire-sdk it is unreachable anyway**: `Annotate` (`store/chat.go:109`) is not on
   the `service.Chat` interface, has zero callers anywhere in the monorepo, and has no
   route; `models.ByStatus` rejects `Deleted` outright with
   `errors.New("action not allowed")`.

So archive and delete get their own columns. The existing `status != Deleted` condition
stays as it is.

### What needs to change

| Layer | File : location | Change |
|---|---|---|
| models | `models/chat.go:216` `ChatRoom` | Add `HiddenAt` / `ClearedAt` with `db:` tags; add the R1 / R2 pure functions |
| models | `models/chat.go:301` `GetOption` / `GetOptionFunc` | Add filter options if needed; note `ByStatus` currently errors on `Deleted` |
| store | `store/chat.go:139` `GetChats` | Add `(CT.hidden_at IS NULL OR C.updated_at > CT.hidden_at)` to `conditions`; **keep** the existing `CT.status != Deleted` |
| store | `store/chat.go:26` `Get` | Add the two columns to the SELECT list. Do **not** add a `hidden_at` filter: archiving only removes a room from the list, and opening it by direct link or from a notification has to keep working and keep showing every message (CHAT-102) |
| store | `store/chat.go:598` `GetMessages`, `:544` `GetNewMessages` | Take a `clearedAt` argument, add `AND created_at > ?` |
| store | `store/chat.go` (new) | `SetHidden` / `SetCleared`, shaped exactly like `Pin` (`store/chat.go:124`): `UPDATE chat_thread SET x=? WHERE chat_id=? AND sender_id=?`, and zeroing `unread_count` the way `Read` (`:65`) does |
| store | `store/store.go:27` `Chat` interface | Add `SetHidden` / `SetCleared`; add `clearedAt` to `GetMessages` / `GetNewMessages` |
| service | `service/service.go:18` `Chat` interface | Add `Archive` / `Unarchive` / `Clear` |
| service | `service/chat.go` | Implement them. **Go through the service rather than following `Pin`'s store-direct precedent**, so they inherit the usual `GetByBundleID` plus `s.c.Get` membership check |
| service | `service/chat.go:592` `aggregateLastMessage`, `:686` `aggregateMessages` | Apply the R2 cutoff; `aggregateLastMessage` must return `nil` when the last message falls before the cutoff. **Also the R4 behaviour change**: the deleted-bit branch in `aggregateMessages` (`:690-694`) currently `continue`s — dropping the row and triggering the paging bug R4 describes. Change it to keep the row and mark it unsent for the viewer (`apen-api/api/aggregator/chat.go:198` is the reference). The skip in `aggregateLastMessage`'s list preview is fine and stays |
| API repos | `{apen,nurse,phar}-api/api/hire.go` | Add `hg.PATCH("chats/:chat_id/archive", ...)` and `hg.DELETE("chats/:chat_id/messages", ...)` right after `chats/:chat_id/pin`; copy the handler shape from `apen-api/api/hire.go`'s `pinChat` |

### Known gaps (worth confirming before you start)

| # | Problem | Location |
|---|---|---|
| G1 | `Get` does not filter `status != Deleted`, so a room marked deleted is still readable and can still be sent to. Note this is only a gap for the legacy "hidden forever" meaning — under the archive rules a hidden room *must* stay openable by direct link and still show every message (CHAT-102), so `Get` is deliberately left unfiltered for `hidden_at` | `store/chat.go:26` |
| G2 | `Annotate` is not on the `service.Chat` interface, has zero callers monorepo-wide, and has no route | `store/chat.go:109`, `service/service.go:18` |
| G3 | `models.ByStatus` errors out on `Deleted`, so a client can never list deleted rooms | `models/chat.go:308` |
| G4 | `MessageStatus`'s `DeletedBySender` / `DeletedByReceiver` are **fully implemented on the read side** (`aggregateMessages` / `aggregateLastMessage`) but nothing in hire-sdk ever writes them | `service/chat.go:592,686` |

G4 is only half good news: the read side of the per-message delete exists, but it
rendered by dropping the row, which violates R4 (scenario 9b). The drop →
mark-unsent change listed in the table above landed in the alignment stage
(`fix/chat-alignment`); what is still missing is the writer.

### The existing per-message mechanism (leave it alone)

`message.status` is a bitmask (`models/chat.go:49`):

| Value | Meaning | Who stops seeing it |
|---|---|---|
| `Unsent` (1) | Unsent | Both sides lose the content |
| `DeletedBySender` (2) | Deleted by the sender | The sender only |
| `DeletedByReceiver` (4) | Deleted by the receiver | The receiver only |

This layer **stacks** with the room-level `cleared_at` (scenario 9): a message must
clear both to be visible.

"Leave it alone" means the bits and the write path — the message-history **rendering**
of a deleted bit does change: `aggregateMessages` must stop dropping the row and mark
it unsent instead (R4, see the table above).

---

## Testing

This repo has exactly one test file (`models/experience_test.go`), and `go.mod`
**deliberately carries no test dependencies**. New tests use only the standard library
`testing` package — no testify, no gomock.

| File | Contents |
|---|---|
| `models/chat_visibility_test.go` | Table-driven tests for the R1 / R2 pure functions, including the scenario 10 boundaries |
| `service/chat_fakes_test.go` | A hand-written in-memory `fakeChatStore` implementing `store.Chat` |
| `service/chat_test.go` | The scenario matrix minus the megaphone-only row, with unimplemented rows marked `t.Skip` |

`store.Chat` is already an interface (`store/store.go:27`), so the service layer can be
tested without touching a database. Note that `service.NewChat` needs six stores
(`store.Chat` / `Resume` / `App` / `Media` / `Subscription` / `BusinessCard`), so the
fakes file has to provide minimal stubs for the other five as well.

Reference template (it lives in an API repo): `apen-api/service/follow_fakes_test.go`
plus `apen-api/service/follow_test.go`.

```bash
go test ./models/... ./service/... -run 'Chat' -v
```
