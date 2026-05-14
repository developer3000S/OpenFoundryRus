package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

const (
	defaultInferenceRows  = 500
	maxInferenceRows      = 5000
	maxInferenceBytes     = 2 * 1024 * 1024
	defaultCSVNullValue   = ""
	defaultCSVDelimiter   = ","
	defaultCSVQuote       = "\""
	defaultCSVEscape      = "\\"
	defaultCSVCharset     = "UTF-8"
	defaultJaggedBehavior = "FILL_NULLS"
	defaultParseBehavior  = "NULL"
)

type schemaInferenceSample struct {
	Path      string
	Text      string
	Bytes     int
	MediaType string
}

type scalarInference int

const (
	scalarUnknown scalarInference = iota
	scalarBoolean
	scalarLong
	scalarDouble
	scalarDate
	scalarTimestamp
	scalarString
)

func (h *Handlers) InferDatasetSchema(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}

	var body models.InferDatasetSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Apply {
		if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
			return
		}
	}

	dataset, err := h.Repo.GetDataset(r.Context(), datasetID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSONErr(w, http.StatusNotFound, "dataset not found")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to load dataset")
		return
	}

	branch := dataset.ActiveBranch
	if branch == "" {
		branch = "main"
	}
	if body.BranchName != nil {
		branch = foundryBranchName(*body.BranchName)
	}
	reader := inferDataframeReader(body.DataframeReader, body.Format)
	options, optionWarnings := normalizeInferenceCSVOptions(body.ParserOptions, body.Format, body.Paths)
	maxRows := normalizeInferenceMaxRows(body.MaxRows)

	schema := models.DatasetSchema{}
	warnings := append([]string{}, optionWarnings...)
	sampleRows := 0
	sources := []models.InferredSchemaSource{}

	if body.ManualSchema != nil {
		schema = models.NormalizeDatasetSchema(*body.ManualSchema)
		if schema.FileFormat == "" {
			schema.FileFormat = models.NormalizeDataframeReader(reader)
		}
	} else {
		samples, nextSources, sampleWarnings, err := h.schemaInferenceSamples(r, datasetID, branch, body, maxRows)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
		warnings = append(warnings, sampleWarnings...)
		sources = nextSources

		format := normalizeInferenceFormat(body.Format, body.Paths, samples)
		if format == "JSON" {
			schema, sampleRows, sampleWarnings = inferJSONSchema(samples, options, maxRows)
		} else {
			schema, sampleRows, sampleWarnings = inferCSVSchema(samples, options, maxRows)
		}
		warnings = append(warnings, sampleWarnings...)
	}

	schema.FileFormat = models.NormalizeDataframeReader(reader)
	if schema.CustomMetadata == nil {
		schema.CustomMetadata = &models.CustomMetadata{}
	}
	if schema.CustomMetadata.CSV == nil {
		schema.CustomMetadata.CSV = &options
	}
	if len(warnings) > 0 {
		schema.CustomMetadata.CSV.Warnings = dedupeStrings(append(schema.CustomMetadata.CSV.Warnings, warnings...))
	}
	schema = models.NormalizeDatasetSchema(schema)
	if errs := models.ValidateDatasetSchema(schema); len(errs) > 0 {
		writeSchemaParseError(w, strings.Join(errs, "; "))
		return
	}

	var applied *models.FoundryDatasetSchemaResponse
	if body.Apply {
		endTxn, ok := parseInferenceEndTransaction(w, body.EndTransactionRID)
		if !ok {
			return
		}
		out, err := h.Repo.PutDatasetSchema(r.Context(), datasetID, branch, endTxn, reader, schema)
		if err != nil {
			writeFoundrySchemaError(w, err)
			return
		}
		out.CustomMetadata = schema.CustomMetadata
		applied = out
	}

	paths := append([]string{}, body.Paths...)
	if len(paths) == 0 {
		for _, source := range sources {
			if source.Path != "" {
				paths = append(paths, source.Path)
			}
		}
	}
	writeJSON(w, http.StatusOK, models.InferDatasetSchemaResponse{
		BranchName:      branch,
		DataframeReader: reader,
		FileFormat:      schema.FileFormat,
		Paths:           paths,
		Sources:         sources,
		Schema:          models.FoundrySchemaFromDatasetSchema(schema),
		DatasetSchema:   schema,
		ParserOptions:   options,
		Warnings:        dedupeStrings(warnings),
		SampleRows:      sampleRows,
		Applied:         applied,
	})
}

func inferDataframeReader(reader *string, format string) string {
	if reader != nil && strings.TrimSpace(*reader) != "" {
		return models.NormalizeDataframeReader(*reader)
	}
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case "CSV", "JSON", "JSONL", "NDJSON", "TSV", "TEXT":
		return models.FileFormatText
	case "AVRO":
		return models.FileFormatAvro
	default:
		return models.FileFormatText
	}
}

func parseInferenceEndTransaction(w http.ResponseWriter, raw *string) (*uuid.UUID, bool) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, true
	}
	parsed, err := parseFoundryTransactionRID(*raw)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid endTransactionRid")
		return nil, false
	}
	return &parsed, true
}

func normalizeInferenceMaxRows(raw int) int {
	if raw <= 0 {
		return defaultInferenceRows
	}
	if raw > maxInferenceRows {
		return maxInferenceRows
	}
	return raw
}

func normalizeInferenceCSVOptions(raw *models.CsvOptions, format string, paths []string) (models.CsvOptions, []string) {
	opts := models.CsvOptions{
		Delimiter:          defaultCSVDelimiter,
		Quote:              defaultCSVQuote,
		Escape:             defaultCSVEscape,
		Header:             true,
		NullValue:          defaultCSVNullValue,
		Charset:            defaultCSVCharset,
		Encoding:           defaultCSVCharset,
		JaggedRowBehavior:  defaultJaggedBehavior,
		ParseErrorBehavior: defaultParseBehavior,
		DynamicTyping:      true,
	}
	if strings.EqualFold(format, "TSV") || anyPathHasSuffix(paths, ".tsv") {
		opts.Delimiter = "\t"
	}
	if raw != nil {
		opts = *raw
		if opts.Delimiter == "" {
			opts.Delimiter = defaultCSVDelimiter
		}
		if opts.Quote == "" {
			opts.Quote = defaultCSVQuote
		}
		if opts.Escape == "" {
			opts.Escape = defaultCSVEscape
		}
		if opts.Charset == "" {
			opts.Charset = defaultCSVCharset
		}
		if opts.Encoding == "" {
			opts.Encoding = opts.Charset
		}
		if opts.JaggedRowBehavior == "" {
			opts.JaggedRowBehavior = defaultJaggedBehavior
		}
		if opts.ParseErrorBehavior == "" {
			opts.ParseErrorBehavior = defaultParseBehavior
		}
	}
	opts.JaggedRowBehavior = strings.ToUpper(strings.TrimSpace(opts.JaggedRowBehavior))
	opts.ParseErrorBehavior = strings.ToUpper(strings.TrimSpace(opts.ParseErrorBehavior))
	if opts.JaggedRowBehavior == "" {
		opts.JaggedRowBehavior = defaultJaggedBehavior
	}
	if opts.ParseErrorBehavior == "" {
		opts.ParseErrorBehavior = defaultParseBehavior
	}
	if opts.Encoding == "" {
		opts.Encoding = opts.Charset
	}

	warnings := []string{}
	if opts.Quote != defaultCSVQuote {
		warnings = append(warnings, "non-default CSV quote characters are preserved in metadata but inference uses RFC 4180 double-quote parsing")
	}
	if opts.Escape != defaultCSVEscape {
		warnings = append(warnings, "non-default CSV escape characters are preserved in metadata but inference uses Go CSV escaping semantics")
	}
	if !strings.EqualFold(opts.Encoding, defaultCSVCharset) && !strings.EqualFold(opts.Charset, defaultCSVCharset) {
		warnings = append(warnings, "schema inference expects UTF-8 samples; encoding was recorded for downstream parsing")
	}
	if opts.DynamicTyping {
		warnings = append(warnings, "dynamic inference is sample-based; review inferred types before applying to production data")
	}
	return opts, warnings
}

func (h *Handlers) schemaInferenceSamples(r *http.Request, datasetID uuid.UUID, branch string, body models.InferDatasetSchemaRequest, maxRows int) ([]schemaInferenceSample, []models.InferredSchemaSource, []string, error) {
	samples := []schemaInferenceSample{}
	warnings := []string{}
	if strings.TrimSpace(body.SampleText) != "" {
		samples = append(samples, schemaInferenceSample{Path: "inline", Text: body.SampleText, Bytes: len(body.SampleText)})
	}
	for i, raw := range body.Samples {
		text := inferenceRawSampleText(raw)
		if strings.TrimSpace(text) == "" {
			continue
		}
		samples = append(samples, schemaInferenceSample{Path: fmt.Sprintf("inline[%d]", i), Text: text, Bytes: len(text)})
	}
	if len(samples) > 0 {
		return samples, inferenceSources(samples, maxRows), warnings, nil
	}

	if h.BackingFS == nil {
		return nil, nil, nil, errors.New("sampleText, samples, or a configured backing filesystem is required")
	}
	local, ok := h.BackingFS.(localObjectStore)
	if !ok || h.BackingFS.FSID() != "local" {
		return nil, nil, nil, errors.New("inline samples are required when the backing filesystem cannot be sampled directly")
	}

	files, err := h.Repo.ListFiles(r.Context(), datasetID, branch, "")
	if err != nil {
		return nil, nil, nil, err
	}
	selected := selectInferenceFiles(files, body.Paths, body.Format)
	if len(selected) == 0 {
		return nil, nil, nil, errors.New("no CSV or JSON files are available for schema inference")
	}
	for _, file := range selected {
		location := storageabstraction.ParsePhysicalURI(file.PhysicalURI)
		key := strings.TrimLeft(path.Join(location.BaseDirectory, location.RelativePath), "/")
		if key == "" {
			key = strings.TrimLeft(location.RelativePath, "/")
		}
		data, err := local.ReadLocalObject(key)
		if err != nil {
			warnings = append(warnings, "could not sample "+file.LogicalPath+": "+err.Error())
			continue
		}
		if len(data) > maxInferenceBytes {
			data = data[:maxInferenceBytes]
			warnings = append(warnings, "sample for "+file.LogicalPath+" was truncated to 2 MiB")
		}
		media := ""
		if file.MediaType != nil {
			media = *file.MediaType
		} else if file.ContentType != nil {
			media = *file.ContentType
		}
		samples = append(samples, schemaInferenceSample{Path: file.LogicalPath, Text: string(data), Bytes: len(data), MediaType: media})
		if len(samples) >= 5 {
			break
		}
	}
	if len(samples) == 0 {
		return nil, nil, nil, errors.New("schema inference could not read any selected files")
	}
	return samples, inferenceSources(samples, maxRows), warnings, nil
}

func inferenceRawSampleText(raw models.JSONValue) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

func inferenceSources(samples []schemaInferenceSample, maxRows int) []models.InferredSchemaSource {
	out := make([]models.InferredSchemaSource, 0, len(samples))
	for _, sample := range samples {
		rowCount := strings.Count(sample.Text, "\n")
		if strings.TrimSpace(sample.Text) != "" && !strings.HasSuffix(sample.Text, "\n") {
			rowCount++
		}
		if rowCount > maxRows {
			rowCount = maxRows
		}
		out = append(out, models.InferredSchemaSource{Path: sample.Path, Bytes: sample.Bytes, RowCount: rowCount, MediaType: sample.MediaType})
	}
	return out
}

func selectInferenceFiles(files []models.DatasetFile, requested []string, format string) []models.DatasetFile {
	requestedSet := map[string]bool{}
	for _, p := range requested {
		p = strings.TrimSpace(strings.TrimPrefix(p, "/"))
		if p != "" {
			requestedSet[p] = true
		}
	}
	out := []models.DatasetFile{}
	for _, file := range files {
		if file.DeletedAt != nil || file.Status == string(models.DatasetFileStatusDeleted) {
			continue
		}
		logical := strings.TrimPrefix(file.LogicalPath, "/")
		if len(requestedSet) > 0 && !requestedSet[logical] {
			continue
		}
		if len(requestedSet) == 0 && !fileLooksInferable(file, format) {
			continue
		}
		out = append(out, file)
	}
	return out
}

func fileLooksInferable(file models.DatasetFile, format string) bool {
	if strings.EqualFold(format, "CSV") || strings.EqualFold(format, "JSON") || strings.EqualFold(format, "JSONL") || strings.EqualFold(format, "NDJSON") {
		return true
	}
	lower := strings.ToLower(file.LogicalPath)
	if strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".tsv") || strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".jsonl") || strings.HasSuffix(lower, ".ndjson") {
		return true
	}
	media := ""
	if file.MediaType != nil {
		media = strings.ToLower(*file.MediaType)
	} else if file.ContentType != nil {
		media = strings.ToLower(*file.ContentType)
	}
	return strings.Contains(media, "csv") || strings.Contains(media, "json")
}

func normalizeInferenceFormat(format string, paths []string, samples []schemaInferenceSample) string {
	raw := strings.ToUpper(strings.TrimSpace(format))
	switch raw {
	case "JSON", "JSONL", "NDJSON":
		return "JSON"
	case "CSV", "TSV", "TEXT":
		return "CSV"
	}
	for _, p := range paths {
		lower := strings.ToLower(p)
		if strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".jsonl") || strings.HasSuffix(lower, ".ndjson") {
			return "JSON"
		}
	}
	for _, sample := range samples {
		if strings.Contains(strings.ToLower(sample.MediaType), "json") {
			return "JSON"
		}
		trimmed := strings.TrimSpace(sample.Text)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return "JSON"
		}
	}
	return "CSV"
}

func inferCSVSchema(samples []schemaInferenceSample, opts models.CsvOptions, maxRows int) (models.DatasetSchema, int, []string) {
	warnings := []string{}
	rows := [][]string{}
	header := []string{}
	width := 0
	totalRows := 0
	for _, sample := range samples {
		reader := csv.NewReader(strings.NewReader(sample.Text))
		reader.FieldsPerRecord = -1
		reader.TrimLeadingSpace = true
		reader.LazyQuotes = opts.ParseErrorBehavior != "ERROR"
		if comma, ok := oneRune(opts.Delimiter); ok {
			reader.Comma = comma
		} else {
			warnings = append(warnings, "delimiter must be a single rune; comma was used for inference")
		}

		line := 0
		for totalRows < maxRows {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				warnings = append(warnings, "CSV parse warning in "+sample.Path+": "+err.Error())
				if opts.ParseErrorBehavior == "ERROR" {
					break
				}
				continue
			}
			if line < opts.SkipLines {
				line++
				continue
			}
			if opts.Header && len(header) == 0 {
				header = sanitizeHeader(record)
				width = len(header)
				line++
				continue
			}
			if width == 0 {
				width = len(record)
			}
			normalized, rowWarnings := normalizeCSVRecord(record, width, opts.JaggedRowBehavior, sample.Path)
			warnings = append(warnings, rowWarnings...)
			rows = append(rows, normalized)
			totalRows++
			line++
		}
	}

	if width == 0 && len(header) > 0 {
		width = len(header)
	}
	if width == 0 {
		return models.DatasetSchema{FileFormat: models.FileFormatText, CustomMetadata: &models.CustomMetadata{CSV: &opts}}, 0, append(warnings, "no CSV rows were available for inference")
	}
	if len(header) == 0 || len(header) != width {
		header = make([]string, width)
		for i := range header {
			header[i] = fmt.Sprintf("column_%d", i+1)
		}
	}

	fields := make([]models.Field, 0, width+3)
	for col := 0; col < width; col++ {
		values := make([]string, 0, len(rows))
		nulls := 0
		for _, row := range rows {
			value := ""
			if col < len(row) {
				value = row[col]
			}
			if isCSVNull(value, opts.NullValue) {
				nulls++
				continue
			}
			values = append(values, value)
		}
		fieldType, typeWarnings := inferCSVScalarType(header[col], values, opts)
		warnings = append(warnings, typeWarnings...)
		if !opts.DynamicTyping {
			fieldType = models.FieldTypeString
		}
		fields = append(fields, models.Field{Name: header[col], Type: fieldType, Nullable: nulls > 0 || len(values) == 0})
	}
	fields = appendHelperFields(fields, opts)

	return models.NormalizeDatasetSchema(models.DatasetSchema{
		Fields:         fields,
		FileFormat:     models.FileFormatText,
		CustomMetadata: &models.CustomMetadata{CSV: &opts},
	}), totalRows, warnings
}

func sanitizeHeader(raw []string) []string {
	seen := map[string]int{}
	out := make([]string, len(raw))
	for i, value := range raw {
		name := strings.TrimSpace(value)
		if name == "" {
			name = fmt.Sprintf("column_%d", i+1)
		}
		name = strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
				return r
			}
			return '_'
		}, name)
		name = strings.Trim(name, "_")
		if name == "" {
			name = fmt.Sprintf("column_%d", i+1)
		}
		if unicode.IsDigit([]rune(name)[0]) {
			name = "column_" + name
		}
		base := name
		if seen[base] > 0 {
			name = fmt.Sprintf("%s_%d", base, seen[base]+1)
		}
		seen[base]++
		out[i] = name
	}
	return out
}

func normalizeCSVRecord(record []string, width int, behavior string, source string) ([]string, []string) {
	warnings := []string{}
	if len(record) == width {
		return record, warnings
	}
	switch behavior {
	case "ERROR":
		warnings = append(warnings, fmt.Sprintf("%s has jagged row with %d fields; expected %d", source, len(record), width))
		return record, warnings
	case "DROP_EXTRA":
		if len(record) > width {
			warnings = append(warnings, fmt.Sprintf("%s row had extra fields; extras were ignored for inference", source))
			return append([]string(nil), record[:width]...), warnings
		}
		fallthrough
	default:
		next := append([]string(nil), record...)
		for len(next) < width {
			next = append(next, "")
		}
		if len(next) > width {
			next = next[:width]
		}
		warnings = append(warnings, fmt.Sprintf("%s row width differed from header; missing values were filled as nulls", source))
		return next, warnings
	}
}

func inferCSVScalarType(name string, values []string, opts models.CsvOptions) (models.SchemaFieldType, []string) {
	if len(values) == 0 {
		return models.FieldTypeString, []string{"column " + name + " only contained nulls in the sample"}
	}
	current := scalarUnknown
	for _, value := range values {
		kind := classifyStringValue(value, opts)
		if current == scalarUnknown {
			current = kind
			continue
		}
		current = mergeScalarKinds(current, kind)
	}
	return scalarKindToFieldType(current), nil
}

func classifyStringValue(value string, opts models.CsvOptions) scalarInference {
	v := strings.TrimSpace(value)
	if v == "" {
		return scalarString
	}
	lower := strings.ToLower(v)
	if lower == "true" || lower == "false" {
		return scalarBoolean
	}
	if _, err := strconv.ParseInt(v, 10, 64); err == nil {
		return scalarLong
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return scalarDouble
	}
	if parseDateLike(v, opts.DateFormat, false) {
		return scalarDate
	}
	if parseDateLike(v, opts.TimestampFormat, true) {
		return scalarTimestamp
	}
	return scalarString
}

func parseDateLike(value string, custom *string, timestamp bool) bool {
	layouts := []string{}
	if custom != nil && strings.TrimSpace(*custom) != "" {
		layouts = append(layouts, *custom)
	}
	if timestamp {
		layouts = append(layouts, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05")
	} else {
		layouts = append(layouts, "2006-01-02", "2006/01/02", "01/02/2006")
	}
	for _, layout := range layouts {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

func mergeScalarKinds(a, b scalarInference) scalarInference {
	if a == b {
		return a
	}
	if a == scalarUnknown {
		return b
	}
	if b == scalarUnknown {
		return a
	}
	if (a == scalarLong && b == scalarDouble) || (a == scalarDouble && b == scalarLong) {
		return scalarDouble
	}
	if (a == scalarDate && b == scalarTimestamp) || (a == scalarTimestamp && b == scalarDate) {
		return scalarTimestamp
	}
	return scalarString
}

func scalarKindToFieldType(kind scalarInference) models.SchemaFieldType {
	switch kind {
	case scalarBoolean:
		return models.FieldTypeBoolean
	case scalarLong:
		return models.FieldTypeLong
	case scalarDouble:
		return models.FieldTypeDouble
	case scalarDate:
		return models.FieldTypeDate
	case scalarTimestamp:
		return models.FieldTypeTimestamp
	default:
		return models.FieldTypeString
	}
}

func inferJSONSchema(samples []schemaInferenceSample, opts models.CsvOptions, maxRows int) (models.DatasetSchema, int, []string) {
	records := []map[string]any{}
	warnings := []string{}
	for _, sample := range samples {
		next, nextWarnings := parseJSONRecords(sample.Text, sample.Path)
		warnings = append(warnings, nextWarnings...)
		for _, record := range next {
			records = append(records, record)
			if len(records) >= maxRows {
				break
			}
		}
		if len(records) >= maxRows {
			break
		}
	}
	if len(records) == 0 {
		return models.DatasetSchema{FileFormat: models.FileFormatText, CustomMetadata: &models.CustomMetadata{CSV: &opts}}, 0, append(warnings, "no JSON object rows were available for inference")
	}

	fieldOrder := []string{}
	stats := map[string][]models.Field{}
	nulls := map[string]int{}
	seenRows := map[string]int{}
	for _, record := range records {
		keys := make([]string, 0, len(record))
		for key := range record {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if _, ok := stats[key]; !ok {
				fieldOrder = append(fieldOrder, key)
			}
		}
		for _, key := range fieldOrder {
			value, ok := record[key]
			if !ok || value == nil {
				nulls[key]++
				continue
			}
			field, fieldWarnings := inferJSONValueField(key, value)
			warnings = append(warnings, fieldWarnings...)
			stats[key] = append(stats[key], field)
			seenRows[key]++
		}
	}

	fields := make([]models.Field, 0, len(fieldOrder)+3)
	for _, key := range fieldOrder {
		field := mergeJSONFields(key, stats[key], &warnings)
		field.Nullable = nulls[key] > 0 || seenRows[key] < len(records)
		fields = append(fields, field)
	}
	fields = appendHelperFields(fields, opts)
	return models.NormalizeDatasetSchema(models.DatasetSchema{
		Fields:         fields,
		FileFormat:     models.FileFormatText,
		CustomMetadata: &models.CustomMetadata{CSV: &opts},
	}), len(records), warnings
}

func parseJSONRecords(text string, source string) ([]map[string]any, []string) {
	warnings := []string{}
	records := []map[string]any{}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return records, warnings
	}
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err == nil {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				if record, ok := item.(map[string]any); ok {
					records = append(records, record)
				}
			}
			return records, warnings
		case map[string]any:
			return append(records, typed), warnings
		default:
			return records, append(warnings, source+" JSON sample did not contain objects")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(text)))
	for {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			if err == io.EOF {
				break
			}
			warnings = append(warnings, source+" JSON parse warning: "+err.Error())
			break
		}
		records = append(records, record)
	}
	return records, warnings
}

func inferJSONValueField(name string, value any) (models.Field, []string) {
	switch typed := value.(type) {
	case bool:
		return models.Field{Name: name, Type: models.FieldTypeBoolean}, nil
	case float64:
		if typed == float64(int64(typed)) {
			return models.Field{Name: name, Type: models.FieldTypeLong}, nil
		}
		return models.Field{Name: name, Type: models.FieldTypeDouble}, nil
	case string:
		kind := classifyStringValue(typed, models.CsvOptions{})
		return models.Field{Name: name, Type: scalarKindToFieldType(kind)}, nil
	case []any:
		if len(typed) == 0 {
			return models.Field{Name: name, Type: models.FieldTypeArray, ArraySubType: &models.Field{Type: models.FieldTypeString, Nullable: true}}, []string{"array field " + name + " was empty in the sample"}
		}
		subFields := []models.Field{}
		warnings := []string{}
		for _, item := range typed {
			sub, subWarnings := inferJSONValueField("", item)
			warnings = append(warnings, subWarnings...)
			subFields = append(subFields, sub)
		}
		sub := mergeJSONFields("", subFields, &warnings)
		return models.Field{Name: name, Type: models.FieldTypeArray, ArraySubType: &sub}, warnings
	case map[string]any:
		fields := make([]models.Field, 0, len(typed))
		warnings := []string{}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			nested := typed[key]
			sub, subWarnings := inferJSONValueField(key, nested)
			warnings = append(warnings, subWarnings...)
			fields = append(fields, sub)
		}
		return models.Field{Name: name, Type: models.FieldTypeStruct, SubSchemas: fields}, warnings
	default:
		return models.Field{Name: name, Type: models.FieldTypeString, Nullable: true}, nil
	}
}

func mergeJSONFields(name string, fields []models.Field, warnings *[]string) models.Field {
	if len(fields) == 0 {
		return models.Field{Name: name, Type: models.FieldTypeString, Nullable: true}
	}
	current := fields[0]
	current.Name = name
	for _, field := range fields[1:] {
		if current.Type == field.Type {
			if current.Type == models.FieldTypeStruct {
				current.SubSchemas = mergeStructFields(current.SubSchemas, field.SubSchemas, warnings)
			}
			if current.Type == models.FieldTypeArray && current.ArraySubType != nil && field.ArraySubType != nil {
				sub := mergeJSONFields("", []models.Field{*current.ArraySubType, *field.ArraySubType}, warnings)
				current.ArraySubType = &sub
			}
			continue
		}
		if (current.Type == models.FieldTypeLong && field.Type == models.FieldTypeDouble) || (current.Type == models.FieldTypeDouble && field.Type == models.FieldTypeLong) {
			current.Type = models.FieldTypeDouble
			continue
		}
		if (current.Type == models.FieldTypeDate && field.Type == models.FieldTypeTimestamp) || (current.Type == models.FieldTypeTimestamp && field.Type == models.FieldTypeDate) {
			current.Type = models.FieldTypeTimestamp
			continue
		}
		*warnings = append(*warnings, "field "+name+" had mixed JSON types; STRING was used")
		current = models.Field{Name: name, Type: models.FieldTypeString, Nullable: true}
	}
	return current
}

func mergeStructFields(a, b []models.Field, warnings *[]string) []models.Field {
	byName := map[string][]models.Field{}
	order := []string{}
	for _, field := range append(append([]models.Field{}, a...), b...) {
		if _, ok := byName[field.Name]; !ok {
			order = append(order, field.Name)
		}
		byName[field.Name] = append(byName[field.Name], field)
	}
	out := make([]models.Field, 0, len(order))
	for _, name := range order {
		out = append(out, mergeJSONFields(name, byName[name], warnings))
	}
	return out
}

func appendHelperFields(fields []models.Field, opts models.CsvOptions) []models.Field {
	if opts.FilePathColumn {
		fields = append(fields, models.Field{Name: "__file_path", Type: models.FieldTypeString, Nullable: false})
	}
	if opts.ImportedAtColumn {
		fields = append(fields, models.Field{Name: "__imported_at", Type: models.FieldTypeTimestamp, Nullable: false})
	}
	if opts.RowNumberColumn {
		fields = append(fields, models.Field{Name: "__row_number", Type: models.FieldTypeLong, Nullable: false})
	}
	return fields
}

func isCSVNull(value string, nullValue string) bool {
	if value == nullValue {
		return true
	}
	return strings.TrimSpace(value) == "" && nullValue == ""
}

func oneRune(value string) (rune, bool) {
	if value == "" {
		return ',', true
	}
	r, size := utf8.DecodeRuneInString(value)
	return r, r != utf8.RuneError && size == len(value)
}

func anyPathHasSuffix(paths []string, suffix string) bool {
	for _, p := range paths {
		if strings.HasSuffix(strings.ToLower(p), suffix) {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
