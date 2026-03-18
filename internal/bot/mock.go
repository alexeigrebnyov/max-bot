// Package bot: MockClient реализует BotClient для тестов в других пакетах (например api).

package bot

import "context"

// MockClient — заглушка BotClient, все методы возвращают nil.
type MockClient struct{}

func (MockClient) SendMessage(ctx context.Context, chat string, thread int, text string, private bool) error {
	return nil
}

func (MockClient) SendMessageWithKeyboard(ctx context.Context, chat string, text string, kb keyboard, private bool) error {
	return nil
}

func (MockClient) SendToChatByID(ctx context.Context, chatID int64, text string) error {
	return nil
}

func (MockClient) SendToChatByIDWithKeyboard(ctx context.Context, chatID int64, text string, kb keyboard) error {
	return nil
}
