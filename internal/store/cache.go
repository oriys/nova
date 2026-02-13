package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oriys/nova/internal/domain"
)

// cacheEntry holds a cached value with an expiration time.
type cacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

func (e *cacheEntry[T]) expired() bool {
	return time.Now().After(e.expiresAt)
}

// CachedMetadataStore wraps a MetadataStore and caches hot-path reads used in the
// execution chain (Invoke / InvokeStream). Writes invalidate the affected cache
// entries immediately, and a short TTL acts as a safety net to bound the
// inconsistency window in multi-instance deployments or direct DB edits.
type CachedMetadataStore struct {
	MetadataStore // underlying store – all uncached methods delegate here

	ttl time.Duration

	// function metadata caches
	fnByName sync.Map // "tenantID\x00namespace\x00name" → *cacheEntry[*domain.Function]
	fnByID   sync.Map // "tenantID\x00namespace\x00id"   → *cacheEntry[*domain.Function]

	// reverse map: funcID → fnByName key, used for invalidation when only ID is known
	fnIDToNameKey sync.Map // funcID → string (fnByName key)

	// runtime config cache (rarely changes)
	runtimes sync.Map // runtimeID → *cacheEntry[*RuntimeRecord]

	// per-function caches keyed by function ID
	fnCode   sync.Map // funcID → *cacheEntry[*domain.FunctionCode]
	fnFiles  sync.Map // funcID → *cacheEntry[map[string][]byte]
	hasFiles sync.Map // funcID → *cacheEntry[bool]
	fnLayers sync.Map // funcID → *cacheEntry[[]*domain.Layer]
}

// DefaultCacheTTL is the default time-to-live for cache entries.
const DefaultCacheTTL = 5 * time.Second

type invocationPaginationDelegate interface {
	CountInvocationLogs(ctx context.Context, functionID string) (int64, error)
	CountAllInvocationLogs(ctx context.Context) (int64, error)
	ListAllInvocationLogsFiltered(ctx context.Context, limit, offset int, search, functionName string, success *bool) ([]*InvocationLog, error)
	CountAllInvocationLogsFiltered(ctx context.Context, search, functionName string, success *bool) (int64, error)
	GetAllInvocationLogsSummary(ctx context.Context) (*InvocationLogSummary, error)
	GetAllInvocationLogsSummaryFiltered(ctx context.Context, search, functionName string, success *bool) (*InvocationLogSummary, error)
}

type asyncInvocationPaginationDelegate interface {
	CountAsyncInvocations(ctx context.Context, statuses []AsyncInvocationStatus) (int64, error)
	CountFunctionAsyncInvocations(ctx context.Context, functionID string, statuses []AsyncInvocationStatus) (int64, error)
	GetAsyncInvocationSummary(ctx context.Context) (*AsyncInvocationSummary, error)
}

// NewCachedMetadataStore returns a MetadataStore that caches hot-path reads.
// Pass ttl <= 0 to use the default (5 s).
func NewCachedMetadataStore(underlying MetadataStore, ttl time.Duration) *CachedMetadataStore {
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &CachedMetadataStore{
		MetadataStore: underlying,
		ttl:           ttl,
	}
}

// UnderlyingMetadataStore exposes the wrapped metadata store so callers can
// recover optional capability interfaces (e.g. workflow/schedule stores).
func (c *CachedMetadataStore) UnderlyingMetadataStore() MetadataStore {
	if c == nil {
		return nil
	}
	return c.MetadataStore
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func fnNameKey(tenantID, namespace, name string) string {
	return tenantID + "\x00" + namespace + "\x00" + name
}

func fnIDKey(tenantID, namespace, id string) string {
	return tenantID + "\x00" + namespace + "\x00" + id
}

func cacheGet[T any](m *sync.Map, key string) (T, bool) {
	v, ok := m.Load(key)
	if !ok {
		var zero T
		return zero, false
	}
	entry := v.(*cacheEntry[T])
	if entry.expired() {
		m.Delete(key)
		var zero T
		return zero, false
	}
	return entry.value, true
}

func cachePut[T any](m *sync.Map, key string, value T, ttl time.Duration) {
	m.Store(key, &cacheEntry[T]{value: value, expiresAt: time.Now().Add(ttl)})
}

// ─── cached reads (hot path) ─────────────────────────────────────────────────

func (c *CachedMetadataStore) GetFunctionByName(ctx context.Context, name string) (*domain.Function, error) {
	scope := TenantScopeFromContext(ctx)
	key := fnNameKey(scope.TenantID, scope.Namespace, name)
	if fn, ok := cacheGet[*domain.Function](&c.fnByName, key); ok {
		return fn, nil
	}
	fn, err := c.MetadataStore.GetFunctionByName(ctx, name)
	if err != nil {
		return nil, err
	}
	cachePut(&c.fnByName, key, fn, c.ttl)
	// maintain reverse lookup for invalidation by ID
	idKey := fnIDKey(fn.TenantID, fn.Namespace, fn.ID)
	cachePut(&c.fnByID, idKey, fn, c.ttl)
	c.fnIDToNameKey.Store(fn.ID, key)
	return fn, nil
}

func (c *CachedMetadataStore) GetFunction(ctx context.Context, id string) (*domain.Function, error) {
	scope := TenantScopeFromContext(ctx)
	key := fnIDKey(scope.TenantID, scope.Namespace, id)
	if fn, ok := cacheGet[*domain.Function](&c.fnByID, key); ok {
		return fn, nil
	}
	fn, err := c.MetadataStore.GetFunction(ctx, id)
	if err != nil {
		return nil, err
	}
	cachePut(&c.fnByID, key, fn, c.ttl)
	nameKey := fnNameKey(fn.TenantID, fn.Namespace, fn.Name)
	cachePut(&c.fnByName, nameKey, fn, c.ttl)
	c.fnIDToNameKey.Store(fn.ID, nameKey)
	return fn, nil
}

func (c *CachedMetadataStore) GetRuntime(ctx context.Context, id string) (*RuntimeRecord, error) {
	if rt, ok := cacheGet[*RuntimeRecord](&c.runtimes, id); ok {
		return rt, nil
	}
	rt, err := c.MetadataStore.GetRuntime(ctx, id)
	if err != nil {
		return nil, err
	}
	cachePut(&c.runtimes, id, rt, c.ttl)
	return rt, nil
}

func (c *CachedMetadataStore) GetFunctionCode(ctx context.Context, funcID string) (*domain.FunctionCode, error) {
	if fc, ok := cacheGet[*domain.FunctionCode](&c.fnCode, funcID); ok {
		return fc, nil
	}
	fc, err := c.MetadataStore.GetFunctionCode(ctx, funcID)
	if err != nil {
		return nil, err
	}
	cachePut(&c.fnCode, funcID, fc, c.ttl)
	return fc, nil
}

func (c *CachedMetadataStore) HasFunctionFiles(ctx context.Context, funcID string) (bool, error) {
	if has, ok := cacheGet[bool](&c.hasFiles, funcID); ok {
		return has, nil
	}
	has, err := c.MetadataStore.HasFunctionFiles(ctx, funcID)
	if err != nil {
		return false, err
	}
	cachePut(&c.hasFiles, funcID, has, c.ttl)
	return has, nil
}

func (c *CachedMetadataStore) GetFunctionFiles(ctx context.Context, funcID string) (map[string][]byte, error) {
	if files, ok := cacheGet[map[string][]byte](&c.fnFiles, funcID); ok {
		return files, nil
	}
	files, err := c.MetadataStore.GetFunctionFiles(ctx, funcID)
	if err != nil {
		return nil, err
	}
	cachePut(&c.fnFiles, funcID, files, c.ttl)
	return files, nil
}

func (c *CachedMetadataStore) GetFunctionLayers(ctx context.Context, funcID string) ([]*domain.Layer, error) {
	if layers, ok := cacheGet[[]*domain.Layer](&c.fnLayers, funcID); ok {
		return layers, nil
	}
	layers, err := c.MetadataStore.GetFunctionLayers(ctx, funcID)
	if err != nil {
		return nil, err
	}
	cachePut(&c.fnLayers, funcID, layers, c.ttl)
	return layers, nil
}

// ─── uncached pagination delegates ──────────────────────────────────────────

func (c *CachedMetadataStore) CountInvocationLogs(ctx context.Context, functionID string) (int64, error) {
	store, ok := c.MetadataStore.(invocationPaginationDelegate)
	if !ok {
		return 0, fmt.Errorf("count invocation logs not supported")
	}
	return store.CountInvocationLogs(ctx, functionID)
}

func (c *CachedMetadataStore) CountAllInvocationLogs(ctx context.Context) (int64, error) {
	store, ok := c.MetadataStore.(invocationPaginationDelegate)
	if !ok {
		return 0, fmt.Errorf("count all invocation logs not supported")
	}
	return store.CountAllInvocationLogs(ctx)
}

func (c *CachedMetadataStore) ListAllInvocationLogsFiltered(
	ctx context.Context,
	limit,
	offset int,
	search,
	functionName string,
	success *bool,
) ([]*InvocationLog, error) {
	store, ok := c.MetadataStore.(invocationPaginationDelegate)
	if !ok {
		return nil, fmt.Errorf("list all invocation logs filtered not supported")
	}
	return store.ListAllInvocationLogsFiltered(ctx, limit, offset, search, functionName, success)
}

func (c *CachedMetadataStore) CountAllInvocationLogsFiltered(
	ctx context.Context,
	search,
	functionName string,
	success *bool,
) (int64, error) {
	store, ok := c.MetadataStore.(invocationPaginationDelegate)
	if !ok {
		return 0, fmt.Errorf("count all invocation logs filtered not supported")
	}
	return store.CountAllInvocationLogsFiltered(ctx, search, functionName, success)
}

func (c *CachedMetadataStore) GetAllInvocationLogsSummary(ctx context.Context) (*InvocationLogSummary, error) {
	store, ok := c.MetadataStore.(invocationPaginationDelegate)
	if !ok {
		return nil, fmt.Errorf("get all invocation logs summary not supported")
	}
	return store.GetAllInvocationLogsSummary(ctx)
}

func (c *CachedMetadataStore) GetAllInvocationLogsSummaryFiltered(
	ctx context.Context,
	search,
	functionName string,
	success *bool,
) (*InvocationLogSummary, error) {
	store, ok := c.MetadataStore.(invocationPaginationDelegate)
	if !ok {
		return nil, fmt.Errorf("get all invocation logs summary filtered not supported")
	}
	return store.GetAllInvocationLogsSummaryFiltered(ctx, search, functionName, success)
}

func (c *CachedMetadataStore) CountAsyncInvocations(ctx context.Context, statuses []AsyncInvocationStatus) (int64, error) {
	store, ok := c.MetadataStore.(asyncInvocationPaginationDelegate)
	if !ok {
		return 0, fmt.Errorf("count async invocations not supported")
	}
	return store.CountAsyncInvocations(ctx, statuses)
}

func (c *CachedMetadataStore) CountFunctionAsyncInvocations(ctx context.Context, functionID string, statuses []AsyncInvocationStatus) (int64, error) {
	store, ok := c.MetadataStore.(asyncInvocationPaginationDelegate)
	if !ok {
		return 0, fmt.Errorf("count function async invocations not supported")
	}
	return store.CountFunctionAsyncInvocations(ctx, functionID, statuses)
}

func (c *CachedMetadataStore) GetAsyncInvocationSummary(ctx context.Context) (*AsyncInvocationSummary, error) {
	store, ok := c.MetadataStore.(asyncInvocationPaginationDelegate)
	if !ok {
		return nil, fmt.Errorf("get async invocation summary not supported")
	}
	return store.GetAsyncInvocationSummary(ctx)
}

// ─── write-through invalidation ──────────────────────────────────────────────

// invalidateFunction removes all cache entries related to a function.
func (c *CachedMetadataStore) invalidateFunction(fn *domain.Function) {
	if fn == nil {
		return
	}
	nameKey := fnNameKey(fn.TenantID, fn.Namespace, fn.Name)
	idKey := fnIDKey(fn.TenantID, fn.Namespace, fn.ID)
	c.fnByName.Delete(nameKey)
	c.fnByID.Delete(idKey)
	c.fnIDToNameKey.Delete(fn.ID)
	c.fnCode.Delete(fn.ID)
	c.fnFiles.Delete(fn.ID)
	c.hasFiles.Delete(fn.ID)
	c.fnLayers.Delete(fn.ID)
}

// invalidateFunctionByID removes cache entries when only the function ID is known.
func (c *CachedMetadataStore) invalidateFunctionByID(funcID string) {
	// Try reverse lookup for name-based cache
	if nameKeyVal, ok := c.fnIDToNameKey.LoadAndDelete(funcID); ok {
		c.fnByName.Delete(nameKeyVal.(string))
	}
	// Delete all ID-keyed entries; we iterate fnByID because we may not know tenant/ns.
	// The key format is "tenant\x00ns\x00id" – extract ID after the second null byte.
	sep := "\x00"
	c.fnByID.Range(func(key, _ any) bool {
		k := key.(string)
		// Find the ID portion: everything after the second separator.
		if idx := strings.LastIndex(k, sep); idx >= 0 && k[idx+1:] == funcID {
			c.fnByID.Delete(key)
		}
		return true
	})
	c.fnCode.Delete(funcID)
	c.fnFiles.Delete(funcID)
	c.hasFiles.Delete(funcID)
	c.fnLayers.Delete(funcID)
}

// --- Function writes ---

func (c *CachedMetadataStore) SaveFunction(ctx context.Context, fn *domain.Function) error {
	err := c.MetadataStore.SaveFunction(ctx, fn)
	if err == nil {
		c.invalidateFunction(fn)
	}
	return err
}

func (c *CachedMetadataStore) UpdateFunction(ctx context.Context, name string, update *FunctionUpdate) (*domain.Function, error) {
	fn, err := c.MetadataStore.UpdateFunction(ctx, name, update)
	if err == nil {
		c.invalidateFunction(fn)
	}
	return fn, err
}

func (c *CachedMetadataStore) DeleteFunction(ctx context.Context, id string) error {
	// Capture name before deletion for cache invalidation
	scope := TenantScopeFromContext(ctx)
	nameKey := ""
	if nk, ok := c.fnIDToNameKey.Load(id); ok {
		nameKey = nk.(string)
	}

	err := c.MetadataStore.DeleteFunction(ctx, id)
	if err == nil {
		if nameKey != "" {
			c.fnByName.Delete(nameKey)
		}
		idKey := fnIDKey(scope.TenantID, scope.Namespace, id)
		c.fnByID.Delete(idKey)
		c.fnIDToNameKey.Delete(id)
		c.fnCode.Delete(id)
		c.fnFiles.Delete(id)
		c.hasFiles.Delete(id)
		c.fnLayers.Delete(id)
	}
	return err
}

// --- Code writes ---

func (c *CachedMetadataStore) SaveFunctionCode(ctx context.Context, funcID, sourceCode, sourceHash string) error {
	err := c.MetadataStore.SaveFunctionCode(ctx, funcID, sourceCode, sourceHash)
	if err == nil {
		c.fnCode.Delete(funcID)
	}
	return err
}

func (c *CachedMetadataStore) UpdateFunctionCode(ctx context.Context, funcID, sourceCode, sourceHash string) error {
	err := c.MetadataStore.UpdateFunctionCode(ctx, funcID, sourceCode, sourceHash)
	if err == nil {
		c.fnCode.Delete(funcID)
	}
	return err
}

func (c *CachedMetadataStore) UpdateCompileResult(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
	err := c.MetadataStore.UpdateCompileResult(ctx, funcID, binary, binaryHash, status, compileError)
	if err == nil {
		c.fnCode.Delete(funcID)
	}
	return err
}

func (c *CachedMetadataStore) DeleteFunctionCode(ctx context.Context, funcID string) error {
	err := c.MetadataStore.DeleteFunctionCode(ctx, funcID)
	if err == nil {
		c.fnCode.Delete(funcID)
	}
	return err
}

// --- File writes ---

func (c *CachedMetadataStore) SaveFunctionFiles(ctx context.Context, funcID string, files map[string][]byte) error {
	err := c.MetadataStore.SaveFunctionFiles(ctx, funcID, files)
	if err == nil {
		c.fnFiles.Delete(funcID)
		c.hasFiles.Delete(funcID)
	}
	return err
}

func (c *CachedMetadataStore) DeleteFunctionFiles(ctx context.Context, funcID string) error {
	err := c.MetadataStore.DeleteFunctionFiles(ctx, funcID)
	if err == nil {
		c.fnFiles.Delete(funcID)
		c.hasFiles.Delete(funcID)
	}
	return err
}

// --- Layer writes ---

func (c *CachedMetadataStore) SetFunctionLayers(ctx context.Context, funcID string, layerIDs []string) error {
	err := c.MetadataStore.SetFunctionLayers(ctx, funcID, layerIDs)
	if err == nil {
		c.fnLayers.Delete(funcID)
	}
	return err
}

// --- Runtime writes ---

func (c *CachedMetadataStore) SaveRuntime(ctx context.Context, rt *RuntimeRecord) error {
	err := c.MetadataStore.SaveRuntime(ctx, rt)
	if err == nil && rt != nil {
		c.runtimes.Delete(rt.ID)
	}
	return err
}

func (c *CachedMetadataStore) DeleteRuntime(ctx context.Context, id string) error {
	err := c.MetadataStore.DeleteRuntime(ctx, id)
	if err == nil {
		c.runtimes.Delete(id)
	}
	return err
}
