package httpapi

import (
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/version"
)

// AdminGetVersion handles GET /admin/version and returns build metadata.
func (h *Handlers) AdminGetVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":         "ai-gateway",
		"backend_version": version.BackendVersion,
		"git_commit":      version.GitCommit,
		"build_time":      version.BuildTime,
		"release_notes":   version.ReleaseNotes,
	})
}
