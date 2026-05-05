// Package media provides the canonical Foundry-style media reference
// embedded in dataset cells / ontology object properties.
//
// JSON layout uses camelCase keys to match the Foundry / OSDK contract
// and the Rust source of truth verbatim.
package media

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SetSchema is the high-level kind of a media set / item.
//
// Values are SCREAMING_SNAKE_CASE on the wire (`"IMAGE"`, `"DOCUMENT"`, …)
// and intentionally match the proto enum value names without the
// `MEDIA_SET_SCHEMA_` prefix.
type SetSchema string

const (
	SchemaImage       SetSchema = "IMAGE"
	SchemaAudio       SetSchema = "AUDIO"
	SchemaVideo       SetSchema = "VIDEO"
	SchemaDocument    SetSchema = "DOCUMENT"
	SchemaSpreadsheet SetSchema = "SPREADSHEET"
	SchemaEmail       SetSchema = "EMAIL"
	SchemaDicom       SetSchema = "DICOM"
)

// ErrUnknownSetSchema is returned when an unrecognised schema token is parsed.
var ErrUnknownSetSchema = errors.New("unknown media set schema")

// ParseSetSchema is case-insensitive — same contract as the Rust FromStr impl.
func ParseSetSchema(s string) (SetSchema, error) {
	switch strings.ToUpper(s) {
	case "IMAGE":
		return SchemaImage, nil
	case "AUDIO":
		return SchemaAudio, nil
	case "VIDEO":
		return SchemaVideo, nil
	case "DOCUMENT":
		return SchemaDocument, nil
	case "SPREADSHEET":
		return SchemaSpreadsheet, nil
	case "EMAIL":
		return SchemaEmail, nil
	case "DICOM":
		return SchemaDicom, nil
	default:
		return "", fmt.Errorf("%w: %q (expected IMAGE|AUDIO|VIDEO|DOCUMENT|SPREADSHEET|EMAIL|DICOM)", ErrUnknownSetSchema, s)
	}
}

// Reference is a typed pointer to a single media item inside a media set.
//
// JSON keys are camelCase to match Foundry's wire format:
//
//	{
//	  "mediaSetRid":  "ri.foundry.main.media_set....",
//	  "mediaItemRid": "ri.foundry.main.media_item....",
//	  "branch":       "master",
//	  "schema":       "IMAGE"
//	}
type Reference struct {
	MediaSetRID  string    `json:"mediaSetRid"`
	MediaItemRID string    `json:"mediaItemRid"`
	Branch       string    `json:"branch"`
	Schema       SetSchema `json:"schema"`
}

// NewReference constructs a Reference, mirroring the Rust constructor.
func NewReference(mediaSetRID, mediaItemRID, branch string, schema SetSchema) Reference {
	return Reference{
		MediaSetRID:  mediaSetRID,
		MediaItemRID: mediaItemRID,
		Branch:       branch,
		Schema:       schema,
	}
}

// ToFoundryJSON encodes the Reference as the Foundry-compatible JSON
// string stored inside dataset cells / property values.
func (r Reference) ToFoundryJSON() (string, error) {
	out, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// FromFoundryJSON parses a Reference from the Foundry-compatible JSON string.
func FromFoundryJSON(s string) (Reference, error) {
	var r Reference
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return Reference{}, err
	}
	return r, nil
}
