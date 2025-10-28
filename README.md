# hire-sdk

A Go SDK for managing hiring-related functionalities including resumes, chat communications, user agreements, and subscriptions.

## Installation

```bash
go get github.com/A-pen-app/hire-sdk
```

## Features

- **Resume Management**: Create, update, and retrieve user resumes with support for doctors, pharmacists, and nurses
- **Chat System**: Real-time messaging between recruiters and job seekers with support for text, images, files, forms, and meetups
- **User Agreements**: Manage user consent and agreement versions
- **Subscription Management**: Handle user subscription status and expiration

## Core Modules

### Resume Service

Manage user resumes with profession-specific fields:

```go
type Resume interface {
    // Update resume content
    Patch(ctx context.Context, bundleID, userID string, resume *models.ResumeContent) error

    // Get user's resume
    Get(ctx context.Context, bundleID, userID string) (*models.Resume, error)

    // Get all post IDs that user has applied to
    GetUserAppliedPostIDs(ctx context.Context, bundleID, userID string) ([]string, error)

    // Get resume snapshot by ID
    GetSnapshot(ctx context.Context, snapshotID string) (*models.ResumeSnapshot, error)

    // Get employer response time medians by post
    GetResponseMediansByPost(ctx context.Context, bundleID string, after time.Time) (map[string]float64, error)
}
```

**Resume Content** supports multiple professions:
- Common fields: name, email, phone, preferred locations, expected salary, collaboration types
- Doctor-specific: position, departments, specialty, expertise, alma mater
- Pharmacist-specific: current organization, job title, alma mater, graduation year
- Nurse-specific: birth year, certificate, hospital experience

### Chat Service

Comprehensive chat system for recruiter-job seeker communication:

```go
type Chat interface {
    // Create new chat room
    New(ctx context.Context, bundleID, senderID, receiverID string, postID *string,
        resume *models.ResumeContent, resumeStatus models.ResumeStatus) (string, error)

    // Get chat room details
    Get(ctx context.Context, bundleID, chatID, userID string) (*models.ChatRoom, error)

    // List user's chat rooms with pagination
    GetChats(ctx context.Context, bundleID, userID string, next string, count int,
        options ...models.GetOptionFunc) ([]*models.ChatRoom, string, error)

    // Get messages in a chat with pagination
    GetChatMessages(ctx context.Context, bundleID, userID, chatID string, next string, count int)
        ([]*models.Message, string, error)

    // Fetch new messages after a specific message
    FetchNewMessages(ctx context.Context, bundleID, userID, chatID string, lastMessageID string)
        ([]*models.Message, error)

    // Send message with various types
    SendMessage(ctx context.Context, bundleID, userID, chatID string,
        options ...models.SendOptionFunc) (*models.Message, error)

    // Unsend a message
    UnsendMessage(ctx context.Context, bundleID, userID, messageID string) error
}
```

**Supported Message Types**:
- Text messages
- Images
- Files
- Forms (surveys/questionnaires)
- Meetups (scheduled events)
- Post references

**Chat Filtering Options**:
```go
// Filter by status (TODO, DONE, NONE)
models.ByStatus(models.Todo, false)

// Filter official role chats
models.IsOfficialRole()
```

**Sending Messages**:
```go
// Text message
models.WithText("Hello!")

// Image message
models.WithMedia([]string{"media-id-1", "media-id-2"})

// File message
models.WithFile([]string{"file-id-1"})

// Reply to message
models.ReplyTo("message-id")
```

### Agreement Service

Manage user agreements and EULA versions:

```go
type Agreement interface {
    // Record user agreement to a specific version
    Agree(ctx context.Context, bundleID, userID, version string) error

    // Get user's agreement record
    Get(ctx context.Context, bundleID, userID string) (*models.AgreementRecord, error)
}
```

### Subscription Service

Handle user subscription status:

```go
type Subscription interface {
    // Get user's subscription status
    Get(ctx context.Context, bundleID, userID string) (*models.UserSubscription, error)

    // Update subscription status and expiration
    Update(ctx context.Context, bundleID, userID string, status models.SubscriptionStatus,
        expiresAt *time.Time) error
}
```

**Subscription Status Types**:
- `SubscriptionSubscribed`: Active subscription
- `SubOptionFree`: Has free voucher
- `SubscriptionNone`: Previously subscribed but expired
- `SubscriptionNever`: Never subscribed

## Models

### Resume Types

**Collaboration Types**:
- Full-time (全職)
- Part-time (兼職)
- Attending (掛牌)
- Lecturer (講座講師)
- Prescription (業配)
- Endorsement (代言)
- Telemedicine (遠距醫療)
- Market Research (市調訪談)
- Academic Editing (學術編輯)
- Product Experience (產品體驗)

**Hospital Experience Years**:
- Less than 1 year
- 1-10 years (in yearly increments)
- More than 10 years

### Chat Types

**Message Status**:
- `Normal`: Active message
- `Unsent`: Message not sent
- `Deleted`: Message deleted by sender/receiver
- `Unavailable`: Message unavailable

**Chat Annotations**:
- `None`: Regular chat
- `Todo`: Marked as todo
- `Done`: Marked as done
- `Deleted`: Deleted chat

**User Roles**:
- `RoleOfficial`: Official account
- `RoleRecruiter`: Hiring party
- `RoleJobSeeker`: Job seeker

## Dependencies

- [feed-sdk](https://github.com/A-pen-app/feed-sdk): Feed management SDK
- [logging](https://github.com/A-pen-app/logging): Structured logging utilities
- [sqlx](https://github.com/jmoiron/sqlx): SQL extensions
- PostgreSQL driver

## License

See LICENSE file for details.
