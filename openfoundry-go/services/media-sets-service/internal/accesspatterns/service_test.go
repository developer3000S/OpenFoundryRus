package accesspatterns

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	audittrail "github.com/openfoundry/openfoundry-go/libs/audit-trail"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/media-sets-service/internal/metrics"
	"github.com/openfoundry/openfoundry-go/services/media-sets-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/media-sets-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/media-sets-service/internal/transformclient"
)

// ── Fakes ────────────────────────────────────────────────────────

// fakeRepo implements Repository against in-memory maps. The
// InsertInvocation / WriteCacheRow methods record what landed so
// tests can assert side effects.
type fakeRepo struct {
	sets        map[string]*models.MediaSet
	items       map[string]*models.MediaItem
	patterns    map[string]*models.AccessPattern
	cacheRows   map[string]*repo.CachedOutput
	invocations []repo.LedgerEntry
	cacheWrites int
	beginTxErr  error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		sets:      map[string]*models.MediaSet{},
		items:     map[string]*models.MediaItem{},
		patterns:  map[string]*models.AccessPattern{},
		cacheRows: map[string]*repo.CachedOutput{},
	}
}

func cacheKey(patternID, itemRID, hash string) string {
	return patternID + "|" + itemRID + "|" + hash
}

func (f *fakeRepo) GetMediaSet(_ context.Context, rid string) (*models.MediaSet, error) {
	return f.sets[rid], nil
}
func (f *fakeRepo) GetMediaItem(_ context.Context, rid string) (*models.MediaItem, error) {
	return f.items[rid], nil
}
func (f *fakeRepo) GetAccessPattern(_ context.Context, id string) (*models.AccessPattern, error) {
	return f.patterns[id], nil
}
func (f *fakeRepo) GetAccessPatternByKind(_ context.Context, mediaSetRID, kind string) (*models.AccessPattern, error) {
	for _, p := range f.patterns {
		if p.MediaSetRID == mediaSetRID && p.Kind == kind {
			return p, nil
		}
	}
	return nil, nil
}
func (f *fakeRepo) ListAccessPatterns(_ context.Context, mediaSetRID string) ([]models.AccessPattern, error) {
	out := []models.AccessPattern{}
	for _, p := range f.patterns {
		if p.MediaSetRID == mediaSetRID {
			out = append(out, *p)
		}
	}
	return out, nil
}
func (f *fakeRepo) CreateAccessPattern(_ context.Context, p repo.CreateAccessPatternParams) (*models.AccessPattern, error) {
	for _, ex := range f.patterns {
		if ex.MediaSetRID == p.MediaSetRID && ex.Kind == p.Kind {
			return nil, &repo.ErrDuplicateKind{Kind: p.Kind, MediaSetRID: p.MediaSetRID}
		}
	}
	id := repo.MintAccessPatternID()
	row := &models.AccessPattern{
		ID: id, MediaSetRID: p.MediaSetRID, Kind: p.Kind,
		Params: json.RawMessage(p.Params), Persistence: p.Persistence,
		TTLSeconds: p.TTLSeconds, CreatedAt: time.Now(), CreatedBy: p.CreatedBy,
	}
	f.patterns[id] = row
	return row, nil
}
func (f *fakeRepo) LookupCachedOutput(_ context.Context, patternID, itemRID, paramsHash string) (*repo.CachedOutput, error) {
	row, ok := f.cacheRows[cacheKey(patternID, itemRID, paramsHash)]
	if !ok {
		return nil, nil
	}
	if row.ExpiresAt != nil && row.ExpiresAt.Before(time.Now()) {
		return nil, nil
	}
	return row, nil
}
func (f *fakeRepo) WriteCacheRow(_ context.Context, patternID, itemRID, paramsHash, storageURI, outputMime string, _ int64, expiresAt *time.Time) error {
	f.cacheRows[cacheKey(patternID, itemRID, paramsHash)] = &repo.CachedOutput{
		StorageURI: storageURI, OutputMime: outputMime, ExpiresAt: expiresAt,
	}
	f.cacheWrites++
	return nil
}
func (f *fakeRepo) InsertInvocation(_ context.Context, _ pgx.Tx, e repo.LedgerEntry) error {
	f.invocations = append(f.invocations, e)
	return nil
}
func (f *fakeRepo) BeginTx(_ context.Context) (pgx.Tx, error) {
	if f.beginTxErr != nil {
		return nil, f.beginTxErr
	}
	return &noopTx{}, nil
}

// noopTx implements pgx.Tx with the two methods the service layer
// actually calls (Commit / Rollback). Other methods panic — they are
// not reached by the service's own code paths in unit tests because
// the fake repo and recordingEmitter never touch the tx.
type noopTx struct{ pgx.Tx }

func (*noopTx) Commit(context.Context) error   { return nil }
func (*noopTx) Rollback(context.Context) error { return nil }

// fakeWorker captures the last request and returns a configured response.
type fakeWorker struct {
	calls   int
	lastReq transformclient.TransformRequest
	resp    *transformclient.TransformResponse
	respErr error
}

func (f *fakeWorker) Transform(_ context.Context, req transformclient.TransformRequest) (*transformclient.TransformResponse, error) {
	f.calls++
	f.lastReq = req
	return f.resp, f.respErr
}

// recordingEmitter is the no-op-with-audit-trail used by tests. Skips
// the real outbox SQL insert (no Postgres) and just records the event.
type recordingEmitter struct {
	events []audittrail.AuditEvent
	ctxs   []audittrail.AuditContext
}

func (r *recordingEmitter) emit(_ context.Context, _ pgx.Tx, event audittrail.AuditEvent, auditCtx audittrail.AuditContext) error {
	r.events = append(r.events, event)
	r.ctxs = append(r.ctxs, auditCtx)
	return nil
}

// ── Fixtures ─────────────────────────────────────────────────────

func newSvc(t *testing.T) (*Service, *fakeRepo, *fakeWorker, *recordingEmitter) {
	t.Helper()
	r := newFakeRepo()
	w := &fakeWorker{}
	em := &recordingEmitter{}
	o := observability.NewMetrics()
	s := New(r, w, metrics.New(o))
	s.EmitAudit = em.emit
	return s, r, w, em
}

func seedSet(r *fakeRepo, rid string) *models.MediaSet {
	s := &models.MediaSet{
		RID: rid, ProjectRID: "ri.proj.1", Name: "test", Schema: "IMAGE",
		Markings: []string{"PUBLIC"},
	}
	r.sets[rid] = s
	return s
}
func seedItem(r *fakeRepo, rid, setRID, mime string, size int64) *models.MediaItem {
	it := &models.MediaItem{RID: rid, MediaSetRID: setRID, MimeType: mime, SizeBytes: size}
	r.items[rid] = it
	return it
}
func seedPattern(r *fakeRepo, id, setRID, kind, persistence string, ttl int64) *models.AccessPattern {
	p := &models.AccessPattern{
		ID: id, MediaSetRID: setRID, Kind: kind,
		Params: json.RawMessage(`{"max_dim":64}`), Persistence: persistence,
		TTLSeconds: ttl, CreatedAt: time.Now(),
	}
	r.patterns[id] = p
	return p
}

// ── Tests ────────────────────────────────────────────────────────

func TestRegisterValidatesPersistence(t *testing.T) {
	t.Parallel()
	s, r, _, _ := newSvc(t)
	seedSet(r, "ri.set.1")

	_, err := s.Register(context.Background(), "ri.set.1", models.RegisterAccessPatternRequest{
		Kind: "thumbnail", Persistence: models.PersistenceCacheTTL,
	}, "actor", audittrail.AuditContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ttl_seconds is required")

	ttl := int64(60)
	row, err := s.Register(context.Background(), "ri.set.1", models.RegisterAccessPatternRequest{
		Kind: "thumbnail", Persistence: models.PersistenceCacheTTL, TTLSeconds: &ttl,
	}, "actor", audittrail.AuditContext{})
	require.NoError(t, err)
	assert.Equal(t, "thumbnail", row.Kind)
}

func TestRegisterRejectsDuplicateKind(t *testing.T) {
	t.Parallel()
	s, r, _, _ := newSvc(t)
	seedSet(r, "ri.set.1")

	_, err := s.Register(context.Background(), "ri.set.1", models.RegisterAccessPatternRequest{
		Kind: "thumbnail",
	}, "actor", audittrail.AuditContext{})
	require.NoError(t, err)

	_, err = s.Register(context.Background(), "ri.set.1", models.RegisterAccessPatternRequest{
		Kind: "thumbnail",
	}, "actor", audittrail.AuditContext{})
	require.Error(t, err)
	var bad *ErrBadRequest
	require.True(t, errors.As(err, &bad))
}

func TestRunRecomputeChargesViaWorkerAndEmitsAudit(t *testing.T) {
	t.Parallel()
	s, r, w, em := newSvc(t)
	seedSet(r, "ri.set.1")
	seedItem(r, "ri.item.1", "ri.set.1", "image/png", 1024*1024*1024) // 1 GB
	seedPattern(r, "ri.ap.1", "ri.set.1", "thumbnail", string(models.PersistenceRecompute), 0)

	w.resp = &transformclient.TransformResponse{
		Status:         transformclient.StatusOK,
		Kind:           "thumbnail",
		OutputMimeType: "image/png",
		ComputeSeconds: 40, // matches Foundry rate for thumbnail @ 1GB
	}

	resp, err := s.Run(context.Background(), RunInput{
		PatternID: "ri.ap.1", ItemRID: "ri.item.1", InvokedBy: "actor",
		AuditCtx: audittrail.AuditContext{ActorID: "actor"},
	})
	require.NoError(t, err)
	assert.False(t, resp.CacheHit)
	assert.Equal(t, uint64(40), resp.ComputeSeconds)
	assert.Equal(t, "image/png", resp.OutputMimeType)

	assert.Equal(t, 1, w.calls, "worker called exactly once on miss")
	assert.Equal(t, "thumbnail", w.lastReq.Kind)
	assert.Equal(t, "IMAGE", w.lastReq.Schema)

	require.Len(t, r.invocations, 1)
	assert.Equal(t, int64(40), r.invocations[0].ComputeSeconds)
	assert.False(t, r.invocations[0].CacheHit)

	require.Len(t, em.events, 1)
	assert.Equal(t, audittrail.KindMediaSetAccessPatternInvoked, em.events[0].Kind)
	assert.Equal(t, "thumbnail", em.events[0].AccessPattern)
	assert.Equal(t, "ri.set.1", em.events[0].ResourceRID)
	assert.Equal(t, []string{"PUBLIC"}, em.events[0].MarkingsAtEvent)
	require.Len(t, em.ctxs, 1)
	assert.Equal(t, "actor", em.ctxs[0].ActorID)
}

func TestRunCacheHitSkipsWorkerAndCharges0(t *testing.T) {
	t.Parallel()
	s, r, w, em := newSvc(t)
	seedSet(r, "ri.set.1")
	item := seedItem(r, "ri.item.1", "ri.set.1", "image/png", 4096)
	p := seedPattern(r, "ri.ap.1", "ri.set.1", "thumbnail", string(models.PersistencePersist), 0)

	hash := repo.ParamsHash(p.Params)
	r.cacheRows[cacheKey(p.ID, item.RID, hash)] = &repo.CachedOutput{
		StorageURI: "media-sets/ri.set.1/derived/thumbnail/ri.item.1/" + hash,
		OutputMime: "image/png",
	}

	resp, err := s.Run(context.Background(), RunInput{
		PatternID: p.ID, ItemRID: item.RID, InvokedBy: "actor",
	})
	require.NoError(t, err)
	assert.True(t, resp.CacheHit)
	assert.Equal(t, uint64(0), resp.ComputeSeconds)
	assert.Contains(t, resp.OutputStorageURI, "media-sets/ri.set.1/derived/thumbnail/")

	assert.Equal(t, 0, w.calls, "worker not called on cache hit")
	require.Len(t, r.invocations, 1)
	assert.True(t, r.invocations[0].CacheHit)
	assert.Equal(t, int64(0), r.invocations[0].ComputeSeconds)

	require.Len(t, em.events, 1, "audit emitted even on cache hit")
}

func TestRunCacheTTLMissWritesRowAndExpires(t *testing.T) {
	t.Parallel()
	s, r, w, _ := newSvc(t)
	seedSet(r, "ri.set.1")
	seedItem(r, "ri.item.1", "ri.set.1", "image/png", 4096)
	seedPattern(r, "ri.ap.1", "ri.set.1", "thumbnail", string(models.PersistenceCacheTTL), 60)

	w.resp = &transformclient.TransformResponse{
		Status: transformclient.StatusOK, Kind: "thumbnail",
		OutputMimeType: "image/png", ComputeSeconds: 1,
	}

	_, err := s.Run(context.Background(), RunInput{
		PatternID: "ri.ap.1", ItemRID: "ri.item.1", InvokedBy: "actor",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, r.cacheWrites, "TTL miss writes the cache row")

	// The cache row carries an expires_at ~now+60s.
	hash := repo.ParamsHash(json.RawMessage(`{"max_dim":64}`))
	row, ok := r.cacheRows[cacheKey("ri.ap.1", "ri.item.1", hash)]
	require.True(t, ok)
	require.NotNil(t, row.ExpiresAt)
	assert.WithinDuration(t, time.Now().Add(60*time.Second), *row.ExpiresAt, 5*time.Second)
}

func TestRunNotImplementedSurfacesReason(t *testing.T) {
	t.Parallel()
	s, r, w, em := newSvc(t)
	seedSet(r, "ri.set.1")
	seedItem(r, "ri.item.1", "ri.set.1", "image/png", 4096)
	seedPattern(r, "ri.ap.1", "ri.set.1", "ocr", string(models.PersistenceRecompute), 0)

	reason := "external binary `tesseract` is not wired yet"
	w.resp = &transformclient.TransformResponse{
		Status: transformclient.StatusNotImplemented, Kind: "ocr",
		OutputMimeType: "image/png", Reason: &reason,
	}

	resp, err := s.Run(context.Background(), RunInput{
		PatternID: "ri.ap.1", ItemRID: "ri.item.1", InvokedBy: "actor",
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(0), resp.ComputeSeconds)
	assert.Equal(t, "external binary `tesseract` is not wired yet", resp.NotImplementedReason)

	require.Len(t, r.invocations, 1, "even unimplemented kinds land an invocation row")
	assert.Equal(t, int64(0), r.invocations[0].ComputeSeconds)
	require.Len(t, em.events, 1, "audit emitted for unimplemented attempts")
}

func TestRunOKTransformationPassesRuntimeEnvelope(t *testing.T) {
	t.Parallel()
	s, r, w, _ := newSvc(t)
	seedSet(r, "ri.set.1")
	seedItem(r, "ri.item.1", "ri.set.1", "image/png", 3)
	seedPattern(r, "ri.ap.1", "ri.set.1", "thumbnail", string(models.PersistenceRecompute), 0)

	outBytes := base64.StdEncoding.EncodeToString([]byte("PNG"))
	w.resp = &transformclient.TransformResponse{
		Status: transformclient.StatusOK, Kind: "thumbnail",
		OutputMimeType: "image/webp", ComputeSeconds: 2, OutputBytesBase64: &outBytes,
	}

	resp, err := s.Run(context.Background(), RunInput{
		PatternID: "ri.ap.1", ItemRID: "ri.item.1", ItemBytes: []byte("raw"), InvokedBy: "actor",
	})
	require.NoError(t, err)

	assert.Equal(t, transformclient.StatusOK, w.resp.Status)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("raw")), w.lastReq.BytesBase64)
	assert.Equal(t, "image/png", w.lastReq.MimeType)
	assert.Equal(t, json.RawMessage(`{"max_dim":64}`), w.lastReq.Params)
	assert.Equal(t, "image/webp", resp.OutputMimeType)
	assert.Equal(t, outBytes, resp.OutputBytesBase64)
	assert.Empty(t, resp.OutputStorageURI, "RECOMPUTE should not synthesize persistent storage")
	require.Len(t, r.invocations, 1)
	assert.Equal(t, int64(2), r.invocations[0].ComputeSeconds)
}

func TestRunNotImplementedPassthroughForCatalogAndExternalReasons(t *testing.T) {
	t.Parallel()
	for name, reason := range map[string]string{
		"catalog_not_implemented": "Geo tile pyramids land in the geospatial-intelligence-service follow-up.",
		"external_binary":         "external binary `tesseract` is not wired yet — handler will land in a follow-up PR",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, r, w, em := newSvc(t)
			seedSet(r, "ri.set.1")
			seedItem(r, "ri.item.1", "ri.set.1", "image/png", 4096)
			seedPattern(r, "ri.ap.1", "ri.set.1", "ocr", string(models.PersistenceRecompute), 0)
			w.resp = &transformclient.TransformResponse{
				Status: transformclient.StatusNotImplemented, Kind: "ocr", OutputMimeType: "image/png", Reason: &reason,
			}

			resp, err := s.Run(context.Background(), RunInput{
				PatternID: "ri.ap.1", ItemRID: "ri.item.1", InvokedBy: "actor",
			})
			require.NoError(t, err)
			assert.Equal(t, uint64(0), resp.ComputeSeconds)
			assert.Equal(t, reason, resp.NotImplementedReason)
			assert.Empty(t, resp.OutputStorageURI)
			require.Len(t, r.invocations, 1)
			assert.Equal(t, int64(0), r.invocations[0].ComputeSeconds)
			require.Len(t, em.events, 1)
		})
	}
}

func TestRunWorkerErrorEnvelopeRemainsInspectable(t *testing.T) {
	t.Parallel()
	s, r, w, _ := newSvc(t)
	seedSet(r, "ri.set.1")
	seedItem(r, "ri.item.1", "ri.set.1", "image/png", 4096)
	seedPattern(r, "ri.ap.1", "ri.set.1", "bogus", string(models.PersistenceRecompute), 0)
	w.respErr = &transformclient.ErrorEnvelope{
		StatusCode: 400, Code: "MEDIA_TRANSFORM_UNKNOWN_KIND", Message: "unknown transformation kind `bogus`",
	}

	_, err := s.Run(context.Background(), RunInput{PatternID: "ri.ap.1", ItemRID: "ri.item.1", InvokedBy: "actor"})
	require.Error(t, err)
	env, ok := transformclient.AsErrorEnvelope(err)
	require.True(t, ok)
	assert.Equal(t, "MEDIA_TRANSFORM_UNKNOWN_KIND", env.Code)
	assert.Equal(t, 400, env.StatusCode)
	assert.Empty(t, r.invocations, "failed runtime invocations must not be charged")
}

func TestRunRejectsItemFromDifferentSet(t *testing.T) {
	t.Parallel()
	s, r, _, _ := newSvc(t)
	seedSet(r, "ri.set.1")
	seedSet(r, "ri.set.2")
	seedItem(r, "ri.item.1", "ri.set.2", "image/png", 4096)
	seedPattern(r, "ri.ap.1", "ri.set.1", "thumbnail", string(models.PersistenceRecompute), 0)

	_, err := s.Run(context.Background(), RunInput{
		PatternID: "ri.ap.1", ItemRID: "ri.item.1", InvokedBy: "actor",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different media set")
}

func TestRunMetricsBumped(t *testing.T) {
	t.Parallel()
	r := newFakeRepo()
	w := &fakeWorker{}
	em := &recordingEmitter{}
	o := observability.NewMetrics()
	mediaMetrics := metrics.New(o)
	s := New(r, w, mediaMetrics)
	s.EmitAudit = em.emit

	seedSet(r, "ri.set.1")
	seedItem(r, "ri.item.1", "ri.set.1", "image/png", 4096)
	seedPattern(r, "ri.ap.1", "ri.set.1", "thumbnail", string(models.PersistenceRecompute), 0)
	w.resp = &transformclient.TransformResponse{
		Status: transformclient.StatusOK, Kind: "thumbnail",
		OutputMimeType: "image/png", ComputeSeconds: 7,
	}

	_, err := s.Run(context.Background(), RunInput{
		PatternID: "ri.ap.1", ItemRID: "ri.item.1", InvokedBy: "actor",
	})
	require.NoError(t, err)

	// Pull the counter value via the prometheus testutil helper —
	// avoids HTTP scraping for a unit test.
	got := readCounter(t, mediaMetrics, "thumbnail", "IMAGE")
	assert.InDelta(t, 7.0, got, 0.0001)
}

// readCounter pulls a single (transformation, schema) counter sample
// via the prometheus client_model dto.
func readCounter(t *testing.T, m *metrics.Metrics, transformation, schema string) float64 {
	t.Helper()
	c, err := m.MediaComputeSecondsTotal.GetMetricWithLabelValues(transformation, schema)
	require.NoError(t, err)
	var pb dto.Metric
	require.NoError(t, c.Write(&pb))
	return pb.GetCounter().GetValue()
}
