package controlplane

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/service"
	"github.com/oriys/nova/internal/store"
)

// ─── UploadRuntime (3.8% → ~90%) ────────────────────────────────────────────

func makeExt4Data() []byte {
	buf := make([]byte, 2048)
	binary.LittleEndian.PutUint16(buf[1024+0x38:], 0xEF53)
	return buf
}

func buildUploadBody(t *testing.T, filename string, fileData []byte, metadata string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if fileData != nil {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(fileData)
	}
	if metadata != "" {
		writer.WriteField("metadata", metadata)
	}
	writer.Close()
	return body, writer.FormDataContentType()
}

func setupUploadHandler(t *testing.T, ms *mockMetadataStore, rootfsDir string) *http.ServeMux {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	s := store.NewStore(ms)
	h := &Handler{Store: s, RootfsDir: rootfsDir}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestUploadRuntime_Success(t *testing.T) {
	tmpDir := t.TempDir()
	ms := &mockMetadataStore{}
	mux := setupUploadHandler(t, ms, tmpDir)

	meta := `{"id":"myrt","name":"My Runtime","entrypoint":["python3"],"file_extension":".py"}`
	body, ct := buildUploadBody(t, "myrt.ext4", makeExt4Data(), meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusCreated)

	// Verify file was written
	if _, err := os.Stat(filepath.Join(tmpDir, "myrt.ext4")); err != nil {
		t.Fatal("expected file to exist")
	}
}

func TestUploadRuntime_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("metadata", `{"id":"test","name":"test","entrypoint":["python3"],"file_extension":".py"}`)
	writer.Close()

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "file field is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUploadRuntime_InvalidExtension(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	meta := `{"id":"test","name":"test","entrypoint":["python3"],"file_extension":".py"}`
	body, ct := buildUploadBody(t, "myrt.img", makeExt4Data(), meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), ".ext4 extension") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUploadRuntime_MissingMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	body, ct := buildUploadBody(t, "test.ext4", makeExt4Data(), "")

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "metadata field is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUploadRuntime_InvalidMetadataJSON(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	body, ct := buildUploadBody(t, "test.ext4", makeExt4Data(), "{bad json")

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "invalid metadata JSON") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUploadRuntime_MissingMetadataID(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	meta := `{"name":"test","entrypoint":["python3"],"file_extension":".py"}`
	body, ct := buildUploadBody(t, "test.ext4", makeExt4Data(), meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "metadata.id is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUploadRuntime_MissingMetadataName(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	meta := `{"id":"test","entrypoint":["python3"],"file_extension":".py"}`
	body, ct := buildUploadBody(t, "test.ext4", makeExt4Data(), meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "metadata.name is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUploadRuntime_MissingMetadataEntrypoint(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	meta := `{"id":"test","name":"test","file_extension":".py"}`
	body, ct := buildUploadBody(t, "test.ext4", makeExt4Data(), meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "metadata.entrypoint is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUploadRuntime_MissingMetadataFileExtension(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	meta := `{"id":"test","name":"test","entrypoint":["python3"]}`
	body, ct := buildUploadBody(t, "test.ext4", makeExt4Data(), meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "metadata.file_extension is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUploadRuntime_InvalidExt4Header(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	badData := make([]byte, 2048) // no ext4 magic
	meta := `{"id":"test","name":"test","entrypoint":["python3"],"file_extension":".py"}`
	body, ct := buildUploadBody(t, "test.ext4", badData, meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "not a valid ext4") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUploadRuntime_FileAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.ext4"), []byte("existing"), 0644)
	mux := setupUploadHandler(t, nil, tmpDir)

	meta := `{"id":"test","name":"test","entrypoint":["python3"],"file_extension":".py"}`
	body, ct := buildUploadBody(t, "test.ext4", makeExt4Data(), meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusConflict)
}

func TestUploadRuntime_StoreError(t *testing.T) {
	tmpDir := t.TempDir()
	ms := &mockMetadataStore{
		saveRuntimeFn: func(ctx context.Context, rt *store.RuntimeRecord) error {
			return fmt.Errorf("db error")
		},
	}
	mux := setupUploadHandler(t, ms, tmpDir)

	meta := `{"id":"test","name":"test","entrypoint":["python3"],"file_extension":".py"}`
	body, ct := buildUploadBody(t, "test.ext4", makeExt4Data(), meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)

	// Verify rollback: the file should have been removed
	if _, err := os.Stat(filepath.Join(tmpDir, "test.ext4")); !os.IsNotExist(err) {
		t.Fatal("expected file to be removed after store error")
	}
}

func TestUploadRuntime_DefaultVersion(t *testing.T) {
	tmpDir := t.TempDir()
	var saved *store.RuntimeRecord
	ms := &mockMetadataStore{
		saveRuntimeFn: func(ctx context.Context, rt *store.RuntimeRecord) error {
			saved = rt
			return nil
		},
	}
	mux := setupUploadHandler(t, ms, tmpDir)

	meta := `{"id":"test","name":"test","entrypoint":["python3"],"file_extension":".py"}`
	body, ct := buildUploadBody(t, "test.ext4", makeExt4Data(), meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusCreated)
	if saved.Version != "custom" {
		t.Fatalf("expected default version 'custom', got %q", saved.Version)
	}
}

func TestUploadRuntime_InvalidRuntimeID(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	// ID with only special chars sanitizes to empty
	meta := `{"id":"!!!","name":"test","entrypoint":["python3"],"file_extension":".py"}`
	body, ct := buildUploadBody(t, "test.ext4", makeExt4Data(), meta)

	req := httptest.NewRequest("POST", "/runtimes/upload", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "invalid runtime id") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUploadRuntime_BadMultipartForm(t *testing.T) {
	tmpDir := t.TempDir()
	mux := setupUploadHandler(t, nil, tmpDir)

	req := httptest.NewRequest("POST", "/runtimes/upload", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

// ─── GetMyPermissions (8.3% → ~90%) ─────────────────────────────────────────

func withIdentity(ctx context.Context, subject string, policies []domain.PolicyBinding) context.Context {
	return auth.WithIdentity(ctx, &auth.Identity{
		Subject:  subject,
		Policies: policies,
	})
}

func TestGetMyPermissions_WithIdentity(t *testing.T) {
	ms := &mockMetadataStore{
		resolveEffectivePermissionsFn: func(ctx context.Context, tenantID, subject string) ([]string, error) {
			return []string{"functions:read"}, nil
		},
		listRoleAssignmentsByPrincipalFn: func(ctx context.Context, tenantID string, pt domain.PrincipalType, pid string, limit, offset int) ([]*store.RoleAssignmentRecord, error) {
			return []*store.RoleAssignmentRecord{{ID: "ra1", RoleID: "viewer"}}, nil
		},
		listTenantButtonPermissionsFn: func(ctx context.Context, tenantID string) ([]*store.ButtonPermissionRecord, error) {
			return []*store.ButtonPermissionRecord{
				{PermissionKey: "functions:read", Enabled: true},
			}, nil
		},
		listTenantMenuPermissionsFn: func(ctx context.Context, tenantID string) ([]*store.MenuPermissionRecord, error) {
			return []*store.MenuPermissionRecord{
				{MenuKey: "dashboard", Enabled: true},
			}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)

	req := httptest.NewRequest("GET", "/rbac/my-permissions", nil)
	ctx := withIdentity(req.Context(), "user:john", []domain.PolicyBinding{
		{Role: domain.RoleViewer, Effect: domain.EffectAllow},
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp EffectivePermissionsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Subject != "user:john" {
		t.Fatalf("expected subject user:john, got %s", resp.Subject)
	}
	if len(resp.Permissions) == 0 {
		t.Fatal("expected permissions")
	}
}

func TestGetMyPermissions_AdminRole(t *testing.T) {
	ms := &mockMetadataStore{}
	_, mux := setupTestHandler(t, ms)

	req := httptest.NewRequest("GET", "/rbac/my-permissions", nil)
	ctx := withIdentity(req.Context(), "user:admin", []domain.PolicyBinding{
		{Role: domain.RoleAdmin, Effect: domain.EffectAllow},
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp EffectivePermissionsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Permissions) == 0 {
		t.Fatal("admin should have permissions")
	}
}

func TestGetMyPermissions_DenyEffect(t *testing.T) {
	ms := &mockMetadataStore{}
	_, mux := setupTestHandler(t, ms)

	req := httptest.NewRequest("GET", "/rbac/my-permissions", nil)
	ctx := withIdentity(req.Context(), "user:denied", []domain.PolicyBinding{
		{Role: domain.RoleViewer, Effect: domain.EffectDeny},
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetMyPermissions_DBPermErrors(t *testing.T) {
	ms := &mockMetadataStore{
		resolveEffectivePermissionsFn: func(ctx context.Context, tenantID, subject string) ([]string, error) {
			return nil, fmt.Errorf("db error")
		},
		listRoleAssignmentsByPrincipalFn: func(ctx context.Context, tenantID string, pt domain.PrincipalType, pid string, limit, offset int) ([]*store.RoleAssignmentRecord, error) {
			return nil, fmt.Errorf("db error")
		},
		listTenantButtonPermissionsFn: func(ctx context.Context, tenantID string) ([]*store.ButtonPermissionRecord, error) {
			return nil, fmt.Errorf("db error")
		},
		listTenantMenuPermissionsFn: func(ctx context.Context, tenantID string) ([]*store.MenuPermissionRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)

	req := httptest.NewRequest("GET", "/rbac/my-permissions", nil)
	ctx := withIdentity(req.Context(), "user:test", []domain.PolicyBinding{
		{Role: domain.RoleViewer, Effect: domain.EffectAllow},
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Still returns 200 because DB errors are gracefully handled
	expectStatus(t, w, http.StatusOK)
}

// ─── UpdateFunctionCode (13.4% → ~80%) ──────────────────────────────────────

func setupFuncCodeTestHandler(t *testing.T, ms *mockMetadataStore) (*Handler, *http.ServeMux) {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	s := store.NewStore(ms)
	b := &testBackend{snapshotDir: t.TempDir()}
	p := pool.NewPool(b, pool.PoolConfig{})
	t.Cleanup(p.Shutdown)
	h := &Handler{
		Store:   s,
		Pool:    p,
		Backend: b,
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestUpdateFunctionCode_JSONSuccess(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		updateCompileResultFn: func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		deleteFunctionFilesFn: func(ctx context.Context, funcID string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := `{"code":"print('hello')"}`
	req := httptest.NewRequest("PUT", "/functions/hello/code", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["compile_status"] != string(domain.CompileStatusNotRequired) {
		t.Fatalf("expected not_required, got %v", resp["compile_status"])
	}
}

func TestUpdateFunctionCode_JSONWithDependencyFiles(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		saveFunctionFilesFn: func(ctx context.Context, funcID string, files map[string][]byte) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		updateCompileResultFn: func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := `{"code":"print('hello')","entry_point":"handler.py","dependency_files":{"requirements.txt":"flask==2.0"}}`
	req := httptest.NewRequest("PUT", "/functions/hello/code", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["file_count"] == nil {
		t.Fatal("expected file_count in response")
	}
}

func TestUpdateFunctionCode_MultipartArchiveZip(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		saveFunctionFilesFn: func(ctx context.Context, funcID string, files map[string][]byte) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		updateCompileResultFn: func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	// Build a zip archive
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	fw, _ := zw.Create("handler.py")
	fw.Write([]byte("print('hello')"))
	zw.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("archive", "code.zip")
	part.Write(zipBuf.Bytes())
	writer.Close()

	req := httptest.NewRequest("PUT", "/functions/hello/code", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestUpdateFunctionCode_MultipartArchiveTarGz(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		saveFunctionFilesFn: func(ctx context.Context, funcID string, files map[string][]byte) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		updateCompileResultFn: func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	// Build a tar.gz archive
	var tarBuf bytes.Buffer
	gzw := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gzw)
	content := []byte("print('hello')")
	tw.WriteHeader(&tar.Header{Name: "handler.py", Size: int64(len(content)), Mode: 0644, Typeflag: tar.TypeReg})
	tw.Write(content)
	tw.Close()
	gzw.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("archive", "code.tar.gz")
	part.Write(tarBuf.Bytes())
	writer.Close()

	req := httptest.NewRequest("PUT", "/functions/hello/code", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestUpdateFunctionCode_MultipartCodeField(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		deleteFunctionFilesFn: func(ctx context.Context, funcID string) error {
			return nil
		},
		updateCompileResultFn: func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("code", "print('hello')")
	writer.Close()

	req := httptest.NewRequest("PUT", "/functions/hello/code", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestUpdateFunctionCode_MultipartCodeFile(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		deleteFunctionFilesFn: func(ctx context.Context, funcID string) error {
			return nil
		},
		updateCompileResultFn: func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("code", "handler.py")
	part.Write([]byte("print('hello')"))
	writer.Close()

	req := httptest.NewRequest("PUT", "/functions/hello/code", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestUpdateFunctionCode_MultipartNoArchiveNoCode(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("other", "value")
	writer.Close()

	req := httptest.NewRequest("PUT", "/functions/hello/code", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "either archive or code is required") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUpdateFunctionCode_UndetectableArchive(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("archive", "code.bin")
	part.Write([]byte("random data that is not an archive"))
	writer.Close()

	req := httptest.NewRequest("PUT", "/functions/hello/code", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "cannot detect archive type") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestUpdateFunctionCode_StoreUpdateCodeError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return fmt.Errorf("db error")
		},
		deleteFunctionFilesFn: func(ctx context.Context, funcID string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := `{"code":"print('hello')"}`
	req := httptest.NewRequest("PUT", "/functions/hello/code", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestUpdateFunctionCode_SaveFilesError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		saveFunctionFilesFn: func(ctx context.Context, funcID string, files map[string][]byte) error {
			return fmt.Errorf("db error")
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	// Build zip
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	fw, _ := zw.Create("handler.py")
	fw.Write([]byte("print('hello')"))
	zw.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("archive", "code.zip")
	part.Write(zipBuf.Bytes())
	writer.Close()

	req := httptest.NewRequest("PUT", "/functions/hello/code", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestUpdateFunctionCode_GoRuntime_PendingCompile(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "go"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		deleteFunctionFilesFn: func(ctx context.Context, funcID string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := `{"code":"package main\nfunc main() {}"}`
	req := httptest.NewRequest("PUT", "/functions/hello/code", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["compile_status"] != string(domain.CompileStatusPending) {
		t.Fatalf("expected pending, got %v", resp["compile_status"])
	}
}

func TestUpdateFunctionCode_WithEntryPoint(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python", Handler: "old_handler.py"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		deleteFunctionFilesFn: func(ctx context.Context, funcID string) error {
			return nil
		},
		updateCompileResultFn: func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := `{"code":"print('hello')","entry_point":"new_handler.py"}`
	req := httptest.NewRequest("PUT", "/functions/hello/code", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["entry_point"] != "new_handler.py" {
		t.Fatalf("expected entry_point new_handler.py, got %v", resp["entry_point"])
	}
}

// ─── DeleteFunction (33.3% → ~90%) ──────────────────────────────────────────

func TestDeleteFunction_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		deleteFunctionFn: func(ctx context.Context, id string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	req := httptest.NewRequest("DELETE", "/functions/hello", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Fatalf("expected deleted, got %v", resp["status"])
	}
}

func TestDeleteFunction_StoreDeleteError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		deleteFunctionFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("db error")
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	req := httptest.NewRequest("DELETE", "/functions/hello", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestDeleteFunction_StoreDeleteNotFoundError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		deleteFunctionFn: func(ctx context.Context, id string) error {
			return fmt.Errorf("function not found")
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	req := httptest.NewRequest("DELETE", "/functions/hello", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

// ─── UpdateFunction (43.3% → ~80%) ──────────────────────────────────────────

func TestUpdateFunction_WithCodeChange(t *testing.T) {
	code := "print('updated')"
	ms := &mockMetadataStore{
		updateFunctionFn: func(ctx context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := fmt.Sprintf(`{"code":"%s"}`, code)
	req := httptest.NewRequest("PATCH", "/functions/hello", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestUpdateFunction_CodeChangeUpdateError(t *testing.T) {
	code := "print('updated')"
	ms := &mockMetadataStore{
		updateFunctionFn: func(ctx context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return fmt.Errorf("db error")
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	body := fmt.Sprintf(`{"code":"%s"}`, code)
	req := httptest.NewRequest("PATCH", "/functions/hello", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── CreateSnapshot more branches (54.3%) ───────────────────────────────────

// CreateSnapshot with compiled binary path is tested via existing tests.
// The handler requires a real Pool to proceed past the nil FCAdapter check.

// ─── getExtension (66.7% → 100%) ────────────────────────────────────────────

func TestGetExtension(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"file.py", "py"},
		{"file.tar.gz", "gz"},
		{"no_extension", ""},
		{".hidden", "hidden"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := getExtension(tt.input)
			if got != tt.expected {
				t.Fatalf("getExtension(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ─── writePaginatedList (73.3% → ~100%) ──────────────────────────────────────

func TestWritePaginatedList_NegativeValues(t *testing.T) {
	w := httptest.NewRecorder()
	writePaginatedList(w, -1, -1, -1, -1, []string{})

	var resp paginatedListResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Pagination.Limit != 0 {
		t.Errorf("expected limit 0, got %d", resp.Pagination.Limit)
	}
	if resp.Pagination.Offset != 0 {
		t.Errorf("expected offset 0, got %d", resp.Pagination.Offset)
	}
	if resp.Pagination.Returned != 0 {
		t.Errorf("expected returned 0, got %d", resp.Pagination.Returned)
	}
	if resp.Pagination.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Pagination.Total)
	}
}

func TestWritePaginatedList_NegativeTotal(t *testing.T) {
	w := httptest.NewRecorder()
	writePaginatedList(w, 10, 0, 3, -5, []string{"a", "b", "c"})

	var resp paginatedListResponse
	json.NewDecoder(w.Body).Decode(&resp)
	// When total is negative, it should be set to returned
	if resp.Pagination.Total != 3 {
		t.Errorf("expected total 3 (from returned), got %d", resp.Pagination.Total)
	}
}

// ─── SetScalingPolicy uncovered branches (75.9%) ────────────────────────────

func TestSetScalingPolicy_NegativeCooldown(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"min_replicas":1,"max_replicas":10,"cooldown_scale_up_s":-1}`
	req := httptest.NewRequest("PUT", "/functions/hello/scaling", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestSetScalingPolicy_NegativeStep(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"min_replicas":1,"max_replicas":10,"scale_down_step":-1}`
	req := httptest.NewRequest("PUT", "/functions/hello/scaling", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestSetScalingPolicy_DefaultUtilization(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			if update.AutoScalePolicy.TargetUtilization != 0.7 {
				return nil, fmt.Errorf("expected default 0.7, got %f", update.AutoScalePolicy.TargetUtilization)
			}
			return &domain.Function{ID: "fn-1", Name: name, AutoScalePolicy: update.AutoScalePolicy}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	// target_utilization = 0 should be defaulted to 0.7
	body := `{"min_replicas":1,"max_replicas":10,"target_utilization":0}`
	req := httptest.NewRequest("PUT", "/functions/hello/scaling", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestSetScalingPolicy_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"min_replicas":1,"max_replicas":10,"target_utilization":0.7}`
	req := httptest.NewRequest("PUT", "/functions/hello/scaling", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── ToggleSchedule uncovered branches (67.4%) ─────────────────────────────

func TestToggleSchedule_Disable(t *testing.T) {
	enabled := false
	ss := &mockScheduleStore{
		getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
			return &store.Schedule{ID: id, FunctionName: "hello", Enabled: true}, nil
		},
		updateScheduleEnabledFn: func(_ context.Context, id string, e bool) error { return nil },
	}
	mux := setupScheduleHandlerFull(t, nil, ss)
	body := fmt.Sprintf(`{"enabled":%v}`, enabled)
	req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestToggleSchedule_GetScheduleErrorAfterUpdate(t *testing.T) {
	callCount := 0
	ss := &mockScheduleStore{
		getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
			callCount++
			if callCount == 1 {
				return &store.Schedule{ID: id, FunctionName: "hello", Enabled: false}, nil
			}
			return nil, fmt.Errorf("db error")
		},
		updateScheduleEnabledFn: func(_ context.Context, id string, enabled bool) error { return nil },
	}
	mux := setupScheduleHandlerFull(t, nil, ss)
	body := `{"enabled":true}`
	req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── CreateSchedule store error (75.9%) ─────────────────────────────────────

func TestCreateSchedule_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	ss := &mockScheduleStore{
		saveScheduleFn: func(_ context.Context, s *store.Schedule) error {
			return fmt.Errorf("db error")
		},
	}
	mux := setupScheduleHandlerFull(t, ms, ss)
	body := `{"cron_expression":"@every 5m"}`
	req := httptest.NewRequest("POST", "/functions/hello/schedules", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── extractZip / extractTarGz more branches (72% / 71%) ───────────────────

func TestExtractZip_CorruptedData(t *testing.T) {
	_, err := ExtractArchive([]byte("not a zip"), "zip")
	if err == nil {
		t.Fatal("expected error for corrupted zip")
	}
}

func TestExtractTarGz_CorruptedData(t *testing.T) {
	_, err := ExtractArchive([]byte("not a tar.gz"), "tar.gz")
	if err == nil {
		t.Fatal("expected error for corrupted tar.gz")
	}
}

func TestExtractTar_CorruptedData(t *testing.T) {
	_, err := ExtractArchive([]byte("not a tar"), "tar")
	if err == nil {
		t.Fatal("expected error for corrupted tar")
	}
}

func TestExtractTar_EmptyTar(t *testing.T) {
	// Create a valid but empty tar
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.Close()

	_, err := ExtractArchive(buf.Bytes(), "tar")
	if err == nil {
		t.Fatal("expected error for empty tar")
	}
}

func TestExtractTar_PlainTarWithFile(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := []byte("hello world")
	tw.WriteHeader(&tar.Header{Name: "test.txt", Size: int64(len(content)), Mode: 0644, Typeflag: tar.TypeReg})
	tw.Write(content)
	tw.Close()

	files, err := ExtractArchive(buf.Bytes(), "tar")
	if err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestExtractTarGz_EmptyTarGz(t *testing.T) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	tw.Close()
	gzw.Close()

	_, err := ExtractArchive(buf.Bytes(), "tar.gz")
	if err == nil {
		t.Fatal("expected error for empty tar.gz")
	}
}

func TestExtractZip_PathTraversal(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fw, _ := zw.Create("../../../etc/passwd")
	fw.Write([]byte("root:x"))
	// Need a valid file too since the traversal path gets cleaned
	fw2, _ := zw.Create("valid.txt")
	fw2.Write([]byte("ok"))
	zw.Close()

	files, err := ExtractArchive(buf.Bytes(), "zip")
	if err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	// Path traversal file should be sanitized; at minimum should not crash
	for path := range files {
		if strings.Contains(path, "..") {
			t.Fatalf("path traversal not sanitized: %s", path)
		}
	}
}

// ─── UpdateConfig uncovered branches (76.5%) ────────────────────────────────

func TestUpdateConfig_StoreSetError(t *testing.T) {
	ms := &mockMetadataStore{
		setConfigFn: func(ctx context.Context, key, value string) error {
			return fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"key1":"value1"}`
	req := httptest.NewRequest("PUT", "/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestUpdateConfig_BadJSON(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("PUT", "/config", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestUpdateConfig_GetConfigError(t *testing.T) {
	callCount := 0
	ms := &mockMetadataStore{
		setConfigFn: func(ctx context.Context, key, value string) error {
			return nil
		},
		getConfigFn: func(ctx context.Context) (map[string]string, error) {
			callCount++
			if callCount > 0 {
				return nil, fmt.Errorf("db error")
			}
			return map[string]string{}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"key1":"value1"}`
	req := httptest.NewRequest("PUT", "/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── ListFunctions_NilFuncs branch ──────────────────────────────────────────

func TestListFunctions_NilResult(t *testing.T) {
	ms := &mockMetadataStore{
		listFunctionsFn: func(ctx context.Context, limit, offset int) ([]*domain.Function, error) {
			return nil, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/functions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestListFunctions_NegativeLimit(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("GET", "/functions?limit=-5", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestListFunctions_NegativeOffset(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("GET", "/functions?offset=-5", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

// ─── CreateFunction quota exceeded ──────────────────────────────────────────

func TestCreateFunction_QuotaExceeded(t *testing.T) {
	ms := &mockMetadataStore{
		checkTenantAbsoluteQuotaFn: func(ctx context.Context, tenantID, dimension string, value int64) (*store.TenantQuotaDecision, error) {
			return &store.TenantQuotaDecision{Allowed: false}, nil
		},
	}
	_, mux := setupFuncTestHandler(t, ms)
	body := `{"name":"hello","runtime":"python","code":"print(1)"}`
	req := httptest.NewRequest("POST", "/functions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Should return 429 or similar quota exceeded response
	if w.Code == http.StatusCreated {
		t.Fatal("expected quota exceeded")
	}
}

func TestCreateFunction_QuotaCheckError(t *testing.T) {
	ms := &mockMetadataStore{
		checkTenantAbsoluteQuotaFn: func(ctx context.Context, tenantID, dimension string, value int64) (*store.TenantQuotaDecision, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupFuncTestHandler(t, ms)
	body := `{"name":"hello","runtime":"python","code":"print(1)"}`
	req := httptest.NewRequest("POST", "/functions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestCreateFunction_FunctionCountError(t *testing.T) {
	ms := &mockMetadataStore{
		getTenantFunctionCountFn: func(ctx context.Context, tenantID string) (int64, error) {
			return 0, fmt.Errorf("db error")
		},
	}
	_, mux := setupFuncTestHandler(t, ms)
	body := `{"name":"hello","runtime":"python","code":"print(1)"}`
	req := httptest.NewRequest("POST", "/functions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestCreateFunction_SaveError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return fmt.Errorf("db error")
		},
	}
	_, mux := setupFuncTestHandler(t, ms)
	body := `{"name":"hello","runtime":"python","code":"print(1)"}`
	req := httptest.NewRequest("POST", "/functions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── GetFunctionCode compile_error and binary_hash branches ─────────────────

func TestGetFunctionCode_WithCompileErrorAndBinaryHash(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		getFunctionCodeFn: func(ctx context.Context, funcID string) (*domain.FunctionCode, error) {
			return &domain.FunctionCode{
				FunctionID:    funcID,
				SourceCode:    "package main",
				SourceHash:    "abc",
				CompileStatus: domain.CompileStatusFailed,
				CompileError:  "build failed",
				BinaryHash:    "def456",
			}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/functions/hello/code", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["compile_error"] != "build failed" {
		t.Fatalf("expected compile_error, got %v", resp["compile_error"])
	}
	if resp["binary_hash"] != "def456" {
		t.Fatalf("expected binary_hash, got %v", resp["binary_hash"])
	}
}

// ─── ListFunctionFiles store error ──────────────────────────────────────────

func TestListFunctionFiles_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		listFunctionFilesFn: func(ctx context.Context, funcID string) ([]store.FunctionFileInfo, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/functions/hello/files", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── ListFunctionVersions store error ───────────────────────────────────────

func TestListFunctionVersions_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		listVersionsFn: func(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionVersion, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/functions/hello/versions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── ListRuntimes store nil result ──────────────────────────────────────────

func TestListRuntimes_NilResult(t *testing.T) {
	ms := &mockMetadataStore{
		listRuntimesFn: func(ctx context.Context, limit, offset int) ([]*store.RuntimeRecord, error) {
			return nil, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/runtimes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

// ─── RevokePermissionFromRole error branch ──────────────────────────────────

func TestRevokePermissionFromRole_Error(t *testing.T) {
	ms := &mockMetadataStore{
		revokePermissionFromRoleFn: func(ctx context.Context, roleID, permID string) error {
			return fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/rbac/roles/admin/permissions/perm1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── ListRoleAssignments error branch ───────────────────────────────────────

func TestListRoleAssignments_Error(t *testing.T) {
	ms := &mockMetadataStore{
		listRoleAssignmentsFn: func(ctx context.Context, tenantID string, limit, offset int) ([]*store.RoleAssignmentRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/rbac/assignments", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestListRoleAssignments_ByPrincipalError(t *testing.T) {
	ms := &mockMetadataStore{
		listRoleAssignmentsByPrincipalFn: func(ctx context.Context, tenantID string, pt domain.PrincipalType, pid string, limit, offset int) ([]*store.RoleAssignmentRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/rbac/assignments?principal_type=user&principal_id=user1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── ListRoles error branch ─────────────────────────────────────────────────

func TestListRoles_Error(t *testing.T) {
	ms := &mockMetadataStore{
		listRolesFn: func(ctx context.Context, tenantID string, limit, offset int) ([]*store.RoleRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/rbac/roles", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── ListPermissions error branch ───────────────────────────────────────────

func TestListPermissions_Error(t *testing.T) {
	ms := &mockMetadataStore{
		listPermissionsFn: func(ctx context.Context, limit, offset int) ([]*store.PermissionRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/rbac/permissions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── CreatePermission store error ───────────────────────────────────────────

func TestCreatePermission_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		createPermissionFn: func(ctx context.Context, perm *store.PermissionRecord) (*store.PermissionRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"id":"perm1","code":"fn:read","resource_type":"function","action":"read"}`
	req := httptest.NewRequest("POST", "/rbac/permissions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── DeleteRuntime empty ID branch ──────────────────────────────────────────

func TestDeleteRuntime_EmptyID(t *testing.T) {
	// The route requires an ID, so this tests the path with empty
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("DELETE", "/runtimes/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Without path value, the mux won't match the route (405 or 404)
	if w.Code == http.StatusOK {
		t.Fatal("expected error for empty runtime id")
	}
}

// ─── detectEntryPoint additional branch (common name fallback) ──────────────

func TestDetectEntryPoint_CommonNameFallback(t *testing.T) {
	// File that matches common name pattern but unknown runtime
	files := map[string][]byte{
		"src/main.py": []byte("code"),
	}
	ep := detectEntryPoint(files, "unknown")
	if ep != "src/main.py" {
		t.Fatalf("expected src/main.py, got %s", ep)
	}
}

// ─── MultipartArchive with explicit archive_type ────────────────────────────

func TestUpdateFunctionCode_MultipartWithExplicitArchiveType(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		saveFunctionFilesFn: func(ctx context.Context, funcID string, files map[string][]byte) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		updateCompileResultFn: func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	fw, _ := zw.Create("handler.py")
	fw.Write([]byte("print('hello')"))
	zw.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("archive", "code.bin")
	part.Write(zipBuf.Bytes())
	writer.WriteField("archive_type", "zip")
	writer.Close()

	req := httptest.NewRequest("PUT", "/functions/hello/code", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

// ─── MultipartArchive .tgz extension ────────────────────────────────────────

func TestUpdateFunctionCode_MultipartTgzExtension(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		saveFunctionFilesFn: func(ctx context.Context, funcID string, files map[string][]byte) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		updateCompileResultFn: func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	var tarBuf bytes.Buffer
	gzw := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gzw)
	content := []byte("print('hello')")
	tw.WriteHeader(&tar.Header{Name: "handler.py", Size: int64(len(content)), Mode: 0644, Typeflag: tar.TypeReg})
	tw.Write(content)
	tw.Close()
	gzw.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("archive", "code.tgz")
	part.Write(tarBuf.Bytes())
	writer.Close()

	req := httptest.NewRequest("PUT", "/functions/hello/code", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

// ─── MultipartArchive .tar extension ────────────────────────────────────────

func TestUpdateFunctionCode_MultipartTarExtension(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		updateFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
			return nil
		},
		saveFunctionFilesFn: func(ctx context.Context, funcID string, files map[string][]byte) error {
			return nil
		},
		saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
			return nil
		},
		updateCompileResultFn: func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
			return nil
		},
	}
	_, mux := setupFuncCodeTestHandler(t, ms)

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	content := []byte("print('hello')")
	tw.WriteHeader(&tar.Header{Name: "handler.py", Size: int64(len(content)), Mode: 0644, Typeflag: tar.TypeReg})
	tw.Write(content)
	tw.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("archive", "code.tar")
	part.Write(tarBuf.Bytes())
	writer.Close()

	req := httptest.NewRequest("PUT", "/functions/hello/code", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

// ─── Suppress unused import warning for service ─────────────────────────────
var _ = service.NewFunctionService
