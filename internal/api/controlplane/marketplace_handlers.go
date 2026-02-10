package controlplane

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// CreateApp handles POST /store/apps
func (h *Handler) CreateApp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug        string   `json:"slug"`
		Title       string   `json:"title"`
		Summary     string   `json:"summary"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		IconURL     string   `json:"icon_url"`
		SourceURL   string   `json:"source_url"`
		HomepageURL string   `json:"homepage_url"`
		Visibility  string   `json:"visibility"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}

	visibility := domain.VisibilityPublic
	if req.Visibility != "" {
		visibility = domain.AppVisibility(req.Visibility)
	}

	// Get owner from context (in real implementation, get from auth)
	owner := "system" // TODO: extract from auth context

	app := &domain.App{
		Slug:        req.Slug,
		Owner:       owner,
		Visibility:  visibility,
		Title:       req.Title,
		Summary:     req.Summary,
		Description: req.Description,
		Tags:        req.Tags,
		IconURL:     req.IconURL,
		SourceURL:   req.SourceURL,
		HomepageURL: req.HomepageURL,
	}

	if err := h.Store.Marketplace.CreateApp(r.Context(), app); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(app)
}

// GetApp handles GET /store/apps/{slug}
func (h *Handler) GetApp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	app, err := h.Store.Marketplace.GetApp(r.Context(), slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(app)
}

// ListApps handles GET /store/apps
func (h *Handler) ListApps(w http.ResponseWriter, r *http.Request) {
	visibility := r.URL.Query().Get("visibility")
	owner := r.URL.Query().Get("owner")
	limit := 30
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	apps, err := h.Store.Marketplace.ListApps(r.Context(), domain.AppVisibility(visibility), owner, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"apps":  apps,
		"total": len(apps),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DeleteApp handles DELETE /store/apps/{slug}
func (h *Handler) DeleteApp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	app, err := h.Store.Marketplace.GetApp(r.Context(), slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if err := h.Store.Marketplace.DeleteApp(r.Context(), app.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PublishRelease handles POST /store/apps/{slug}/releases
func (h *Handler) PublishRelease(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100 MB max
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	version := r.FormValue("version")
	changelog := r.FormValue("changelog")

	if version == "" {
		http.Error(w, "version is required", http.StatusBadRequest)
		return
	}

	// Get bundle file
	file, _, err := r.FormFile("bundle")
	if err != nil {
		http.Error(w, "bundle file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "bundle-*.tar.gz")
	if err != nil {
		http.Error(w, "failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, file); err != nil {
		http.Error(w, "failed to save bundle", http.StatusInternalServerError)
		return
	}

	// Get owner from context
	owner := "system" // TODO: extract from auth context

	// Publish via service
	if h.MarketplaceService == nil {
		http.Error(w, "marketplace service not initialized", http.StatusInternalServerError)
		return
	}

	release, err := h.MarketplaceService.PublishBundle(r.Context(), slug, version, tmpFile.Name(), owner)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Add changelog
	if changelog != "" {
		release.Changelog = changelog
		// Update release in DB (simplified, should use proper update method)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(release)
}

// GetRelease handles GET /store/apps/{slug}/releases/{version}
func (h *Handler) GetRelease(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	version := r.PathValue("version")

	if slug == "" || version == "" {
		http.Error(w, "slug and version are required", http.StatusBadRequest)
		return
	}

	app, err := h.Store.Marketplace.GetApp(r.Context(), slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	release, err := h.Store.Marketplace.GetRelease(r.Context(), app.ID, version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(release)
}

// ListReleases handles GET /store/apps/{slug}/releases
func (h *Handler) ListReleases(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	app, err := h.Store.Marketplace.GetApp(r.Context(), slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	limit := 30
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	releases, err := h.Store.Marketplace.ListReleases(r.Context(), app.ID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"releases": releases,
		"total":    len(releases),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// PlanInstall handles POST /store/installations:plan
func (h *Handler) PlanInstall(w http.ResponseWriter, r *http.Request) {
	var req domain.InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.AppSlug == "" {
		http.Error(w, "app_slug is required", http.StatusBadRequest)
		return
	}
	if req.Version == "" {
		http.Error(w, "version is required", http.StatusBadRequest)
		return
	}
	if req.InstallName == "" {
		http.Error(w, "install_name is required", http.StatusBadRequest)
		return
	}

	// Get tenant/namespace from context
	scope := store.TenantScopeFromContext(r.Context())
	req.TenantID = scope.TenantID
	req.Namespace = scope.Namespace
	req.DryRun = true

	if h.MarketplaceService == nil {
		http.Error(w, "marketplace service not initialized", http.StatusInternalServerError)
		return
	}

	plan, err := h.MarketplaceService.PlanInstallation(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plan)
}

// InstallApp handles POST /store/installations
func (h *Handler) InstallApp(w http.ResponseWriter, r *http.Request) {
	var req domain.InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.AppSlug == "" {
		http.Error(w, "app_slug is required", http.StatusBadRequest)
		return
	}
	if req.Version == "" {
		http.Error(w, "version is required", http.StatusBadRequest)
		return
	}
	if req.InstallName == "" {
		http.Error(w, "install_name is required", http.StatusBadRequest)
		return
	}

	// Get tenant/namespace from context
	scope := store.TenantScopeFromContext(r.Context())
	req.TenantID = scope.TenantID
	req.Namespace = scope.Namespace

	if h.MarketplaceService == nil {
		http.Error(w, "marketplace service not initialized", http.StatusInternalServerError)
		return
	}

	installation, job, err := h.MarketplaceService.Install(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := map[string]interface{}{
		"installation_id": installation.ID,
		"job_id":          job.ID,
		"status":          installation.Status,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}

// GetInstallation handles GET /store/installations/{id}
func (h *Handler) GetInstallation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	installation, err := h.Store.Marketplace.GetInstallation(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Get resources
	resources, _ := h.Store.Marketplace.ListInstallationResources(r.Context(), id)

	response := map[string]interface{}{
		"installation": installation,
		"resources":    resources,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListInstallations handles GET /store/installations
func (h *Handler) ListInstallations(w http.ResponseWriter, r *http.Request) {
	scope := store.TenantScopeFromContext(r.Context())

	limit := 30
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	installations, err := h.Store.Marketplace.ListInstallations(r.Context(), scope.TenantID, scope.Namespace, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"installations": installations,
		"total":         len(installations),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UninstallApp handles DELETE /store/installations/{id}
func (h *Handler) UninstallApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	force := r.URL.Query().Get("force") == "true"

	if h.MarketplaceService == nil {
		http.Error(w, "marketplace service not initialized", http.StatusInternalServerError)
		return
	}

	if err := h.MarketplaceService.Uninstall(r.Context(), id, force); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetInstallJob handles GET /store/jobs/{id}
func (h *Handler) GetInstallJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	job, err := h.Store.Marketplace.GetInstallJob(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// RegisterMarketplaceRoutes registers marketplace routes
func (h *Handler) RegisterMarketplaceRoutes(mux *http.ServeMux) {
	// App management
	mux.HandleFunc("POST /store/apps", h.CreateApp)
	mux.HandleFunc("GET /store/apps", h.ListApps)
	mux.HandleFunc("GET /store/apps/{slug}", h.GetApp)
	mux.HandleFunc("DELETE /store/apps/{slug}", h.DeleteApp)

	// Release management
	mux.HandleFunc("POST /store/apps/{slug}/releases", h.PublishRelease)
	mux.HandleFunc("GET /store/apps/{slug}/releases", h.ListReleases)
	mux.HandleFunc("GET /store/apps/{slug}/releases/{version}", h.GetRelease)

	// Installation management
	mux.HandleFunc("POST /store/installations:plan", h.PlanInstall)
	mux.HandleFunc("POST /store/installations", h.InstallApp)
	mux.HandleFunc("GET /store/installations", h.ListInstallations)
	mux.HandleFunc("GET /store/installations/{id}", h.GetInstallation)
	mux.HandleFunc("DELETE /store/installations/{id}", h.UninstallApp)

	// Job tracking
	mux.HandleFunc("GET /store/jobs/{id}", h.GetInstallJob)
}
