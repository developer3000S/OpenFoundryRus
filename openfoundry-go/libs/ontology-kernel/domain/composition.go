// Composition helpers shared by handlers/links and the indexer.
//
// Ports just `stable_link_id` from
// `libs/ontology-kernel/src/domain/composition.rs`. The rest of that
// Rust module (link CRUD, validation) lands together with the
// `handlers/links` port — only the deterministic id derivation is
// needed up-front because the storage handler's link-instance
// collector consumes it via `link_instance_from_store_link`.
package domain

import (
	"github.com/google/uuid"

	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// stableLinkNamespace mirrors `Uuid::NAMESPACE_OID` (RFC 4122 OID
// namespace) used by the Rust impl. Hard-coded so the Go output is
// byte-identical to the Rust UUIDv5 derivation.
var stableLinkNamespace = uuid.NameSpaceOID

// StableLinkID mirrors `pub fn stable_link_id`. Material is the same
// `openfoundry/ontology-link/<lt>/<from>/<to>` string the Rust
// version hashes through UUIDv5 → SHA-1.
func StableLinkID(linkType storage.LinkTypeId, from, to storage.ObjectId) uuid.UUID {
	material := "openfoundry/ontology-link/" + string(linkType) + "/" + string(from) + "/" + string(to)
	return uuid.NewSHA1(stableLinkNamespace, []byte(material))
}
