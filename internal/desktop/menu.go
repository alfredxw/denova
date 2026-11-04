package desktop

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

func (s *shell) menus() {
	action := func(key string, fn func() error) *mygo.MenuItem {
		return &mygo.MenuItem{ID: key, Label: s.t(key), Click: func(*mygo.MenuItem, *mygo.Window) {
			slog.Info("Desktop menu action", "action", key)
			s.async(key, fn)
		}}
	}
	items := []*mygo.MenuItem{{Role: mygo.RoleAbout, Label: s.t("about")}, updater.MenuItem(), action("open", func() error { s.focus(); return nil }), action("data", func() error { return s.changeData(context.Background()) }), action("logs", func() error { return mygo.Shell.OpenPath(filepath.Join(s.userData, "log")) }), {Type: mygo.MenuItemSeparator}}
	if runtime.GOOS == "darwin" {
		items = append(items, &mygo.MenuItem{Role: mygo.RoleHide, Label: s.t("hide")})
	}
	quit := action("quit", s.quit)
	quit.Accelerator = "CmdOrCtrl+Q"
	items = append(items, quit)
	roles := func(names []string) []*mygo.MenuItem {
		var result []*mygo.MenuItem
		for _, name := range names {
			key := name
			if name == "togglefullscreen" {
				key = "fullscreen"
			}
			result = append(result, &mygo.MenuItem{Role: mygo.MenuRole(name), Label: s.t(key)})
		}
		return result
	}
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{ID: "application", Label: "Denova", Submenu: items}, {ID: "edit", Label: s.t("edit"), Submenu: roles([]string{"undo", "redo", "cut", "copy", "paste", "selectAll"})}, {ID: "view", Label: s.t("view"), Submenu: roles([]string{"reload", "resetZoom", "zoomIn", "zoomOut", "togglefullscreen", "minimize"})}}))
	if s.tray != nil {
		var tray []*mygo.MenuItem
		for _, item := range items {
			if item.Role == "" {
				tray = append(tray, item)
			}
		}
		s.tray.SetMenu(mygo.NewMenu(tray))
	}
}

// OpenMenu exposes only the three existing application menus to the owning
// page. Coordinates are page pixels; MyGo owns popup placement and commands.
func (b *Bridge) OpenMenu(ctx context.Context, id string, x, y int) error {
	if err := b.authorize(ctx); err != nil {
		return err
	}
	if id != "application" && id != "edit" && id != "view" {
		return errors.New("unknown desktop menu")
	}
	mygo.RunOnMain(func() {
		if menu := mygo.App.Menu(); menu != nil {
			if item := menu.ItemByID(id); item != nil {
				slog.Info("Desktop menu opened", "menu", id)
				w := b.shell.window.Load()
				zoom := w.Page().ZoomFactor()
				mygo.NewMenu(item.Submenu).PopupAt(w, int(float64(x)*zoom), int(float64(y)*zoom))
			}
		}
	})
	return nil
}
