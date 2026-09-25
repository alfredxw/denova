package handlers

import (
	"bytes"
	"context"
	"denova/internal/app/resourceexchange"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"denova/internal/platform"
	hertzapp "github.com/cloudwego/hertz/pkg/app"
)

// HandlePlatformManagement is reachable only from the trusted App origin.
// Third-party views and backends use their own scoped HTTP runtime listener.
func (h *Handlers) HandlePlatformManagement(ctx context.Context, c *hertzapp.RequestContext) {
	manager := h.app.Platform()
	if manager == nil {
		platformManagementError(c, fmt.Errorf("platform manager is unavailable"))
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(string(c.Path()), "/api/platform/manage"), "/"), "/")
	method := string(c.Method())
	decode := func(target any) bool {
		if len(c.Request.Body()) > platform.MaxDefinitionBytes {
			platformManagementError(c, &platform.Error{Code: "LIMIT_EXCEEDED", MessageKey: "platform.errors.LIMIT_EXCEEDED", Diagnostic: "Management request is too large"})
			return false
		}
		if err := decodeStrictJSONRequest(c.Request.Body(), target); err != nil {
			platformManagementError(c, &platform.Error{Code: "INVALID_ARGUMENT", MessageKey: "platform.errors.INVALID_ARGUMENT", Diagnostic: err.Error()})
			return false
		}
		return true
	}
	respond := func(result any, err error) {
		if err != nil {
			platformManagementError(c, err)
		} else {
			c.JSON(200, result)
		}
	}
	if len(parts) == 1 && parts[0] == "catalog" && method == "GET" {
		respond(h.app.ResourceExchange().ExtensionCatalog(ctx))
		return
	}
	if len(parts) == 4 && parts[0] == "packages" && parts[1] == "game" && parts[3] == "cover" && method == "GET" {
		data, contentType, err := manager.GameCover(parts[2], c.Query("releaseId"))
		if err != nil {
			platformManagementError(c, err)
			return
		}
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cross-Origin-Resource-Policy", "same-origin")
		c.Header("Cache-Control", "private, max-age=3600")
		c.Data(200, contentType, data)
		return
	}
	if len(parts) == 1 && parts[0] == "game-preferences" {
		if method == "GET" {
			respond(manager.GamePreferences())
			return
		}
		if method == "PATCH" {
			var input platform.GamePreferences
			if !decode(&input) {
				return
			}
			respond(map[string]bool{"ok": true}, manager.SetDefaultGame(input.DefaultGameID))
			return
		}
	}
	if len(parts) == 3 && parts[0] == "packages" && parts[1] == "github" && method == "POST" {
		var source platform.GitHubSource
		if !decode(&source) {
			return
		}
		switch parts[2] {
		case "preview":
			respond(manager.PreviewGitHub(ctx, source))
		case "import":
			respond(manager.ImportGitHub(ctx, source))
		default:
			c.Status(404)
		}
		return
	}
	if len(parts) == 4 && parts[0] == "packages" && parts[3] == "update" {
		ref := platform.PackageRef{Kind: platform.Kind(parts[1]), ID: parts[2]}
		if method == "GET" {
			respond(h.app.ResourceExchange().CheckExtensionUpdate(ctx, ref))
			return
		}
		if method == "POST" {
			var input struct {
				Commit string `json:"commit"`
			}
			if decode(&input) {
				respond(h.app.ResourceExchange().PreviewExtensionUpdate(ctx, ref, input.Commit))
			}
			return
		}
	}
	if len(parts) == 2 && parts[0] == "packages" && parts[1] == "preview" && method == "POST" {
		if strings.HasPrefix(string(c.GetHeader("Content-Type")), "application/zip") {
			respond(manager.PreviewExtensionZIP(c.Request.Body()))
			return
		}
		var input struct {
			Directory string `json:"directory"`
		}
		if !decode(&input) {
			return
		}
		respond(manager.PreviewExtensionDirectory(input.Directory))
		return
	}
	if len(parts) == 2 && parts[0] == "packages" && method == "GET" {
		respond(manager.List(platform.Kind(parts[1])))
		return
	}
	if method == "GET" && (len(parts) == 3 && parts[0] == "candidates" && parts[2] == "setup" || len(parts) == 4 && parts[0] == "packages" && parts[1] == "game" && parts[3] == "setup") {
		locale := c.Query("locale")
		if locale != "zh-CN" {
			locale = "en-US"
		}
		if parts[0] == "candidates" {
			respond(manager.CandidateSetup(parts[1], locale))
		} else {
			respond(manager.SetupConfiguration(parts[2], c.Query("releaseId"), locale))
		}
		return
	}
	if len(parts) == 3 && parts[0] == "packages" && parts[2] == "preview" && method == "POST" {
		kind := platform.Kind(parts[1])
		if strings.HasPrefix(string(c.GetHeader("Content-Type")), "application/zip") {
			respond(manager.PreviewZIP(kind, c.Request.Body()))
			return
		}
		var input struct {
			Directory string `json:"directory"`
		}
		if !decode(&input) {
			return
		}
		respond(manager.PreviewDirectory(kind, input.Directory))
		return
	}
	if len(parts) == 2 && parts[0] == "packages" && parts[1] == "install" && method == "POST" {
		var input struct {
			CandidateID string   `json:"candidateId"`
			Grants      []string `json:"grants"`
		}
		if !decode(&input) {
			return
		}
		respond(h.app.InstallExtensionCandidate(ctx, input.CandidateID, input.Grants))
		return
	}
	if len(parts) == 3 && parts[0] == "candidates" && parts[2] == "archive" && method == "GET" {
		var output bytes.Buffer
		if err := manager.ExportCandidate(parts[1], &output); err != nil {
			platformManagementError(c, err)
			return
		}
		c.Header("Content-Disposition", `attachment; filename="denova-package.zip"`)
		c.Data(200, "application/zip", output.Bytes())
		return
	}
	if len(parts) == 4 && parts[0] == "packages" && parts[3] == "archive" && method == "GET" {
		ref := platform.ReleaseRef{Package: platform.PackageRef{Kind: platform.Kind(parts[1]), ID: parts[2]}, ReleaseID: c.Query("releaseId")}
		var output bytes.Buffer
		if err := manager.ExportInstalled(ref, &output); err != nil {
			platformManagementError(c, err)
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, ref.Package.ID))
		c.Header("Cache-Control", "no-store")
		c.Data(200, "application/zip", output.Bytes())
		return
	}
	if len(parts) == 2 && parts[0] == "candidates" && method == "DELETE" {
		manager.DiscardCandidate(parts[1])
		c.Status(204)
		return
	}
	if len(parts) == 3 && parts[0] == "candidates" && parts[2] == "prepare-preview" && method == "POST" {
		var input struct {
			Grants []string `json:"grants"`
		}
		if !decode(&input) {
			return
		}
		respond(manager.PreparePreview(parts[1], input.Grants))
		return
	}
	if len(parts) == 4 && parts[0] == "packages" && parts[3] == "impact" && method == "GET" {
		respond(manager.Impact(platform.Kind(parts[1]), parts[2]))
		return
	}
	if len(parts) == 4 && parts[0] == "packages" && parts[3] == "settings" {
		locale := c.Query("locale")
		if locale != "zh-CN" {
			locale = "en-US"
		}
		if len(parts) == 4 && method == "GET" {
			respond(manager.PackageConfiguration(platform.ReleaseRef{Package: platform.PackageRef{Kind: platform.Kind(parts[1]), ID: parts[2]}, ReleaseID: c.Query("releaseId")}, locale))
			return
		}
		if method == "PUT" {
			var input platform.ConfigurationInput
			if !decode(&input) {
				return
			}
			respond(manager.SavePackageConfiguration(platform.Kind(parts[1]), parts[2], locale, input))
			return
		}
	}
	if len(parts) == 4 && parts[0] == "packages" && parts[3] == "permissions" && method == "PUT" {
		var input platform.PackagePermissions
		if !decode(&input) {
			return
		}
		respond(map[string]bool{"ok": true}, manager.SetPackagePermissions(ctx, platform.Kind(parts[1]), parts[2], input))
		return
	}
	if len(parts) == 3 && parts[0] == "packages" && method == "PATCH" {
		var input platform.PackageAvailability
		if !decode(&input) {
			return
		}
		respond(map[string]bool{"ok": true}, manager.SetAvailability(ctx, platform.Kind(parts[1]), parts[2], input))
		return
	}
	if len(parts) == 1 && parts[0] == "instances" {
		if method == "GET" {
			respond(manager.Instances())
			return
		}
		if method == "POST" {
			var input platform.CreateInstance
			if !decode(&input) {
				return
			}
			respond(manager.CreateInstance(input))
			return
		}
	}
	if len(parts) == 2 && parts[0] == "instances" {
		if method == "PATCH" {
			var input struct {
				Title string `json:"title"`
			}
			if !decode(&input) {
				return
			}
			respond(manager.RenameInstance(parts[1], input.Title))
			return
		}
		if method == "GET" {
			respond(manager.Instance(parts[1]))
			return
		}
		if method == "DELETE" {
			backup, err := manager.RemoveInstance(ctx, parts[1])
			respond(map[string]string{"backup": backup}, err)
			return
		}
	}
	if len(parts) == 3 && parts[0] == "instances" {
		switch {
		case method == "POST" && parts[2] == "open":
			var options platform.OpenOptions
			if !decode(&options) {
				return
			}
			options.ParentOrigin = platformParentOrigin(c)
			respond(manager.OpenInstance(ctx, parts[1], options))
			return
		case method == "POST" && parts[2] == "stop":
			respond(map[string]bool{"ok": true}, manager.Stop(ctx, parts[1]))
			return
		case method == "POST" && parts[2] == "upgrade":
			var input struct {
				ReleaseID    string                   `json:"releaseId"`
				Dependencies []platform.DependencyPin `json:"dependencies"`
			}
			if !decode(&input) {
				return
			}
			respond(manager.UpgradeInstance(ctx, parts[1], input.ReleaseID, input.Dependencies))
			return
		case method == "GET" && parts[2] == "export":
			var output bytes.Buffer
			if err := manager.ExportInstance(ctx, parts[1], &output); err != nil {
				platformManagementError(c, err)
				return
			}
			c.Header("Content-Disposition", `attachment; filename="game-save.zip"`)
			c.Data(200, "application/zip", output.Bytes())
			return
		}
	}
	if len(parts) == 1 && parts[0] == "runtimes" && method == "GET" {
		c.JSON(200, manager.RuntimeSnapshots())
		return
	}
	if len(parts) == 2 && parts[0] == "runtimes" && parts[1] == "plugin" && method == "POST" {
		var input platform.ActivatePlugin
		if !decode(&input) {
			return
		}
		input.ParentOrigin = platformParentOrigin(c)
		respond(manager.ActivatePlugin(ctx, input))
		return
	}
	if len(parts) == 3 && parts[0] == "runtimes" && parts[2] == "stop" && method == "POST" {
		respond(map[string]bool{"ok": true}, manager.Stop(ctx, parts[1]))
		return
	}
	if len(parts) == 1 && parts[0] == "development" {
		if method == "GET" {
			respond(manager.DevelopmentSources())
			return
		}
		if method == "POST" {
			var input platform.CreateDevelopment
			if !decode(&input) {
				return
			}
			respond(manager.CreateDevelopment(input))
			return
		}
	}
	if len(parts) == 2 && parts[0] == "development" && parts[1] == "link" && method == "POST" {
		var input struct {
			Kind         platform.Kind `json:"kind"`
			ProjectID    string        `json:"projectId"`
			RelativePath string        `json:"relativePath"`
		}
		if !decode(&input) {
			return
		}
		respond(manager.LinkDevelopment(input.Kind, input.ProjectID, input.RelativePath))
		return
	}
	if len(parts) == 3 && parts[0] == "development" && method == "GET" {
		switch parts[2] {
		case "check":
			respond(manager.CheckDevelopment(parts[1]))
			return
		case "build":
			command, directory, err := manager.BuildRecipe(parts[1])
			respond(map[string]any{"command": command, "directory": directory}, err)
			return
		}
	}
	platformManagementError(c, &platform.Error{Code: "NOT_FOUND", MessageKey: "platform.errors.NOT_FOUND", Diagnostic: "Unknown management route"})
}

func platformManagementError(c *hertzapp.RequestContext, err error) {
	for cause, key := range map[error]string{resourceexchange.ErrSourceChanged: "market.errors.sourceChanged", resourceexchange.ErrBundleOwned: "market.errors.bundleOwned", resourceexchange.ErrReferenceChanged: "market.errors.referenceChanged", resourceexchange.ErrResourcesBusy: "market.errors.busy"} {
		if errors.Is(err, cause) {
			err = &platform.Error{Code: "DOCUMENT_CONFLICT", MessageKey: key, Diagnostic: err.Error()}
			break
		}
	}
	status, body := platform.ErrorResponse(err)
	slog.Warn("platform_management_failed", "code", body.Code, "diagnostic", body.Diagnostic)
	c.JSON(status, body)
}

func platformParentOrigin(c *hertzapp.RequestContext) string {
	for _, name := range []string{"Origin", "Referer"} {
		if parsed, err := url.Parse(string(c.GetHeader(name))); err == nil && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host
		}
	}
	return string(c.Request.URI().Scheme()) + "://" + string(c.Host())
}
