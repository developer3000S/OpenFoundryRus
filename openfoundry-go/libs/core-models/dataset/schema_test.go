package dataset_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/core-models/dataset"
)

func u8(v uint8) *uint8 { return &v }

func primitive(name string, ft dataset.FieldType) dataset.SchemaField {
	return dataset.SchemaField{Name: name, Type: ft, Nullable: true}
}

func TestAllFieldTypeTagsSerialize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		field dataset.SchemaField
		tag   string
	}{
		{primitive("a", dataset.FTBoolean), "BOOLEAN"},
		{primitive("a", dataset.FTByte), "BYTE"},
		{primitive("a", dataset.FTShort), "SHORT"},
		{primitive("a", dataset.FTInteger), "INTEGER"},
		{primitive("a", dataset.FTLong), "LONG"},
		{primitive("a", dataset.FTFloat), "FLOAT"},
		{primitive("a", dataset.FTDouble), "DOUBLE"},
		{primitive("a", dataset.FTString), "STRING"},
		{primitive("a", dataset.FTBinary), "BINARY"},
		{primitive("a", dataset.FTDate), "DATE"},
		{primitive("a", dataset.FTTimestamp), "TIMESTAMP"},
		{
			dataset.SchemaField{Name: "a", Type: dataset.FTDecimal, Nullable: true, Precision: u8(10), Scale: u8(2)},
			"DECIMAL",
		},
	}
	require.Len(t, cases, 12) // primitives + decimal; composite covered separately

	for _, c := range cases {
		out, err := json.Marshal(c.field)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(out, &decoded))
		assert.Equal(t, c.tag, decoded["type"])
	}
}

func TestSchemaRoundTrip(t *testing.T) {
	t.Parallel()
	subItem := primitive("item", dataset.FTString)
	mapKey := primitive("k", dataset.FTString)
	mapVal := primitive("v", dataset.FTString)
	zip := primitive("zip", dataset.FTInteger)
	street := primitive("street", dataset.FTString)

	original := dataset.Schema{
		Format: "parquet",
		Fields: []dataset.SchemaField{
			primitive("id", dataset.FTLong),
			primitive("name", dataset.FTString),
			{
				Name: "tags", Type: dataset.FTArray, Nullable: true,
				Description:  "list of tags",
				ArraySubType: &subItem,
			},
			{
				Name: "attrs", Type: dataset.FTMap, Nullable: false,
				MapKeyType:   &mapKey,
				MapValueType: &mapVal,
			},
			{
				Name: "address", Type: dataset.FTStruct, Nullable: true,
				SubSchemas: []dataset.SchemaField{street, zip},
			},
			{
				Name: "amount", Type: dataset.FTDecimal, Nullable: true,
				Precision: u8(18), Scale: u8(4),
			},
		},
	}

	require.NoError(t, original.Validate())
	out, err := json.Marshal(original)
	require.NoError(t, err)
	var back dataset.Schema
	require.NoError(t, json.Unmarshal(out, &back))
	assert.Equal(t, original, back)
}

func TestNullableDefaultsTrueWhenMissing(t *testing.T) {
	t.Parallel()
	// Rust serde default: missing "nullable" → true.
	raw := []byte(`{"name":"a","type":"LONG"}`)
	var f dataset.SchemaField
	require.NoError(t, json.Unmarshal(raw, &f))
	assert.True(t, f.Nullable)
}

func TestDecimalPrecisionRange(t *testing.T) {
	t.Parallel()
	bad := dataset.SchemaField{Name: "x", Type: dataset.FTDecimal, Nullable: true, Precision: u8(0), Scale: u8(0)}
	err := bad.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, dataset.ErrDecimalPrecisionOutOfRange))

	bad = dataset.SchemaField{Name: "x", Type: dataset.FTDecimal, Nullable: true, Precision: u8(39), Scale: u8(0)}
	assert.True(t, errors.Is(bad.Validate(), dataset.ErrDecimalPrecisionOutOfRange))
}

func TestDecimalScaleCannotExceedPrecision(t *testing.T) {
	t.Parallel()
	bad := dataset.SchemaField{Name: "x", Type: dataset.FTDecimal, Nullable: true, Precision: u8(4), Scale: u8(5)}
	assert.True(t, errors.Is(bad.Validate(), dataset.ErrDecimalScaleOutOfRange))
}

func TestMapKeyMustBePrimitive(t *testing.T) {
	t.Parallel()
	innerVal := primitive("inner", dataset.FTLong)
	composite := dataset.SchemaField{
		Name: "k", Type: dataset.FTArray, Nullable: true, ArraySubType: &innerVal,
	}
	scalarVal := primitive("v", dataset.FTLong)
	bad := dataset.SchemaField{
		Name: "m", Type: dataset.FTMap, Nullable: true,
		MapKeyType:   &composite,
		MapValueType: &scalarVal,
	}
	err := bad.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, dataset.ErrMapKeyNotPrimitive))
}

func TestStructRequiresSubSchemas(t *testing.T) {
	t.Parallel()
	bad := dataset.SchemaField{Name: "s", Type: dataset.FTStruct, Nullable: true}
	assert.True(t, errors.Is(bad.Validate(), dataset.ErrStructEmpty))
}

func TestDuplicateTopLevelFieldNames(t *testing.T) {
	t.Parallel()
	bad := dataset.Schema{
		Format: "parquet",
		Fields: []dataset.SchemaField{
			primitive("a", dataset.FTLong),
			primitive("a", dataset.FTString),
		},
	}
	assert.Error(t, bad.Validate())
}

func TestCSVOptionsRoundTrip(t *testing.T) {
	t.Parallel()
	in := dataset.CSVOptions{Delimiter: ';', Quote: '\'', Escape: '\\', Header: false, NullValue: `\N`}
	back, err := dataset.CSVOptionsFromMetadata(in.IntoMetadata())
	require.NoError(t, err)
	assert.Equal(t, in, back)
}

func TestCSVOptionsDefaults(t *testing.T) {
	t.Parallel()
	back, err := dataset.CSVOptionsFromMetadata(map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, dataset.DefaultCSVOptions(), back)
}

func TestCSVOptionsRejectMultiCharDelimiter(t *testing.T) {
	t.Parallel()
	_, err := dataset.CSVOptionsFromMetadata(map[string]string{"delimiter": "||"})
	assert.True(t, errors.Is(err, dataset.ErrInvalidCSVOption))
}

func TestTextFormatValidatesCSVMetadata(t *testing.T) {
	t.Parallel()
	bad := dataset.Schema{
		Format:         "text",
		Fields:         []dataset.SchemaField{primitive("a", dataset.FTString)},
		CustomMetadata: map[string]string{"header": "yes"},
	}
	err := bad.Validate()
	require.Error(t, err)
	assert.True(t, errors.Is(err, dataset.ErrInvalidCSVOption))
}
