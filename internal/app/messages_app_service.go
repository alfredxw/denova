package app

import "denova/internal/messages"

func (a *App) Messages(locale string) (messages.ListResult, error) {
	return messages.NewService(a.novaDir()).ListForLocale(locale)
}

func (a *App) MarkMessageRead(id, locale string) (messages.Message, error) {
	return messages.NewService(a.novaDir()).MarkReadForLocale(id, locale)
}

func (a *App) MarkAllMessagesRead(locale string) (messages.ListResult, error) {
	return messages.NewService(a.novaDir()).MarkAllReadForLocale(locale)
}

func (a *App) novaDir() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg == nil {
		return ""
	}
	return a.cfg.NovaDir
}
