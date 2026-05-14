package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

const maxPreviewScanRows = 10000

var errTableReadUnavailable = errors.New("table read unavailable")

type tableRecord struct {
	values map[string]any
}

func (h *Handlers) ReadTableDataset(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	q, ok := foundryReadTableQuery(w, r)
	if !ok {
		return
	}
	format := "CSV"
	if q.Format != nil && strings.TrimSpace(*q.Format) != "" {
		format = strings.ToUpper(strings.TrimSpace(*q.Format))
	}
	if format == "ARROW" {
		writeJSONErr(w, http.StatusBadRequest, "ARROW readTable output is not supported by this deployment")
		return
	}
	if format != "CSV" {
		writeJSONErr(w, http.StatusBadRequest, "format must be CSV or ARROW")
		return
	}
	out, ok, err := h.previewRowsFromTableFiles(r.Context(), datasetID, nil, q, true)
	if err != nil {
		writeTableReadError(w, err)
		return
	}
	if !ok {
		writeJSONErr(w, http.StatusNotFound, "schema not found")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	writer := csv.NewWriter(w)
	_ = writer.Write(out.Columns)
	for _, row := range out.Rows {
		record := make([]string, len(row))
		for i := range row {
			record[i] = csvCell(row[i])
		}
		_ = writer.Write(record)
	}
	writer.Flush()
}

func foundryReadTableQuery(w http.ResponseWriter, r *http.Request) (models.PreviewQuery, bool) {
	q := previewQuery(r)
	params := r.URL.Query()
	if raw := strings.TrimSpace(params.Get("branchName")); raw != "" {
		q.Branch = &raw
	}
	if raw := strings.TrimSpace(params.Get("rowLimit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeJSONErr(w, http.StatusBadRequest, "invalid rowLimit")
			return q, false
		}
		q.Limit = &n
	}
	if raw := strings.TrimSpace(params.Get("endTransactionRid")); raw != "" {
		txn, err := parseFoundryTransactionRID(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid endTransactionRid")
			return q, false
		}
		q.TransactionID = &txn
	}
	if raw := strings.TrimSpace(params.Get("startTransactionRid")); raw != "" {
		if _, err := parseFoundryTransactionRID(raw); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid startTransactionRid")
			return q, false
		}
	}
	return q, true
}

func (h *Handlers) previewRowsFromTableFiles(ctx context.Context, datasetID uuid.UUID, scopedViewID *uuid.UUID, q models.PreviewQuery, strict bool) (*models.PreviewDataResponse, bool, error) {
	local, ok := h.BackingFS.(localObjectStore)
	if h.BackingFS == nil || !ok || h.BackingFS.FSID() != "local" {
		if strict {
			return nil, false, errTableReadUnavailable
		}
		return nil, false, nil
	}
	if scopedViewID != nil {
		if out, ok, err := h.previewRowsFromLogicalView(ctx, local, datasetID, *scopedViewID, q, strict); err != nil || ok {
			return out, ok, err
		}
	}

	view, schema, err := h.tableReadViewAndSchema(ctx, datasetID, scopedViewID, q)
	if err != nil {
		if !strict && errors.Is(err, repo.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if schema == nil || len(schema.Schema.Fields) == 0 {
		if strict {
			return nil, false, repo.ErrNotFound
		}
		return nil, false, nil
	}

	opts, optionWarnings := normalizeInferenceCSVOptions(csvOptionsFromSchema(schema.Schema), "", nil)
	warnings := append([]string{}, optionWarnings...)
	records, parseErrors := readTableRecordsFromRuntimeFiles(local, view.Files, schema.Schema.Fields, opts, &warnings)
	return finalizeTablePreview(datasetID, view.ID, view.ResolvedBranch, schema.Schema, records, warnings, parseErrors, q), true, nil
}

func (h *Handlers) previewRowsFromLogicalView(ctx context.Context, local localObjectStore, datasetID uuid.UUID, viewID uuid.UUID, q models.PreviewQuery, strict bool) (*models.PreviewDataResponse, bool, error) {
	backing, err := h.Repo.ListViewBackingDatasets(ctx, datasetID, viewID)
	if err != nil {
		if !strict && errors.Is(err, repo.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(backing) == 0 {
		return nil, false, nil
	}
	schema, err := h.Repo.GetViewSchema(ctx, viewID)
	if err != nil {
		return nil, false, err
	}
	if schema == nil || len(schema.Schema.Fields) == 0 {
		if strict {
			return nil, false, repo.ErrNotFound
		}
		return nil, false, nil
	}
	opts, optionWarnings := normalizeInferenceCSVOptions(csvOptionsFromSchema(schema.Schema), "", nil)
	warnings := append([]string{"logical view read unions backing datasets; it does not read stored view files"}, optionWarnings...)
	parseErrors := []models.TableParseError{}
	records := []tableRecord{}
	for _, ref := range backing {
		if len(records) >= maxPreviewScanRows {
			warnings = append(warnings, fmt.Sprintf("preview scan stopped after %d rows", maxPreviewScanRows))
			break
		}
		branch := strings.TrimSpace(ref.Branch)
		if q.Branch != nil && strings.TrimSpace(*q.Branch) != "" {
			branch = strings.TrimSpace(*q.Branch)
		}
		if branch == "" {
			dataset, err := h.Repo.GetDataset(ctx, ref.DatasetID)
			if err != nil {
				return nil, false, err
			}
			if dataset != nil {
				branch = strings.TrimSpace(dataset.ActiveBranch)
			}
		}
		if branch == "" {
			branch = "main"
		}
		view, err := h.Repo.GetViewAt(ctx, ref.DatasetID, branch, nil, nil, nil)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				warnings = append(warnings, "backing dataset has no readable view: "+ref.DatasetRID)
				continue
			}
			return nil, false, err
		}
		files := annotateBackingFiles(view.Files, ref)
		next, nextErrors := readTableRecordsFromRuntimeFiles(local, files, schema.Schema.Fields, opts, &warnings)
		parseErrors = append(parseErrors, nextErrors...)
		remaining := maxPreviewScanRows - len(records)
		if len(next) > remaining {
			next = next[:remaining]
		}
		records = append(records, next...)
	}
	primaryKey, err := h.Repo.GetViewPrimaryKey(ctx, datasetID, viewID)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		return nil, false, err
	}
	records = dedupeTableRecordsByPrimaryKey(records, primaryKey)
	return finalizeTablePreview(datasetID, viewID, "logical", schema.Schema, records, warnings, parseErrors, q), true, nil
}

func readTableRecordsFromRuntimeFiles(local localObjectStore, files []models.RuntimeViewFile, fields []models.Field, opts models.CsvOptions, warnings *[]string) ([]tableRecord, []models.TableParseError) {
	records := []tableRecord{}
	parseErrors := []models.TableParseError{}
	files = append([]models.RuntimeViewFile(nil), files...)
	sort.Slice(files, func(i, j int) bool { return files[i].LogicalPath < files[j].LogicalPath })
	for _, file := range files {
		if len(records) >= maxPreviewScanRows {
			*warnings = append(*warnings, fmt.Sprintf("preview scan stopped after %d rows", maxPreviewScanRows))
			break
		}
		data, err := readRuntimeViewFile(local, file)
		if err != nil {
			*warnings = append(*warnings, "could not read "+file.LogicalPath+": "+err.Error())
			continue
		}
		var next []tableRecord
		var nextErrors []models.TableParseError
		if tableFileLooksJSON(file) {
			next, nextErrors = parseJSONTableRecords(file.LogicalPath, string(data), fields, opts)
		} else {
			next, nextErrors = parseCSVTableRecords(file.LogicalPath, string(data), fields, opts)
		}
		parseErrors = append(parseErrors, nextErrors...)
		remaining := maxPreviewScanRows - len(records)
		if len(next) > remaining {
			next = next[:remaining]
		}
		records = append(records, next...)
	}
	return records, parseErrors
}

func annotateBackingFiles(files []models.RuntimeViewFile, ref models.ViewBackingDataset) []models.RuntimeViewFile {
	prefix := strings.TrimSpace(ref.Alias)
	if prefix == "" {
		prefix = ref.DatasetRID
	}
	out := make([]models.RuntimeViewFile, 0, len(files))
	for _, file := range files {
		file.LogicalPath = strings.Trim(prefix, "/") + "/" + strings.TrimLeft(file.LogicalPath, "/")
		out = append(out, file)
	}
	return out
}

func finalizeTablePreview(datasetID uuid.UUID, viewID uuid.UUID, branch string, schema models.DatasetSchema, records []tableRecord, warnings []string, parseErrors []models.TableParseError, q models.PreviewQuery) *models.PreviewDataResponse {
	limit, offset := previewLimitOffset(q)
	var filterWarnings []string
	records, filterWarnings = applyTableFilter(records, q.Filter)
	warnings = append(warnings, filterWarnings...)
	if len(q.Sort) > 0 {
		sortTableRecords(records, q.Sort)
	}
	totalRows := len(records)
	if q.Sample {
		records = sampleTableRecords(records, q)
	}
	columns, columnWarnings := selectPreviewColumns(schema.Fields, q.Columns)
	warnings = append(warnings, columnWarnings...)
	rows := projectTableRows(records, columns)
	if offset > len(rows) {
		offset = len(rows)
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	if offset < end {
		rows = rows[offset:end]
	} else {
		rows = [][]models.JSONValue{}
	}
	format := schema.FileFormat
	if format == "" {
		format = models.FileFormatText
	}
	return &models.PreviewDataResponse{
		DatasetID:   datasetID,
		ViewID:      &viewID,
		Branch:      branch,
		Columns:     columns,
		Rows:        rows,
		Format:      format,
		Limit:       limit,
		Offset:      offset,
		TotalRows:   totalRows,
		Warnings:    dedupeStrings(warnings),
		ParseErrors: parseErrors,
		Sampled:     q.Sample,
	}
}

func (h *Handlers) tableReadViewAndSchema(ctx context.Context, datasetID uuid.UUID, scopedViewID *uuid.UUID, q models.PreviewQuery) (*models.ViewOut, *models.SchemaResponse, error) {
	if scopedViewID != nil {
		files, err := h.Repo.ListViewFiles(ctx, datasetID, *scopedViewID)
		if err != nil {
			return nil, nil, err
		}
		schema, err := h.Repo.GetViewSchema(ctx, *scopedViewID)
		if err != nil {
			return nil, nil, err
		}
		return &models.ViewOut{ID: *scopedViewID, DatasetID: datasetID, Files: files, FileCount: int32(len(files))}, schema, nil
	}
	branch := ""
	if q.Branch != nil {
		branch = strings.TrimSpace(*q.Branch)
	}
	if branch == "" {
		dataset, err := h.Repo.GetDataset(ctx, datasetID)
		if err != nil {
			return nil, nil, err
		}
		branch = strings.TrimSpace(dataset.ActiveBranch)
	}
	if branch == "" {
		branch = "main"
	}
	view, err := h.Repo.GetViewAt(ctx, datasetID, branch, nil, q.TransactionID, q.Version)
	if errors.Is(err, repo.ErrNotFound) && branch == "master" {
		branch = "main"
		view, err = h.Repo.GetViewAt(ctx, datasetID, branch, nil, q.TransactionID, q.Version)
	}
	if err != nil {
		return nil, nil, err
	}
	schema, err := h.Repo.GetViewSchema(ctx, view.ID)
	if err != nil {
		return nil, nil, err
	}
	if schema == nil {
		schema, err = h.Repo.GetCurrentSchema(ctx, datasetID, view.ResolvedBranch)
		if err != nil {
			return nil, nil, err
		}
	}
	return view, schema, nil
}

func previewLimitOffset(q models.PreviewQuery) (int, int) {
	limit := 100
	if q.Limit != nil {
		limit = *q.Limit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := 0
	if q.Offset != nil && *q.Offset > 0 {
		offset = *q.Offset
	}
	return limit, offset
}

func readRuntimeViewFile(local localObjectStore, file models.RuntimeViewFile) ([]byte, error) {
	location := storageabstraction.ParsePhysicalURI(file.PhysicalPath)
	key := strings.TrimLeft(path.Join(location.BaseDirectory, location.RelativePath), "/")
	if key == "" {
		key = strings.TrimLeft(location.RelativePath, "/")
	}
	return local.ReadLocalObject(key)
}

func tableFileLooksJSON(file models.RuntimeViewFile) bool {
	lower := strings.ToLower(file.LogicalPath)
	return strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".jsonl") || strings.HasSuffix(lower, ".ndjson")
}

func csvOptionsFromSchema(schema models.DatasetSchema) *models.CsvOptions {
	if schema.CustomMetadata == nil || schema.CustomMetadata.CSV == nil {
		return nil
	}
	return schema.CustomMetadata.CSV
}

func parseCSVTableRecords(filePath string, text string, fields []models.Field, opts models.CsvOptions) ([]tableRecord, []models.TableParseError) {
	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = opts.ParseErrorBehavior != "ERROR"
	if comma, ok := oneRune(opts.Delimiter); ok {
		reader.Comma = comma
	}
	records := []tableRecord{}
	parseErrors := []models.TableParseError{}
	header := []string{}
	line := 0
	dataRow := 0
	for {
		row, err := reader.Read()
		if err != nil {
			if errors.Is(err, csv.ErrFieldCount) {
				parseErrors = append(parseErrors, models.TableParseError{FilePath: filePath, Row: line + 1, Kind: "CSV_FIELD_COUNT", Message: err.Error()})
				if opts.ParseErrorBehavior == "ERROR" {
					break
				}
				continue
			}
			if errors.Is(err, csv.ErrBareQuote) || errors.Is(err, csv.ErrQuote) {
				parseErrors = append(parseErrors, models.TableParseError{FilePath: filePath, Row: line + 1, Kind: "CSV_PARSE", Message: err.Error()})
				if opts.ParseErrorBehavior == "ERROR" {
					break
				}
				continue
			}
			if err != nil {
				break
			}
		}
		line++
		if line <= opts.SkipLines {
			continue
		}
		if opts.Header && len(header) == 0 {
			header = sanitizeHeader(row)
			continue
		}
		dataRow++
		values := map[string]any{}
		headerIndex := headerLookup(header)
		for fieldIndex, field := range fields {
			value, column, perr := csvFieldValue(filePath, dataRow, fieldIndex, field, row, headerIndex, opts)
			if perr != nil {
				if column != nil {
					perr.Column = column
				}
				parseErrors = append(parseErrors, *perr)
			}
			values[field.Name] = value
		}
		records = append(records, tableRecord{values: values})
	}
	return records, parseErrors
}

func headerLookup(header []string) map[string]int {
	out := map[string]int{}
	for i, name := range header {
		out[name] = i
	}
	return out
}

func csvFieldValue(filePath string, row int, fieldIndex int, field models.Field, record []string, header map[string]int, opts models.CsvOptions) (any, *int, *models.TableParseError) {
	if helper, ok := helperColumnValue(field.Name, filePath, row); ok {
		return helper, nil, nil
	}
	column := fieldIndex
	if len(header) > 0 {
		idx, ok := header[field.Name]
		if !ok {
			return nil, nil, &models.TableParseError{FilePath: filePath, Row: row, Field: field.Name, Kind: "MISSING_COLUMN", Message: "column not found in CSV header"}
		}
		column = idx
	}
	if column >= len(record) {
		c := column + 1
		return nil, &c, &models.TableParseError{FilePath: filePath, Row: row, Column: &c, Field: field.Name, Kind: "MISSING_VALUE", Message: "row does not contain this column"}
	}
	c := column + 1
	value := record[column]
	coerced, err := coerceTableValue(field, value, opts)
	if err != nil {
		return nil, &c, &models.TableParseError{FilePath: filePath, Row: row, Column: &c, Field: field.Name, Kind: "TYPE_MISMATCH", Message: err.Error(), Value: value}
	}
	return coerced, &c, nil
}

func parseJSONTableRecords(filePath string, text string, fields []models.Field, opts models.CsvOptions) ([]tableRecord, []models.TableParseError) {
	rows, warnings := parseJSONRecords(text, filePath)
	parseErrors := make([]models.TableParseError, 0, len(warnings))
	for _, warning := range warnings {
		parseErrors = append(parseErrors, models.TableParseError{FilePath: filePath, Kind: "JSON_PARSE", Message: warning})
	}
	out := make([]tableRecord, 0, len(rows))
	for rowIndex, row := range rows {
		values := map[string]any{}
		for _, field := range fields {
			if helper, ok := helperColumnValue(field.Name, filePath, rowIndex+1); ok {
				values[field.Name] = helper
				continue
			}
			raw, ok := row[field.Name]
			if !ok || raw == nil {
				if !field.Nullable {
					parseErrors = append(parseErrors, models.TableParseError{FilePath: filePath, Row: rowIndex + 1, Field: field.Name, Kind: "MISSING_VALUE", Message: "required JSON field is missing"})
				}
				values[field.Name] = nil
				continue
			}
			coerced, err := coerceTableValue(field, raw, opts)
			if err != nil {
				parseErrors = append(parseErrors, models.TableParseError{FilePath: filePath, Row: rowIndex + 1, Field: field.Name, Kind: "TYPE_MISMATCH", Message: err.Error(), Value: fmt.Sprint(raw)})
				values[field.Name] = nil
				continue
			}
			values[field.Name] = coerced
		}
		out = append(out, tableRecord{values: values})
	}
	return out, parseErrors
}

func helperColumnValue(name string, filePath string, row int) (any, bool) {
	switch name {
	case "__file_path":
		return filePath, true
	case "__imported_at":
		return time.Now().UTC().Format(time.RFC3339), true
	case "__row_number":
		return row, true
	default:
		return nil, false
	}
}

func coerceTableValue(field models.Field, raw any, opts models.CsvOptions) (any, error) {
	if raw == nil {
		if field.Nullable {
			return nil, nil
		}
		return nil, fmt.Errorf("field %s is not nullable", field.Name)
	}
	if s, ok := raw.(string); ok && isCSVNull(s, opts.NullValue) {
		if field.Nullable {
			return nil, nil
		}
		return nil, fmt.Errorf("field %s is not nullable", field.Name)
	}
	switch field.Type {
	case models.FieldTypeString:
		return stringifyTableValue(raw), nil
	case models.FieldTypeBoolean:
		return coerceBool(raw)
	case models.FieldTypeByte, models.FieldTypeShort, models.FieldTypeInteger, models.FieldTypeLong:
		return coerceInteger(raw, field.Type)
	case models.FieldTypeFloat, models.FieldTypeDouble, models.FieldTypeDecimal:
		return coerceFloat(raw)
	case models.FieldTypeDate:
		return coerceDate(raw, opts.DateFormat)
	case models.FieldTypeTimestamp:
		return coerceTimestamp(raw, opts.TimestampFormat)
	case models.FieldTypeArray:
		return coerceComplex(raw, "array")
	case models.FieldTypeMap, models.FieldTypeStruct:
		return coerceComplex(raw, "object")
	case models.FieldTypeBinary:
		return stringifyTableValue(raw), nil
	default:
		return raw, nil
	}
}

func coerceBool(raw any) (any, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case string:
		lower := strings.ToLower(strings.TrimSpace(v))
		if lower == "true" {
			return true, nil
		}
		if lower == "false" {
			return false, nil
		}
	}
	return nil, fmt.Errorf("expected BOOLEAN")
}

func coerceInteger(raw any, fieldType models.SchemaFieldType) (any, error) {
	var n int64
	switch v := raw.(type) {
	case float64:
		if math.Trunc(v) != v {
			return nil, fmt.Errorf("expected integral %s", fieldType)
		}
		n = int64(v)
	case int:
		n = int64(v)
	case int64:
		n = v
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("expected %s", fieldType)
		}
		n = parsed
	default:
		return nil, fmt.Errorf("expected %s", fieldType)
	}
	switch fieldType {
	case models.FieldTypeByte:
		if n < -128 || n > 127 {
			return nil, fmt.Errorf("BYTE out of range")
		}
	case models.FieldTypeShort:
		if n < -32768 || n > 32767 {
			return nil, fmt.Errorf("SHORT out of range")
		}
	case models.FieldTypeInteger:
		if n < math.MinInt32 || n > math.MaxInt32 {
			return nil, fmt.Errorf("INTEGER out of range")
		}
	}
	return n, nil
}

func coerceFloat(raw any) (any, error) {
	switch v := raw.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, fmt.Errorf("expected numeric value")
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("expected numeric value")
	}
}

func coerceDate(raw any, custom *string) (any, error) {
	value := stringifyTableValue(raw)
	layouts := []string{"2006-01-02", "2006/01/02", "01/02/2006"}
	if custom != nil && strings.TrimSpace(*custom) != "" {
		layouts = append([]string{*custom}, layouts...)
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}
	return nil, fmt.Errorf("expected DATE")
}

func coerceTimestamp(raw any, custom *string) (any, error) {
	value := stringifyTableValue(raw)
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}
	if custom != nil && strings.TrimSpace(*custom) != "" {
		layouts = append([]string{*custom}, layouts...)
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339), nil
		}
	}
	return nil, fmt.Errorf("expected TIMESTAMP")
}

func coerceComplex(raw any, kind string) (any, error) {
	if s, ok := raw.(string); ok {
		var decoded any
		if err := json.Unmarshal([]byte(s), &decoded); err != nil {
			return nil, fmt.Errorf("expected JSON %s", kind)
		}
		raw = decoded
	}
	switch kind {
	case "array":
		if _, ok := raw.([]any); !ok {
			return nil, fmt.Errorf("expected ARRAY")
		}
	case "object":
		if _, ok := raw.(map[string]any); !ok {
			return nil, fmt.Errorf("expected object")
		}
	}
	return raw, nil
}

func stringifyTableValue(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func applyTableFilter(records []tableRecord, raw *string) ([]tableRecord, []string) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return records, nil
	}
	filter, ok := parseTableFilter(*raw)
	if !ok {
		return records, []string{"filter was ignored because it could not be parsed"}
	}
	out := []tableRecord{}
	for _, record := range records {
		if filter.match(record.values[filter.column]) {
			out = append(out, record)
		}
	}
	return out, nil
}

type tableFilter struct {
	column string
	op     string
	value  string
}

func parseTableFilter(raw string) (tableFilter, bool) {
	raw = strings.TrimSpace(raw)
	for _, op := range []string{">=", "<=", "!=", "=", ">", "<", "~"} {
		if before, after, ok := strings.Cut(raw, op); ok {
			column := strings.TrimSpace(before)
			value := strings.Trim(strings.TrimSpace(after), `"'`)
			return tableFilter{column: column, op: op, value: value}, column != ""
		}
	}
	return tableFilter{}, false
}

func (f tableFilter) match(raw any) bool {
	left := stringifyTableValue(raw)
	switch f.op {
	case "=":
		return left == f.value
	case "!=":
		return left != f.value
	case "~":
		return strings.Contains(strings.ToLower(left), strings.ToLower(f.value))
	}
	lf, lerr := strconv.ParseFloat(left, 64)
	rf, rerr := strconv.ParseFloat(f.value, 64)
	if lerr != nil || rerr != nil {
		switch f.op {
		case ">":
			return left > f.value
		case "<":
			return left < f.value
		case ">=":
			return left >= f.value
		case "<=":
			return left <= f.value
		}
		return false
	}
	switch f.op {
	case ">":
		return lf > rf
	case "<":
		return lf < rf
	case ">=":
		return lf >= rf
	case "<=":
		return lf <= rf
	default:
		return false
	}
}

func sortTableRecords(records []tableRecord, sortSpec []string) {
	sort.SliceStable(records, func(i, j int) bool {
		for _, raw := range sortSpec {
			column, desc := parseSortToken(raw)
			if column == "" {
				continue
			}
			left := records[i].values[column]
			right := records[j].values[column]
			if left == nil && right != nil {
				return false
			}
			if left != nil && right == nil {
				return true
			}
			cmp := compareTableValues(records[i].values[column], records[j].values[column])
			if cmp == 0 {
				continue
			}
			if desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
}

func parseSortToken(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	desc := false
	if strings.HasPrefix(raw, "-") {
		desc = true
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "-"))
	}
	if before, after, ok := strings.Cut(raw, ":"); ok {
		raw = strings.TrimSpace(before)
		desc = strings.EqualFold(strings.TrimSpace(after), "desc")
	}
	if parts := strings.Fields(raw); len(parts) == 2 {
		raw = parts[0]
		desc = strings.EqualFold(parts[1], "desc")
	}
	return raw, desc
}

func compareTableValues(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1
	}
	if b == nil {
		return -1
	}
	af, aerr := strconv.ParseFloat(stringifyTableValue(a), 64)
	bf, berr := strconv.ParseFloat(stringifyTableValue(b), 64)
	if aerr == nil && berr == nil {
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}
	return strings.Compare(stringifyTableValue(a), stringifyTableValue(b))
}

func sampleTableRecords(records []tableRecord, q models.PreviewQuery) []tableRecord {
	size := 100
	if q.SampleSize != nil && *q.SampleSize > 0 {
		size = *q.SampleSize
	} else if q.Limit != nil && *q.Limit > 0 {
		size = *q.Limit
	}
	if size >= len(records) {
		return records
	}
	seed := int64(1)
	if q.SampleSeed != nil {
		seed = *q.SampleSeed
	}
	rng := rand.New(rand.NewSource(seed))
	out := append([]tableRecord(nil), records...)
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out[:size]
}

func dedupeTableRecordsByPrimaryKey(records []tableRecord, primaryKey []string) []tableRecord {
	keys := []string{}
	for _, column := range primaryKey {
		column = strings.TrimSpace(column)
		if column != "" {
			keys = append(keys, column)
		}
	}
	if len(keys) == 0 || len(records) < 2 {
		return records
	}
	order := []string{}
	seen := map[string]bool{}
	latest := map[string]tableRecord{}
	for _, record := range records {
		key, ok := tableRecordPrimaryKey(record, keys)
		if !ok {
			key = "__row_without_primary_key__" + strconv.Itoa(len(order))
			order = append(order, key)
			latest[key] = record
			continue
		}
		if !seen[key] {
			order = append(order, key)
			seen[key] = true
		}
		latest[key] = record
	}
	out := make([]tableRecord, 0, len(order))
	for _, key := range order {
		if record, ok := latest[key]; ok {
			out = append(out, record)
		}
	}
	return out
}

func tableRecordPrimaryKey(record tableRecord, columns []string) (string, bool) {
	values := make([]string, 0, len(columns))
	for _, column := range columns {
		value, ok := record.values[column]
		if !ok || value == nil {
			return "", false
		}
		values = append(values, stringifyTableValue(value))
	}
	return strings.Join(values, "\x00"), true
}

func selectPreviewColumns(fields []models.Field, requested []string) ([]string, []string) {
	all := make([]string, 0, len(fields))
	known := map[string]bool{}
	for _, field := range fields {
		all = append(all, field.Name)
		known[field.Name] = true
	}
	if len(requested) == 0 {
		return all, nil
	}
	out := []string{}
	warnings := []string{}
	for _, column := range requested {
		column = strings.TrimSpace(column)
		if column == "" {
			continue
		}
		if !known[column] {
			warnings = append(warnings, "unknown column ignored: "+column)
			continue
		}
		out = append(out, column)
	}
	if len(out) == 0 {
		return all, warnings
	}
	return out, warnings
}

func projectTableRows(records []tableRecord, columns []string) [][]models.JSONValue {
	out := make([][]models.JSONValue, 0, len(records))
	for _, record := range records {
		row := make([]models.JSONValue, 0, len(columns))
		for _, column := range columns {
			raw, _ := models.MarshalJSONValue(record.values[column])
			row = append(row, raw)
		}
		out = append(out, row)
	}
	return out
}

func csvCell(raw models.JSONValue) string {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	if decoded == nil {
		return ""
	}
	switch v := decoded.(type) {
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func writeTableReadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errTableReadUnavailable):
		writeDependencyUnavailable(w, "table_read_unavailable", "local table read requires a configured local backing filesystem")
	case errors.Is(err, repo.ErrNotFound):
		writeJSONErr(w, http.StatusNotFound, "schema not found")
	default:
		writeJSONErr(w, http.StatusInternalServerError, "read table failed")
	}
}
