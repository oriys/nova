package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/oriys/nova/internal/domain"
)

// CreateApp creates a new app in the marketplace
func (s *PostgresStore) CreateApp(ctx context.Context, app *domain.App) error {
	if app.ID == "" {
		app.ID = uuid.New().String()
	}
	if app.Tags == nil {
		app.Tags = []string{}
	}
	now := time.Now()
	app.CreatedAt = now
	app.UpdatedAt = now

	query := `
		INSERT INTO app_store_apps (id, slug, owner, visibility, title, summary, description, tags, icon_url, source_url, homepage_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := s.pool.Exec(ctx, query,
		app.ID, app.Slug, app.Owner, app.Visibility, app.Title, app.Summary, app.Description,
		app.Tags, app.IconURL, app.SourceURL, app.HomepageURL, app.CreatedAt, app.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create app: %w", err)
	}
	return nil
}

// GetApp retrieves an app by ID or slug
func (s *PostgresStore) GetApp(ctx context.Context, idOrSlug string) (*domain.App, error) {
	query := `
		SELECT id, slug, owner, visibility, title, summary, description, tags, icon_url, source_url, homepage_url, created_at, updated_at
		FROM app_store_apps
		WHERE id = $1 OR slug = $1
	`
	var app domain.App
	var tags []string
	err := s.pool.QueryRow(ctx, query, idOrSlug).Scan(
		&app.ID, &app.Slug, &app.Owner, &app.Visibility, &app.Title, &app.Summary, &app.Description,
		&tags, &app.IconURL, &app.SourceURL, &app.HomepageURL, &app.CreatedAt, &app.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("app not found: %s", idOrSlug)
		}
		return nil, fmt.Errorf("get app: %w", err)
	}
	app.Tags = tags
	return &app, nil
}

// ListApps returns all apps matching filters
func (s *PostgresStore) ListApps(ctx context.Context, visibility domain.AppVisibility, owner string, limit, offset int) ([]*domain.App, error) {
	query := `
		SELECT id, slug, owner, visibility, title, summary, description, tags, icon_url, source_url, homepage_url, created_at, updated_at
		FROM app_store_apps
		WHERE ($1 = '' OR visibility = $1)
		  AND ($2 = '' OR owner = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := s.pool.Query(ctx, query, visibility, owner, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	defer rows.Close()

	var apps []*domain.App
	for rows.Next() {
		var app domain.App
		var tags []string
		err := rows.Scan(
			&app.ID, &app.Slug, &app.Owner, &app.Visibility, &app.Title, &app.Summary, &app.Description,
			&tags, &app.IconURL, &app.SourceURL, &app.HomepageURL, &app.CreatedAt, &app.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan app: %w", err)
		}
		app.Tags = tags
		apps = append(apps, &app)
	}
	return apps, rows.Err()
}

// DeleteApp deletes an app and all its releases
func (s *PostgresStore) DeleteApp(ctx context.Context, appID string) error {
	query := `DELETE FROM app_store_apps WHERE id = $1`
	result, err := s.pool.Exec(ctx, query, appID)
	if err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("app not found: %s", appID)
	}
	return nil
}

// CreateRelease creates a new release for an app
func (s *PostgresStore) CreateRelease(ctx context.Context, release *domain.AppRelease) error {
	if release.ID == "" {
		release.ID = uuid.New().String()
	}
	now := time.Now()
	release.CreatedAt = now
	release.UpdatedAt = now

	query := `
		INSERT INTO app_store_releases (id, app_id, version, manifest_json, artifact_uri, artifact_digest, signature, status, changelog, requires_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := s.pool.Exec(ctx, query,
		release.ID, release.AppID, release.Version, release.ManifestJSON, release.ArtifactURI,
		release.ArtifactDigest, release.Signature, release.Status, release.Changelog,
		release.RequiresVersion, release.CreatedAt, release.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create release: %w", err)
	}
	return nil
}

// GetRelease retrieves a release by app and version
func (s *PostgresStore) GetRelease(ctx context.Context, appID, version string) (*domain.AppRelease, error) {
	query := `
		SELECT id, app_id, version, manifest_json, artifact_uri, artifact_digest, signature, status, changelog, requires_version, created_at, updated_at
		FROM app_store_releases
		WHERE app_id = $1 AND version = $2
	`
	var release domain.AppRelease
	err := s.pool.QueryRow(ctx, query, appID, version).Scan(
		&release.ID, &release.AppID, &release.Version, &release.ManifestJSON, &release.ArtifactURI,
		&release.ArtifactDigest, &release.Signature, &release.Status, &release.Changelog,
		&release.RequiresVersion, &release.CreatedAt, &release.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("release not found: %s@%s", appID, version)
		}
		return nil, fmt.Errorf("get release: %w", err)
	}
	return &release, nil
}

// GetReleaseByID retrieves a release by ID
func (s *PostgresStore) GetReleaseByID(ctx context.Context, releaseID string) (*domain.AppRelease, error) {
	query := `
		SELECT id, app_id, version, manifest_json, artifact_uri, artifact_digest, signature, status, changelog, requires_version, created_at, updated_at
		FROM app_store_releases
		WHERE id = $1
	`
	var release domain.AppRelease
	err := s.pool.QueryRow(ctx, query, releaseID).Scan(
		&release.ID, &release.AppID, &release.Version, &release.ManifestJSON, &release.ArtifactURI,
		&release.ArtifactDigest, &release.Signature, &release.Status, &release.Changelog,
		&release.RequiresVersion, &release.CreatedAt, &release.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("release not found: %s", releaseID)
		}
		return nil, fmt.Errorf("get release by ID: %w", err)
	}
	return &release, nil
}

// ListReleases returns all releases for an app
func (s *PostgresStore) ListReleases(ctx context.Context, appID string, limit, offset int) ([]*domain.AppRelease, error) {
	query := `
		SELECT id, app_id, version, manifest_json, artifact_uri, artifact_digest, signature, status, changelog, requires_version, created_at, updated_at
		FROM app_store_releases
		WHERE app_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := s.pool.Query(ctx, query, appID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	defer rows.Close()

	var releases []*domain.AppRelease
	for rows.Next() {
		var release domain.AppRelease
		err := rows.Scan(
			&release.ID, &release.AppID, &release.Version, &release.ManifestJSON, &release.ArtifactURI,
			&release.ArtifactDigest, &release.Signature, &release.Status, &release.Changelog,
			&release.RequiresVersion, &release.CreatedAt, &release.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan release: %w", err)
		}
		releases = append(releases, &release)
	}
	return releases, rows.Err()
}

// UpdateReleaseStatus updates the status of a release
func (s *PostgresStore) UpdateReleaseStatus(ctx context.Context, releaseID string, status domain.ReleaseStatus) error {
	query := `UPDATE app_store_releases SET status = $1, updated_at = $2 WHERE id = $3`
	result, err := s.pool.Exec(ctx, query, status, time.Now(), releaseID)
	if err != nil {
		return fmt.Errorf("update release status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("release not found: %s", releaseID)
	}
	return nil
}

// CreateInstallation creates a new installation record
func (s *PostgresStore) CreateInstallation(ctx context.Context, inst *domain.Installation) error {
	if inst.ID == "" {
		inst.ID = uuid.New().String()
	}
	now := time.Now()
	inst.CreatedAt = now
	inst.UpdatedAt = now

	query := `
		INSERT INTO app_store_installations (id, tenant_id, namespace, app_id, release_id, install_name, status, values_json, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := s.pool.Exec(ctx, query,
		inst.ID, inst.TenantID, inst.Namespace, inst.AppID, inst.ReleaseID, inst.InstallName,
		inst.Status, inst.ValuesJSON, inst.CreatedBy, inst.CreatedAt, inst.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create installation: %w", err)
	}
	return nil
}

// GetInstallation retrieves an installation by ID
func (s *PostgresStore) GetInstallation(ctx context.Context, installationID string) (*domain.Installation, error) {
	query := `
		SELECT id, tenant_id, namespace, app_id, release_id, install_name, status, values_json, created_by, created_at, updated_at
		FROM app_store_installations
		WHERE id = $1
	`
	var inst domain.Installation
	err := s.pool.QueryRow(ctx, query, installationID).Scan(
		&inst.ID, &inst.TenantID, &inst.Namespace, &inst.AppID, &inst.ReleaseID, &inst.InstallName,
		&inst.Status, &inst.ValuesJSON, &inst.CreatedBy, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("installation not found: %s", installationID)
		}
		return nil, fmt.Errorf("get installation: %w", err)
	}
	return &inst, nil
}

// GetInstallationByName retrieves an installation by tenant, namespace, and name
func (s *PostgresStore) GetInstallationByName(ctx context.Context, tenantID, namespace, installName string) (*domain.Installation, error) {
	query := `
		SELECT id, tenant_id, namespace, app_id, release_id, install_name, status, values_json, created_by, created_at, updated_at
		FROM app_store_installations
		WHERE tenant_id = $1 AND namespace = $2 AND install_name = $3
	`
	var inst domain.Installation
	err := s.pool.QueryRow(ctx, query, tenantID, namespace, installName).Scan(
		&inst.ID, &inst.TenantID, &inst.Namespace, &inst.AppID, &inst.ReleaseID, &inst.InstallName,
		&inst.Status, &inst.ValuesJSON, &inst.CreatedBy, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("installation not found: %s/%s/%s", tenantID, namespace, installName)
		}
		return nil, fmt.Errorf("get installation by name: %w", err)
	}
	return &inst, nil
}

// ListInstallations returns all installations for a tenant/namespace
func (s *PostgresStore) ListInstallations(ctx context.Context, tenantID, namespace string, limit, offset int) ([]*domain.Installation, error) {
	query := `
		SELECT id, tenant_id, namespace, app_id, release_id, install_name, status, values_json, created_by, created_at, updated_at
		FROM app_store_installations
		WHERE tenant_id = $1 AND namespace = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := s.pool.Query(ctx, query, tenantID, namespace, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list installations: %w", err)
	}
	defer rows.Close()

	var installations []*domain.Installation
	for rows.Next() {
		var inst domain.Installation
		err := rows.Scan(
			&inst.ID, &inst.TenantID, &inst.Namespace, &inst.AppID, &inst.ReleaseID, &inst.InstallName,
			&inst.Status, &inst.ValuesJSON, &inst.CreatedBy, &inst.CreatedAt, &inst.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan installation: %w", err)
		}
		installations = append(installations, &inst)
	}
	return installations, rows.Err()
}

// UpdateInstallationStatus updates the status of an installation
func (s *PostgresStore) UpdateInstallationStatus(ctx context.Context, installationID string, status domain.InstallationStatus) error {
	query := `UPDATE app_store_installations SET status = $1, updated_at = $2 WHERE id = $3`
	result, err := s.pool.Exec(ctx, query, status, time.Now(), installationID)
	if err != nil {
		return fmt.Errorf("update installation status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("installation not found: %s", installationID)
	}
	return nil
}

// DeleteInstallation deletes an installation and its resources
func (s *PostgresStore) DeleteInstallation(ctx context.Context, installationID string) error {
	query := `DELETE FROM app_store_installations WHERE id = $1`
	result, err := s.pool.Exec(ctx, query, installationID)
	if err != nil {
		return fmt.Errorf("delete installation: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("installation not found: %s", installationID)
	}
	return nil
}

// CreateInstallationResource creates a resource record for an installation
func (s *PostgresStore) CreateInstallationResource(ctx context.Context, resource *domain.InstallationResource) error {
	if resource.ID == "" {
		resource.ID = uuid.New().String()
	}
	resource.CreatedAt = time.Now()

	query := `
		INSERT INTO app_store_installation_resources (id, installation_id, resource_type, resource_name, resource_id, content_digest, managed_mode, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := s.pool.Exec(ctx, query,
		resource.ID, resource.InstallationID, resource.ResourceType, resource.ResourceName,
		resource.ResourceID, resource.ContentDigest, resource.ManagedMode, resource.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create installation resource: %w", err)
	}
	return nil
}

// ListInstallationResources returns all resources for an installation
func (s *PostgresStore) ListInstallationResources(ctx context.Context, installationID string) ([]*domain.InstallationResource, error) {
	query := `
		SELECT id, installation_id, resource_type, resource_name, resource_id, content_digest, managed_mode, created_at
		FROM app_store_installation_resources
		WHERE installation_id = $1
		ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, query, installationID)
	if err != nil {
		return nil, fmt.Errorf("list installation resources: %w", err)
	}
	defer rows.Close()

	var resources []*domain.InstallationResource
	for rows.Next() {
		var resource domain.InstallationResource
		err := rows.Scan(
			&resource.ID, &resource.InstallationID, &resource.ResourceType, &resource.ResourceName,
			&resource.ResourceID, &resource.ContentDigest, &resource.ManagedMode, &resource.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan installation resource: %w", err)
		}
		resources = append(resources, &resource)
	}
	return resources, rows.Err()
}

// DeleteInstallationResource deletes a resource record
func (s *PostgresStore) DeleteInstallationResource(ctx context.Context, resourceID string) error {
	query := `DELETE FROM app_store_installation_resources WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, resourceID)
	if err != nil {
		return fmt.Errorf("delete installation resource: %w", err)
	}
	return nil
}

// CreateInstallJob creates a new install/upgrade/uninstall job
func (s *PostgresStore) CreateInstallJob(ctx context.Context, job *domain.InstallJob) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	job.StartedAt = time.Now()

	query := `
		INSERT INTO app_store_jobs (id, installation_id, operation, status, step, error, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := s.pool.Exec(ctx, query,
		job.ID, job.InstallationID, job.Operation, job.Status, job.Step, job.Error, job.StartedAt, job.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("create install job: %w", err)
	}
	return nil
}

// GetInstallJob retrieves a job by ID
func (s *PostgresStore) GetInstallJob(ctx context.Context, jobID string) (*domain.InstallJob, error) {
	query := `
		SELECT id, installation_id, operation, status, step, error, started_at, finished_at
		FROM app_store_jobs
		WHERE id = $1
	`
	var job domain.InstallJob
	err := s.pool.QueryRow(ctx, query, jobID).Scan(
		&job.ID, &job.InstallationID, &job.Operation, &job.Status, &job.Step, &job.Error, &job.StartedAt, &job.FinishedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("job not found: %s", jobID)
		}
		return nil, fmt.Errorf("get install job: %w", err)
	}
	return &job, nil
}

// UpdateInstallJob updates a job's status and details
func (s *PostgresStore) UpdateInstallJob(ctx context.Context, job *domain.InstallJob) error {
	query := `UPDATE app_store_jobs SET status = $1, step = $2, error = $3, finished_at = $4 WHERE id = $5`
	result, err := s.pool.Exec(ctx, query, job.Status, job.Step, job.Error, job.FinishedAt, job.ID)
	if err != nil {
		return fmt.Errorf("update install job: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("job not found: %s", job.ID)
	}
	return nil
}

// ListInstallJobs returns all jobs for an installation
func (s *PostgresStore) ListInstallJobs(ctx context.Context, installationID string, limit, offset int) ([]*domain.InstallJob, error) {
	query := `
		SELECT id, installation_id, operation, status, step, error, started_at, finished_at
		FROM app_store_jobs
		WHERE installation_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := s.pool.Query(ctx, query, installationID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list install jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*domain.InstallJob
	for rows.Next() {
		var job domain.InstallJob
		err := rows.Scan(
			&job.ID, &job.InstallationID, &job.Operation, &job.Status, &job.Step, &job.Error, &job.StartedAt, &job.FinishedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan install job: %w", err)
		}
		jobs = append(jobs, &job)
	}
	return jobs, rows.Err()
}

// AcquireInstallLock acquires an advisory lock for install/uninstall serialization
// Returns true if lock acquired, false if already held
func (s *PostgresStore) AcquireInstallLock(ctx context.Context, tenantID, namespace string) (bool, error) {
	// Hash tenant+namespace to a bigint for advisory lock
	lockKey := hashToInt64(fmt.Sprintf("install:%s:%s", tenantID, namespace))
	var acquired bool
	err := s.pool.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&acquired)
	if err != nil {
		return false, fmt.Errorf("acquire install lock: %w", err)
	}
	return acquired, nil
}

// ReleaseInstallLock releases an advisory lock
func (s *PostgresStore) ReleaseInstallLock(ctx context.Context, tenantID, namespace string) error {
	lockKey := hashToInt64(fmt.Sprintf("install:%s:%s", tenantID, namespace))
	_, err := s.pool.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockKey)
	if err != nil {
		return fmt.Errorf("release install lock: %w", err)
	}
	return nil
}

// hashToInt64 converts a string to a 64-bit integer for advisory locks
func hashToInt64(s string) int64 {
	h := sha256.Sum256([]byte(s))
	// Use first 8 bytes as int64
	return int64(h[0]) | int64(h[1])<<8 | int64(h[2])<<16 | int64(h[3])<<24 |
		int64(h[4])<<32 | int64(h[5])<<40 | int64(h[6])<<48 | int64(h[7])<<56
}

// GetManifestFromRelease parses and returns the manifest from a release
func GetManifestFromRelease(release *domain.AppRelease) (*domain.BundleManifest, error) {
	var manifest domain.BundleManifest
	if err := json.Unmarshal(release.ManifestJSON, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &manifest, nil
}
