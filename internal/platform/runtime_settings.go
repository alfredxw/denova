package platform

import (
	"net/http"
	"slices"
)

// serveSettings exposes release-scoped extension preferences to its owning view.
// Preview credentials and dependency tools cannot edit an installed user's
// settings. The manager validates the bound release and observed file revision.
func (r *Runtime) serveSettings(w http.ResponseWriter, request *http.Request, caller *activation) {
	if caller != r.owner || r.hostOnly {
		writeError(w, failure("PERMISSION_DENIED", "Settings belong to the owning extension view"))
		return
	}
	if caller.context.Environment != "installed" {
		writeError(w, failure("UNSUPPORTED", "Preview settings are local to the preview"))
		return
	}
	ref := caller.release.Ref.Package
	var document ConfigurationDocument
	var err error
	switch request.Method {
	case http.MethodGet:
		document, err = r.manager.PackageConfiguration(caller.release.Ref, caller.context.Locale)
	case http.MethodPut:
		if !slices.Contains(caller.grants, "settings.write") {
			writeError(w, failure("PERMISSION_DENIED", "settings.write is not granted"))
			return
		}
		var input ConfigurationInput
		if err = readRequest(request, &input); err == nil {
			if input.ReleaseID != caller.release.Ref.ReleaseID {
				err = failure("PERMISSION_DENIED", "Settings belong to the bound release")
			} else {
				document, err = r.manager.SavePackageConfiguration(ref.Kind, ref.ID, caller.context.Locale, input)
			}
		}
	default:
		err = failure("NOT_FOUND", "Settings route is unavailable")
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeResponse(w, http.StatusOK, document)
}
