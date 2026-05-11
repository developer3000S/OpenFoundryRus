// Package actions ports `libs/ontology-kernel/src/handlers/actions.rs`.
//
// The Rust file is the largest single handler in the kernel (5,618 LOC): it
// covers action-type CRUD, validate, execute, inline-edit (single + batch),
// what-if branches, attachment upload, and metrics. The Go port executes the
// concrete object/link/function operations plus the interface-typed action
// operations (`create_interface`, `modify_interface`, `delete_interface`,
// `create_interface_link`, `delete_interface_link`) by resolving interface IDs
// to concrete implementing object types at plan time.
package actions

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// Mount registers every actions endpoint on the chi router under the
// same path / verb shape as the Rust source.
func Mount(r chi.Router, state *ontologykernel.AppState) {
	r.Get("/actions", ListActionTypes(state))
	r.Post("/actions", CreateActionType(state))
	r.Get("/actions/{id}", GetActionType(state))
	r.Put("/actions/{id}", UpdateActionType(state))
	// Keep PUT for route parity, and accept PATCH for the frontend edit drawer contract.
	r.Patch("/actions/{id}", UpdateActionType(state))
	r.Delete("/actions/{id}", DeleteActionType(state))
	r.Post("/actions/{id}/validate", ValidateAction(state))
	r.Post("/actions/{id}/execute", ExecuteAction(state))
	r.Get("/actions/{id}/metrics", GetActionMetrics(state))
	r.Post("/actions/{id}/execute-batch", ExecuteActionBatchHandler(state))
	r.Get("/actions/{id}/what-if", ListActionWhatIfBranches(state))
	r.Post("/actions/{id}/what-if", CreateActionWhatIfBranch(state))
	r.Delete("/actions/{id}/what-if/{branch_id}", DeleteActionWhatIfBranch(state))
	r.Post("/types/{type_id}/properties/{property_id}/objects/{obj_id}/inline-edit", ExecuteInlineEditHandler(state))
	r.Post("/types/{type_id}/inline-edit-batch", ExecuteInlineEditBatchHandler(state))
	r.Get("/types/{type_id}/applicable-actions", ListApplicableActions(state))
	r.Post("/actions/uploads", UploadActionAttachment(state))
}

// ── Action-type CRUD (full 1:1) ─────────────────────────────────────

// ListActionTypes mirrors `pub async fn list_action_types`. Pages
// through the DefinitionStore via the action_repository helper.
func ListActionTypes(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := parseListActionTypesQuery(r)
		page := defaultPage(query.Page)
		perPage := defaultPerPage(query.PerPage)
		offset := (page - 1) * perPage

		total, err := domain.CountActionRows(r.Context(), state.Stores.Definitions, query.ObjectTypeID, query.Search)
		if err != nil {
			total = 0 // matches Rust `.unwrap_or(0)`.
		}

		offsetTok := strconv.FormatInt(offset, 10)
		listed, err := domain.ListActionRows(r.Context(), state.Stores.Definitions, domain.ActionTypeListQuery{
			ObjectTypeID: query.ObjectTypeID,
			Search:       query.Search,
			Page:         storage.Page{Size: uint32(perPage), Token: &offsetTok},
		})
		var rows []models.ActionTypeRow
		if err == nil {
			rows = listed.Items
		}

		data := make([]models.ActionType, 0, len(rows))
		for _, row := range rows {
			data = append(data, row.IntoAction())
		}
		writeJSON(w, http.StatusOK, models.ListActionTypesResponse{
			Data:    data,
			Total:   int64(total),
			Page:    page,
			PerPage: perPage,
		})
	}
}

// CreateActionType mirrors `pub async fn create_action_type`.
func CreateActionType(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		var body models.CreateActionTypeRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			invalid(w, "action type name is required")
			return
		}
		config := body.Config
		if len(config) == 0 {
			config = json.RawMessage(`null`)
		}
		if err := validateActionDefinition(r, state, body.ObjectTypeID, body.OperationKind, body.InputSchema, body.AuthorizationPolicy, config); err != nil {
			invalid(w, err.Error())
			return
		}

		displayName := body.Name
		if body.DisplayName != nil {
			displayName = *body.DisplayName
		}
		description := ""
		if body.Description != nil {
			description = *body.Description
		}
		var inputSchema []models.ActionInputField
		if body.InputSchema != nil {
			inputSchema = *body.InputSchema
		}
		var formSchema models.ActionFormSchema
		if body.FormSchema != nil {
			formSchema = *body.FormSchema
		}
		var authPolicy models.ActionAuthorizationPolicy
		if body.AuthorizationPolicy != nil {
			authPolicy = *body.AuthorizationPolicy
		}
		confirmation := false
		if body.ConfirmationRequired != nil {
			confirmation = *body.ConfirmationRequired
		}

		now := nowUTC()
		id, _ := uuid.NewV7()
		action := models.ActionType{
			ID:                   id,
			Name:                 body.Name,
			DisplayName:          displayName,
			Description:          description,
			ObjectTypeID:         body.ObjectTypeID,
			OperationKind:        body.OperationKind,
			InputSchema:          inputSchema,
			FormSchema:           formSchema,
			Config:               config,
			ConfirmationRequired: confirmation,
			PermissionKey:        body.PermissionKey,
			AuthorizationPolicy:  authPolicy,
			OwnerID:              claims.Sub,
			CreatedAt:            now,
			UpdatedAt:            now,
		}

		if _, err := domain.PutAction(r.Context(), state.Stores.Definitions, action); err != nil {
			dbError(w, "create action type failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, action)
	}
}

// GetActionType mirrors `pub async fn get_action_type`.
func GetActionType(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		row, err := domain.GetActionRow(r.Context(), state.Stores.Definitions, id)
		if err != nil {
			dbError(w, "failed to load action type: "+err.Error())
			return
		}
		if row == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		writeJSON(w, http.StatusOK, row.IntoAction())
	}
}

// UpdateActionType mirrors `pub async fn update_action_type`.
func UpdateActionType(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		var body models.UpdateActionTypeRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		row, err := domain.GetActionRow(r.Context(), state.Stores.Definitions, id)
		if err != nil {
			dbError(w, "failed to load action type: "+err.Error())
			return
		}
		if row == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		existing := row.IntoAction()

		operationKind := existing.OperationKind
		if body.OperationKind != nil {
			operationKind = *body.OperationKind
		}
		inputSchema := existing.InputSchema
		if body.InputSchema != nil {
			inputSchema = *body.InputSchema
		}
		formSchema := existing.FormSchema
		if body.FormSchema != nil {
			formSchema = *body.FormSchema
		}
		config := existing.Config
		if len(body.Config) > 0 {
			config = body.Config
		}
		authPolicy := existing.AuthorizationPolicy
		if body.AuthorizationPolicy != nil {
			authPolicy = *body.AuthorizationPolicy
		}
		// Validate the new envelope.
		schemaPtr := &inputSchema
		policyPtr := &authPolicy
		if err := validateActionDefinition(r, state, existing.ObjectTypeID, operationKind, schemaPtr, policyPtr, config); err != nil {
			invalid(w, err.Error())
			return
		}

		updated := models.ActionType{
			ID:                   existing.ID,
			Name:                 existing.Name,
			DisplayName:          coalesceString(body.DisplayName, existing.DisplayName),
			Description:          coalesceString(body.Description, existing.Description),
			ObjectTypeID:         existing.ObjectTypeID,
			OperationKind:        operationKind,
			InputSchema:          inputSchema,
			FormSchema:           formSchema,
			Config:               config,
			ConfirmationRequired: coalesceBool(body.ConfirmationRequired, existing.ConfirmationRequired),
			PermissionKey:        coalescePermissionKey(body.PermissionKey, existing.PermissionKey),
			AuthorizationPolicy:  authPolicy,
			OwnerID:              existing.OwnerID,
			CreatedAt:            existing.CreatedAt,
			UpdatedAt:            nowUTC(),
		}
		if _, err := domain.PutAction(r.Context(), state.Stores.Definitions, updated); err != nil {
			dbError(w, "failed to update action type: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

// DeleteActionType mirrors `pub async fn delete_action_type`.
func DeleteActionType(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		ok, err := domain.DeleteAction(r.Context(), state.Stores.Definitions, id)
		if err != nil {
			dbError(w, "failed to delete action type: "+err.Error())
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ListApplicableActions mirrors `pub async fn list_applicable_actions`.
// Filters by selection_kind ("single" / "bulk"); empty filter returns
// both. Mirrors the inline `action_selection_kind` classifier.
func ListApplicableActions(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		typeID, err := pathUUID(r, "type_id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		// Note: Rust uses "selection_kind" lower-cased; we match
		// verbatim so `?selection_kind=Single` is still accepted.
		selection := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("selection_kind")))

		largePage := "0"
		listed, err := domain.ListActionRows(r.Context(), state.Stores.Definitions, domain.ActionTypeListQuery{
			ObjectTypeID: &typeID,
			Page:         storage.Page{Size: 500, Token: &largePage},
		})
		var rows []models.ActionTypeRow
		if err == nil {
			rows = listed.Items
		}
		data := []map[string]any{}
		for _, row := range rows {
			action := row.IntoAction()
			kind := actionSelectionKind(action)
			include := true
			switch selection {
			case "single":
				include = kind == "single"
			case "bulk":
				include = kind == "bulk"
			}
			if include {
				data = append(data, map[string]any{
					"action_type":    action,
					"selection_kind": kind,
				})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": data, "total": len(data)})
	}
}

// actionSelectionKind mirrors `fn action_selection_kind`. Bulk if
// any input field references a list of objects.
func actionSelectionKind(a models.ActionType) string {
	for _, field := range a.InputSchema {
		switch field.PropertyType {
		case "object_reference_list", "object_set":
			return "bulk"
		}
	}
	return "single"
}

// ── validateActionDefinition (Phase 5A subset) ──────────────────────

// validateActionDefinition mirrors the Rust `validate_action_definition`
// helper at the contract level: object_type must exist, operation_kind
// must be a known enum, every input field must declare a valid
// property_type, authorization_policy must look well-formed.
//
// The full Rust function (~500 LOC) also walks form_schema sections,
// notification side-effects, webhook configs, update_object property
// mappings and create_link configs. Those land in Phase 5B alongside
// the execute-path that consumes them — every handler that
// constructs an ActionType today uses this lightweight validator,
// so wire-format errors stay predictable.
func validateActionDefinition(
	r *http.Request,
	state *ontologykernel.AppState,
	objectTypeID uuid.UUID,
	operationKindRaw string,
	inputSchema *[]models.ActionInputField,
	authorizationPolicy *models.ActionAuthorizationPolicy,
	config json.RawMessage,
) error {
	exists, err := domain.ActionRepoObjectTypeExists(r.Context(), state.Stores.Definitions, objectTypeID)
	if err != nil {
		return errors.New("failed to validate object type: " + err.Error())
	}
	if !exists {
		return errors.New("referenced object type does not exist")
	}
	operationKind, err := parseOperationKind(operationKindRaw)
	if err != nil {
		return err
	}
	if operationKind == "invoke_function" {
		if err := validateInvokeFunctionActionDefinition(r, state, config); err != nil {
			return err
		}
	}
	if inputSchema != nil {
		for _, field := range *inputSchema {
			if strings.TrimSpace(field.Name) == "" {
				return errors.New("action input field name is required")
			}
			if err := validateActionParameterType(field.PropertyType); err != nil {
				return errors.New(field.Name + ": " + err.Error())
			}
		}
	}
	_ = authorizationPolicy // shape-validated at JSON decode; plan-time checks enforce target policy.
	return nil
}

func validateInvokeFunctionActionDefinition(
	r *http.Request,
	state *ontologykernel.AppState,
	config json.RawMessage,
) error {
	opConfig := operationConfigBytes(config)
	opMap, err := decodeJSONObject(opConfig)
	if err != nil {
		return errors.New("invalid invoke_function config: " + err.Error())
	}
	hasFunctionRule := configHasAnyMeaningfulKey(opMap, "function_package_id", "function_package_name", "runtime")
	hasHTTPRule := configHasMeaningfulKey(opMap, "url")
	if !hasFunctionRule {
		if !hasHTTPRule {
			return errors.New("invoke_function action requires a function_package_id, function_package_name, inline runtime/source, or HTTP invocation config")
		}
		_, err := validateHTTPInvocationConfig(opConfig)
		return err
	}
	for _, key := range functionRuleOntologyConflictKeys() {
		if configHasMeaningfulKey(opMap, key) {
			return errors.New("function rule cannot be combined with other Ontology rules")
		}
	}
	resolved, err := domain.ResolveInlineFunctionConfig(r.Context(), state, opConfig)
	if err != nil {
		return err
	}
	if resolved == nil {
		return errors.New("function rule could not be resolved from invoke_function config")
	}
	return nil
}

func decodeJSONObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]json.RawMessage{}, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return map[string]json.RawMessage{}, nil
	}
	return obj, nil
}

func configHasAnyMeaningfulKey(obj map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		if configHasMeaningfulKey(obj, key) {
			return true
		}
	}
	return false
}

func configHasMeaningfulKey(obj map[string]json.RawMessage, key string) bool {
	raw, ok := obj[key]
	if !ok {
		return false
	}
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != `""`
}

func functionRuleOntologyConflictKeys() []string {
	return []string{
		"property_mappings",
		"static_patch",
		"object_id_input_name",
		"object_id_value",
		"generate_object_id",
		"link_type_id",
		"source_input_name",
		"target_input_name",
		"source_role",
		"properties_input_name",
		"concrete_object_type_input",
		"interface_id",
		"object_patch",
		"delete_object",
		"rules",
		"ontology_rules",
		"submission_criteria",
		"criteria",
	}
}

// parseOperationKind mirrors `fn parse_operation_kind`. Mirrors the
// `ActionOperationKind` enum exactly — every variant from the Rust
// source is accepted here so downstream switches can route concrete
// operations and preserve Rust's interface-resolution gate.
func parseOperationKind(raw string) (string, error) {
	switch raw {
	case "modify_object":
		return "update_object", nil
	case "create_object", "update_object", "create_or_modify_object",
		"create_link", "delete_link", "delete_object",
		"invoke_function", "invoke_webhook",
		"create_interface", "modify_interface", "delete_interface",
		"create_interface_link", "delete_interface_link":
		return raw, nil
	default:
		return "", errors.New("invalid action operation kind '" + raw + "'")
	}
}

func validateActionParameterType(propertyType string) error {
	switch strings.TrimSpace(strings.ToLower(propertyType)) {
	case "object_reference", "object_reference_list", "object_set":
		return nil
	default:
		return domain.ValidatePropertyType(propertyType)
	}
}
