package marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/models"
)

var (
	ErrNotFound   = errors.New("marketplace listing not found")
	ErrValidation = errors.New("marketplace validation failed")
)

type Repository interface {
	CreateListing(ctx context.Context, req models.CreateListingRequest) (*models.ListingDefinition, error)
	ListListings(ctx context.Context, limit, offset int) ([]models.ListingDefinition, int, error)
	GetListing(ctx context.Context, ref string) (*models.ListingDetail, error)
	UpdateListing(ctx context.Context, id uuid.UUID, req models.UpdateListingRequest) (*models.ListingDefinition, error)
	PublishVersion(ctx context.Context, listingID uuid.UUID, req models.PublishVersionRequest) (*models.PackageVersion, error)
}

type PGXRepository struct{ Pool *pgxpool.Pool }

func NewPGXRepository(pool *pgxpool.Pool) *PGXRepository { return &PGXRepository{Pool: pool} }

func (r *PGXRepository) CreateListing(ctx context.Context, req models.CreateListingRequest) (*models.ListingDefinition, error) {
	if err := ValidateCreateListing(req); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	id, err := uuid.NewV7()
	if err != nil {
		id = uuid.New()
	}
	if req.Visibility == "" {
		req.Visibility = "private"
	}
	tags := jsonArray(req.Tags)
	capabilities := jsonArray(req.Capabilities)
	row := r.Pool.QueryRow(ctx, `
INSERT INTO marketplace_listings (id, name, slug, summary, description, publisher, category_slug, package_kind, repository_slug, visibility, tags, capabilities, install_count, average_rating, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12::jsonb, 0, 0, $13, $14)
RETURNING id, name, slug, summary, description, publisher, category_slug, package_kind, repository_slug, visibility, tags, capabilities, install_count, average_rating, created_at, updated_at`,
		id, req.Name, req.Slug, req.Summary, req.Description, req.Publisher, req.CategorySlug, string(req.PackageKind), req.RepositorySlug, req.Visibility, tags, capabilities, now, now)
	listing, err := scanListing(row)
	if err != nil {
		return nil, mapPGError(err)
	}
	return listing, nil
}

func (r *PGXRepository) ListListings(ctx context.Context, limit, offset int) ([]models.ListingDefinition, int, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT id, name, slug, summary, description, publisher, category_slug, package_kind, repository_slug, visibility, tags, capabilities, install_count, average_rating, created_at, updated_at, count(*) OVER() AS total
FROM marketplace_listings
ORDER BY install_count DESC, average_rating DESC, updated_at DESC
LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []models.ListingDefinition{}
	total := 0
	for rows.Next() {
		listing, rowTotal, err := scanListingWithTotal(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *listing)
		total = rowTotal
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if total == 0 && offset > 0 {
		if err := r.Pool.QueryRow(ctx, `SELECT count(*) FROM marketplace_listings`).Scan(&total); err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}

func (r *PGXRepository) GetListing(ctx context.Context, ref string) (*models.ListingDetail, error) {
	listing, err := r.getListingDefinition(ctx, ref)
	if err != nil {
		return nil, err
	}
	versions, err := r.listVersions(ctx, listing.ID)
	if err != nil {
		return nil, err
	}
	reviews, err := r.listReviews(ctx, listing.ID)
	if err != nil {
		return nil, err
	}
	var latest *models.PackageVersion
	if len(versions) > 0 {
		latest = &versions[0]
	}
	return &models.ListingDetail{Listing: *listing, LatestVersion: latest, Versions: versions, Reviews: reviews}, nil
}

func (r *PGXRepository) UpdateListing(ctx context.Context, id uuid.UUID, req models.UpdateListingRequest) (*models.ListingDefinition, error) {
	current, err := r.getListingDefinition(ctx, id.String())
	if err != nil {
		return nil, err
	}
	applyUpdate(current, req)
	if err := ValidateListingDefinition(*current); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row := r.Pool.QueryRow(ctx, `
UPDATE marketplace_listings
SET name = $2, summary = $3, description = $4, category_slug = $5, repository_slug = $6, visibility = $7, tags = $8::jsonb, capabilities = $9::jsonb, updated_at = $10
WHERE id = $1
RETURNING id, name, slug, summary, description, publisher, category_slug, package_kind, repository_slug, visibility, tags, capabilities, install_count, average_rating, created_at, updated_at`,
		id, current.Name, current.Summary, current.Description, current.CategorySlug, current.RepositorySlug, current.Visibility, jsonArray(current.Tags), jsonArray(current.Capabilities), now)
	updated, err := scanListing(row)
	if err != nil {
		return nil, mapPGError(err)
	}
	return updated, nil
}

func (r *PGXRepository) PublishVersion(ctx context.Context, listingID uuid.UUID, req models.PublishVersionRequest) (*models.PackageVersion, error) {
	if err := ValidatePublishVersion(req); err != nil {
		return nil, err
	}
	if _, err := r.getListingDefinition(ctx, listingID.String()); err != nil {
		return nil, err
	}
	versionID, err := uuid.NewV7()
	if err != nil {
		versionID = uuid.New()
	}
	if len(req.Dependencies) == 0 {
		req.Dependencies = json.RawMessage(`[]`)
	}
	if len(req.PackagedResources) == 0 {
		req.PackagedResources = json.RawMessage(`[]`)
	}
	if len(req.Manifest) == 0 {
		req.Manifest = json.RawMessage(`{}`)
	}
	if req.ReleaseChannel == "" {
		req.ReleaseChannel = "stable"
	}
	if req.DependencyMode == "" {
		req.DependencyMode = "strict"
	}
	publishedAt := time.Now().UTC()
	row := r.Pool.QueryRow(ctx, `
INSERT INTO marketplace_package_versions (id, listing_id, version, release_channel, changelog, dependency_mode, dependencies, packaged_resources, manifest, published_at)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9::jsonb, $10)
RETURNING id, listing_id, version, release_channel, changelog, dependency_mode, dependencies, packaged_resources, manifest, published_at`,
		versionID, listingID, req.Version, req.ReleaseChannel, req.Changelog, req.DependencyMode, req.Dependencies, req.PackagedResources, req.Manifest, publishedAt)
	version, err := scanVersion(row)
	if err != nil {
		return nil, mapPGError(err)
	}
	return version, nil
}

func (r *PGXRepository) getListingDefinition(ctx context.Context, ref string) (*models.ListingDefinition, error) {
	query := `SELECT id, name, slug, summary, description, publisher, category_slug, package_kind, repository_slug, visibility, tags, capabilities, install_count, average_rating, created_at, updated_at FROM marketplace_listings WHERE id = $1`
	arg := any(ref)
	if id, err := uuid.Parse(ref); err == nil {
		arg = id
	} else {
		query = strings.Replace(query, "id = $1", "slug = $1", 1)
	}
	listing, err := scanListing(r.Pool.QueryRow(ctx, query, arg))
	if err != nil {
		return nil, mapPGError(err)
	}
	return listing, nil
}

func (r *PGXRepository) listVersions(ctx context.Context, listingID uuid.UUID) ([]models.PackageVersion, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, listing_id, version, release_channel, changelog, dependency_mode, dependencies, packaged_resources, manifest, published_at FROM marketplace_package_versions WHERE listing_id = $1 ORDER BY published_at DESC`, listingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := []models.PackageVersion{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, *v)
	}
	return versions, rows.Err()
}

func (r *PGXRepository) listReviews(ctx context.Context, listingID uuid.UUID) ([]models.ListingReview, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, listing_id, author, rating, headline, body, recommended, created_at FROM marketplace_reviews WHERE listing_id = $1 ORDER BY created_at DESC`, listingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reviews := []models.ListingReview{}
	for rows.Next() {
		var review models.ListingReview
		if err := rows.Scan(&review.ID, &review.ListingID, &review.Author, &review.Rating, &review.Headline, &review.Body, &review.Recommended, &review.CreatedAt); err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanListing(row scanner) (*models.ListingDefinition, error) {
	var l models.ListingDefinition
	var packageKind string
	var tags, capabilities []byte
	if err := row.Scan(&l.ID, &l.Name, &l.Slug, &l.Summary, &l.Description, &l.Publisher, &l.CategorySlug, &packageKind, &l.RepositorySlug, &l.Visibility, &tags, &capabilities, &l.InstallCount, &l.AverageRating, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, err
	}
	l.PackageKind = models.PackageType(packageKind)
	if err := json.Unmarshal(tags, &l.Tags); err != nil {
		return nil, fmt.Errorf("decode tags: %w", err)
	}
	if err := json.Unmarshal(capabilities, &l.Capabilities); err != nil {
		return nil, fmt.Errorf("decode capabilities: %w", err)
	}
	return &l, nil
}

func scanListingWithTotal(row scanner) (*models.ListingDefinition, int, error) {
	var l models.ListingDefinition
	var packageKind string
	var tags, capabilities []byte
	var total int
	if err := row.Scan(&l.ID, &l.Name, &l.Slug, &l.Summary, &l.Description, &l.Publisher, &l.CategorySlug, &packageKind, &l.RepositorySlug, &l.Visibility, &tags, &capabilities, &l.InstallCount, &l.AverageRating, &l.CreatedAt, &l.UpdatedAt, &total); err != nil {
		return nil, 0, err
	}
	l.PackageKind = models.PackageType(packageKind)
	if err := json.Unmarshal(tags, &l.Tags); err != nil {
		return nil, 0, fmt.Errorf("decode tags: %w", err)
	}
	if err := json.Unmarshal(capabilities, &l.Capabilities); err != nil {
		return nil, 0, fmt.Errorf("decode capabilities: %w", err)
	}
	return &l, total, nil
}

func scanVersion(row scanner) (*models.PackageVersion, error) {
	var v models.PackageVersion
	if err := row.Scan(&v.ID, &v.ListingID, &v.Version, &v.ReleaseChannel, &v.Changelog, &v.DependencyMode, &v.Dependencies, &v.PackagedResources, &v.Manifest, &v.PublishedAt); err != nil {
		return nil, err
	}
	return &v, nil
}

func applyUpdate(l *models.ListingDefinition, req models.UpdateListingRequest) {
	if req.Name != nil {
		l.Name = *req.Name
	}
	if req.Summary != nil {
		l.Summary = *req.Summary
	}
	if req.Description != nil {
		l.Description = *req.Description
	}
	if req.CategorySlug != nil {
		l.CategorySlug = *req.CategorySlug
	}
	if req.RepositorySlug != nil {
		l.RepositorySlug = *req.RepositorySlug
	}
	if req.Visibility != nil {
		l.Visibility = *req.Visibility
	}
	if req.Tags != nil {
		l.Tags = *req.Tags
	}
	if req.Capabilities != nil {
		l.Capabilities = *req.Capabilities
	}
}

func jsonArray(values []string) json.RawMessage {
	if values == nil {
		values = []string{}
	}
	b, _ := json.Marshal(values)
	return b
}

func mapPGError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: listing slug already exists", ErrValidation)
	}
	return err
}
