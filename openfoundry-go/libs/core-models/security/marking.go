// Package security carries marking / classification primitives that
// propagate downstream along dataset lineage.
//
// Wire format mirrors the Rust source verbatim:
//
//	{ "kind": "direct" }
//	{ "kind": "inherited_from_upstream", "upstream_rid": "ri.foundry.main.dataset...." }
package security

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
)

// MarkingID is the stable identifier of a marking definition.
type MarkingID struct {
	uuid.UUID
}

// NewMarkingID mints a fresh v7 marking id.
func NewMarkingID() MarkingID { return MarkingID{UUID: ids.New()} }

// ParseMarkingID parses a UUID string into a MarkingID.
func ParseMarkingID(s string) (MarkingID, error) {
	parsed, err := uuid.Parse(s)
	if err != nil {
		return MarkingID{}, fmt.Errorf("invalid MarkingId: %s", s)
	}
	return MarkingID{UUID: parsed}, nil
}

func (m MarkingID) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.UUID.String())
}

func (m *MarkingID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseMarkingID(s)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// MarkingSourceKind enumerates why a marking applies (direct vs inherited).
type MarkingSourceKind string

const (
	SourceDirect                MarkingSourceKind = "direct"
	SourceInheritedFromUpstream MarkingSourceKind = "inherited_from_upstream"
)

// MarkingSource describes why a marking applies to a dataset. The
// `kind` discriminator + optional `upstream_rid` payload is preserved
// verbatim from the Rust serde representation.
type MarkingSource struct {
	Kind         MarkingSourceKind `json:"kind"`
	UpstreamRID  string            `json:"upstream_rid,omitempty"`
}

// Direct returns the source for a directly-attached marking.
func Direct() MarkingSource { return MarkingSource{Kind: SourceDirect} }

// InheritedFrom returns the source for a marking that propagated
// downstream from `upstreamRID`.
func InheritedFrom(upstreamRID string) MarkingSource {
	return MarkingSource{Kind: SourceInheritedFromUpstream, UpstreamRID: upstreamRID}
}

// IsDirect reports whether the marking was attached directly.
func (s MarkingSource) IsDirect() bool { return s.Kind == SourceDirect }

// EffectiveMarking is the per-dataset projection: a marking + the
// reason it applies.
type EffectiveMarking struct {
	ID     MarkingID     `json:"id"`
	Source MarkingSource `json:"source"`
}

// EffectiveDirect builds an EffectiveMarking for a direct attachment.
func EffectiveDirect(id MarkingID) EffectiveMarking {
	return EffectiveMarking{ID: id, Source: Direct()}
}

// EffectiveInherited builds an EffectiveMarking for an upstream-inherited attachment.
func EffectiveInherited(id MarkingID, upstreamRID string) EffectiveMarking {
	return EffectiveMarking{ID: id, Source: InheritedFrom(upstreamRID)}
}
