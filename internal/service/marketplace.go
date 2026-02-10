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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/pkg/crypto"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/workflow"
	"gopkg.in/yaml.v3"
)

// MarketplaceService handles marketplace operations
type MarketplaceService struct {
	store         *store.PostgresStore
	functionSvc   *FunctionService
	workflowSvc   *workflow.Service
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

func NewMarketplaceService(store *store.PostgresStore, functionSvc *FunctionService, workflowSvc *workflow.Service, artifactStore ArtifactStore) *MarketplaceService {
	return &MarketplaceService{
		store:         store,
		functionSvc:   functionSvc,
		workflowSvc:   workflowSvc,
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

var bundleKeySanitizer = regexp.MustCompile(`[^a-z0-9_-]+`)

// PublishFromResources builds a release bundle from existing functions/workflows and publishes it.
func (m *MarketplaceService) PublishFromResources(
	ctx context.Context,
	appSlug string,
	version string,
	owner string,
	functionNames []string,
	workflowNames []string,
) (*domain.AppRelease, error) {
	bundlePath, err := m.buildBundleFromResources(ctx, appSlug, version, functionNames, workflowNames)
	if err != nil {
		return nil, err
	}
	defer os.Remove(bundlePath)

	return m.PublishBundle(ctx, appSlug, version, bundlePath, owner)
}

func normalizeNames(input []string) []string {
	out := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, raw := range input {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func sanitizeBundleKey(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = bundleKeySanitizer.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		return "fn"
	}
	return normalized
}

func runtimeSourceFilename(runtime domain.Runtime) string {
	rt := strings.ToLower(string(runtime))
	switch {
	case strings.HasPrefix(rt, "python"):
		return "handler.py"
	case strings.HasPrefix(rt, "node"):
		return "handler.js"
	case strings.HasPrefix(rt, "deno"):
		return "handler.ts"
	case strings.HasPrefix(rt, "go"):
		return "main.go"
	case strings.HasPrefix(rt, "rust"):
		return "main.rs"
	case strings.HasPrefix(rt, "ruby"):
		return "handler.rb"
	case strings.HasPrefix(rt, "java"):
		return "Main.java"
	case strings.HasPrefix(rt, "bun"):
		return "handler.ts"
	case strings.HasPrefix(rt, "php"):
		return "handler.php"
	case strings.HasPrefix(rt, "dotnet"):
		return "Handler.cs"
	case strings.HasPrefix(rt, "elixir"):
		return "handler.exs"
	case strings.HasPrefix(rt, "kotlin"):
		return "Main.kt"
	case strings.HasPrefix(rt, "swift"):
		return "main.swift"
	case strings.HasPrefix(rt, "zig"):
		return "main.zig"
	case strings.HasPrefix(rt, "lua"):
		return "handler.lua"
	case strings.HasPrefix(rt, "perl"):
		return "handler.pl"
	case strings.HasPrefix(rt, "julia"):
		return "handler.jl"
	case rt == "r":
		return "handler.r"
	case strings.HasPrefix(rt, "scala"):
		return "Main.scala"
	case strings.HasPrefix(rt, "wasm"):
		return "module.wasm"
	default:
		return "handler.txt"
	}
}

func (m *MarketplaceService) buildBundleFromResources(
	ctx context.Context,
	appSlug string,
	version string,
	functionNames []string,
	workflowNames []string,
) (string, error) {
	functionNames = normalizeNames(functionNames)
	workflowNames = normalizeNames(workflowNames)

	if len(functionNames) == 0 && len(workflowNames) == 0 {
		return "", fmt.Errorf("at least one function or workflow must be selected")
	}
	if len(workflowNames) > 1 {
		return "", fmt.Errorf("only one workflow can be selected per release")
	}

	files := make(map[string]string)
	functionSpecs := make([]domain.FunctionSpec, 0, len(functionNames)+8)
	functionKeyByName := make(map[string]string)
	usedKeys := make(map[string]struct{})

	addFunction := func(functionName string) error {
		if _, exists := functionKeyByName[functionName]; exists {
			return nil
		}

		fn, err := m.store.GetFunctionByName(ctx, functionName)
		if err != nil {
			return fmt.Errorf("function not found: %s", functionName)
		}

		code, err := m.store.GetFunctionCode(ctx, fn.ID)
		if err != nil {
			return fmt.Errorf("read function code %s: %w", functionName, err)
		}
		if strings.TrimSpace(code.SourceCode) == "" {
			return fmt.Errorf("function %s has empty source code", functionName)
		}

		baseKey := sanitizeBundleKey(fn.Name)
		key := baseKey
		suffix := 2
		for {
			if _, ok := usedKeys[key]; !ok {
				break
			}
			key = fmt.Sprintf("%s-%d", baseKey, suffix)
			suffix++
		}
		usedKeys[key] = struct{}{}
		functionKeyByName[fn.Name] = key

		filePath := fmt.Sprintf("functions/%s/%s", key, runtimeSourceFilename(fn.Runtime))
		files[filePath] = code.SourceCode

		spec := domain.FunctionSpec{
			Key:         key,
			Runtime:     fn.Runtime,
			Handler:     fn.Handler,
			Files:       []string{filePath},
			MemoryMB:    fn.MemoryMB,
			TimeoutS:    fn.TimeoutS,
			EnvVars:     fn.EnvVars,
			Description: fmt.Sprintf("Imported from function %s", fn.Name),
		}
		functionSpecs = append(functionSpecs, spec)
		return nil
	}

	for _, name := range functionNames {
		if err := addFunction(name); err != nil {
			return "", err
		}
	}

	var workflowSpec *domain.WorkflowSpec
	if len(workflowNames) == 1 {
		wfName := workflowNames[0]
		wf, err := m.store.GetWorkflowByName(ctx, wfName)
		if err != nil {
			return "", fmt.Errorf("workflow not found: %s", wfName)
		}
		if wf.CurrentVersion <= 0 {
			return "", fmt.Errorf("workflow %s has no published version", wfName)
		}

		versionEntry, err := m.store.GetWorkflowVersionByNumber(ctx, wf.ID, wf.CurrentVersion)
		if err != nil {
			return "", fmt.Errorf("read workflow version %s@%d: %w", wfName, wf.CurrentVersion, err)
		}

		var def domain.WorkflowDefinition
		if err := json.Unmarshal(versionEntry.Definition, &def); err != nil {
			return "", fmt.Errorf("parse workflow definition %s: %w", wfName, err)
		}

		bundleNodes := make([]domain.BundleNodeDefinition, 0, len(def.Nodes))
		for _, node := range def.Nodes {
			if node.FunctionName == "" {
				return "", fmt.Errorf("workflow %s contains node %s without function_name", wfName, node.NodeKey)
			}
			if err := addFunction(node.FunctionName); err != nil {
				return "", err
			}

			functionRef, ok := functionKeyByName[node.FunctionName]
			if !ok {
				return "", fmt.Errorf("failed to resolve function reference for %s", node.FunctionName)
			}

			bundleNodes = append(bundleNodes, domain.BundleNodeDefinition{
				NodeKey:      node.NodeKey,
				FunctionRef:  functionRef,
				InputMapping: node.InputMapping,
				RetryPolicy:  node.RetryPolicy,
				TimeoutS:     node.TimeoutS,
			})
		}

		workflowSpec = &domain.WorkflowSpec{
			Description: wf.Description,
			Definition: domain.WorkflowBundleDAG{
				Nodes: bundleNodes,
				Edges: def.Edges,
			},
		}
	}

	if len(functionSpecs) == 0 {
		return "", fmt.Errorf("no functions were selected for this release")
	}

	sort.Slice(functionSpecs, func(i, j int) bool {
		return functionSpecs[i].Key < functionSpecs[j].Key
	})

	manifestName := appSlug
	manifestDescription := "Generated from existing resources"
	if app, err := m.store.GetApp(ctx, appSlug); err == nil {
		if strings.TrimSpace(app.Title) != "" {
			manifestName = app.Title
		}
		if strings.TrimSpace(app.Summary) != "" {
			manifestDescription = app.Summary
		}
	}

	manifestType := domain.BundleTypeFunction
	if workflowSpec != nil {
		manifestType = domain.BundleTypeWorkflow
	}

	manifest := &domain.BundleManifest{
		Name:        manifestName,
		Version:     version,
		Type:        manifestType,
		Description: manifestDescription,
		Functions:   functionSpecs,
		Workflow:    workflowSpec,
	}

	return writeGeneratedBundle(manifest, files)
}

func writeGeneratedBundle(manifest *domain.BundleManifest, files map[string]string) (string, error) {
	tmpFile, err := os.CreateTemp("", "bundle-generated-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("create temp bundle: %w", err)
	}
	defer tmpFile.Close()

	gz := gzip.NewWriter(tmpFile)
	tw := tar.NewWriter(gz)

	writeFile := func(name string, content []byte) error {
		cleanName := filepath.ToSlash(filepath.Clean(name))
		if strings.HasPrefix(cleanName, "../") || cleanName == ".." || cleanName == "." {
			return fmt.Errorf("invalid bundle path: %s", name)
		}
		header := &tar.Header{
			Name:    cleanName,
			Mode:    0644,
			Size:    int64(len(content)),
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tw.Write(content); err != nil {
			return err
		}
		return nil
	}

	manifestYAML, err := yaml.Marshal(manifest)
	if err != nil {
		tw.Close()
		gz.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("marshal manifest yaml: %w", err)
	}
	if err := writeFile("manifest.yaml", manifestYAML); err != nil {
		tw.Close()
		gz.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write manifest.yaml: %w", err)
	}

	filePaths := make([]string, 0, len(files))
	for path := range files {
		filePaths = append(filePaths, path)
	}
	sort.Strings(filePaths)

	for _, path := range filePaths {
		if err := writeFile(path, []byte(files[path])); err != nil {
			tw.Close()
			gz.Close()
			os.Remove(tmpFile.Name())
			return "", fmt.Errorf("write bundle file %s: %w", path, err)
		}
	}

	if err := tw.Close(); err != nil {
		gz.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("finalize tar writer: %w", err)
	}
	if err := gz.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("finalize gzip writer: %w", err)
	}

	return tmpFile.Name(), nil
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

	// Check quota - simplified check for now
	// TODO: Implement proper quota check when tenant quota methods are exposed at store level
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
		CreatedBy:   "system", // TODO: Extract from auth context when available
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
	ctx = store.WithTenantScope(ctx, installation.TenantID, installation.Namespace)

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
			Name:     funcName,
			Runtime:  string(fnSpec.Runtime),
			Handler:  fnSpec.Handler,
			Code:     code,
			MemoryMB: fnSpec.MemoryMB,
			TimeoutS: fnSpec.TimeoutS,
			EnvVars:  fnSpec.EnvVars,
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
		if m.workflowSvc == nil {
			m.failJob(ctx, job, "install workflow", fmt.Errorf("workflow service is not initialized"))
			m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
			return
		}

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

		// Create workflow metadata and publish DAG version.
		if _, err := m.workflowSvc.CreateWorkflow(ctx, workflowName, manifest.Workflow.Description); err != nil {
			m.failJob(ctx, job, fmt.Sprintf("create workflow %s", workflowName), err)
			m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
			return
		}
		if _, err := m.workflowSvc.PublishVersion(ctx, workflowName, resolvedDef); err != nil {
			m.failJob(ctx, job, fmt.Sprintf("publish workflow %s", workflowName), err)
			m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
			return
		}

		wf, err := m.workflowSvc.GetWorkflow(ctx, workflowName)
		if err != nil {
			m.failJob(ctx, job, fmt.Sprintf("fetch workflow %s", workflowName), err)
			m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
			return
		}

		defJSON, _ := json.Marshal(resolvedDef)
		resource := &domain.InstallationResource{
			InstallationID: installation.ID,
			ResourceType:   "workflow",
			ResourceName:   workflowName,
			ResourceID:     wf.ID,
			ContentDigest:  crypto.HashString(string(defJSON)),
			ManagedMode:    domain.ManagedModeExclusive,
		}
		if err := m.store.CreateInstallationResource(ctx, resource); err != nil {
			m.failJob(ctx, job, fmt.Sprintf("track workflow resource %s", workflowName), err)
			m.store.UpdateInstallationStatus(ctx, installation.ID, domain.InstallStatusFailed)
			return
		}
	}

	// Mark as succeeded
	job.Status = domain.InstallStatusSucceeded
	job.Step = "Installation complete"
	finishedAt := time.Now()
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

		// Prevent path traversal attacks
		if strings.Contains(header.Name, "..") {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("invalid path in archive: %s", header.Name)
		}

		target := filepath.Join(tmpDir, filepath.Clean(header.Name))

		// Ensure target is within tmpDir
		if !strings.HasPrefix(target, filepath.Clean(tmpDir)+string(os.PathSeparator)) {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("path traversal detected: %s", header.Name)
		}

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
	finishedAt := time.Now()
	job.FinishedAt = &finishedAt
	m.store.UpdateInstallJob(ctx, job)
}
