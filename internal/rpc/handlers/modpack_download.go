package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nickheyer/discopanel/internal/auth"
	db "github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/rbac"
	"github.com/nickheyer/discopanel/pkg/logger"
)

// NewModpackDownloadHandler creates an HTTP handler for downloading modpack
// archives so they can be distributed to clients.
//
//	GET /api/v1/modpacks/{idOrSlug}/download[?version=<fileId>]
//	Auth: Authorization header OR ?token= query param
//	Behavior: streams locally stored (manually uploaded) archives; redirects
//	to the indexer CDN for Modrinth/CurseForge modpacks.
func NewModpackDownloadHandler(store *db.Store, authManager *auth.Manager, enforcer *rbac.Enforcer, log *logger.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/v1/modpacks/")
		path = strings.TrimSuffix(path, "/download")
		if path == "" || strings.Contains(path, "/") {
			http.Error(w, "invalid modpack identifier", http.StatusBadRequest)
			return
		}

		// Get auth header, fall back to ?token= query param
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			if token := r.URL.Query().Get("token"); token != "" {
				authHeader = "Bearer " + token
			}
		}

		user, err := authManager.AuthenticateFromHeader(r.Context(), authHeader)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Check RBAC permission (modpacks:read)
		if enforcer != nil {
			allowed, rbacErr := enforcer.Enforce(user.Roles, rbac.ResourceModpacks, rbac.ActionRead, "*")
			if rbacErr != nil || !allowed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		ctx := r.Context()
		modpack, err := store.GetIndexedModpack(ctx, path)
		if err != nil || modpack == nil {
			modpack, err = store.GetModpackBySlug(ctx, path)
			if err != nil || modpack == nil {
				http.Error(w, "modpack not found", http.StatusNotFound)
				return
			}
		}

		files, err := store.GetIndexedModpackFiles(ctx, modpack.ID)
		if err != nil || len(files) == 0 {
			http.Error(w, "modpack has no downloadable files", http.StatusNotFound)
			return
		}

		// Pick the requested version, falling back to the newest release
		requestedVersion := r.URL.Query().Get("version")
		var chosen *db.IndexedModpackFile
		for _, f := range files {
			if requestedVersion != "" && f.ID == requestedVersion {
				chosen = f
				break
			}
			if chosen == nil || (f.ReleaseType == "release" && chosen.ReleaseType != "release") ||
				(f.ReleaseType == chosen.ReleaseType && f.FileDate.After(chosen.FileDate)) {
				chosen = f
			}
		}
		if chosen == nil {
			http.Error(w, "requested modpack version not found", http.StatusNotFound)
			return
		}

		// Manually uploaded modpacks store a local file path
		if info, err := os.Stat(chosen.DownloadURL); err == nil && !info.IsDir() {
			file, err := os.Open(chosen.DownloadURL)
			if err != nil {
				log.Error("Failed to open modpack file %s: %v", chosen.DownloadURL, err)
				http.Error(w, "modpack file not available", http.StatusInternalServerError)
				return
			}
			defer file.Close()

			filename := chosen.FileName
			if filename == "" {
				filename = modpack.Slug + ".zip"
			}

			rc := http.NewResponseController(w)
			if err := rc.SetWriteDeadline(time.Now().Add(30 * time.Minute)); err != nil {
				log.Warn("Failed to set write deadline: %v", err)
			}

			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
			http.ServeContent(w, r, filename, info.ModTime(), file)
			return
		}

		// Indexer-hosted file: send the client to the CDN
		if chosen.DownloadURL != "" {
			http.Redirect(w, r, chosen.DownloadURL, http.StatusTemporaryRedirect)
			return
		}

		http.Error(w, "modpack file not available", http.StatusNotFound)
	})
}
