package handlers

import (
	"bytes"
	"context"

	"denova/internal/platform"

	hertzapp "github.com/cloudwego/hertz/pkg/app"
)

// Authenticated LAN clients can read contributions and change Project settings.
// Runtime activation remains host-only; extensions receive scoped connections.
func handlePlatformPluginActions(ctx context.Context, c *hertzapp.RequestContext, manager *platform.Manager, parts []string, decode func(any) bool, respond func(any, error)) bool {
	method := string(c.Method())
	if len(parts) == 1 && parts[0] == "plugin-pages" && method == "GET" {
		respond(manager.PluginPages())
		return true
	}
	if len(parts) == 5 && parts[0] == "projects" && parts[2] == "plugins" && parts[4] == "content" {
		if method == "GET" {
			var output bytes.Buffer
			if err := manager.ExportProjectPluginContent(parts[1], parts[3], &output); err != nil {
				platformManagementError(c, err)
			} else {
				c.Header("Content-Disposition", `attachment; filename="plugin-content.zip"`)
				c.Data(200, "application/zip", output.Bytes())
			}
			return true
		}
		if method == "POST" {
			respond(map[string]bool{"ok": true}, manager.ImportProjectPluginContent(parts[1], parts[3], c.Request.Body()))
			return true
		}
	}
	if len(parts) == 3 && parts[0] == "projects" {
		switch parts[2] {
		case "plugins":
			if method == "GET" {
				respond(manager.ProjectPlugins(parts[1], platform.ContributionContext(c.Query("context")), c.Query("locale")))
				return true
			}
		case "extensions":
			if method == "GET" {
				respond(manager.ProjectConfiguration(parts[1]))
				return true
			}
			if method == "PUT" {
				var input platform.ProjectConfigurationInput
				if decode(&input) {
					respond(manager.SaveProjectConfiguration(ctx, parts[1], input))
				}
				return true
			}
		}
	}
	if len(parts) == 2 && parts[0] == "plugin-actions" && parts[1] == "open" && method == "POST" {
		var input platform.OpenPluginAction
		if decode(&input) {
			input.ParentOrigin = platformParentOrigin(c)
			respond(manager.OpenPluginAction(ctx, input))
		}
		return true
	}
	if len(parts) == 4 && parts[0] == "runtimes" && parts[2] == "consumers" && method == "DELETE" {
		respond(map[string]bool{"ok": true}, manager.ReleaseConsumer(ctx, parts[1], parts[3]))
		return true
	}
	return false
}
