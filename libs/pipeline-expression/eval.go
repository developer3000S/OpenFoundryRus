package pipelineexpression

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	geospatialcore "github.com/openfoundry/openfoundry-go/libs/geospatial-core"
)

// EvalValueKind tags an [EvalValue].
type EvalValueKind int

const (
	// EvalKindBool — boolean payload.
	EvalKindBool EvalValueKind = iota
	// EvalKindInteger — int64 payload.
	EvalKindInteger
	// EvalKindDouble — float64 payload.
	EvalKindDouble
	// EvalKindString — string payload.
	EvalKindString
	// EvalKindNull — null marker.
	EvalKindNull
)

// EvalValue is a runtime value produced by the row evaluator.
type EvalValue struct {
	Kind   EvalValueKind
	Bool   bool
	Int    int64
	Double float64
	Str    string
}

// EvalBool builds a boolean EvalValue.
func EvalBool(v bool) EvalValue { return EvalValue{Kind: EvalKindBool, Bool: v} }

// EvalInt builds an integer EvalValue.
func EvalInt(v int64) EvalValue { return EvalValue{Kind: EvalKindInteger, Int: v} }

// EvalDouble builds a double EvalValue.
func EvalDouble(v float64) EvalValue { return EvalValue{Kind: EvalKindDouble, Double: v} }

// EvalString builds a string EvalValue.
func EvalString(v string) EvalValue { return EvalValue{Kind: EvalKindString, Str: v} }

// EvalNull builds the null EvalValue.
func EvalNull() EvalValue { return EvalValue{Kind: EvalKindNull} }

// Equal compares two EvalValue payloads structurally.
func (v EvalValue) Equal(other EvalValue) bool {
	if v.Kind != other.Kind {
		return false
	}
	switch v.Kind {
	case EvalKindBool:
		return v.Bool == other.Bool
	case EvalKindInteger:
		return v.Int == other.Int
	case EvalKindDouble:
		return v.Double == other.Double
	case EvalKindString:
		return v.Str == other.Str
	}
	return true
}

// Row is a single record consumed by the evaluator: `{"col": json}`.
type Row map[string]json.RawMessage

// EvalValueFromJSON converts a JSON value into the closest [EvalValue].
// Numeric literals: integers become [EvalKindInteger]; everything else
// (including non-integer numbers) becomes [EvalKindDouble]. Arrays and
// objects collapse to [EvalKindNull] — only scalars are inspected.
func EvalValueFromJSON(raw json.RawMessage) EvalValue {
	if len(raw) == 0 {
		return EvalNull()
	}
	var head any
	if err := json.Unmarshal(raw, &head); err != nil {
		return EvalNull()
	}
	switch v := head.(type) {
	case nil:
		return EvalNull()
	case bool:
		return EvalBool(v)
	case float64:
		// json.Unmarshal decodes all numbers as float64. We promote
		// integral-valued floats back to Integer so downstream
		// arithmetic stays in the integer lane (matches the Rust
		// behaviour where serde_json exposes both as_i64 and as_f64).
		if v == math.Trunc(v) && v >= math.MinInt64 && v <= math.MaxInt64 && !looksLikeFloatJSON(raw) {
			return EvalInt(int64(v))
		}
		return EvalDouble(v)
	case string:
		return EvalString(v)
	}
	return EvalNull()
}

// looksLikeFloatJSON returns true when the raw JSON encoding contains
// a `.`, `e` or `E` — the Rust `serde_json::Number::as_i64` returns
// None in that case so we mirror the same behaviour.
func looksLikeFloatJSON(raw json.RawMessage) bool {
	for _, b := range raw {
		switch b {
		case '.', 'e', 'E':
			return true
		}
	}
	return false
}

// ToJSON renders an [EvalValue] as the equivalent JSON value.
func (v EvalValue) ToJSON() json.RawMessage {
	switch v.Kind {
	case EvalKindBool:
		if v.Bool {
			return json.RawMessage("true")
		}
		return json.RawMessage("false")
	case EvalKindInteger:
		return json.RawMessage(strconv.FormatInt(v.Int, 10))
	case EvalKindDouble:
		if math.IsNaN(v.Double) || math.IsInf(v.Double, 0) {
			return json.RawMessage("null")
		}
		return json.RawMessage(strconv.FormatFloat(v.Double, 'g', -1, 64))
	case EvalKindString:
		// json.Marshal handles escaping for free.
		out, _ := json.Marshal(v.Str)
		return out
	}
	return json.RawMessage("null")
}

// AsBool returns the boolean payload if this value is a bool, else
// (false, false).
func (v EvalValue) AsBool() (bool, bool) {
	if v.Kind == EvalKindBool {
		return v.Bool, true
	}
	return false, false
}

// TypeHint maps an EvalValue back to its [PipelineType] hint.
func (v EvalValue) TypeHint() PipelineType {
	switch v.Kind {
	case EvalKindBool:
		return BooleanType()
	case EvalKindInteger:
		return IntegerType()
	case EvalKindDouble:
		return DoubleType()
	case EvalKindString:
		return StringType()
	}
	return StringType()
}

// EvalErrorKind tags an [EvalError].
type EvalErrorKind int

const (
	// EvalErrUnknownColumn — column missing from the row.
	EvalErrUnknownColumn EvalErrorKind = iota
	// EvalErrUnknownFunction — function not registered.
	EvalErrUnknownFunction
	// EvalErrArity — function called with wrong arity.
	EvalErrArity
	// EvalErrTypeMismatch — operator/function received the wrong types.
	EvalErrTypeMismatch
)

// EvalError is the failure mode for [Eval].
type EvalError struct {
	Kind          EvalErrorKind
	Name          string
	Message       string
	ExpectedArity int
	GotArity      int
}

// Error implements [error].
func (e *EvalError) Error() string {
	switch e.Kind {
	case EvalErrUnknownColumn:
		return fmt.Sprintf("unknown column '%s'", e.Name)
	case EvalErrUnknownFunction:
		return fmt.Sprintf("unknown function '%s'", e.Name)
	case EvalErrArity:
		return fmt.Sprintf("function '%s' expects %d args, got %d", e.Name, e.ExpectedArity, e.GotArity)
	case EvalErrTypeMismatch:
		return e.Message
	}
	return "unknown evaluation error"
}

// Eval walks `expr` against `row` and returns the produced value.
// Mirrors the Rust pure-row evaluator — designed to be fast enough for
// the ~50K-row preview sample.
func Eval(expr Expr, row Row) (EvalValue, error) {
	switch expr.Kind {
	case ExprLit:
		return evalLit(expr.Lit), nil
	case ExprColumn:
		raw, ok := row[expr.Name]
		if !ok {
			return EvalValue{}, &EvalError{Kind: EvalErrUnknownColumn, Name: expr.Name}
		}
		return EvalValueFromJSON(raw), nil
	case ExprUnary:
		if expr.Operand == nil {
			return EvalValue{}, &EvalError{Kind: EvalErrTypeMismatch, Message: "unary operand missing"}
		}
		inner, err := Eval(*expr.Operand, row)
		if err != nil {
			return EvalValue{}, err
		}
		switch expr.UnaryOp {
		case UnaryNeg:
			switch inner.Kind {
			case EvalKindInteger:
				return EvalInt(-inner.Int), nil
			case EvalKindDouble:
				return EvalDouble(-inner.Double), nil
			}
			return EvalValue{}, &EvalError{
				Kind:    EvalErrTypeMismatch,
				Message: fmt.Sprintf("unary '-' not defined for %s", typeHintDebug(inner.TypeHint())),
			}
		case UnaryNot:
			if inner.Kind == EvalKindBool {
				return EvalBool(!inner.Bool), nil
			}
			return EvalValue{}, &EvalError{
				Kind:    EvalErrTypeMismatch,
				Message: fmt.Sprintf("unary 'not' not defined for %s", typeHintDebug(inner.TypeHint())),
			}
		}
		return EvalValue{}, &EvalError{Kind: EvalErrTypeMismatch, Message: "unknown unary op"}
	case ExprBinary:
		return evalBinary(expr, row)
	case ExprCall:
		return evalCall(expr.Name, expr.Args, row)
	}
	return EvalValue{}, &EvalError{Kind: EvalErrTypeMismatch, Message: "unknown expression kind"}
}

func evalLit(lit Literal) EvalValue {
	switch lit.Kind {
	case LitBool:
		return EvalBool(lit.Bool)
	case LitInteger:
		return EvalInt(lit.Int)
	case LitDouble:
		return EvalDouble(lit.Double)
	case LitString:
		return EvalString(lit.Str)
	}
	return EvalNull()
}

func evalBinary(expr Expr, row Row) (EvalValue, error) {
	if expr.Left == nil || expr.Right == nil {
		return EvalValue{}, &EvalError{Kind: EvalErrTypeMismatch, Message: "binary operand missing"}
	}
	l, err := Eval(*expr.Left, row)
	if err != nil {
		return EvalValue{}, err
	}
	r, err := Eval(*expr.Right, row)
	if err != nil {
		return EvalValue{}, err
	}
	switch expr.BinaryOp {
	case BinaryAnd:
		if l.Kind == EvalKindBool && r.Kind == EvalKindBool {
			return EvalBool(l.Bool && r.Bool), nil
		}
		return EvalValue{}, typeMismatchOp("and", l, r)
	case BinaryOr:
		if l.Kind == EvalKindBool && r.Kind == EvalKindBool {
			return EvalBool(l.Bool || r.Bool), nil
		}
		return EvalValue{}, typeMismatchOp("or", l, r)
	case BinaryEq:
		return EvalBool(valueEq(l, r)), nil
	case BinaryNotEq:
		return EvalBool(!valueEq(l, r)), nil
	case BinaryLt:
		o, err := compareValues(l, r)
		if err != nil {
			return EvalValue{}, err
		}
		return EvalBool(o < 0), nil
	case BinaryLte:
		o, err := compareValues(l, r)
		if err != nil {
			return EvalValue{}, err
		}
		return EvalBool(o <= 0), nil
	case BinaryGt:
		o, err := compareValues(l, r)
		if err != nil {
			return EvalValue{}, err
		}
		return EvalBool(o > 0), nil
	case BinaryGte:
		o, err := compareValues(l, r)
		if err != nil {
			return EvalValue{}, err
		}
		return EvalBool(o >= 0), nil
	case BinaryAdd:
		return arithAdd(l, r)
	case BinarySub:
		return arith(l, r,
			func(a, b int64) int64 { return a - b },
			func(a, b float64) float64 { return a - b },
			"-")
	case BinaryMul:
		return arith(l, r,
			func(a, b int64) int64 { return a * b },
			func(a, b float64) float64 { return a * b },
			"*")
	case BinaryDiv:
		if (l.Kind == EvalKindInteger && r.Kind == EvalKindInteger && r.Int == 0) ||
			(l.Kind == EvalKindDouble && r.Kind == EvalKindDouble && r.Double == 0.0) {
			return EvalNull(), nil
		}
		return arith(l, r,
			func(a, b int64) int64 { return a / b },
			func(a, b float64) float64 { return a / b },
			"/")
	}
	return EvalValue{}, &EvalError{Kind: EvalErrTypeMismatch, Message: "unknown binary op"}
}

func arithAdd(l, r EvalValue) (EvalValue, error) {
	if l.Kind == EvalKindString && r.Kind == EvalKindString {
		return EvalString(l.Str + r.Str), nil
	}
	return arith(l, r,
		func(a, b int64) int64 { return a + b },
		func(a, b float64) float64 { return a + b },
		"+")
}

func arith(l, r EvalValue, fi func(int64, int64) int64, fd func(float64, float64) float64, op string) (EvalValue, error) {
	switch {
	case l.Kind == EvalKindInteger && r.Kind == EvalKindInteger:
		return EvalInt(fi(l.Int, r.Int)), nil
	case l.Kind == EvalKindInteger && r.Kind == EvalKindDouble:
		return EvalDouble(fd(float64(l.Int), r.Double)), nil
	case l.Kind == EvalKindDouble && r.Kind == EvalKindInteger:
		return EvalDouble(fd(l.Double, float64(r.Int))), nil
	case l.Kind == EvalKindDouble && r.Kind == EvalKindDouble:
		return EvalDouble(fd(l.Double, r.Double)), nil
	}
	return EvalValue{}, typeMismatchOp(op, l, r)
}

func compareValues(l, r EvalValue) (int, error) {
	switch {
	case l.Kind == EvalKindInteger && r.Kind == EvalKindInteger:
		return cmpInt64(l.Int, r.Int), nil
	case l.Kind == EvalKindInteger && r.Kind == EvalKindDouble:
		return f64Cmp(float64(l.Int), r.Double)
	case l.Kind == EvalKindDouble && r.Kind == EvalKindInteger:
		return f64Cmp(l.Double, float64(r.Int))
	case l.Kind == EvalKindDouble && r.Kind == EvalKindDouble:
		return f64Cmp(l.Double, r.Double)
	case l.Kind == EvalKindString && r.Kind == EvalKindString:
		return strings.Compare(l.Str, r.Str), nil
	case l.Kind == EvalKindBool && r.Kind == EvalKindBool:
		return cmpBool(l.Bool, r.Bool), nil
	case l.Kind == EvalKindNull || r.Kind == EvalKindNull:
		return 0, nil
	}
	return 0, typeMismatchOp("compare", l, r)
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpBool(a, b bool) int {
	switch {
	case !a && b:
		return -1
	case a && !b:
		return 1
	}
	return 0
}

func f64Cmp(a, b float64) (int, error) {
	if math.IsNaN(a) || math.IsNaN(b) {
		return 0, &EvalError{Kind: EvalErrTypeMismatch, Message: "NaN comparison"}
	}
	switch {
	case a < b:
		return -1, nil
	case a > b:
		return 1, nil
	}
	return 0, nil
}

// f64Epsilon mirrors Rust's `f64::EPSILON` constant.
const f64Epsilon = 2.220446049250313e-16

func valueEq(l, r EvalValue) bool {
	switch {
	case l.Kind == EvalKindInteger && r.Kind == EvalKindDouble:
		return math.Abs(float64(l.Int)-r.Double) < f64Epsilon
	case l.Kind == EvalKindDouble && r.Kind == EvalKindInteger:
		return math.Abs(l.Double-float64(r.Int)) < f64Epsilon
	}
	return l.Equal(r)
}

func typeMismatchOp(op string, l, r EvalValue) error {
	return &EvalError{
		Kind:    EvalErrTypeMismatch,
		Message: fmt.Sprintf("operator '%s' not defined for %s and %s", op, typeHintDebug(l.TypeHint()), typeHintDebug(r.TypeHint())),
	}
}

// typeHintDebug renders a [PipelineType] using Rust's `{:?}` debug
// format so error messages match the Rust originals.
func typeHintDebug(t PipelineType) string {
	switch t.Kind {
	case KindArray:
		if t.Inner == nil {
			return "Array { inner: ? }"
		}
		return fmt.Sprintf("Array { inner: %s }", typeHintDebug(*t.Inner))
	case KindStruct:
		parts := make([]string, len(t.Fields))
		for i, f := range t.Fields {
			parts[i] = fmt.Sprintf("(%q, %s)", f.Name, typeHintDebug(f.Type))
		}
		return fmt.Sprintf("Struct { fields: [%s] }", strings.Join(parts, ", "))
	}
	return typeName(t)
}

func evalCall(name string, args []Expr, row Row) (EvalValue, error) {
	nameLC := strings.ToLower(name)
	switch nameLC {
	case "is_null":
		if err := needArity(name, args, 1); err != nil {
			return EvalValue{}, err
		}
		v, err := Eval(args[0], row)
		if err != nil {
			return EvalValue{}, err
		}
		return EvalBool(v.Kind == EvalKindNull), nil
	case "is_not_null":
		if err := needArity(name, args, 1); err != nil {
			return EvalValue{}, err
		}
		v, err := Eval(args[0], row)
		if err != nil {
			return EvalValue{}, err
		}
		return EvalBool(v.Kind != EvalKindNull), nil
	case "title_case":
		if err := needArity(name, args, 1); err != nil {
			return EvalValue{}, err
		}
		v, err := Eval(args[0], row)
		if err != nil {
			return EvalValue{}, err
		}
		switch v.Kind {
		case EvalKindString:
			return EvalString(toTitleCase(v.Str)), nil
		case EvalKindNull:
			return EvalNull(), nil
		}
		return EvalValue{}, &EvalError{
			Kind:    EvalErrTypeMismatch,
			Message: fmt.Sprintf("title_case expects String, got %s", typeHintDebug(v.TypeHint())),
		}
	case "lower":
		return unaryString(name, args, row, strings.ToLower)
	case "upper":
		return unaryString(name, args, row, strings.ToUpper)
	case "trim":
		return unaryString(name, args, row, func(s string) string { return strings.TrimSpace(s) })
	case "clean_string":
		return unaryString(name, args, row, func(s string) string {
			return strings.Join(strings.Fields(s), " ")
		})
	case "concat":
		if err := needArity(name, args, 2); err != nil {
			return EvalValue{}, err
		}
		a, err := Eval(args[0], row)
		if err != nil {
			return EvalValue{}, err
		}
		b, err := Eval(args[1], row)
		if err != nil {
			return EvalValue{}, err
		}
		if a.Kind == EvalKindString && b.Kind == EvalKindString {
			return EvalString(a.Str + b.Str), nil
		}
		return EvalValue{}, &EvalError{Kind: EvalErrTypeMismatch, Message: "concat expects two strings"}
	case "abs":
		if err := needArity(name, args, 1); err != nil {
			return EvalValue{}, err
		}
		v, err := Eval(args[0], row)
		if err != nil {
			return EvalValue{}, err
		}
		switch v.Kind {
		case EvalKindInteger:
			abs := v.Int
			if abs < 0 {
				abs = -abs
			}
			return EvalInt(abs), nil
		case EvalKindDouble:
			return EvalDouble(math.Abs(v.Double)), nil
		}
		return EvalValue{}, &EvalError{
			Kind:    EvalErrTypeMismatch,
			Message: fmt.Sprintf("abs expects numeric, got %s", typeHintDebug(v.TypeHint())),
		}
	case "haversine_meters":
		return evalHaversineDistance(name, args, row, "meters")
	case "haversine_km":
		return evalHaversineDistance(name, args, row, "km")
	case "haversine_miles":
		return evalHaversineDistance(name, args, row, "miles")
	case "haversine_distance":
		return evalHaversineDistance(name, args, row, "")
	case "cast":
		if err := needArity(name, args, 2); err != nil {
			return EvalValue{}, err
		}
		v, err := Eval(args[0], row)
		if err != nil {
			return EvalValue{}, err
		}
		var target string
		if args[1].Kind == ExprLit && args[1].Lit.Kind == LitString {
			target = args[1].Lit.Str
		} else {
			return EvalValue{}, &EvalError{Kind: EvalErrTypeMismatch, Message: "cast target must be a String literal"}
		}
		return castValue(v, target)
	}
	return EvalValue{}, &EvalError{Kind: EvalErrUnknownFunction, Name: name}
}

func needArity(name string, args []Expr, expected int) error {
	if len(args) != expected {
		return &EvalError{
			Kind:          EvalErrArity,
			Name:          name,
			ExpectedArity: expected,
			GotArity:      len(args),
		}
	}
	return nil
}

func evalHaversineDistance(name string, args []Expr, row Row, fixedUnit string) (EvalValue, error) {
	expectedArity := 4
	if fixedUnit == "" {
		expectedArity = 5
	}
	if err := needArity(name, args, expectedArity); err != nil {
		return EvalValue{}, err
	}
	coords := make([]float64, 4)
	for i := 0; i < 4; i++ {
		value, isNull, err := evalNumericFloatArg(name, i, args[i], row)
		if err != nil {
			return EvalValue{}, err
		}
		if isNull {
			return EvalNull(), nil
		}
		coords[i] = value
	}
	unit := fixedUnit
	if unit == "" {
		value, err := Eval(args[4], row)
		if err != nil {
			return EvalValue{}, err
		}
		switch value.Kind {
		case EvalKindString:
			unit = value.Str
		case EvalKindNull:
			return EvalNull(), nil
		default:
			return EvalValue{}, &EvalError{
				Kind:    EvalErrTypeMismatch,
				Message: fmt.Sprintf("%s expects unit String, got %s", name, typeHintDebug(value.TypeHint())),
			}
		}
	}
	distance, err := geospatialcore.HaversineDistance(coords[0], coords[1], coords[2], coords[3], unit)
	if err != nil {
		return EvalValue{}, &EvalError{Kind: EvalErrTypeMismatch, Message: err.Error()}
	}
	return EvalDouble(distance), nil
}

func evalNumericFloatArg(name string, index int, arg Expr, row Row) (float64, bool, error) {
	value, err := Eval(arg, row)
	if err != nil {
		return 0, false, err
	}
	switch value.Kind {
	case EvalKindInteger:
		return float64(value.Int), false, nil
	case EvalKindDouble:
		return value.Double, false, nil
	case EvalKindNull:
		return 0, true, nil
	default:
		return 0, false, &EvalError{
			Kind:    EvalErrTypeMismatch,
			Message: fmt.Sprintf("%s expects numeric coordinate at argument %d, got %s", name, index+1, typeHintDebug(value.TypeHint())),
		}
	}
}

func unaryString(name string, args []Expr, row Row, f func(string) string) (EvalValue, error) {
	if err := needArity(name, args, 1); err != nil {
		return EvalValue{}, err
	}
	v, err := Eval(args[0], row)
	if err != nil {
		return EvalValue{}, err
	}
	switch v.Kind {
	case EvalKindString:
		return EvalString(f(v.Str)), nil
	case EvalKindNull:
		return EvalNull(), nil
	}
	return EvalValue{}, &EvalError{
		Kind:    EvalErrTypeMismatch,
		Message: fmt.Sprintf("%s expects String, got %s", name, typeHintDebug(v.TypeHint())),
	}
}

func castValue(value EvalValue, target string) (EvalValue, error) {
	upper := strings.ToUpper(strings.TrimSpace(target))
	switch upper {
	case "STRING":
		switch value.Kind {
		case EvalKindString:
			return value, nil
		case EvalKindInteger:
			return EvalString(strconv.FormatInt(value.Int, 10)), nil
		case EvalKindDouble:
			return EvalString(strconv.FormatFloat(value.Double, 'g', -1, 64)), nil
		case EvalKindBool:
			if value.Bool {
				return EvalString("true"), nil
			}
			return EvalString("false"), nil
		case EvalKindNull:
			return EvalString(""), nil
		}
	case "INTEGER":
		switch value.Kind {
		case EvalKindInteger:
			return value, nil
		case EvalKindDouble:
			return EvalInt(int64(value.Double)), nil
		case EvalKindString:
			i, err := strconv.ParseInt(value.Str, 10, 64)
			if err != nil {
				return EvalValue{}, &EvalError{
					Kind:    EvalErrTypeMismatch,
					Message: fmt.Sprintf("cannot cast '%s' to INTEGER", value.Str),
				}
			}
			return EvalInt(i), nil
		}
	case "LONG":
		switch value.Kind {
		case EvalKindInteger:
			return EvalInt(value.Int), nil
		case EvalKindDouble:
			return EvalInt(int64(value.Double)), nil
		case EvalKindString:
			i, err := strconv.ParseInt(value.Str, 10, 64)
			if err != nil {
				return EvalValue{}, &EvalError{
					Kind:    EvalErrTypeMismatch,
					Message: fmt.Sprintf("cannot cast '%s' to LONG", value.Str),
				}
			}
			return EvalInt(i), nil
		}
	case "DOUBLE", "DECIMAL":
		switch value.Kind {
		case EvalKindInteger:
			return EvalDouble(float64(value.Int)), nil
		case EvalKindDouble:
			return value, nil
		case EvalKindString:
			d, err := strconv.ParseFloat(value.Str, 64)
			if err != nil {
				return EvalValue{}, &EvalError{
					Kind:    EvalErrTypeMismatch,
					Message: fmt.Sprintf("cannot cast '%s' to DOUBLE", value.Str),
				}
			}
			return EvalDouble(d), nil
		}
	case "BOOLEAN":
		switch value.Kind {
		case EvalKindBool:
			return value, nil
		case EvalKindString:
			switch strings.ToLower(value.Str) {
			case "true":
				return EvalBool(true), nil
			case "false":
				return EvalBool(false), nil
			}
			return EvalValue{}, &EvalError{
				Kind:    EvalErrTypeMismatch,
				Message: fmt.Sprintf("cannot cast '%s' to BOOLEAN", value.Str),
			}
		}
	}
	if value.Kind == EvalKindNull {
		return EvalNull(), nil
	}
	return EvalValue{}, &EvalError{
		Kind:    EvalErrTypeMismatch,
		Message: fmt.Sprintf("cannot cast %s to %s", typeHintDebug(value.TypeHint()), upper),
	}
}

func toTitleCase(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevAlpha := false
	for _, ch := range s {
		if isLetter(ch) {
			if prevAlpha {
				b.WriteString(strings.ToLower(string(ch)))
			} else {
				b.WriteString(strings.ToUpper(string(ch)))
			}
			prevAlpha = true
		} else {
			b.WriteRune(ch)
			prevAlpha = false
		}
	}
	return b.String()
}

func isLetter(r rune) bool {
	if r < 0x80 {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return unicode.IsLetter(r)
}
