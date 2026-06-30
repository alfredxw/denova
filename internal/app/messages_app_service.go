package app

import "nova/internal/messages"

func (a *App) Messages() (messages.ListResult, error) {
	return messages.NewService(a.novaDir()).List()
}

func (a *App) MarkMessageRead(id string) (messages.Message, error) {
	return messages.NewService(a.novaDir()).MarkRead(id)
}

func (a *App) MarkAllMessagesRead() (messages.ListResult, error) {
	return messages.NewService(a.novaDir()).MarkAllRead()
}

func (a *App) novaDir() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg == nil {
		return ""
	}
	return a.cfg.NovaDir
}
