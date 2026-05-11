package pipelineexpression

import (
	"math"
	"strings"
)

// ParamConstraintKind tags a [ParamConstraint].
type ParamConstraintKind int

const (
	// ParamExactly — argument must be exactly the listed type.
	ParamExactly ParamConstraintKind = iota
	// ParamPromotable — argument must be promotable to the listed type.
	ParamPromotable
	// ParamNumeric — argument must satisfy [PipelineType.IsNumeric].
	ParamNumeric
	// ParamTextual — argument must satisfy [PipelineType.IsTextual].
	ParamTextual
	// ParamTemporal — argument must satisfy [PipelineType.IsTemporal].
	ParamTemporal
	// ParamAny — anything goes (used for `cast` and runtime-only checks).
	ParamAny
)

// ParamConstraint describes the type constraint applied to a single
// scalar argument.
type ParamConstraint struct {
	Kind ParamConstraintKind
	Type PipelineType // populated for ParamExactly / ParamPromotable
}

// Equal compares two constraints structurally.
func (p ParamConstraint) Equal(other ParamConstraint) bool {
	if p.Kind != other.Kind {
		return false
	}
	if p.Kind == ParamExactly || p.Kind == ParamPromotable {
		return p.Type.Equal(other.Type)
	}
	return true
}

// ResultRuleKind tags a [ResultRule].
type ResultRuleKind int

const (
	// ResultFixed — result is the listed type.
	ResultFixed ResultRuleKind = iota
	// ResultPromoteOf — result is the LUB of the listed argument indexes.
	ResultPromoteOf
	// ResultTypeFromStringArg — result is the type literal extracted
	// from arg N (used for `cast(value, type_literal)`).
	ResultTypeFromStringArg
)

// ResultRule describes how a scalar's result type is computed.
type ResultRule struct {
	Kind  ResultRuleKind
	Type  PipelineType // populated for ResultFixed
	Args  []int        // populated for ResultPromoteOf
	Index int          // populated for ResultTypeFromStringArg
}

// ScalarSignature describes a scalar helper visible inside expression
// bodies.
type ScalarSignature struct {
	Name   string
	Params []ParamConstraint
	Result ResultRule
}

// ScalarSignatureFor looks up a scalar function by name. Returns
// (zero, false) when the name is not registered.
func ScalarSignatureFor(name string) (ScalarSignature, bool) {
	n := strings.ToLower(name)
	switch n {
	// String helpers.
	case "title_case", "lower", "upper", "trim", "clean_string":
		return ScalarSignature{
			Name:   n,
			Params: []ParamConstraint{{Kind: ParamTextual}},
			Result: ResultRule{Kind: ResultFixed, Type: StringType()},
		}, true
	case "concat":
		return ScalarSignature{
			Name:   n,
			Params: []ParamConstraint{{Kind: ParamTextual}, {Kind: ParamTextual}},
			Result: ResultRule{Kind: ResultFixed, Type: StringType()},
		}, true
	// Numeric helpers.
	case "abs", "floor", "ceil", "round":
		return ScalarSignature{
			Name:   n,
			Params: []ParamConstraint{{Kind: ParamNumeric}},
			Result: ResultRule{Kind: ResultPromoteOf, Args: []int{0}},
		}, true
	case "haversine_meters", "haversine_km", "haversine_miles":
		return ScalarSignature{
			Name: n,
			Params: []ParamConstraint{
				{Kind: ParamNumeric},
				{Kind: ParamNumeric},
				{Kind: ParamNumeric},
				{Kind: ParamNumeric},
			},
			Result: ResultRule{Kind: ResultFixed, Type: DoubleType()},
		}, true
	case "haversine_distance":
		return ScalarSignature{
			Name: n,
			Params: []ParamConstraint{
				{Kind: ParamNumeric},
				{Kind: ParamNumeric},
				{Kind: ParamNumeric},
				{Kind: ParamNumeric},
				{Kind: ParamTextual},
			},
			Result: ResultRule{Kind: ResultFixed, Type: DoubleType()},
		}, true
	// Temporal.
	case "to_date":
		return ScalarSignature{
			Name:   n,
			Params: []ParamConstraint{{Kind: ParamTextual}},
			Result: ResultRule{Kind: ResultFixed, Type: DateType()},
		}, true
	case "to_timestamp":
		return ScalarSignature{
			Name:   n,
			Params: []ParamConstraint{{Kind: ParamTextual}},
			Result: ResultRule{Kind: ResultFixed, Type: TimestampType()},
		}, true
	// Type cast: special-cased — second arg must be a STRING literal
	// naming the target type.
	case "cast":
		return ScalarSignature{
			Name: n,
			Params: []ParamConstraint{
				{Kind: ParamAny},
				{Kind: ParamExactly, Type: StringType()},
			},
			Result: ResultRule{Kind: ResultTypeFromStringArg, Index: 1},
		}, true
	// Boolean.
	case "is_null", "is_not_null":
		return ScalarSignature{
			Name:   n,
			Params: []ParamConstraint{{Kind: ParamAny}},
			Result: ResultRule{Kind: ResultFixed, Type: BooleanType()},
		}, true
	// Geometry helpers.
	case "geom_within":
		return ScalarSignature{
			Name: n,
			Params: []ParamConstraint{
				{Kind: ParamExactly, Type: GeometryType()},
				{Kind: ParamExactly, Type: GeometryType()},
			},
			Result: ResultRule{Kind: ResultFixed, Type: BooleanType()},
		}, true
	}
	return ScalarSignature{}, false
}

// TransformSignature describes one of the canonical Pipeline Builder
// transforms.
type TransformSignature struct {
	Name string
	// MinInputs / MaxInputs bound the upstream-node fan-in.
	MinInputs int
	MaxInputs int
	// RequiredConfigKeys lists the keys the node `config` JSON object
	// must expose (informative).
	RequiredConfigKeys []string
}

// TransformMaxInputsUnbounded is the sentinel used for transforms that
// accept any number of upstream inputs (mirrors Rust `usize::MAX`).
const TransformMaxInputsUnbounded = math.MaxInt

// TransformSignatureFor looks up a transform by name. Returns
// (zero, false) when the name is not registered.
func TransformSignatureFor(name string) (TransformSignature, bool) {
	n := strings.ToLower(name)
	switch n {
	case "cast":
		return TransformSignature{
			Name: "cast", MinInputs: 1, MaxInputs: 1,
			RequiredConfigKeys: []string{"columns"},
		}, true
	case "title_case":
		return TransformSignature{
			Name: "title_case", MinInputs: 1, MaxInputs: 1,
			RequiredConfigKeys: []string{"columns"},
		}, true
	case "clean_string":
		return TransformSignature{
			Name: "clean_string", MinInputs: 1, MaxInputs: 1,
			RequiredConfigKeys: []string{"columns"},
		}, true
	case "filter":
		return TransformSignature{
			Name: "filter", MinInputs: 1, MaxInputs: 1,
			RequiredConfigKeys: []string{"predicate"},
		}, true
	case "join":
		return TransformSignature{
			Name: "join", MinInputs: 2, MaxInputs: 2,
			RequiredConfigKeys: []string{"how", "on"},
		}, true
	case "union":
		return TransformSignature{
			Name: "union", MinInputs: 2, MaxInputs: TransformMaxInputsUnbounded,
			RequiredConfigKeys: []string{},
		}, true
	case "group_by":
		return TransformSignature{
			Name: "group_by", MinInputs: 1, MaxInputs: 1,
			RequiredConfigKeys: []string{"keys", "aggregations"},
		}, true
	case "window":
		return TransformSignature{
			Name: "window", MinInputs: 1, MaxInputs: 1,
			RequiredConfigKeys: []string{"partition_by", "order_by"},
		}, true
	case "pivot":
		return TransformSignature{
			Name: "pivot", MinInputs: 1, MaxInputs: 1,
			RequiredConfigKeys: []string{"pivot_column", "value_column"},
		}, true
	}
	return TransformSignature{}, false
}

// ParseTypeLiteral parses a string like "INTEGER" / "ARRAY<INTEGER>"
// into a [PipelineType]. Used by `cast(value, "INTEGER")`. Returns
// (zero, false) when the literal isn't a recognised type name.
func ParseTypeLiteral(literal string) (PipelineType, bool) {
	trimmed := strings.TrimSpace(literal)
	upper := strings.ToUpper(trimmed)
	switch upper {
	case "BOOLEAN":
		return BooleanType(), true
	case "INTEGER":
		return IntegerType(), true
	case "LONG":
		return LongType(), true
	case "DOUBLE":
		return DoubleType(), true
	case "DECIMAL":
		return DecimalType(), true
	case "STRING":
		return StringType(), true
	case "DATE":
		return DateType(), true
	case "TIMESTAMP":
		return TimestampType(), true
	case "GEOMETRY":
		return GeometryType(), true
	}
	if strings.HasPrefix(upper, "ARRAY<") && strings.HasSuffix(upper, ">") {
		inner := upper[len("ARRAY<") : len(upper)-1]
		t, ok := ParseTypeLiteral(inner)
		if !ok {
			return PipelineType{}, false
		}
		return ArrayOf(t), true
	}
	return PipelineType{}, false
}
