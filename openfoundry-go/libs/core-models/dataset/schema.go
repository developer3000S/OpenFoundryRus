package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// FieldType is the discriminator for the 15 Foundry-compatible field
// types. Composite types carry their payload in the surrounding
// SchemaField; primitive variants carry no payload.
type FieldType string

const (
	FTBoolean   FieldType = "BOOLEAN"
	FTByte      FieldType = "BYTE"
	FTShort     FieldType = "SHORT"
	FTInteger   FieldType = "INTEGER"
	FTLong      FieldType = "LONG"
	FTFloat     FieldType = "FLOAT"
	FTDouble    FieldType = "DOUBLE"
	FTString    FieldType = "STRING"
	FTBinary    FieldType = "BINARY"
	FTDate      FieldType = "DATE"
	FTTimestamp FieldType = "TIMESTAMP"
	FTDecimal   FieldType = "DECIMAL"
	FTArray     FieldType = "ARRAY"
	FTMap       FieldType = "MAP"
	FTStruct    FieldType = "STRUCT"
)

// IsPrimitive reports whether ft is one of the primitive variants
// (everything except ARRAY / MAP / STRUCT).
func (ft FieldType) IsPrimitive() bool {
	switch ft {
	case FTArray, FTMap, FTStruct:
		return false
	default:
		return true
	}
}

// SchemaField is one column in a Schema. Composite types nest further
// fields through ArraySubType / MapKeyType / MapValueType / SubSchemas.
//
// JSON layout exactly mirrors the Rust `SchemaField` with `#[serde(flatten)]`:
//
//	{
//	  "name": "...",
//	  "type": "DECIMAL",          // discriminator
//	  "precision": 10, "scale": 2, // payload (per type)
//	  "nullable": true,            // default true if absent
//	  "description": "..."         // omitted if empty
//	}
type SchemaField struct {
	Name        string    `json:"name"`
	Type        FieldType `json:"type"`
	Nullable    bool      `json:"nullable"`
	Description string    `json:"description,omitempty"`

	// DECIMAL payload.
	Precision *uint8 `json:"precision,omitempty"`
	Scale     *uint8 `json:"scale,omitempty"`

	// ARRAY payload.
	ArraySubType *SchemaField `json:"arraySubType,omitempty"`

	// MAP payload.
	MapKeyType   *SchemaField `json:"mapKeyType,omitempty"`
	MapValueType *SchemaField `json:"mapValueType,omitempty"`

	// STRUCT payload.
	SubSchemas []SchemaField `json:"subSchemas,omitempty"`
}

// schemaFieldWire is used to honour the Rust default: when the JSON
// payload omits "nullable", treat it as true.
type schemaFieldWire struct {
	Name         string        `json:"name"`
	Type         FieldType     `json:"type"`
	Nullable     *bool         `json:"nullable,omitempty"`
	Description  string        `json:"description,omitempty"`
	Precision    *uint8        `json:"precision,omitempty"`
	Scale        *uint8        `json:"scale,omitempty"`
	ArraySubType *SchemaField  `json:"arraySubType,omitempty"`
	MapKeyType   *SchemaField  `json:"mapKeyType,omitempty"`
	MapValueType *SchemaField  `json:"mapValueType,omitempty"`
	SubSchemas   []SchemaField `json:"subSchemas,omitempty"`
}

func (f *SchemaField) UnmarshalJSON(data []byte) error {
	var w schemaFieldWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	nullable := true
	if w.Nullable != nil {
		nullable = *w.Nullable
	}
	*f = SchemaField{
		Name:         w.Name,
		Type:         w.Type,
		Nullable:     nullable,
		Description:  w.Description,
		Precision:    w.Precision,
		Scale:        w.Scale,
		ArraySubType: w.ArraySubType,
		MapKeyType:   w.MapKeyType,
		MapValueType: w.MapValueType,
		SubSchemas:   w.SubSchemas,
	}
	return nil
}

// Schema is a top-level dataset schema. Format drives how
// CustomMetadata is interpreted (e.g. "text" → CSV options).
type Schema struct {
	Fields         []SchemaField     `json:"fields"`
	Format         string            `json:"format"`
	CustomMetadata map[string]string `json:"custom_metadata,omitempty"`
}

// MarshalJSON ensures CustomMetadata serialises in deterministic key
// order so the output matches the Rust BTreeMap-backed implementation.
func (s Schema) MarshalJSON() ([]byte, error) {
	type alias Schema
	if len(s.CustomMetadata) == 0 {
		return json.Marshal(alias{
			Fields:         s.Fields,
			Format:         s.Format,
			CustomMetadata: nil,
		})
	}
	keys := make([]string, 0, len(s.CustomMetadata))
	for k := range s.CustomMetadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(keys))
	for _, k := range keys {
		ordered[k] = s.CustomMetadata[k]
	}
	return json.Marshal(alias{Fields: s.Fields, Format: s.Format, CustomMetadata: ordered})
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// SchemaValidationError categorises why a Schema or SchemaField failed validation.
type SchemaValidationError struct {
	Code    string
	Message string
}

func (e *SchemaValidationError) Error() string { return e.Message }

// Sentinel error codes — kept stable so callers can branch with errors.Is.
var (
	ErrEmptyFieldName             = &SchemaValidationError{Code: "empty_field_name", Message: "field name must not be empty"}
	ErrDecimalPrecisionOutOfRange = &SchemaValidationError{Code: "decimal_precision_out_of_range", Message: "decimal precision must be 1..=38"}
	ErrDecimalScaleOutOfRange     = &SchemaValidationError{Code: "decimal_scale_out_of_range", Message: "decimal scale must be 0..=precision"}
	ErrArrayMissingSubType        = &SchemaValidationError{Code: "array_missing_subtype", Message: "array field is missing arraySubType"}
	ErrMapKeyNotPrimitive         = &SchemaValidationError{Code: "map_key_not_primitive", Message: "map key must be a primitive"}
	ErrStructEmpty                = &SchemaValidationError{Code: "struct_empty", Message: "struct must declare at least one subSchema"}
	ErrInvalidCSVOption           = &SchemaValidationError{Code: "invalid_csv_option", Message: "invalid csv option"}
)

func (e *SchemaValidationError) Is(target error) bool {
	t, ok := target.(*SchemaValidationError)
	if !ok {
		return false
	}
	return t.Code == e.Code
}

// Validate checks a single field recursively. Implementation mirrors
// the Rust `SchemaField::validate` rule set.
func (f SchemaField) Validate() error {
	if f.Name == "" {
		return ErrEmptyFieldName
	}
	switch f.Type {
	case FTDecimal:
		if f.Precision == nil || *f.Precision == 0 || *f.Precision > 38 {
			got := uint8(0)
			if f.Precision != nil {
				got = *f.Precision
			}
			return &SchemaValidationError{
				Code:    ErrDecimalPrecisionOutOfRange.Code,
				Message: fmt.Sprintf("decimal precision must be 1..=38, got %d", got),
			}
		}
		if f.Scale == nil || *f.Scale > *f.Precision {
			scale := uint8(0)
			if f.Scale != nil {
				scale = *f.Scale
			}
			return &SchemaValidationError{
				Code:    ErrDecimalScaleOutOfRange.Code,
				Message: fmt.Sprintf("decimal scale must be 0..=precision (%d), got %d", *f.Precision, scale),
			}
		}
	case FTArray:
		if f.ArraySubType == nil {
			return &SchemaValidationError{
				Code:    ErrArrayMissingSubType.Code,
				Message: fmt.Sprintf("array field %q is missing arraySubType", f.Name),
			}
		}
		if err := f.ArraySubType.Validate(); err != nil {
			return err
		}
	case FTMap:
		if f.MapKeyType == nil || f.MapValueType == nil {
			return &SchemaValidationError{
				Code:    "map_missing_keyvalue",
				Message: fmt.Sprintf("map field %q requires mapKeyType and mapValueType", f.Name),
			}
		}
		if !f.MapKeyType.Type.IsPrimitive() {
			return &SchemaValidationError{
				Code:    ErrMapKeyNotPrimitive.Code,
				Message: fmt.Sprintf("map field %q requires mapKeyType to be primitive, got %s", f.Name, f.MapKeyType.Type),
			}
		}
		if err := f.MapKeyType.Validate(); err != nil {
			return err
		}
		if err := f.MapValueType.Validate(); err != nil {
			return err
		}
	case FTStruct:
		if len(f.SubSchemas) == 0 {
			return &SchemaValidationError{
				Code:    ErrStructEmpty.Code,
				Message: fmt.Sprintf("struct field %q must declare at least one subSchema", f.Name),
			}
		}
		seen := make(map[string]struct{}, len(f.SubSchemas))
		for _, sub := range f.SubSchemas {
			if _, dup := seen[sub.Name]; dup {
				return &SchemaValidationError{
					Code:    "duplicate_field_name",
					Message: fmt.Sprintf("duplicate field name %q", sub.Name),
				}
			}
			seen[sub.Name] = struct{}{}
			if err := sub.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

// Validate enforces top-level rules: unique field names + recursive
// per-field validation, and a sanity-check of CSV options when format == "text".
func (s Schema) Validate() error {
	seen := make(map[string]struct{}, len(s.Fields))
	for _, f := range s.Fields {
		if _, dup := seen[f.Name]; dup {
			return &SchemaValidationError{
				Code:    "duplicate_field_name",
				Message: fmt.Sprintf("duplicate field name %q", f.Name),
			}
		}
		seen[f.Name] = struct{}{}
		if err := f.Validate(); err != nil {
			return err
		}
	}
	if s.Format == "text" {
		if _, err := CSVOptionsFromMetadata(s.CustomMetadata); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// CSV options
// ---------------------------------------------------------------------------

// CSV metadata keys — kept in sync with the Rust constants.
const (
	CSVKeyDelimiter = "delimiter"
	CSVKeyQuote     = "quote"
	CSVKeyEscape    = "escape"
	CSVKeyHeader    = "header"
	CSVKeyNullValue = "nullValue"
)

// CSVOptions are the typed CSV/TSV options for `format = "text"`.
type CSVOptions struct {
	Delimiter rune
	Quote     rune
	Escape    rune
	Header    bool
	NullValue string
}

// DefaultCSVOptions returns the Rust default (`,`, `"`, `\`, header=true, no null token).
func DefaultCSVOptions() CSVOptions {
	return CSVOptions{Delimiter: ',', Quote: '"', Escape: '\\', Header: true}
}

// IntoMetadata renders the options as the BTreeMap-shaped
// custom_metadata payload the Rust side persists.
func (c CSVOptions) IntoMetadata() map[string]string {
	return map[string]string{
		CSVKeyDelimiter: string(c.Delimiter),
		CSVKeyQuote:     string(c.Quote),
		CSVKeyEscape:    string(c.Escape),
		CSVKeyHeader:    boolToken(c.Header),
		CSVKeyNullValue: c.NullValue,
	}
}

// CSVOptionsFromMetadata parses CSV options from a Schema's
// custom_metadata map. Missing keys fall back to defaults.
func CSVOptionsFromMetadata(meta map[string]string) (CSVOptions, error) {
	opts := DefaultCSVOptions()
	if v, ok := meta[CSVKeyDelimiter]; ok {
		ch, err := oneRune(v, CSVKeyDelimiter)
		if err != nil {
			return opts, err
		}
		opts.Delimiter = ch
	}
	if v, ok := meta[CSVKeyQuote]; ok {
		ch, err := oneRune(v, CSVKeyQuote)
		if err != nil {
			return opts, err
		}
		opts.Quote = ch
	}
	if v, ok := meta[CSVKeyEscape]; ok {
		ch, err := oneRune(v, CSVKeyEscape)
		if err != nil {
			return opts, err
		}
		opts.Escape = ch
	}
	if v, ok := meta[CSVKeyHeader]; ok {
		switch v {
		case "true", "TRUE", "True", "1":
			opts.Header = true
		case "false", "FALSE", "False", "0":
			opts.Header = false
		default:
			return opts, &SchemaValidationError{
				Code:    ErrInvalidCSVOption.Code,
				Message: fmt.Sprintf("invalid csv option %q: %q", CSVKeyHeader, v),
			}
		}
	}
	if v, ok := meta[CSVKeyNullValue]; ok {
		opts.NullValue = v
	}
	return opts, nil
}

func oneRune(value, key string) (rune, error) {
	runes := []rune(value)
	if len(runes) != 1 {
		return 0, &SchemaValidationError{
			Code:    ErrInvalidCSVOption.Code,
			Message: fmt.Sprintf("invalid csv option %q: %q", key, value),
		}
	}
	return runes[0], nil
}

func boolToken(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ErrSchemaValidation is returned for any schema validation problem
// (use errors.As to extract the typed *SchemaValidationError).
var ErrSchemaValidation = errors.New("schema validation failed")
