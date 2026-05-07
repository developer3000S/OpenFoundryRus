package marketplace

import (
	"fmt"
	"strings"

	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/models"
)

func ValidateCreateListing(req models.CreateListingRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: listing name is required", ErrValidation)
	}
	if strings.TrimSpace(req.Slug) == "" {
		return fmt.Errorf("%w: listing slug is required", ErrValidation)
	}
	if strings.TrimSpace(req.Summary) == "" {
		return fmt.Errorf("%w: listing summary is required", ErrValidation)
	}
	if strings.TrimSpace(req.Publisher) == "" {
		return fmt.Errorf("%w: publisher is required", ErrValidation)
	}
	if strings.TrimSpace(req.CategorySlug) == "" {
		return fmt.Errorf("%w: category is required", ErrValidation)
	}
	if strings.TrimSpace(req.RepositorySlug) == "" {
		return fmt.Errorf("%w: repository slug is required", ErrValidation)
	}
	if req.PackageKind == "" || !req.PackageKind.Valid() {
		return fmt.Errorf("%w: package_kind is invalid", ErrValidation)
	}
	if req.Visibility != "" && !validVisibility(req.Visibility) {
		return fmt.Errorf("%w: visibility must be private, internal, or public", ErrValidation)
	}
	return nil
}

func ValidateListingDefinition(l models.ListingDefinition) error {
	return ValidateCreateListing(models.CreateListingRequest{
		Name:           l.Name,
		Slug:           l.Slug,
		Summary:        l.Summary,
		Publisher:      l.Publisher,
		CategorySlug:   l.CategorySlug,
		PackageKind:    l.PackageKind,
		RepositorySlug: l.RepositorySlug,
		Visibility:     l.Visibility,
	})
}

func ValidatePublishVersion(req models.PublishVersionRequest) error {
	if strings.TrimSpace(req.Version) == "" {
		return fmt.Errorf("%w: version is required", ErrValidation)
	}
	if strings.TrimSpace(req.Changelog) == "" {
		return fmt.Errorf("%w: changelog is required", ErrValidation)
	}
	return nil
}

func validVisibility(v string) bool {
	switch v {
	case "private", "internal", "public":
		return true
	default:
		return false
	}
}
