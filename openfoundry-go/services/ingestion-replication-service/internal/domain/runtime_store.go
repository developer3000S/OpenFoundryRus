package domain

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RuntimeEvent mirrors event_streaming::domain::runtime_store::RuntimeEvent.
type RuntimeEvent struct {
	ID          uuid.UUID       `json:"id"`
	StreamID    uuid.UUID       `json:"stream_id"`
	SequenceNo  int64           `json:"sequence_no"`
	Payload     json.RawMessage `json:"payload"`
	EventTime   time.Time       `json:"event_time"`
	ProcessedAt *time.Time      `json:"processed_at,omitempty"`
	ArchivedAt  *time.Time      `json:"archived_at,omitempty"`
	ArchivePath *string         `json:"archive_path,omitempty"`
}

// StreamCheckpointOffset mirrors the Rust struct of the same name. Tracks
// the last processed sequence number per (topology, stream) pair.
type StreamCheckpointOffset struct {
	StreamID       uuid.UUID `json:"stream_id"`
	LastSequenceNo int64     `json:"last_sequence_no"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// RuntimeStore is the trait the in-process engine consumes. Limited to
// the methods needed by the IRF-1 runtime — the wider Rust trait covers
// cold archives + per-stream activity which land in follow-up slices.
type RuntimeStore interface {
	AppendEvent(ctx context.Context, streamID uuid.UUID, payload json.RawMessage, eventTime time.Time) (RuntimeEvent, error)
	ListEventsSince(ctx context.Context, streamID uuid.UUID, afterSequenceNo int64) ([]RuntimeEvent, error)
	ListRecentEvents(ctx context.Context, streamID uuid.UUID, limit int) ([]RuntimeEvent, error)
	MarkEventsProcessed(ctx context.Context, eventIDs []uuid.UUID) error
	RestoreEvents(ctx context.Context, streamIDs []uuid.UUID, fromSequenceNo *int64) (int64, error)
	SaveTopologyOffsets(ctx context.Context, topologyID uuid.UUID, offsets map[uuid.UUID]int64) error
	LoadTopologyOffsets(ctx context.Context, topologyID uuid.UUID) (map[uuid.UUID]StreamCheckpointOffset, error)
}

// MemoryRuntimeStore mirrors event_streaming::domain::runtime_store::MemoryRuntimeStore.
//
// Goroutine-safe in-memory implementation used by tests + dev runs. Holds
// per-stream event slices ordered by sequence number, plus per-topology
// offset tables. Production deployments swap in a SQL-backed implementation.
type MemoryRuntimeStore struct {
	mu                   sync.RWMutex
	nextSequenceByStream map[uuid.UUID]int64
	eventsByStream       map[uuid.UUID][]RuntimeEvent
	topologyOffsets      map[uuid.UUID]map[uuid.UUID]StreamCheckpointOffset
	now                  func() time.Time
}

// NewMemoryRuntimeStore constructs an empty in-memory runtime store.
func NewMemoryRuntimeStore() *MemoryRuntimeStore {
	return &MemoryRuntimeStore{
		nextSequenceByStream: make(map[uuid.UUID]int64),
		eventsByStream:       make(map[uuid.UUID][]RuntimeEvent),
		topologyOffsets:      make(map[uuid.UUID]map[uuid.UUID]StreamCheckpointOffset),
		now:                  func() time.Time { return time.Now().UTC() },
	}
}

// SetClock overrides the clock used for processed/updated_at stamps.
// Test-only — production code should leave it at time.Now.
func (s *MemoryRuntimeStore) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	s.now = now
}

// AppendEvent assigns a per-stream sequence number and appends the event.
func (s *MemoryRuntimeStore) AppendEvent(_ context.Context, streamID uuid.UUID, payload json.RawMessage, eventTime time.Time) (RuntimeEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.nextSequenceByStream[streamID] + 1
	s.nextSequenceByStream[streamID] = next
	evt := RuntimeEvent{
		ID:         uuid.New(),
		StreamID:   streamID,
		SequenceNo: next,
		Payload:    payload,
		EventTime:  eventTime,
	}
	s.eventsByStream[streamID] = append(s.eventsByStream[streamID], evt)
	return evt, nil
}

// ListEventsSince returns events past the offset, skipping archived rows.
func (s *MemoryRuntimeStore) ListEventsSince(_ context.Context, streamID uuid.UUID, afterSequenceNo int64) ([]RuntimeEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RuntimeEvent, 0)
	for _, evt := range s.eventsByStream[streamID] {
		if evt.SequenceNo > afterSequenceNo && evt.ArchivedAt == nil {
			out = append(out, evt)
		}
	}
	return out, nil
}

// ListRecentEvents returns the trailing limit events for a stream.
func (s *MemoryRuntimeStore) ListRecentEvents(_ context.Context, streamID uuid.UUID, limit int) ([]RuntimeEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.eventsByStream[streamID]
	live := make([]RuntimeEvent, 0, len(all))
	for _, evt := range all {
		if evt.ArchivedAt == nil {
			live = append(live, evt)
		}
	}
	if limit > 0 && len(live) > limit {
		live = live[len(live)-limit:]
	}
	return live, nil
}

// MarkEventsProcessed stamps the supplied event ids with `processed_at`.
func (s *MemoryRuntimeStore) MarkEventsProcessed(_ context.Context, eventIDs []uuid.UUID) error {
	if len(eventIDs) == 0 {
		return nil
	}
	idSet := make(map[uuid.UUID]struct{}, len(eventIDs))
	for _, id := range eventIDs {
		idSet[id] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for streamID, events := range s.eventsByStream {
		for i := range events {
			if _, ok := idSet[events[i].ID]; ok {
				ts := now
				events[i].ProcessedAt = &ts
			}
		}
		s.eventsByStream[streamID] = events
	}
	return nil
}

// RestoreEvents clears processed/archived flags for events past
// `fromSequenceNo` (or all events if nil) on the supplied streams.
func (s *MemoryRuntimeStore) RestoreEvents(_ context.Context, streamIDs []uuid.UUID, fromSequenceNo *int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var restored int64
	for _, streamID := range streamIDs {
		events, ok := s.eventsByStream[streamID]
		if !ok {
			continue
		}
		for i := range events {
			matches := fromSequenceNo == nil || events[i].SequenceNo >= *fromSequenceNo
			if matches {
				events[i].ProcessedAt = nil
				events[i].ArchivedAt = nil
				events[i].ArchivePath = nil
				restored++
			}
		}
		s.eventsByStream[streamID] = events
	}
	return restored, nil
}

// SaveTopologyOffsets upserts the per-stream offsets for a topology.
func (s *MemoryRuntimeStore) SaveTopologyOffsets(_ context.Context, topologyID uuid.UUID, offsets map[uuid.UUID]int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	bucket, ok := s.topologyOffsets[topologyID]
	if !ok {
		bucket = make(map[uuid.UUID]StreamCheckpointOffset, len(offsets))
		s.topologyOffsets[topologyID] = bucket
	}
	for streamID, sequenceNo := range offsets {
		bucket[streamID] = StreamCheckpointOffset{
			StreamID:       streamID,
			LastSequenceNo: sequenceNo,
			UpdatedAt:      now,
		}
	}
	return nil
}

// LoadTopologyOffsets returns the persisted offset map for a topology.
func (s *MemoryRuntimeStore) LoadTopologyOffsets(_ context.Context, topologyID uuid.UUID) (map[uuid.UUID]StreamCheckpointOffset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket, ok := s.topologyOffsets[topologyID]
	if !ok {
		return map[uuid.UUID]StreamCheckpointOffset{}, nil
	}
	out := make(map[uuid.UUID]StreamCheckpointOffset, len(bucket))
	for k, v := range bucket {
		out[k] = v
	}
	return out, nil
}

// Compile-time assertion that MemoryRuntimeStore implements RuntimeStore.
var _ RuntimeStore = (*MemoryRuntimeStore)(nil)
