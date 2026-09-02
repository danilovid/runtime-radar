package database

import (
	"context"

	"github.com/google/uuid"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChatRepository stores the assistant's conversations. Every method takes the
// user the chat belongs to, and every query filters by it: a conversation holds
// whatever its author was allowed to read, so it is theirs alone.
type ChatRepository interface {
	Add(context.Context, *model.Chat) error
	AppendMessages(ctx context.Context, userID string, chatID uuid.UUID, messages []*model.ChatMessage) error
	GetByID(ctx context.Context, userID string, chatID uuid.UUID) (*model.Chat, error)
	GetAll(ctx context.Context, userID string, limit int) ([]*model.Chat, error)
	DeleteByID(ctx context.Context, userID string, chatID uuid.UUID) error
}

type ChatDatabase struct {
	*gorm.DB
}

func (c *ChatDatabase) Add(ctx context.Context, chat *model.Chat) error {
	return c.WithContext(ctx).
		Create(chat).
		Error
}

// AppendMessages adds turns to a chat that must already belong to the user. The
// position each one takes is read inside the same transaction, so two answers
// racing to be written cannot land on the same place in the conversation.
func (c *ChatDatabase) AppendMessages(
	ctx context.Context, userID string, chatID uuid.UUID, messages []*model.ChatMessage,
) error {
	if len(messages) == 0 {
		return nil
	}

	return c.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var chat model.Chat
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(&model.Chat{UserID: userID}).
			Where("id = ?", chatID).
			Take(&chat).
			Error; err != nil {
			return err
		}

		var next int
		if err := tx.Model(&model.ChatMessage{}).
			Where("chat_id = ?", chatID).
			Select("COALESCE(MAX(position), -1) + 1").
			Scan(&next).
			Error; err != nil {
			return err
		}

		for i, message := range messages {
			message.ChatID = chatID
			message.Position = next + i
		}

		return tx.Create(&messages).Error
	})
}

func (c *ChatDatabase) GetByID(ctx context.Context, userID string, chatID uuid.UUID) (*model.Chat, error) {
	var chat model.Chat

	err := c.WithContext(ctx).
		Preload("Messages", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		Where(&model.Chat{UserID: userID}).
		Where("id = ?", chatID).
		Take(&chat).
		Error
	if err != nil {
		return nil, err
	}

	return &chat, nil
}

// GetAll lists the user's chats, newest first and without their messages: the
// list only shows a title and a count.
func (c *ChatDatabase) GetAll(ctx context.Context, userID string, limit int) ([]*model.Chat, error) {
	var chats []*model.Chat

	err := c.WithContext(ctx).
		Where(&model.Chat{UserID: userID}).
		Order("updated_at DESC").
		Limit(limit).
		Find(&chats).
		Error
	if err != nil {
		return nil, err
	}

	if err := c.countMessages(ctx, chats); err != nil {
		return nil, err
	}

	return chats, nil
}

// countMessages fills in how long each conversation is with one grouped query,
// rather than a count per chat or a load of every message.
func (c *ChatDatabase) countMessages(ctx context.Context, chats []*model.Chat) error {
	if len(chats) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(chats))
	for _, chat := range chats {
		ids = append(ids, chat.ID)
	}

	var counts []struct {
		ChatID uuid.UUID
		Total  int
	}

	err := c.WithContext(ctx).
		Model(&model.ChatMessage{}).
		Select("chat_id, COUNT(*) AS total").
		Where("chat_id IN ?", ids).
		Group("chat_id").
		Scan(&counts).
		Error
	if err != nil {
		return err
	}

	byID := make(map[uuid.UUID]int, len(counts))
	for _, count := range counts {
		byID[count.ChatID] = count.Total
	}

	for _, chat := range chats {
		chat.MessageCount = byID[chat.ID]
	}

	return nil
}

func (c *ChatDatabase) DeleteByID(ctx context.Context, userID string, chatID uuid.UUID) error {
	return c.WithContext(ctx).
		Where(&model.Chat{UserID: userID}).
		Where("id = ?", chatID).
		Delete(&model.Chat{}).
		Error
}
