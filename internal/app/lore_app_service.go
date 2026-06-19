package app

import "nova/internal/book"

// LoreAppService 负责资料库 CRUD。
type LoreAppService struct {
	app *App
}

func (a *App) LoreItems() ([]book.LoreItem, error) {
	return a.lore().LoreItems()
}

func (s *LoreAppService) LoreItems() ([]book.LoreItem, error) {
	state := s.bookState()
	if state == nil {
		return nil, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).ListAll()
}

func (a *App) CreateLoreItem(input book.LoreItemInput) (book.LoreItem, error) {
	return a.lore().CreateLoreItem(input)
}

func (s *LoreAppService) CreateLoreItem(input book.LoreItemInput) (book.LoreItem, error) {
	state := s.bookState()
	if state == nil {
		return book.LoreItem{}, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).Create(input)
}

func (a *App) UpdateLoreItem(id string, input book.LoreItemInput) (book.LoreItem, error) {
	return a.lore().UpdateLoreItem(id, input)
}

func (s *LoreAppService) UpdateLoreItem(id string, input book.LoreItemInput) (book.LoreItem, error) {
	state := s.bookState()
	if state == nil {
		return book.LoreItem{}, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).Update(id, input)
}

func (a *App) DeleteLoreItem(id string) error {
	return a.lore().DeleteLoreItem(id)
}

func (s *LoreAppService) DeleteLoreItem(id string) error {
	state := s.bookState()
	if state == nil {
		return ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).Delete(id)
}

func (s *LoreAppService) bookState() *book.State {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.bookState
}
