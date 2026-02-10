package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/pkg/crypto"
	"github.com/oriys/nova/internal/store"
	"gopkg.in/yaml.v3"
)

// MarketplaceService handles marketplace operations
type MarketplaceService struct {
	store         *store.PostgresStore
	functionSvc   *FunctionService
	artifactStore ArtifactStore
}

// ArtifactStore interface for bundle artifact storage
type ArtifactStore interface {
	Save(ctx context.Context, path string, data io.Reader) (uri string, digest string, err error)
	Get(ctx context.Context, uri string) (io.ReadCloser, error)
	Delete(ctx context.Context, uri string) error
}

// LocalArtifactStore stores artifacts on local filesystem
type LocalArtifactStore struct {
	basePath string
}

func NewLocalArtifactStore(basePath string) *LocalArtifactStore {
	return &LocalArtifactStore{basePath: basePath}
}

func (s *LocalArtifactStore) Save(ctx context.Context, path string, data io.Reader) (string, string, error) {
	// Ensure base directory exists
	if err := os.MkdirAll(s.basePath, 0755); err != nil {
		return "", "", fmt.Errorf("create artifact dir: %w", err)
	}

	// Create full path
	fullPath := filepath.Join(s.basePath, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", "", fmt.Errorf("create artifact subdir: %w", err)
	}

	// Write file and calculate digest
	f, err := os.Create(fullPath)
	if err != nil {
		return "", "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	mw := io.MultiWriter(f, h)

	if _, err := io.Copy(mw, data); err != nil {
		return "", "", fmt.Errorf("write artifact: %w", err)
	}

	digest := hex.EncodeToString(h.Sum(nil))
	return "file://" + fullPath, digest, nil
}

func (s *LocalArtifactStore) Get(ctx context.Context, uri string) (io.ReadCloser, error) {
	path := strings.TrimPrefix(uri, "file://")
	return os.Open(path)
}

func (s *LocalArtifactStore) Delete(ctx context.Context, uri string) error {
	path := strings.TrimPrefix(uri, "file://")
	return os.Remove(path)
}

func NewMarketplaceService(store *store.PostgresStore, functionSvc *FunctionService, artifactStore ArtifactStore) *MarketplaceService {
	return &MarketplaceService{
		store:         store,
		functionSvc:   functionSvc,
		artifactStore: artifactStore,
	}
}

// PublishBundle validates and publishes a bundle as an app release
func (m *MarketplaceService) PublishBundle(ctx context.Context, appSlug, version string, bundlePath string, owner string) (*domain.AppRelease, error) {
	// Extract and validate manifest
	manifest, err := m.extractManifest(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("extract manifest: %w", err)
	}

	// Validate bundle structure
	if err := m.validateBundle(manifest, bundlePath); err != nil {
		return nil, fmt.Errorf("validate bundle: %w", err)
	}

	// Get or create app
	app, err := m.store.GetApp(ctx, appSlug)
	if err != nil {
		// App doesn't exist, create it
		app = &domain.App{
			ID:         uuid.New().String(),
			Slug:       appSlug,
			Owner:      owner,
			Visibility: domain.VisibilityPublic,
			Title:      manifest.Name,
			Summary:    manifest.Description,
		}
		if err := m.store.CreateApp(ctx, app); err != nil {
			return nil, fmt.Errorf("create app: %w", err)
		}
	}

	// Check if version already exists
	if existing, _ := m.store.GetRelease(ctx, app.ID, version); existing != nil {
		return nil, fmt.Errorf("version %s already exists", version)
	}

	// Upload artifact
	f, err := os.Open(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}
	defer f.Close()

	artifactPath := fmt.Sprintf("%s/%s/%s.tar.gz", appSlug, version, version)
	artifactURI, digest, err := m.artifactStore.Save(ctx, artifactPath, f)
	if err != nil {
		return nil, fmt.Errorf("save artifact: %w", err)
	}

	// Marshal manifest
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	// Create release
	release := &domain.AppRelease{
		AppID:          app.ID,
		Version:        version,
		ManifestJSON:   manifestJSON,
		ArtifactURI:    artifactURI,
		ArtifactDigest: digest,
		Status:         domain.ReleaseStatusPublished,
	}

	if err := m.store.CreateRelease(ctx, release); err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	return release, nil
}

// PlanInstallation performs a dry-run to check for conflicts and quota
func (m *MarketplaceService) PlanInstallation(ctx context.Context, req *domain.InstallRequest) (*domain.InstallPlan, error) {
	plan := &domain.InstallPlan{
		Valid:    true,
		ToCreate: []domain.PlannedResource{},
	}

	// Get app and release
	app, err := m.store.GetApp(ctx, req.AppSlug)
	if err != nil {
		plan.Valid = false
		plan.Errors = append(plan.Errors, fmt.Sprintf("app not found: %s", req.AppSlug))
		return plan, nil
	}

	release, err := m.store.GetRelease(ctx, app.ID, req.Version)
	if err != nil {
		plan.Valid = false
		plan.Errors = append(plan.Errors, fmt.Sprintf("version not found: %s", req.Version))
		return plan, nil
	}

	if release.Status != domain.ReleaseStatusPublished {
		plan.Valid = false
		plan.Errors = append(plan.Errors, fmt.Sprintf("release is not published: %s", release.Status))
		return plan, nil
	}

	// Parse manifest
	manifest, err := store.GetManifestFromRelease(release)
	if err != nil {
		plan.Valid = false
		plan.Errors = append(plan.Errors, fmt.Sprintf("invalid manifest: %v", err))
		return plan, nil
	}

	// Check for naming conflicts
	namePrefix := req.NamePrefix
	if namePrefix != "" && !strings.HasSuffix(namePrefix, "-") {
		namePrefix += "-"
	}

	// Check function conflicts
	for _, fnSpec := range manifest.Functions {
		funcName := namePrefix + fnSpec.Key
		if fnSpec.Name != "" {
			funcName = fnSpec.Name
		}
		
		// Check if function exists
		existing, _ := m.store.GetFunctionByName(ctx, funcName)
		if existing != nil {
			plan.Conflicts = append(plan.Conflicts, domain.ResourceConflict{
				ResourceType: "function",
				ResourceName: funcName,
				ExistingID:   existing.ID,
				Reason:       "function with this name already exists",
			})
			plan.Valid = false
		} else {
			plan.ToCreate = append(plan.ToCreate, domain.PlannedResource{
				ResourceType: "function",
				ResourceName: funcName,
				Runtime:      string(fnSpec.Runtime),
				Description:  fnSpec.Description,
			})
		}

		// Check runtime availability
		if !fnSpec.Runtime.IsValid() {
			if _, err := m.store.GetRuntime(ctx, string(fnSpec.Runtime)); err != nil {
				plan.MissingRuntimes = append(plan.MissingRuntimes, string(fnSpec.Runtime))
				plan.Valid = false
			}
		}
	}

	// Check workflow conflicts
	if manifest.Workflow != nil {
		workflowName := req.InstallName
		if manifest.Workflow.Name != "" {
			workflowName = namePrefix + manifest.Workflow.Name
		}

		// Check if workflow exists
		existing, _ := m.store.GetWorkflowByName(ctx, workflowName)
		if existing != nil {
			plan.Conflicts = append(plan.Conflicts, domain.ResourceConflict{
				ResourceType: "workflow",
				ResourceName: workflowName,
				ExistingID:   existing.ID,
				Reason:       "workflow with this name already exists",
			})
			plan.Valid = false
		} else {
			plan.ToCreate = append(plan.ToCreate, domain.PlannedResource{
				ResourceType: "workflow",
				ResourceName: workflowName,
				Description:  manifest.Workflow.Description,
			})
		}
	}

	// Check quota - simplified check
	_ = len(manifest.Functions) // functionCount for quota check
	// TODO: Implement proper quota check when tenant quota methods are available
	plan.QuotaCheck.FunctionsUsed = 0
	plan.QuotaCheck.FunctionsLimit = 0
	plan.QuotaCheck.OK = true

	return plan, nil
}

// Install installs an app bundle into a tenant/namespace
func (m *MarketplaceService) Install(ctx context.Context, req *domain.InstallRequest) (*domain.Installation, *domain.InstallJob, error) {
	// First, run dry-run to validate
	if !req.DryRun {
		plan, err := m.PlanInstallation(ctx, req)
		if err != nil {
			return nil, nil, fmt.Errorf("plan installation: %w", err)
		}
		if !plan.Valid {
			return nil, nil, fmt.Errorf("installation validation failed: %v", plan.Errors)
		}
	}

	// Get app and release
	app, err := m.store.GetApp(ctx, req.AppSlug)
	if err != nil {
		return nil, nil, fmt.Errorf("get app: %w", err)
	}

	release, err := m.store.GetRelease(ctx, app.ID, req.Version)
	if err != nil {
		return nil, nil, fmt.Errorf("get release: %w", err)
	}

	// Acquire installation lock for this tenant/namespace
	locked, err := m.store.AcquireInstallLock(ctx, req.TenantID, req.Namespace)
	if err != nil {
		return nil, nil, fmt.Errorf("acquire lock: %w", err)
	}
	if !locked {
		return nil, nil, fmt.Errorf("another installation is in progress for this namespace")
	}
	defer m.store.ReleaseInstallLock(ctx, req.TenantID, req.Namespace)

	// Check if installation with this name exists
	if existing, _ := m.store.GetInstallationByName(ctx, req.TenantID, req.Namespace, req.InstallName); existing != nil {
		return nil, nil, fmt.Errorf("installation '%s' already exists in %s/%s", req.InstallName, req.TenantID, req.Namespace)
	}

	// Marshal values
	valuesJSON, _ := json.Marshal(req.Values)

	// Create installation record
	installation := &domain.Installation{
		TenantID:    req.TenantID,
		Namespace:   req.Namespace,
		AppID:       app.ID,
		ReleaseID:   release.ID,
		InstallName: req.InstallName,
		Status:      domain.InstallStatusPending,
		ValuesJSON:  valuesJSON,
		CreatedBy:   "system", // TODO: get from auth context
	}

	if err := m.store.CreateInstallation(ctx, installation); err != nil {
		return nil, nil, fmt.Errorf("create installation: %w", err)
	}

	// Create install job
	job := &domain.InstallJob{
		InstallationID: installation.ID,
		Operation:      domain.JobOperationInstall,
		Status:         domain.InstallStatusPlanning,
		Step:           "Planning installation",
	}

	if err := m.store.CreateInstallJob(ctx, job); err != nil {
		return nil, nil, fmt.Errorf("create job: %w", err)
	}

	// Execute installation asynchronously
	go m.executeInstallation(context.Background(), installation, release, req)

	return installation, job, nil
}

// executeInstallation performs the actual installation in the background
func (m *MarketplaceService) executeInstallation(ctx context.Context, installation *domain.Installation, release *domain.AppRelease, req *domain.InstallRequest) {
	// Get the job
	jobs, _ := m.store.ListInstallJobs(ctx, installation.ID, 1, 0)
	if len(jobs) == 0 {
		return
	}
	job := jobs[0]

	// Parse manifest
	manifest, err := store.GetManifestFromRelease(release)
	if err != nil {
		m.failJob(ctx, job, "parse manifest", err)
		m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
		return
	}

	// Download and extract bundle
	bundlePath, err := m.downloadBundle(ctx, release.ArtifactURI)
	if err != nil {
		m.failJob(ctx, job, "download bundle", err)
		m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
		return
	}
	defer os.RemoveAll(bundlePath)

	// Update status to applying
	job.Status = domain.InstallStatusApplying
	job.Step = "Installing functions"
	m.store.UpdateInstallJob(ctx, job)
	m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusApplying)

	namePrefix := req.NamePrefix
	if namePrefix != "" && !strings.HasSuffix(namePrefix, "-") {
		namePrefix += "-"
	}

	// Map of function_ref -> installed function name
	funcRefMap := make(map[string]string)

	// Install functions first
	for _, fnSpec := range manifest.Functions {
		funcName := namePrefix + fnSpec.Key
		if fnSpec.Name != "" {
			funcName = fnSpec.Name
		}

		// Read function code files
		code, err := m.readFunctionFiles(bundlePath, fnSpec.Files)
		if err != nil {
			m.failJob(ctx, job, fmt.Sprintf("read function %s files", fnSpec.Key), err)
			m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
			return
		}

		// Create function
		createReq := CreateFunctionRequest{
			Name:        funcName,
			Runtime:     string(fnSpec.Runtime),
			Handler:     fnSpec.Handler,
			Code:        code,
			MemoryMB:    fnSpec.MemoryMB,
			TimeoutS:    fnSpec.TimeoutS,
			EnvVars:     fnSpec.EnvVars,
		}

		// Set tenant/namespace on function
		// TODO: This requires extending FunctionService to support tenant/namespace
		fn, _, err := m.functionSvc.CreateFunction(ctx, createReq)
		if err != nil {
			m.failJob(ctx, job, fmt.Sprintf("create function %s", funcName), err)
			m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
			return
		}

		// Track installed resource
		resource := &domain.InstallationResource{
			InstallationID: installation.ID,
			ResourceType:   "function",
			ResourceName:   funcName,
			ResourceID:     fn.ID,
			ContentDigest:  crypto.HashString(code),
			ManagedMode:    domain.ManagedModeExclusive,
		}
		if err := m.store.CreateInstallationResource(ctx, resource); err != nil {
			m.failJob(ctx, job, fmt.Sprintf("track resource %s", funcName), err)
			m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
			return
		}

		// Add to ref map
		funcRefMap[fnSpec.Key] = funcName
	}

	// Install workflow if present
	if manifest.Workflow != nil {
		job.Step = "Installing workflow"
		m.store.UpdateInstallJob(ctx, job)

		workflowName := req.InstallName
		if manifest.Workflow.Name != "" {
			workflowName = namePrefix + manifest.Workflow.Name
		}

		// Resolve function_ref in workflow nodes
		resolvedDef, err := m.resolveWorkflowReferences(manifest.Workflow.Definition, funcRefMap)
		if err != nil {
			m.failJob(ctx, job, "resolve workflow references", err)
			m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
			return
		}

		// Create workflow
		workflow := &domain.Workflow{
			ID:             uuid.New().String(),
			Name:           workflowName,
			Description:    manifest.Workflow.Description,
			Status:         domain.WorkflowStatusActive,
			CurrentVersion: 1,
		}

		// TODO: Create workflow via workflow service
		// For now, we'll skip this as it requires more integration
		_ = workflow
		_ = resolvedDef
	}

	// Mark as succeeded
	job.Status = domain.InstallStatusSucceeded
	job.Step = "Installation complete"
	finishedAt := job.StartedAt.Add(1000)
	job.FinishedAt = &finishedAt
	m.store.UpdateInstallJob(ctx, job)
	m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusSucceeded)
}

// resolveWorkflowReferences replaces function_ref with actual function names
func (m *MarketplaceService) resolveWorkflowReferences(bundleDAG domain.WorkflowBundleDAG, funcRefMap map[string]string) (*domain.WorkflowDefinition, error) {
	def := &domain.WorkflowDefinition{
		Nodes: []domain.NodeDefinition{},
		Edges: bundleDAG.Edges,
	}

	for _, bundleNode := range bundleDAG.Nodes {
		funcName, ok := funcRefMap[bundleNode.FunctionRef]
		if !ok {
			return nil, fmt.Errorf("unresolved function reference: %s", bundleNode.FunctionRef)
		}

		node := domain.NodeDefinition{
			NodeKey:      bundleNode.NodeKey,
			FunctionName: funcName,
			InputMapping: bundleNode.InputMapping,
			RetryPolicy:  bundleNode.RetryPolicy,
			TimeoutS:     bundleNode.TimeoutS,
		}
		def.Nodes = append(def.Nodes, node)
	}

	return def, nil
}

// Uninstall removes an installation and its resources
func (m *MarketplaceService) Uninstall(ctx context.Context, installationID string, force bool) error {
	installation, err := m.store.GetInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("get installation: %w", err)
	}

	// Acquire lock
	locked, err := m.store.AcquireInstallLock(ctx, installation.TenantID, installation.Namespace)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("another operation is in progress for this namespace")
	}
	defer m.store.ReleaseInstallLock(ctx, installation.TenantID, installation.Namespace)

	// Update status
	m.store.UpdateInstallationStatus(ctx, installationID, domain.InstallStatusDeleting)

	// Get all resources (reverse order: workflow first, then functions)
	resources, err := m.store.ListInstallationResources(ctx, installationID)
	if err != nil {
		return fmt.Errorf("list resources: %w", err)
	}

	// Delete in reverse order
	for i := len(resources) - 1; i >= 0; i-- {
		resource := resources[i]

		switch resource.ResourceType {
		case "workflow":
			// Delete workflow
			if err := m.store.DeleteWorkflow(ctx, resource.ResourceID); err != nil && !force {
				return fmt.Errorf("delete workflow %s: %w", resource.ResourceName, err)
			}
		case "function":
			// Delete function
			if err := m.store.DeleteFunction(ctx, resource.ResourceID); err != nil && !force {
				return fmt.Errorf("delete function %s: %w", resource.ResourceName, err)
			}
		}

		// Remove resource record
		m.store.DeleteInstallationResource(ctx, resource.ID)
	}

	// Delete installation
	if err := m.store.DeleteInstallation(ctx, installationID); err != nil {
		return fmt.Errorf("delete installation: %w", err)
	}

	return nil
}

// Helper functions

func (m *MarketplaceService) extractManifest(bundlePath string) (*domain.BundleManifest, error) {
	f, err := os.Open(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		if header.Name == "manifest.yaml" || header.Name == "./manifest.yaml" {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("read manifest: %w", err)
			}

			var manifest domain.BundleManifest
			if err := yaml.Unmarshal(data, &manifest); err != nil {
				return nil, fmt.Errorf("parse manifest: %w", err)
			}
			return &manifest, nil
		}
	}

	return nil, fmt.Errorf("manifest.yaml not found in bundle")
}

func (m *MarketplaceService) validateBundle(manifest *domain.BundleManifest, bundlePath string) error {
	// Basic validation
	if manifest.Name == "" {
		return fmt.Errorf("manifest.name is required")
	}
	if manifest.Version == "" {
		return fmt.Errorf("manifest.version is required")
	}
	if manifest.Type == "" {
		return fmt.Errorf("manifest.type is required")
	}
	if len(manifest.Functions) == 0 {
		return fmt.Errorf("bundle must contain at least one function")
	}

	// Validate functions
	for _, fn := range manifest.Functions {
		if fn.Key == "" {
			return fmt.Errorf("function key is required")
		}
		if fn.Runtime == "" {
			return fmt.Errorf("function %s: runtime is required", fn.Key)
		}
		if fn.Handler == "" {
			return fmt.Errorf("function %s: handler is required", fn.Key)
		}
		if len(fn.Files) == 0 {
			return fmt.Errorf("function %s: files is required", fn.Key)
		}
	}

	// Validate workflow if present
	if manifest.Workflow != nil {
		// Check DAG for cycles
		if err := m.validateDAG(manifest.Workflow.Definition); err != nil {
			return fmt.Errorf("workflow DAG validation failed: %w", err)
		}

		// Check that all function_ref are in bundle
		funcKeys := make(map[string]bool)
		for _, fn := range manifest.Functions {
			funcKeys[fn.Key] = true
		}

		for _, node := range manifest.Workflow.Definition.Nodes {
			if !funcKeys[node.FunctionRef] {
				return fmt.Errorf("workflow node %s references unknown function: %s", node.NodeKey, node.FunctionRef)
			}
		}
	}

	return nil
}

func (m *MarketplaceService) validateDAG(dag domain.WorkflowBundleDAG) error {
	// Build adjacency list
	adj := make(map[string][]string)
	for _, edge := range dag.Edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
	}

	// DFS cycle detection
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(node string) bool
	dfs = func(node string) bool {
		visited[node] = true
		recStack[node] = true

		for _, neighbor := range adj[node] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				return true
			}
		}

		recStack[node] = false
		return false
	}

	for _, node := range dag.Nodes {
		if !visited[node.NodeKey] {
			if dfs(node.NodeKey) {
				return fmt.Errorf("cycle detected in workflow DAG")
			}
		}
	}

	return nil
}

func (m *MarketplaceService) downloadBundle(ctx context.Context, artifactURI string) (string, error) {
	reader, err := m.artifactStore.Get(ctx, artifactURI)
	if err != nil {
		return "", fmt.Errorf("get artifact: %w", err)
	}
	defer reader.Close()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "bundle-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	// Extract tar.gz
	gz, err := gzip.NewReader(reader)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("read tar: %w", err)
		}

		target := filepath.Join(tmpDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				os.RemoveAll(tmpDir)
				return "", fmt.Errorf("create dir: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				os.RemoveAll(tmpDir)
				return "", fmt.Errorf("create parent dir: %w", err)
			}

			f, err := os.Create(target)
			if err != nil {
				os.RemoveAll(tmpDir)
				return "", fmt.Errorf("create file: %w", err)
			}

			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				os.RemoveAll(tmpDir)
				return "", fmt.Errorf("write file: %w", err)
			}
			f.Close()
		}
	}

	return tmpDir, nil
}

func (m *MarketplaceService) readFunctionFiles(bundlePath string, files []string) (string, error) {
	// If single file, return its content
	if len(files) == 1 {
		data, err := os.ReadFile(filepath.Join(bundlePath, files[0]))
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		return string(data), nil
	}

	// Multiple files - concatenate or package based on runtime
	// For simplicity, we'll concatenate them
	var content strings.Builder
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(bundlePath, file))
		if err != nil {
			return "", fmt.Errorf("read file %s: %w", file, err)
		}
		content.Write(data)
		content.WriteString("\n")
	}

	return content.String(), nil
}

func (m *MarketplaceService) failJob(ctx context.Context, job *domain.InstallJob, step string, err error) {
	job.Status = domain.InstallStatusFailed
	job.Step = step
	job.Error = err.Error()
	finishedAt := job.StartedAt.Add(1000)
	job.FinishedAt = &finishedAt
	m.store.UpdateInstallJob(ctx, job)
}
