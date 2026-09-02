package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Chat roles, matching what the model protocol calls them. Tool traffic is the
// server's own business and is never stored: it is neither what the user wrote
// nor what they were shown.
const (
	ChatRoleUser      = "user"
	ChatRoleAssistant = "assistant"
)

// Chat is one conversation with the assistant. It belongs to the user who held
// it: the assistant answers with that user's own permissions, so a conversation
// can contain anything they were allowed to read, and nobody else may see it.
type Chat struct {
	Base
	UserID string `gorm:"index;not null"`
	// Title is the opening of the first question, kept so that a list of chats
	// can be read without loading every message.
	Title string
	// EventID and EventKind record the event the chat was opened from, when it
	// was opened from one.
	EventID   string
	EventKind string
	// Mode is what the chat was opened to do: chat, explain, digest or support.
	Mode string

	Messages []*ChatMessage `gorm:"constraint:OnDelete:CASCADE"`
	// MessageCount is how long the conversation is. It is not a column: a
	// listing fills it in with one grouped count instead of loading the turns.
	MessageCount int `gorm:"-"`

	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// ChatMessage is one turn of a conversation, in the order it was written.
type ChatMessage struct {
	Base
	ChatID uuid.UUID `gorm:"index;not null"`
	// Position orders the messages within a chat. Timestamps are not enough:
	// a question and its answer can land in the same millisecond.
	Position int    `gorm:"not null"`
	Role     string `gorm:"not null"`
	Content  string
}

// TableName keeps the table out of the way of anything else called "chats".
func (Chat) TableName() string {
	return "assistant_chats"
}

func (ChatMessage) TableName() string {
	return "assistant_chat_messages"
}
