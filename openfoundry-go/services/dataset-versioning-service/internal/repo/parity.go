package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
)

var (
	ErrNotFound           = errors.New("not found")
	ErrPreconditionFailed = errors.New("precondition failed")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrValidation         = errors.New("validation failed")
)

func nullableRaw(v models.JSONValue) any {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}

func defaultRawObject(v models.JSONValue) []byte {
	if len(v) == 0 {
		return []byte(`{}`)
	}
	return []byte(v)
}

// Dataset model / metadata / markings / permissions / lineage / file index.
func (r *Repo) GetCatalogDataset(ctx context.Context, datasetID uuid.UUID) (*models.CatalogDataset, error) {
	row := r.Pool.QueryRow(ctx, `SELECT id, name, description, format, storage_path, size_bytes, row_count,
		owner_id, tags, current_version, active_branch, metadata, health_status, current_view_id,
		created_at, updated_at FROM datasets WHERE id = $1`, datasetID)
	v, err := scanCatalogDataset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) PatchDatasetMetadata(ctx context.Context, datasetID uuid.UUID, body *models.DatasetMetadataPatch) (*models.CatalogDataset, error) {
	row := r.Pool.QueryRow(ctx, `UPDATE datasets SET
		name = COALESCE($2, name),
		description = COALESCE($3, description),
		owner_id = COALESCE($4, owner_id),
		tags = COALESCE($5, tags),
		format = COALESCE($6, format),
		metadata = COALESCE($7::jsonb, metadata),
		health_status = COALESCE($8, health_status),
		current_view_id = COALESCE($9, current_view_id),
		updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, description, format, storage_path, size_bytes, row_count,
		owner_id, tags, current_version, active_branch, metadata, health_status, current_view_id,
		created_at, updated_at`, datasetID, body.Name, body.Description, body.OwnerID, body.Tags, body.Format,
		nullableRaw(body.Metadata), body.HealthStatus, body.CurrentViewID)
	v, err := scanCatalogDataset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(body.Schema) > 0 {
		if _, err := r.Pool.Exec(ctx, `INSERT INTO dataset_schemas (id, dataset_id, fields)
			VALUES ($1, $2, $3::jsonb)
			ON CONFLICT (dataset_id) DO UPDATE SET fields = EXCLUDED.fields, created_at = NOW()`, uuid.New(), datasetID, []byte(body.Schema)); err != nil {
			return nil, err
		}
	}
	return v, nil
}

func (r *Repo) ListDatasetMarkings(ctx context.Context, datasetID uuid.UUID) ([]models.EffectiveMarking, error) {
	datasetRID := datasetID.String()
	rows, err := r.Pool.Query(ctx, `SELECT marking_id, source, inherited_from FROM dataset_markings
		WHERE dataset_rid = $1 ORDER BY source, marking_id, inherited_from`, datasetRID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.EffectiveMarking{}
	for rows.Next() {
		var id uuid.UUID
		var source string
		var inheritedFrom *string
		if err := rows.Scan(&id, &source, &inheritedFrom); err != nil {
			return nil, err
		}
		markingSource := models.MarkingReason{Kind: source}
		if source == "inherited_from_upstream" {
			markingSource.UpstreamRID = inheritedFrom
		}
		out = append(out, models.EffectiveMarking{ID: id, Source: markingSource})
	}
	return out, rows.Err()
}

func (r *Repo) ReplaceDatasetMarkings(ctx context.Context, datasetID uuid.UUID, markings []uuid.UUID, appliedBy uuid.UUID) error {
	datasetRID := datasetID.String()
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM dataset_markings WHERE dataset_rid = $1 AND source = 'direct'`, datasetRID); err != nil {
		return err
	}
	for _, markingID := range markings {
		if _, err := tx.Exec(ctx, `INSERT INTO dataset_markings (dataset_rid, marking_id, source, applied_by)
			VALUES ($1, $2, 'direct', $3) ON CONFLICT DO NOTHING`, datasetRID, markingID, appliedBy); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repo) ListDatasetPermissions(ctx context.Context, datasetID uuid.UUID) ([]models.DatasetPermissionEdge, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, dataset_id, principal_kind, principal_id, role, actions,
		source, inherited_from, created_at, updated_at FROM dataset_permission_edges WHERE dataset_id = $1
		ORDER BY source, principal_kind, principal_id, role`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DatasetPermissionEdge{}
	for rows.Next() {
		v, err := scanPermission(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) ReplaceDatasetPermissions(ctx context.Context, datasetID uuid.UUID, permissions []models.PutDatasetPermissionEdge) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM dataset_permission_edges WHERE dataset_id = $1`, datasetID); err != nil {
		return err
	}
	for _, p := range permissions {
		source := "direct"
		if p.Source != nil && *p.Source != "" {
			source = *p.Source
		}
		_, err := tx.Exec(ctx, `INSERT INTO dataset_permission_edges
			(id, dataset_id, principal_kind, principal_id, role, actions, source, inherited_from)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			uuid.New(), datasetID, p.PrincipalKind, p.PrincipalID, p.Role, p.Actions, source, p.InheritedFrom)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repo) ListDatasetLineageLinks(ctx context.Context, datasetID uuid.UUID) ([]models.DatasetLineageLink, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, dataset_id, direction, target_rid, target_kind, relation_kind,
		pipeline_id, workflow_id, metadata, created_at, updated_at FROM dataset_lineage_links WHERE dataset_id = $1
		ORDER BY direction, target_rid`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DatasetLineageLink{}
	for rows.Next() {
		v, err := scanLineageLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) ReplaceDatasetLineageLinks(ctx context.Context, datasetID uuid.UUID, links []models.PutDatasetLineageLink) error {
	if _, err := r.Pool.Exec(ctx, `DELETE FROM dataset_lineage_links WHERE dataset_id = $1`, datasetID); err != nil {
		return err
	}
	for _, l := range links {
		targetKind := "dataset"
		if l.TargetKind != nil && *l.TargetKind != "" {
			targetKind = *l.TargetKind
		}
		relationKind := "derives_from"
		if l.RelationKind != nil && *l.RelationKind != "" {
			relationKind = *l.RelationKind
		}
		_, err := r.Pool.Exec(ctx, `INSERT INTO dataset_lineage_links
			(id, dataset_id, direction, target_rid, target_kind, relation_kind, pipeline_id, workflow_id, metadata)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
			ON CONFLICT (dataset_id, direction, target_rid, relation_kind) DO UPDATE
			SET target_kind = EXCLUDED.target_kind, pipeline_id = EXCLUDED.pipeline_id,
			workflow_id = EXCLUDED.workflow_id, metadata = EXCLUDED.metadata, updated_at = NOW()`,
			uuid.New(), datasetID, l.Direction, l.TargetRID, targetKind, relationKind, l.PipelineID, l.WorkflowID, defaultRawObject(l.Metadata))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) ListDatasetFileIndex(ctx context.Context, datasetID uuid.UUID) ([]models.DatasetFileIndexEntry, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, dataset_id, path, storage_path, entry_type, size_bytes,
		content_type, metadata, last_modified, created_at, updated_at FROM dataset_file_index WHERE dataset_id = $1 ORDER BY path`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DatasetFileIndexEntry{}
	for rows.Next() {
		v, err := scanFileIndexEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) ReplaceDatasetFileIndex(ctx context.Context, datasetID uuid.UUID, files []models.PutDatasetFileIndexEntry) error {
	if _, err := r.Pool.Exec(ctx, `DELETE FROM dataset_file_index WHERE dataset_id = $1`, datasetID); err != nil {
		return err
	}
	for _, f := range files {
		entryType := "file"
		if f.EntryType != nil && *f.EntryType != "" {
			entryType = *f.EntryType
		}
		size := int64(0)
		if f.SizeBytes != nil {
			size = *f.SizeBytes
		}
		_, err := r.Pool.Exec(ctx, `INSERT INTO dataset_file_index
			(id, dataset_id, path, storage_path, entry_type, size_bytes, content_type, metadata, last_modified)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9)
			ON CONFLICT (dataset_id, path) DO UPDATE SET storage_path = EXCLUDED.storage_path,
			entry_type = EXCLUDED.entry_type, size_bytes = EXCLUDED.size_bytes, content_type = EXCLUDED.content_type,
			metadata = EXCLUDED.metadata, last_modified = EXCLUDED.last_modified, updated_at = NOW()`,
			uuid.New(), datasetID, f.Path, f.StoragePath, entryType, size, f.ContentType, defaultRawObject(f.Metadata), f.LastModified)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) ListActiveRuntimeBranches(ctx context.Context, datasetID uuid.UUID) ([]models.RuntimeBranch, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, rid, dataset_id, dataset_rid, name, parent_branch_id, head_transaction_id,
		created_from_transaction_id, last_activity_at, labels, fallback_chain, created_at, updated_at
		FROM dataset_branches WHERE dataset_id = $1 AND deleted_at IS NULL AND archived_at IS NULL
		ORDER BY parent_branch_id NULLS FIRST, name ASC`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RuntimeBranch{}
	for rows.Next() {
		v, err := scanRuntimeBranch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func parseTransactionRID(input string) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(input)
	trimmed = strings.TrimPrefix(trimmed, "ri.foundry.main.transaction.")
	return uuid.Parse(trimmed)
}

func (r *Repo) CreateRuntimeBranch(ctx context.Context, datasetID uuid.UUID, body *models.CreateBranchBody, actor uuid.UUID) (*models.RuntimeBranch, error) {
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: branch name is required", ErrValidation)
	}
	var datasetRID string
	if err := r.Pool.QueryRow(ctx, `SELECT rid FROM datasets WHERE id = $1`, datasetID).Scan(&datasetRID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var parentID *uuid.UUID
	var headID *uuid.UUID
	var createdFrom *uuid.UUID
	fallback := body.FallbackChain
	if body.Source != nil && body.Source.AsRoot != nil && *body.Source.AsRoot {
		var exists bool
		if err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset_branches WHERE dataset_id = $1 AND deleted_at IS NULL)`, datasetID).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("%w: root branch already exists", ErrConflict)
		}
	} else if body.Source != nil && body.Source.FromTransactionRID != nil && strings.TrimSpace(*body.Source.FromTransactionRID) != "" {
		txnID, err := parseTransactionRID(*body.Source.FromTransactionRID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid transaction rid", ErrValidation)
		}
		var status, branchName string
		var branchID uuid.UUID
		if err := r.Pool.QueryRow(ctx, `SELECT id, branch_id, branch_name, status FROM dataset_transactions WHERE dataset_id = $1 AND id = $2`, datasetID, txnID).Scan(&createdFrom, &branchID, &branchName, &status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if status != string(models.TransactionStatusCommitted) {
			return nil, fmt.Errorf("%w: source transaction must be COMMITTED", ErrConflict)
		}
		parentID = &branchID
		headID = &txnID
		if len(fallback) == 0 {
			fallback = []string{branchName}
		}
	} else {
		parentName := ""
		if body.Source != nil && body.Source.FromBranch != nil {
			parentName = strings.TrimSpace(*body.Source.FromBranch)
		}
		if parentName == "" && body.ParentBranch != nil {
			parentName = strings.TrimSpace(*body.ParentBranch)
		}
		if parentName != "" {
			parent, err := r.GetRuntimeBranch(ctx, datasetID, parentName)
			if err != nil {
				return nil, err
			}
			parentID = &parent.ID
			headID = parent.HeadTransactionID
			if len(fallback) == 0 {
				fallback = []string{parent.Name}
			}
		} else {
			var exists bool
			if err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset_branches WHERE dataset_id = $1 AND deleted_at IS NULL)`, datasetID).Scan(&exists); err != nil {
				return nil, err
			}
			if exists {
				return nil, fmt.Errorf("%w: parent_branch or source is required", ErrValidation)
			}
		}
	}
	labels := defaultRawObject(body.Labels)
	desc := ""
	if body.Description != nil {
		desc = *body.Description
	}
	row := r.Pool.QueryRow(ctx, `INSERT INTO dataset_branches
		(id, dataset_id, dataset_rid, name, parent_branch_id, head_transaction_id, created_from_transaction_id,
		description, created_by, labels, fallback_chain)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)
		RETURNING id, rid, dataset_id, dataset_rid, name, parent_branch_id, head_transaction_id,
		created_from_transaction_id, last_activity_at, labels, fallback_chain, created_at, updated_at`,
		uuid.New(), datasetID, datasetRID, name, parentID, headID, createdFrom, desc, actor, labels, fallback)
	v, err := scanRuntimeBranch(row)
	if IsConflict(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	if parentID != nil {
		_, _ = r.Pool.Exec(ctx, `INSERT INTO branch_markings_snapshot (branch_id, marking_id, source, set_by)
			SELECT $1, marking_id, 'PARENT', $2 FROM branch_markings_snapshot WHERE branch_id = $3
			ON CONFLICT (branch_id, marking_id) DO NOTHING`, v.ID, actor, *parentID)
	}
	return v, nil
}

// Branches, transactions and fallbacks.
func (r *Repo) DeleteBranchReparentChildren(ctx context.Context, datasetID uuid.UUID, branch string, reparentTo *uuid.UUID) error {
	row, err := r.GetBranch(ctx, datasetID, branch)
	if err != nil {
		return err
	}
	if row == nil {
		return ErrNotFound
	}
	if row.IsDefault || row.ParentBranchID == nil {
		return fmt.Errorf("%w: cannot delete root/default branch", ErrValidation)
	}
	if _, err := r.Pool.Exec(ctx, `UPDATE dataset_branches SET parent_branch_id = $3, updated_at = NOW()
		WHERE dataset_id = $1 AND parent_branch_id = $2 AND deleted_at IS NULL`, datasetID, row.ID, reparentTo); err != nil {
		return err
	}
	cmd, err := r.Pool.Exec(ctx, `UPDATE dataset_branches SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, row.ID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) parentNameAndRID(ctx context.Context, parentID *uuid.UUID) (*string, *string, error) {
	if parentID == nil {
		return nil, nil, nil
	}
	var name string
	if err := r.Pool.QueryRow(ctx, `SELECT name FROM dataset_branches WHERE id = $1 AND deleted_at IS NULL`, *parentID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	rid := "ri.foundry.main.branch." + parentID.String()
	return &name, &rid, nil
}

func (r *Repo) directChildren(ctx context.Context, branchID uuid.UUID) ([]struct {
	ID   uuid.UUID
	Name string
}, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, name FROM dataset_branches WHERE parent_branch_id = $1 AND deleted_at IS NULL ORDER BY name`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []struct {
		ID   uuid.UUID
		Name string
	}{}
	for rows.Next() {
		var v struct {
			ID   uuid.UUID
			Name string
		}
		if err := rows.Scan(&v.ID, &v.Name); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) PreviewDeleteBranch(ctx context.Context, datasetID uuid.UUID, branch string) (*models.BranchDeletePreview, error) {
	target, err := r.GetRuntimeBranch(ctx, datasetID, branch)
	if err != nil {
		return nil, err
	}
	children, err := r.directChildren(ctx, target.ID)
	if err != nil {
		return nil, err
	}
	parentName, parentRID, err := r.parentNameAndRID(ctx, target.ParentBranchID)
	if err != nil {
		return nil, err
	}
	items := []models.BranchDeleteChildReparent{}
	for _, child := range children {
		items = append(items, models.BranchDeleteChildReparent{Branch: child.Name, BranchRID: "ri.foundry.main.branch." + child.ID.String(), NewParent: parentName, NewParentRID: parentRID})
	}
	var head any
	if target.HeadTransactionID != nil {
		head = map[string]any{"id": *target.HeadTransactionID, "rid": "ri.foundry.main.transaction." + target.HeadTransactionID.String()}
	}
	return &models.BranchDeletePreview{Branch: target.Name, BranchRID: target.RID, CurrentParent: parentName, CurrentParentRID: parentRID, ChildrenToReparent: items, TransactionsPreserved: true, HeadTransaction: head}, nil
}

func (r *Repo) DeleteRuntimeBranch(ctx context.Context, datasetID uuid.UUID, branch string) (*models.BranchDeleteResponse, error) {
	target, err := r.GetRuntimeBranch(ctx, datasetID, branch)
	if err != nil {
		return nil, err
	}
	children, err := r.directChildren(ctx, target.ID)
	if err != nil {
		return nil, err
	}
	parentName, parentRID, err := r.parentNameAndRID(ctx, target.ParentBranchID)
	if err != nil {
		return nil, err
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE dataset_branches SET parent_branch_id = $2, updated_at = NOW() WHERE parent_branch_id = $1 AND deleted_at IS NULL`, target.ID, target.ParentBranchID); err != nil {
		return nil, err
	}
	cmd, err := tx.Exec(ctx, `UPDATE dataset_branches SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, target.ID)
	if err != nil {
		return nil, err
	}
	if cmd.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	_, _ = tx.Exec(ctx, `INSERT INTO outbox.events (event_id, aggregate, aggregate_id, payload, topic, created_at) VALUES ($1,'dataset_branch',$2,$3::jsonb,'branch.deleted',NOW()) ON CONFLICT (event_id) DO NOTHING`, uuid.New(), target.ID.String(), []byte(`{}`))
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	items := []models.BranchDeleteChildReparent{}
	for _, child := range children {
		items = append(items, models.BranchDeleteChildReparent{ChildBranch: child.Name, ChildBranchRID: "ri.foundry.main.branch." + child.ID.String(), NewParent: parentName, NewParentRID: parentRID})
	}
	return &models.BranchDeleteResponse{Branch: target.Name, BranchRID: target.RID, Reparented: items}, nil
}

func (r *Repo) ReparentRuntimeBranch(ctx context.Context, datasetID uuid.UUID, branch string, newParent *string) (*models.RuntimeBranch, error) {
	target, err := r.GetRuntimeBranch(ctx, datasetID, branch)
	if err != nil {
		return nil, err
	}
	var newParentID *uuid.UUID
	if newParent != nil && strings.TrimSpace(*newParent) != "" {
		parent, err := r.GetRuntimeBranch(ctx, datasetID, strings.TrimSpace(*newParent))
		if err != nil {
			return nil, err
		}
		if parent.ID == target.ID {
			return nil, fmt.Errorf("%w: a branch cannot be its own parent", ErrValidation)
		}
		newParentID = &parent.ID
	}
	row := r.Pool.QueryRow(ctx, `UPDATE dataset_branches SET parent_branch_id = $2, updated_at = NOW() WHERE id = $1
		RETURNING id, rid, dataset_id, dataset_rid, name, parent_branch_id, head_transaction_id, created_from_transaction_id, last_activity_at, labels, fallback_chain, created_at, updated_at`, target.ID, newParentID)
	return scanRuntimeBranch(row)
}

func (r *Repo) RollbackBranch(ctx context.Context, datasetID uuid.UUID, branch string, body *models.RollbackBody, actor uuid.UUID) (map[string]any, error) {
	target, err := r.GetRuntimeBranch(ctx, datasetID, branch)
	if err != nil {
		return nil, err
	}
	var status string
	if err := r.Pool.QueryRow(ctx, `SELECT status FROM dataset_transactions WHERE dataset_id = $1 AND branch_id = $2 AND id = $3`, datasetID, target.ID, body.TransactionID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if status != string(models.TransactionStatusCommitted) {
		return nil, fmt.Errorf("%w: rollback target must be COMMITTED", ErrValidation)
	}
	summary := "rollback"
	if body.Summary != nil {
		summary = *body.Summary
	}
	txn, err := r.StartTransaction(ctx, datasetID, target.ID, target.Name, models.TransactionTypeSnapshot, summary, models.JSONValue(`{"rollback":true}`), actor)
	if err != nil {
		return nil, err
	}
	if err := r.CommitTransaction(ctx, datasetID, txn.ID); err != nil {
		return nil, err
	}
	committed, _ := r.GetRuntimeTransaction(ctx, datasetID, txn.ID)
	return map[string]any{"transaction": committed, "view": map[string]any{"branch": branch}}, nil
}
func (r *Repo) BranchAncestry(ctx context.Context, datasetID uuid.UUID, branch string) ([]models.RuntimeBranch, error) {
	rows, err := r.Pool.Query(ctx, `WITH RECURSIVE ancestry AS (
		SELECT id, rid, dataset_id, dataset_rid, name, parent_branch_id, head_transaction_id,
		created_from_transaction_id, last_activity_at, labels, fallback_chain, created_at, updated_at, 0 AS depth
		FROM dataset_branches WHERE dataset_id = $1 AND name = $2 AND deleted_at IS NULL
		UNION ALL
		SELECT p.id, p.rid, p.dataset_id, p.dataset_rid, p.name, p.parent_branch_id, p.head_transaction_id,
		p.created_from_transaction_id, p.last_activity_at, p.labels, p.fallback_chain, p.created_at, p.updated_at, a.depth + 1
		FROM dataset_branches p JOIN ancestry a ON a.parent_branch_id = p.id WHERE p.deleted_at IS NULL)
		SELECT id, rid, dataset_id, dataset_rid, name, parent_branch_id, head_transaction_id,
		created_from_transaction_id, last_activity_at, labels, fallback_chain, created_at, updated_at FROM ancestry ORDER BY depth`, datasetID, branch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RuntimeBranch{}
	for rows.Next() {
		v, err := scanRuntimeBranch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) UpdateBranchRetention(ctx context.Context, datasetID uuid.UUID, branch string, policy models.RetentionPolicy, ttlDays *int32) (*models.RuntimeBranch, error) {
	row := r.Pool.QueryRow(ctx, `UPDATE dataset_branches SET retention_policy = $3, retention_ttl_days = $4, updated_at = NOW()
		WHERE dataset_id = $1 AND name = $2 AND deleted_at IS NULL
		RETURNING id, rid, dataset_id, dataset_rid, name, parent_branch_id, head_transaction_id,
		created_from_transaction_id, last_activity_at, labels, fallback_chain, created_at, updated_at`, datasetID, branch, policy, ttlDays)
	v, err := scanRuntimeBranch(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

func (r *Repo) RestoreBranch(ctx context.Context, datasetID uuid.UUID, branch string) (*models.RuntimeBranch, error) {
	row := r.Pool.QueryRow(ctx, `UPDATE dataset_branches SET deleted_at = NULL, archived_at = NULL,
		archive_grace_until = NULL, updated_at = NOW() WHERE dataset_id = $1 AND name = $2
		RETURNING id, rid, dataset_id, dataset_rid, name, parent_branch_id, head_transaction_id,
		created_from_transaction_id, last_activity_at, labels, fallback_chain, created_at, updated_at`, datasetID, branch)
	v, err := scanRuntimeBranch(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

func (r *Repo) ListBranchMarkings(ctx context.Context, branchID uuid.UUID) ([]models.BranchMarking, error) {
	rows, err := r.Pool.Query(ctx, `SELECT branch_id, marking_id, source FROM branch_markings_snapshot WHERE branch_id = $1 ORDER BY marking_id`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.BranchMarking{}
	for rows.Next() {
		var v models.BranchMarking
		if err := rows.Scan(&v.BranchID, &v.MarkingID, &v.Source); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) ListFallbacks(ctx context.Context, branchID uuid.UUID) ([]models.RuntimeFallbackEntry, error) {
	rows, err := r.Pool.Query(ctx, `SELECT position, fallback_branch_name FROM dataset_branch_fallbacks WHERE branch_id = $1 ORDER BY position`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RuntimeFallbackEntry{}
	for rows.Next() {
		var v models.RuntimeFallbackEntry
		if err := rows.Scan(&v.Position, &v.FallbackBranchName); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) ReplaceFallbacks(ctx context.Context, branchID uuid.UUID, fallbackNames []string) error {
	if _, err := r.Pool.Exec(ctx, `DELETE FROM dataset_branch_fallbacks WHERE branch_id = $1`, branchID); err != nil {
		return err
	}
	for i, name := range fallbackNames {
		if _, err := r.Pool.Exec(ctx, `INSERT INTO dataset_branch_fallbacks (branch_id, position, fallback_branch_name)
			VALUES ($1, $2, $3) ON CONFLICT (branch_id, position) DO UPDATE SET fallback_branch_name = EXCLUDED.fallback_branch_name`, branchID, int32(i), name); err != nil {
			return err
		}
	}
	_, err := r.Pool.Exec(ctx, `UPDATE dataset_branches SET fallback_chain = $2, updated_at = NOW() WHERE id = $1`, branchID, fallbackNames)
	return err
}

func (r *Repo) StartTransaction(ctx context.Context, datasetID uuid.UUID, branchID uuid.UUID, branchName string, txType models.TransactionType, summary string, providence models.JSONValue, startedBy uuid.UUID) (*models.RuntimeTransaction, error) {
	var openExists bool
	if err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset_transactions WHERE branch_id = $1 AND status = 'OPEN')`, branchID).Scan(&openExists); err != nil {
		return nil, err
	}
	if openExists {
		return nil, ErrConflict
	}
	row := r.Pool.QueryRow(ctx, `INSERT INTO dataset_transactions
		(id, dataset_id, branch_id, branch_name, tx_type, status, summary, metadata, providence, started_by)
		VALUES ($1,$2,$3,$4,$5,'OPEN',$6,'{}'::jsonb,$7::jsonb,$8)
		RETURNING id, dataset_id, branch_id, branch_name, tx_type, status, summary, metadata, providence, started_by, started_at, committed_at, aborted_at`,
		uuid.New(), datasetID, branchID, branchName, txType, summary, defaultRawObject(providence), startedBy)
	return scanRuntimeTransaction(row)
}

func (r *Repo) GetRuntimeTransaction(ctx context.Context, datasetID uuid.UUID, txnID uuid.UUID) (*models.RuntimeTransaction, error) {
	row := r.Pool.QueryRow(ctx, runtimeTransactionSelect()+` WHERE dataset_id = $1 AND id = $2`, datasetID, txnID)
	v, err := scanRuntimeTransaction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) ListRuntimeTransactions(ctx context.Context, datasetID uuid.UUID, branch *string, before *time.Time, limit int) ([]models.RuntimeTransaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.Pool.Query(ctx, runtimeTransactionSelect()+` WHERE dataset_id = $1
		AND ($2::text IS NULL OR branch_name = $2)
		AND ($3::timestamptz IS NULL OR started_at < $3)
		ORDER BY started_at DESC LIMIT $4`, datasetID, branch, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RuntimeTransaction{}
	for rows.Next() {
		v, err := scanRuntimeTransaction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) CommitTransaction(ctx context.Context, datasetID uuid.UUID, txnID uuid.UUID) error {
	cmd, err := r.Pool.Exec(ctx, `UPDATE dataset_transactions SET status = 'COMMITTED', committed_at = NOW()
		WHERE dataset_id = $1 AND id = $2 AND status = 'OPEN'`, datasetID, txnID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvalidTransition
	}
	_, err = r.Pool.Exec(ctx, `UPDATE dataset_branches SET head_transaction_id = $2, last_activity_at = NOW(), updated_at = NOW()
		WHERE dataset_id = $1 AND id = (SELECT branch_id FROM dataset_transactions WHERE id = $2)`, datasetID, txnID)
	return err
}

func (r *Repo) AbortTransaction(ctx context.Context, datasetID uuid.UUID, txnID uuid.UUID) error {
	cmd, err := r.Pool.Exec(ctx, `UPDATE dataset_transactions SET status = 'ABORTED', aborted_at = NOW()
		WHERE dataset_id = $1 AND id = $2 AND status = 'OPEN'`, datasetID, txnID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvalidTransition
	}
	return nil
}

// Views / schemas / files.
func (r *Repo) ListViews(ctx context.Context, datasetID uuid.UUID) ([]models.DatasetView, error) {
	rows, err := r.Pool.Query(ctx, datasetViewSelect()+` WHERE dataset_id = $1 ORDER BY created_at DESC`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DatasetView{}
	for rows.Next() {
		v, err := scanDatasetView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) CreateView(ctx context.Context, datasetID uuid.UUID, body *models.CreateDatasetViewRequest) (*models.DatasetView, error) {
	row := r.Pool.QueryRow(ctx, `INSERT INTO dataset_views
		(id, dataset_id, name, description, sql_text, source_branch, source_version, materialized, refresh_on_source_update)
		VALUES ($1,$2,$3,COALESCE($4,''),$5,$6,$7,COALESCE($8,false),COALESCE($9,false))
		ON CONFLICT (dataset_id, name) DO UPDATE SET description = EXCLUDED.description,
		sql_text = EXCLUDED.sql_text, source_branch = EXCLUDED.source_branch, source_version = EXCLUDED.source_version,
		materialized = EXCLUDED.materialized, refresh_on_source_update = EXCLUDED.refresh_on_source_update, updated_at = NOW()
		RETURNING id, dataset_id, name, description, sql_text, source_branch, source_version, materialized,
		refresh_on_source_update, format, current_version, storage_path, row_count, schema_fields,
		last_refreshed_at, created_at, updated_at`, uuid.New(), datasetID, body.Name, body.Description, body.SQL,
		body.SourceBranch, body.SourceVersion, body.Materialized, body.RefreshOnSourceUpdate)
	v, err := scanDatasetView(row)
	if IsConflict(err) {
		return nil, ErrConflict
	}
	return v, err
}

func (r *Repo) GetCurrentView(ctx context.Context, datasetID uuid.UUID, branch string) (*models.ViewOut, error) {
	row := r.Pool.QueryRow(ctx, `SELECT v.id, v.dataset_id, b.id, b.head_transaction_id,
		$2::text AS requested_branch, b.name AS resolved_branch, b.fallback_chain, NOW() AS computed_at,
		COUNT(df.id)::int AS file_count, COALESCE(SUM(df.size_bytes),0)::bigint AS size_bytes
		FROM dataset_branches b JOIN dataset_views v ON v.dataset_id = b.dataset_id
		LEFT JOIN dataset_files df ON df.dataset_id = b.dataset_id AND df.deleted_at IS NULL
		WHERE b.dataset_id = $1 AND b.name = $2 AND b.deleted_at IS NULL
		GROUP BY v.id, v.dataset_id, b.id, b.head_transaction_id, b.name, b.fallback_chain
		ORDER BY v.created_at DESC LIMIT 1`, datasetID, branch)
	v, err := scanViewOutHeader(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	files, err := r.ListViewFiles(ctx, datasetID, v.ID)
	if err != nil {
		return nil, err
	}
	v.Files = files
	return v, nil
}

func (r *Repo) ListViewFiles(ctx context.Context, datasetID uuid.UUID, viewID uuid.UUID) ([]models.RuntimeViewFile, error) {
	rows, err := r.Pool.Query(ctx, `SELECT logical_path, physical_uri AS physical_path, size_bytes, transaction_id AS introduced_by
		FROM dataset_files WHERE dataset_id = $1 AND deleted_at IS NULL ORDER BY logical_path`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RuntimeViewFile{}
	for rows.Next() {
		var v models.RuntimeViewFile
		if err := rows.Scan(&v.LogicalPath, &v.PhysicalPath, &v.SizeBytes, &v.IntroducedBy); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) GetDatasetView(ctx context.Context, datasetID uuid.UUID, viewOrName string) (*models.DatasetView, error) {
	var row pgx.Row
	if id, err := uuid.Parse(viewOrName); err == nil {
		row = r.Pool.QueryRow(ctx, datasetViewSelect()+` WHERE dataset_id = $1 AND id = $2`, datasetID, id)
	} else {
		row = r.Pool.QueryRow(ctx, datasetViewSelect()+` WHERE dataset_id = $1 AND name = $2`, datasetID, viewOrName)
	}
	v, err := scanDatasetView(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

func (r *Repo) RefreshDatasetView(ctx context.Context, datasetID uuid.UUID, viewOrName string) (*models.DatasetView, error) {
	view, err := r.GetDatasetView(ctx, datasetID, viewOrName)
	if err != nil {
		return nil, err
	}
	row := r.Pool.QueryRow(ctx, `UPDATE dataset_views SET last_refreshed_at = NOW(), updated_at = NOW()
		WHERE dataset_id = $1 AND id = $2
		RETURNING id, dataset_id, name, description, sql_text, source_branch, source_version, materialized,
		refresh_on_source_update, format, current_version, storage_path, row_count, schema_fields,
		last_refreshed_at, created_at, updated_at`, datasetID, view.ID)
	return scanDatasetView(row)
}

func (r *Repo) GetViewAt(ctx context.Context, datasetID uuid.UUID, branch string, at *time.Time, transactionID *uuid.UUID) (*models.ViewOut, error) {
	// The Rust implementation resolves the branch-effective view at a timestamp or transaction.
	// The Go parity query uses the current projection until transaction file replay lands.
	return r.GetCurrentView(ctx, datasetID, branch)
}

func (r *Repo) GetViewSchema(ctx context.Context, viewID uuid.UUID) (*models.SchemaResponse, error) {
	row := r.Pool.QueryRow(ctx, `SELECT view_id, dataset_id, branch, schema_json, content_hash, created_at, false AS unchanged
		FROM dataset_view_schemas WHERE view_id = $1`, viewID)
	v, err := scanSchemaResponse(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) PutViewSchema(ctx context.Context, viewID uuid.UUID, datasetID uuid.UUID, branch *string, schema models.DatasetSchema, contentHash string) (*models.SchemaResponse, error) {
	schemaJSON, err := models.MarshalJSONValue(schema)
	if err != nil {
		return nil, err
	}
	row := r.Pool.QueryRow(ctx, `INSERT INTO dataset_view_schemas (view_id, dataset_id, branch, schema_json, content_hash)
		VALUES ($1,$2,$3,$4::jsonb,$5)
		ON CONFLICT (view_id) DO UPDATE SET schema_json = EXCLUDED.schema_json,
		content_hash = EXCLUDED.content_hash, branch = EXCLUDED.branch, updated_at = NOW()
		RETURNING view_id, dataset_id, branch, schema_json, content_hash, created_at,
		(xmax = 0) AS unchanged`, viewID, datasetID, branch, schemaJSON, contentHash)
	return scanSchemaResponse(row)
}

func (r *Repo) GetCurrentSchema(ctx context.Context, datasetID uuid.UUID, branch string) (*models.SchemaResponse, error) {
	view, err := r.GetCurrentView(ctx, datasetID, branch)
	if err == nil && view != nil {
		return r.GetViewSchema(ctx, view.ID)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	legacy, err := r.GetLegacyDatasetSchema(ctx, datasetID)
	if err != nil || legacy == nil {
		return nil, err
	}
	var fields []models.Field
	_ = models.UnmarshalJSONValue(legacy.Fields, &fields)
	return &models.SchemaResponse{ViewID: uuid.Nil, DatasetID: datasetID, Branch: &branch, Schema: models.DatasetSchema{Fields: fields, FileFormat: models.FileFormatParquet}, ContentHash: "legacy", CreatedAt: legacy.CreatedAt}, nil
}

func (r *Repo) PreviewData(ctx context.Context, datasetID uuid.UUID, viewID *uuid.UUID, q models.PreviewQuery) (*models.PreviewDataResponse, error) {
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
	format := "json"
	if q.Format != nil && *q.Format != "" {
		format = *q.Format
	}
	columns := []string{}
	if viewID != nil {
		if schema, err := r.GetViewSchema(ctx, *viewID); err == nil && schema != nil {
			for _, f := range schema.Schema.Fields {
				columns = append(columns, f.Name)
			}
		}
	}
	if len(columns) == 0 {
		rows, err := r.Pool.Query(ctx, `SELECT logical_path FROM dataset_files WHERE dataset_id = $1 AND deleted_at IS NULL ORDER BY logical_path LIMIT $2 OFFSET $3`, datasetID, limit, offset)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		columns = []string{"logical_path"}
		outRows := [][]models.JSONValue{}
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				return nil, err
			}
			b, _ := models.MarshalJSONValue(path)
			outRows = append(outRows, []models.JSONValue{b})
		}
		return &models.PreviewDataResponse{Columns: columns, Rows: outRows, Format: format, Limit: limit, Offset: offset}, rows.Err()
	}
	return &models.PreviewDataResponse{Columns: columns, Rows: [][]models.JSONValue{}, Format: format, Limit: limit, Offset: offset}, nil
}

func (r *Repo) ValidateSchema(ctx context.Context, datasetID uuid.UUID, schema models.DatasetSchema) (*models.ValidateResponse, error) {
	errs := []string{}
	seen := map[string]bool{}
	for _, f := range schema.Fields {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			errs = append(errs, "field name is required")
			continue
		}
		if seen[name] {
			errs = append(errs, "duplicate field: "+name)
		}
		seen[name] = true
	}
	if schema.FileFormat == "" {
		errs = append(errs, "file_format is required")
	}
	return &models.ValidateResponse{Conforms: len(errs) == 0, Files: []models.FileValidationReport{}, SchemaErrors: errs}, nil
}

func (r *Repo) StorageDetails(ctx context.Context, datasetID uuid.UUID, fsID string, driver string, baseDir string, ttlSeconds uint64) (*models.StorageDetailsOut, error) {
	row := r.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(CASE WHEN deleted_at IS NULL THEN size_bytes ELSE 0 END),0)::bigint AS total_active_bytes,
		COUNT(*) FILTER (WHERE deleted_at IS NULL)::bigint AS total_active_files,
		COALESCE(SUM(CASE WHEN deleted_at IS NOT NULL THEN size_bytes ELSE 0 END),0)::bigint AS total_deleted_bytes,
		COUNT(*) FILTER (WHERE deleted_at IS NOT NULL)::bigint AS total_deleted_files
		FROM dataset_files WHERE dataset_id = $1`, datasetID)
	out := &models.StorageDetailsOut{FSID: fsID, Driver: driver, BaseDirectory: baseDir, PresignTTLSeconds: ttlSeconds}
	return out, row.Scan(&out.TotalActiveBytes, &out.TotalActiveFiles, &out.TotalDeletedBytes, &out.TotalDeletedFiles)
}

// Quality / lint / health / retention worker.
func (r *Repo) GetDatasetQuality(ctx context.Context, datasetID uuid.UUID) (*models.DatasetQualityResponse, error) {
	row := r.Pool.QueryRow(ctx, `SELECT profile, score, profiled_at FROM dataset_profiles WHERE dataset_id = $1`, datasetID)
	var profileRaw []byte
	var err error
	out := &models.DatasetQualityResponse{History: []models.DatasetQualityHistoryEntry{}, Alerts: []models.DatasetQualityAlert{}, Rules: []models.DatasetQualityRule{}}
	if err := row.Scan(&profileRaw, &out.Score, &out.ProfiledAt); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if len(profileRaw) > 0 {
		var profile models.DatasetQualityProfile
		if err := models.UnmarshalJSONValue(profileRaw, &profile); err != nil {
			return nil, err
		}
		out.Profile = &profile
	}
	if out.History, err = r.ListQualityHistory(ctx, datasetID, 20); err != nil {
		return nil, err
	}
	active := "active"
	if out.Alerts, err = r.ListQualityAlerts(ctx, datasetID, &active, 100); err != nil {
		return nil, err
	}
	if out.Rules, err = r.ListQualityRules(ctx, datasetID); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repo) ListQualityRules(ctx context.Context, datasetID uuid.UUID) ([]models.DatasetQualityRule, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, dataset_id, name, rule_type, severity, config, enabled, created_at, updated_at
		FROM dataset_quality_rules WHERE dataset_id = $1 ORDER BY name`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DatasetQualityRule{}
	for rows.Next() {
		v, err := scanQualityRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) UpsertQualityRule(ctx context.Context, datasetID uuid.UUID, body *models.CreateQualityRuleRequest) (*models.DatasetQualityRule, error) {
	severity := "medium"
	if body.Severity != nil && *body.Severity != "" {
		severity = *body.Severity
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	row := r.Pool.QueryRow(ctx, `INSERT INTO dataset_quality_rules (id, dataset_id, name, rule_type, severity, config, enabled)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7)
		RETURNING id, dataset_id, name, rule_type, severity, config, enabled, created_at, updated_at`,
		uuid.New(), datasetID, body.Name, body.RuleType, severity, defaultRawObject(body.Config), enabled)
	return scanQualityRule(row)
}

func (r *Repo) UpdateQualityRule(ctx context.Context, datasetID uuid.UUID, ruleID uuid.UUID, body *models.UpdateQualityRuleRequest) error {
	cmd, err := r.Pool.Exec(ctx, `UPDATE dataset_quality_rules
		SET name = COALESCE($3, name), severity = COALESCE($4, severity), enabled = COALESCE($5, enabled),
		config = COALESCE($6::jsonb, config), updated_at = NOW()
		WHERE dataset_id = $1 AND id = $2`, datasetID, ruleID, body.Name, body.Severity, body.Enabled, nullableRaw(body.Config))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) GetDatasetHealth(ctx context.Context, datasetRID string) (*models.DatasetHealth, error) {
	row := r.Pool.QueryRow(ctx, `SELECT dataset_rid, dataset_id, row_count, col_count, null_pct_by_column,
		freshness_seconds, last_commit_at, txn_failure_rate_24h, last_build_status, schema_drift_flag,
		extras, last_computed_at FROM dataset_health WHERE dataset_rid = $1`, datasetRID)
	v, err := scanDatasetHealth(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) ListRetentionCandidates(ctx context.Context, now time.Time, limit int) ([]models.RetentionRow, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.Pool.Query(ctx, `SELECT id, parent_branch_id, retention_policy, retention_ttl_days, last_activity_at,
		EXISTS(SELECT 1 FROM dataset_transactions t WHERE t.branch_id = dataset_branches.id AND t.status = 'OPEN') AS has_open_transaction,
		(parent_branch_id IS NULL) AS is_root, archived_at
		FROM dataset_branches WHERE deleted_at IS NULL AND archived_at IS NULL
		ORDER BY last_activity_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RetentionRow{}
	for rows.Next() {
		var v models.RetentionRow
		if err := rows.Scan(&v.ID, &v.ParentBranchID, &v.Policy, &v.TTLDays, &v.LastActivityAt, &v.HasOpenTransaction, &v.IsRoot, &v.ArchivedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) ArchiveBranchForRetention(ctx context.Context, branchID uuid.UUID, graceUntil time.Time) error {
	cmd, err := r.Pool.Exec(ctx, `UPDATE dataset_branches SET archived_at = NOW(), archive_grace_until = $2, updated_at = NOW()
		WHERE id = $1 AND archived_at IS NULL AND deleted_at IS NULL`, branchID, graceUntil)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrPreconditionFailed
	}
	return nil
}

func (r *Repo) ResolveDatasetID(ctx context.Context, raw string) (uuid.UUID, error) {
	if id, err := uuid.Parse(raw); err == nil {
		var exists bool
		if err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM datasets WHERE id = $1)`, id).Scan(&exists); err != nil {
			return uuid.Nil, err
		}
		if !exists {
			return uuid.Nil, ErrNotFound
		}
		return id, nil
	}
	return r.ResolveDatasetIDByRID(ctx, raw)
}

func (r *Repo) DatasetExists(ctx context.Context, datasetID uuid.UUID) (bool, error) {
	var exists bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM datasets WHERE id = $1)`, datasetID).Scan(&exists)
	return exists, err
}

func (r *Repo) DatasetViewBelongsToDataset(ctx context.Context, datasetID uuid.UUID, viewID uuid.UUID) (bool, error) {
	var exists bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset_views WHERE id = $1 AND dataset_id = $2)`, viewID, datasetID).Scan(&exists)
	return exists, err
}

func (r *Repo) GetLegacyDatasetSchema(ctx context.Context, datasetID uuid.UUID) (*models.LegacyDatasetSchema, error) {
	row := r.Pool.QueryRow(ctx, `SELECT id, dataset_id, fields, created_at FROM dataset_schemas WHERE dataset_id = $1`, datasetID)
	v := &models.LegacyDatasetSchema{}
	var fields []byte
	if err := row.Scan(&v.ID, &v.DatasetID, &fields, &v.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v.Fields = fields
	return v, nil
}

func (r *Repo) GetLatestDatasetView(ctx context.Context, datasetID uuid.UUID, currentViewID *uuid.UUID) (*models.DatasetView, error) {
	query := datasetViewSelect()
	var row pgx.Row
	if currentViewID != nil {
		row = r.Pool.QueryRow(ctx, query+` WHERE id = $1 AND dataset_id = $2`, *currentViewID, datasetID)
	} else {
		row = r.Pool.QueryRow(ctx, query+` WHERE dataset_id = $1 ORDER BY updated_at DESC LIMIT 1`, datasetID)
	}
	v, err := scanDatasetView(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) DatasetHealthSummary(ctx context.Context, datasetID uuid.UUID, healthStatus string) (models.DatasetHealthSummary, error) {
	out := models.DatasetHealthSummary{Status: healthStatus}
	var generatedAt *time.Time
	if err := r.Pool.QueryRow(ctx, `SELECT score, profiled_at FROM dataset_profiles WHERE dataset_id = $1`, datasetID).Scan(&out.QualityScore, &generatedAt); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if generatedAt != nil {
		v := generatedAt.Format(time.RFC3339)
		out.ProfileGeneratedAt = &v
	}
	_ = r.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM dataset_quality_alerts WHERE dataset_id = $1 AND status <> 'resolved'`, datasetID).Scan(&out.ActiveAlertCount)
	return out, nil
}

func (r *Repo) GetDatasetRichModel(ctx context.Context, datasetID uuid.UUID) (*models.DatasetRichModel, error) {
	catalog, err := r.GetCatalogDataset(ctx, datasetID)
	if err != nil || catalog == nil {
		return nil, err
	}
	dataset := models.Dataset{ID: catalog.ID, Name: catalog.Name, Description: catalog.Description, Format: catalog.Format, StoragePath: catalog.StoragePath, SizeBytes: catalog.SizeBytes, RowCount: catalog.RowCount, OwnerID: catalog.OwnerID, Tags: catalog.Tags, CurrentVersion: catalog.CurrentVersion, CreatedAt: catalog.CreatedAt, UpdatedAt: catalog.UpdatedAt}
	schema, err := r.GetLegacyDatasetSchema(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	files, err := r.ListDatasetFileIndex(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	branches, err := r.ListBranches(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	versions, err := r.ListVersions(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	view, err := r.GetLatestDatasetView(ctx, datasetID, catalog.CurrentViewID)
	if err != nil {
		return nil, err
	}
	health, err := r.DatasetHealthSummary(ctx, datasetID, catalog.HealthStatus)
	if err != nil {
		return nil, err
	}
	markings, err := r.ListDatasetMarkings(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	permissions, err := r.ListDatasetPermissions(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	links, err := r.ListDatasetLineageLinks(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	return &models.DatasetRichModel{Dataset: dataset, Schema: schema, Files: files, Branches: branches, Versions: versions, CurrentView: view, Health: health, Markings: markings, Permissions: permissions, LineageLinks: links}, nil
}

// Scan helpers.
func scanCatalogDataset(r rowLikeT) (*models.CatalogDataset, error) {
	v := &models.CatalogDataset{}
	var metadata []byte
	if err := r.Scan(&v.ID, &v.Name, &v.Description, &v.Format, &v.StoragePath, &v.SizeBytes, &v.RowCount,
		&v.OwnerID, &v.Tags, &v.CurrentVersion, &v.ActiveBranch, &metadata, &v.HealthStatus, &v.CurrentViewID, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Metadata = metadata
	return v, nil
}

func scanPermission(r rowLikeT) (*models.DatasetPermissionEdge, error) {
	v := &models.DatasetPermissionEdge{}
	return v, r.Scan(&v.ID, &v.DatasetID, &v.PrincipalKind, &v.PrincipalID, &v.Role, &v.Actions, &v.Source, &v.InheritedFrom, &v.CreatedAt, &v.UpdatedAt)
}

func scanLineageLink(r rowLikeT) (*models.DatasetLineageLink, error) {
	v := &models.DatasetLineageLink{}
	var metadata []byte
	if err := r.Scan(&v.ID, &v.DatasetID, &v.Direction, &v.TargetRID, &v.TargetKind, &v.RelationKind, &v.PipelineID, &v.WorkflowID, &metadata, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Metadata = metadata
	return v, nil
}

func scanFileIndexEntry(r rowLikeT) (*models.DatasetFileIndexEntry, error) {
	v := &models.DatasetFileIndexEntry{}
	var metadata []byte
	if err := r.Scan(&v.ID, &v.DatasetID, &v.Path, &v.StoragePath, &v.EntryType, &v.SizeBytes, &v.ContentType, &metadata, &v.LastModified, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Metadata = metadata
	return v, nil
}

func scanRuntimeBranch(r rowLikeT) (*models.RuntimeBranch, error) {
	v := &models.RuntimeBranch{}
	var labels []byte
	if err := r.Scan(&v.ID, &v.RID, &v.DatasetID, &v.DatasetRID, &v.Name, &v.ParentBranchID, &v.HeadTransactionID, &v.CreatedFromTransactionID, &v.LastActivityAt, &labels, &v.FallbackChain, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	if len(labels) == 0 {
		labels = []byte(`{}`)
	}
	v.Labels = labels
	if v.FallbackChain == nil {
		v.FallbackChain = []string{}
	}
	return v, nil
}

func runtimeTransactionSelect() string {
	return `SELECT id, dataset_id, branch_id, branch_name, tx_type, status, summary, metadata, providence, started_by, started_at, committed_at, aborted_at FROM dataset_transactions`
}
func scanRuntimeTransaction(r rowLikeT) (*models.RuntimeTransaction, error) {
	v := &models.RuntimeTransaction{}
	var metadata, providence []byte
	if err := r.Scan(&v.ID, &v.DatasetID, &v.BranchID, &v.BranchName, &v.TxType, &v.Status, &v.Summary, &metadata, &providence, &v.StartedBy, &v.StartedAt, &v.CommittedAt, &v.AbortedAt); err != nil {
		return nil, err
	}
	v.Metadata = metadata
	v.Providence = providence
	return v, nil
}

func datasetViewSelect() string {
	return `SELECT id, dataset_id, name, description, sql_text, source_branch, source_version, materialized, refresh_on_source_update, format, current_version, storage_path, row_count, schema_fields, last_refreshed_at, created_at, updated_at FROM dataset_views`
}
func scanDatasetView(r rowLikeT) (*models.DatasetView, error) {
	v := &models.DatasetView{}
	var schema []byte
	if err := r.Scan(&v.ID, &v.DatasetID, &v.Name, &v.Description, &v.SQLText, &v.SourceBranch, &v.SourceVersion, &v.Materialized, &v.RefreshOnSourceUpdate, &v.Format, &v.CurrentVersion, &v.StoragePath, &v.RowCount, &schema, &v.LastRefreshedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.SchemaFields = schema
	return v, nil
}

func scanViewOutHeader(r rowLikeT) (*models.ViewOut, error) {
	v := &models.ViewOut{}
	return v, r.Scan(&v.ID, &v.DatasetID, &v.BranchID, &v.HeadTransactionID, &v.RequestedBranch, &v.ResolvedBranch, &v.FallbackChain, &v.ComputedAt, &v.FileCount, &v.SizeBytes)
}

func scanSchemaResponse(r rowLikeT) (*models.SchemaResponse, error) {
	v := &models.SchemaResponse{}
	var schemaRaw []byte
	if err := r.Scan(&v.ViewID, &v.DatasetID, &v.Branch, &schemaRaw, &v.ContentHash, &v.CreatedAt, &v.Unchanged); err != nil {
		return nil, err
	}
	if err := models.UnmarshalJSONValue(schemaRaw, &v.Schema); err != nil {
		return nil, err
	}
	return v, nil
}

func scanQualityRule(r rowLikeT) (*models.DatasetQualityRule, error) {
	v := &models.DatasetQualityRule{}
	var config []byte
	if err := r.Scan(&v.ID, &v.DatasetID, &v.Name, &v.RuleType, &v.Severity, &config, &v.Enabled, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Config = config
	return v, nil
}

func scanDatasetHealth(r rowLikeT) (*models.DatasetHealth, error) {
	v := &models.DatasetHealth{}
	var nullPct, extras []byte
	if err := r.Scan(&v.DatasetRID, &v.DatasetID, &v.RowCount, &v.ColCount, &nullPct, &v.FreshnessSeconds, &v.LastCommitAt, &v.TxnFailureRate24H, &v.LastBuildStatus, &v.SchemaDriftFlag, &extras, &v.LastComputedAt); err != nil {
		return nil, err
	}
	if err := models.UnmarshalJSONValue(nullPct, &v.NullPctByColumn); err != nil {
		return nil, err
	}
	v.Extras = extras
	return v, nil
}

// Compare, preview, lint, health-policy, and retention-outbox parity helpers.
func (r *Repo) ResolveDatasetIDByRID(ctx context.Context, datasetRID string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.Pool.QueryRow(ctx, `SELECT id FROM datasets WHERE rid = $1`, datasetRID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}

func (r *Repo) GetRuntimeBranch(ctx context.Context, datasetID uuid.UUID, branch string) (*models.RuntimeBranch, error) {
	row := r.Pool.QueryRow(ctx, `SELECT id, rid, dataset_id, dataset_rid, name, parent_branch_id, head_transaction_id,
		created_from_transaction_id, last_activity_at, labels, fallback_chain, created_at, updated_at
		FROM dataset_branches WHERE dataset_id = $1 AND name = $2 AND deleted_at IS NULL AND archived_at IS NULL`, datasetID, branch)
	v, err := scanRuntimeBranch(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

func (r *Repo) FindLowestCommonAncestorRID(ctx context.Context, baseBranchID uuid.UUID, compareBranchID uuid.UUID) (*string, error) {
	row := r.Pool.QueryRow(ctx, `WITH RECURSIVE base_chain AS (
		SELECT id, rid, parent_branch_id, 0 AS depth FROM dataset_branches WHERE id = $1 AND deleted_at IS NULL
		UNION ALL
		SELECT p.id, p.rid, p.parent_branch_id, c.depth + 1 FROM dataset_branches p JOIN base_chain c ON p.id = c.parent_branch_id WHERE p.deleted_at IS NULL
	), compare_chain AS (
		SELECT id, rid, parent_branch_id FROM dataset_branches WHERE id = $2 AND deleted_at IS NULL
		UNION ALL
		SELECT p.id, p.rid, p.parent_branch_id FROM dataset_branches p JOIN compare_chain c ON p.id = c.parent_branch_id WHERE p.deleted_at IS NULL
	)
	SELECT b.rid FROM base_chain b JOIN compare_chain c USING (id) ORDER BY b.depth ASC LIMIT 1`, baseBranchID, compareBranchID)
	var rid string
	if err := row.Scan(&rid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &rid, nil
}

func (r *Repo) ListCommittedTransactionSummaries(ctx context.Context, branchID uuid.UUID, branchName string, limit int) ([]models.TransactionSummary, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	rows, err := r.Pool.Query(ctx, `SELECT 'ri.foundry.main.transaction.' || t.id::text AS transaction_rid,
		t.id, $2::text AS branch, t.tx_type, t.status, t.committed_at,
		COUNT(f.logical_path)::int AS files_changed
		FROM dataset_transactions t
		LEFT JOIN dataset_transaction_files f ON f.transaction_id = t.id
		WHERE t.branch_id = $1 AND t.status = 'COMMITTED'
		GROUP BY t.id, t.tx_type, t.status, t.committed_at
		ORDER BY COALESCE(t.committed_at, t.started_at) ASC, t.started_at ASC
		LIMIT $3`, branchID, branchName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TransactionSummary{}
	for rows.Next() {
		var v models.TransactionSummary
		if err := rows.Scan(&v.TransactionRID, &v.TransactionID, &v.Branch, &v.TxType, &v.Status, &v.CommittedAt, &v.FilesChanged); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) ListConflictingFiles(ctx context.Context, baseBranchID uuid.UUID, compareBranchID uuid.UUID) ([]models.ConflictingFile, error) {
	rows, err := r.Pool.Query(ctx, `WITH a AS (
		SELECT DISTINCT ON (f.logical_path) f.logical_path, t.id AS transaction_id, f.content_hash
		FROM dataset_transactions t JOIN dataset_transaction_files f ON f.transaction_id = t.id
		WHERE t.branch_id = $1 AND t.status = 'COMMITTED'
		ORDER BY f.logical_path, COALESCE(t.committed_at, t.started_at) DESC
	), b AS (
		SELECT DISTINCT ON (f.logical_path) f.logical_path, t.id AS transaction_id, f.content_hash
		FROM dataset_transactions t JOIN dataset_transaction_files f ON f.transaction_id = t.id
		WHERE t.branch_id = $2 AND t.status = 'COMMITTED'
		ORDER BY f.logical_path, COALESCE(t.committed_at, t.started_at) DESC
	)
	SELECT a.logical_path,
		'ri.foundry.main.transaction.' || a.transaction_id::text AS a_transaction_rid,
		'ri.foundry.main.transaction.' || b.transaction_id::text AS b_transaction_rid,
		a.content_hash AS content_hash_a, b.content_hash AS content_hash_b
	FROM a JOIN b USING (logical_path)
	WHERE a.content_hash IS DISTINCT FROM b.content_hash
	ORDER BY a.logical_path`, baseBranchID, compareBranchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ConflictingFile{}
	for rows.Next() {
		var v models.ConflictingFile
		if err := rows.Scan(&v.LogicalPath, &v.ATransactionRID, &v.BTransactionRID, &v.ContentHashA, &v.ContentHashB); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) CompareBranches(ctx context.Context, datasetID uuid.UUID, base string, compare string) (*models.BranchCompareResponse, error) {
	if base == compare {
		return nil, fmt.Errorf("%w: base and compare must differ", ErrValidation)
	}
	baseBranch, err := r.GetRuntimeBranch(ctx, datasetID, base)
	if err != nil {
		return nil, err
	}
	compareBranch, err := r.GetRuntimeBranch(ctx, datasetID, compare)
	if err != nil {
		return nil, err
	}
	lca, err := r.FindLowestCommonAncestorRID(ctx, baseBranch.ID, compareBranch.ID)
	if err != nil {
		return nil, err
	}
	aOnly, err := r.ListCommittedTransactionSummaries(ctx, baseBranch.ID, baseBranch.Name, 200)
	if err != nil {
		return nil, err
	}
	bOnly, err := r.ListCommittedTransactionSummaries(ctx, compareBranch.ID, compareBranch.Name, 200)
	if err != nil {
		return nil, err
	}
	conflicts, err := r.ListConflictingFiles(ctx, baseBranch.ID, compareBranch.ID)
	if err != nil {
		return nil, err
	}
	return &models.BranchCompareResponse{BaseBranch: baseBranch.Name, CompareBranch: compareBranch.Name, LCABranchRID: lca, AOnlyTransactions: aOnly, BOnlyTransactions: bOnly, ConflictingFiles: conflicts}, nil
}

func (r *Repo) PreviewMetadata(ctx context.Context, datasetID uuid.UUID, branch string) (int64, int64, error) {
	row := r.Pool.QueryRow(ctx, `SELECT COUNT(*)::bigint AS file_count, COALESCE(SUM(f.size_bytes),0)::bigint AS size_bytes
		FROM dataset_branches b
		JOIN dataset_files f ON f.dataset_id = b.dataset_id
		WHERE b.dataset_id = $1 AND b.name = $2 AND b.deleted_at IS NULL AND f.deleted_at IS NULL`, datasetID, branch)
	var files, bytes int64
	return files, bytes, row.Scan(&files, &bytes)
}

func (r *Repo) ListQualityHistory(ctx context.Context, datasetID uuid.UUID, limit int) ([]models.DatasetQualityHistoryEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.Pool.Query(ctx, `SELECT id, dataset_id, score, passed_rules, failed_rules, alerts_count, created_at
		FROM dataset_quality_history WHERE dataset_id = $1 ORDER BY created_at DESC LIMIT $2`, datasetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DatasetQualityHistoryEntry{}
	for rows.Next() {
		var v models.DatasetQualityHistoryEntry
		if err := rows.Scan(&v.ID, &v.DatasetID, &v.Score, &v.PassedRules, &v.FailedRules, &v.AlertsCount, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) ListQualityAlerts(ctx context.Context, datasetID uuid.UUID, status *string, limit int) ([]models.DatasetQualityAlert, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.Pool.Query(ctx, `SELECT id, dataset_id, level, kind, message, status, details, created_at, resolved_at
		FROM dataset_quality_alerts WHERE dataset_id = $1 AND ($2::text IS NULL OR status = $2)
		ORDER BY created_at DESC LIMIT $3`, datasetID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DatasetQualityAlert{}
	for rows.Next() {
		v, err := scanQualityAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) UpsertDatasetHealth(ctx context.Context, h *models.DatasetHealth) (*models.DatasetHealth, error) {
	nullPct, err := models.MarshalJSONValue(h.NullPctByColumn)
	if err != nil {
		return nil, err
	}
	row := r.Pool.QueryRow(ctx, `INSERT INTO dataset_health
		(dataset_rid, dataset_id, row_count, col_count, null_pct_by_column, freshness_seconds, last_commit_at, txn_failure_rate_24h, last_build_status, schema_drift_flag, extras)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10,$11::jsonb)
		ON CONFLICT (dataset_rid) DO UPDATE SET dataset_id = EXCLUDED.dataset_id, row_count = EXCLUDED.row_count,
		col_count = EXCLUDED.col_count, null_pct_by_column = EXCLUDED.null_pct_by_column, freshness_seconds = EXCLUDED.freshness_seconds,
		last_commit_at = EXCLUDED.last_commit_at, txn_failure_rate_24h = EXCLUDED.txn_failure_rate_24h,
		last_build_status = EXCLUDED.last_build_status, schema_drift_flag = EXCLUDED.schema_drift_flag, extras = EXCLUDED.extras,
		last_computed_at = NOW()
		RETURNING dataset_rid, dataset_id, row_count, col_count, null_pct_by_column, freshness_seconds, last_commit_at, txn_failure_rate_24h, last_build_status, schema_drift_flag, extras, last_computed_at`,
		h.DatasetRID, h.DatasetID, h.RowCount, h.ColCount, nullPct, h.FreshnessSeconds, h.LastCommitAt, h.TxnFailureRate24H, h.LastBuildStatus, h.SchemaDriftFlag, defaultRawObject(h.Extras))
	return scanDatasetHealth(row)
}

func (r *Repo) ListHealthPolicies(ctx context.Context, datasetRID *string, activeOnly bool) ([]models.DatasetHealthPolicy, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, name, dataset_rid, all_datasets, check_kind, threshold, severity, active, created_at, updated_at
		FROM dataset_health_policies WHERE ($1::text IS NULL OR dataset_rid = $1 OR all_datasets = TRUE)
		AND (NOT $2::boolean OR active = TRUE) ORDER BY name`, datasetRID, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DatasetHealthPolicy{}
	for rows.Next() {
		v, err := scanHealthPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) DatasetLintSummary(ctx context.Context, datasetID uuid.UUID) (*models.DatasetLintSummary, error) {
	row := r.Pool.QueryRow(ctx, `SELECT
		COUNT(DISTINCT v.id)::int AS tracked_versions,
		COUNT(DISTINCT b.id)::int AS branch_count,
		COUNT(DISTINCT b.id) FILTER (WHERE b.last_activity_at < NOW() - INTERVAL '30 days')::int AS stale_branch_count,
		COUNT(DISTINCT dv.id) FILTER (WHERE dv.materialized)::int AS materialized_view_count,
		COUNT(DISTINCT dv.id) FILTER (WHERE dv.refresh_on_source_update)::int AS auto_refresh_view_count,
		COUNT(DISTINCT t.id)::int AS transaction_count,
		COUNT(DISTINCT t.id) FILTER (WHERE t.status = 'ABORTED')::int AS failed_transaction_count,
		COUNT(DISTINCT t.id) FILTER (WHERE t.status = 'OPEN')::int AS pending_transaction_count,
		COUNT(DISTINCT qr.id) FILTER (WHERE qr.enabled)::int AS enabled_rule_count,
		COUNT(DISTINCT qa.id) FILTER (WHERE qa.status = 'active')::int AS active_alert_count,
		COUNT(DISTINCT f.id)::int AS object_count,
		COUNT(DISTINCT f.id) FILTER (WHERE f.size_bytes < 33554432)::int AS small_file_count,
		COALESCE(MAX(f.size_bytes),0)::bigint AS largest_object_bytes,
		COALESCE(AVG(f.size_bytes),0)::bigint AS average_object_size_bytes,
		MAX(qp.score) AS quality_score
		FROM datasets d
		LEFT JOIN dataset_versions v ON v.dataset_id = d.id
		LEFT JOIN dataset_branches b ON b.dataset_id = d.id AND b.deleted_at IS NULL
		LEFT JOIN dataset_views dv ON dv.dataset_id = d.id
		LEFT JOIN dataset_transactions t ON t.dataset_id = d.id
		LEFT JOIN dataset_quality_rules qr ON qr.dataset_id = d.id
		LEFT JOIN dataset_quality_alerts qa ON qa.dataset_id = d.id
		LEFT JOIN dataset_files f ON f.dataset_id = d.id AND f.deleted_at IS NULL
		LEFT JOIN dataset_profiles qp ON qp.dataset_id = d.id
		WHERE d.id = $1`, datasetID)
	v := &models.DatasetLintSummary{}
	if err := row.Scan(&v.TrackedVersions, &v.BranchCount, &v.StaleBranchCount, &v.MaterializedViewCount, &v.AutoRefreshViewCount, &v.TransactionCount, &v.FailedTransactionCount, &v.PendingTransactionCount, &v.EnabledRuleCount, &v.ActiveAlertCount, &v.ObjectCount, &v.SmallFileCount, &v.LargestObjectBytes, &v.AverageObjectSizeBytes, &v.QualityScore); err != nil {
		return nil, err
	}
	if v.HighSeverity > 0 {
		v.ResourcePosture = "critical"
	} else if v.MediumSeverity > 0 || v.ActiveAlertCount > 0 {
		v.ResourcePosture = "warning"
	} else {
		v.ResourcePosture = "healthy"
	}
	return v, nil
}

func (r *Repo) ArchiveBranchForRetentionWithOutbox(ctx context.Context, row models.RetentionRow, graceUntil time.Time, eventPayload models.JSONValue) (bool, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE dataset_branches SET parent_branch_id = $2, updated_at = NOW() WHERE parent_branch_id = $1 AND deleted_at IS NULL`, row.ID, row.ParentBranchID); err != nil {
		return false, err
	}
	cmd, err := tx.Exec(ctx, `UPDATE dataset_branches SET archived_at = NOW(), archive_grace_until = $2, updated_at = NOW() WHERE id = $1 AND archived_at IS NULL AND deleted_at IS NULL`, row.ID, graceUntil)
	if err != nil {
		return false, err
	}
	if cmd.RowsAffected() == 0 {
		return false, tx.Rollback(ctx)
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox.events (event_id, aggregate, aggregate_id, payload, topic, created_at)
		VALUES ($1,'dataset_branch',$2,$3::jsonb,'branch.archived',NOW())
		ON CONFLICT (event_id) DO NOTHING`, uuid.New(), row.ID.String(), defaultRawObject(eventPayload))
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func scanQualityAlert(r rowLikeT) (*models.DatasetQualityAlert, error) {
	v := &models.DatasetQualityAlert{}
	var details []byte
	if err := r.Scan(&v.ID, &v.DatasetID, &v.Level, &v.Kind, &v.Message, &v.Status, &details, &v.CreatedAt, &v.ResolvedAt); err != nil {
		return nil, err
	}
	v.Details = details
	return v, nil
}

func scanHealthPolicy(r rowLikeT) (*models.DatasetHealthPolicy, error) {
	v := &models.DatasetHealthPolicy{}
	var threshold []byte
	if err := r.Scan(&v.ID, &v.Name, &v.DatasetRID, &v.AllDatasets, &v.CheckKind, &threshold, &v.Severity, &v.Active, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Threshold = threshold
	return v, nil
}
