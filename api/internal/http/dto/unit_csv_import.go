package dto

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

// GetUnitImportTemplateInput is GET
// /v1/communities/:id/units/import/template's input (unit-csv-import:
// Downloadable Import Template).
type GetUnitImportTemplateInput struct {
	CommunityID string `path:"id" format:"uuid"`
}

// ScopeCommunityID satisfies authz.CommunityScoped, identically to
// dto.CreateUnitInput's.
func (i *GetUnitImportTemplateInput) ScopeCommunityID() uuid.UUID {
	id, _ := uuid.Parse(i.CommunityID)
	return id
}

// UnitImportFile is the multipart form shape POST
// /v1/communities/:id/units/import accepts: a single required "file"
// field, streamed rather than buffered to any client-supplied path
// (unit-csv-import: Path-Traversal-Safe Filenames). No contentType
// constraint: real CSV uploads arrive under inconsistent MIME types
// (text/csv, application/vnd.ms-excel, application/octet-stream,
// text/plain depending on client/OS) -- huma's own MIME check would
// otherwise reject a legitimate upload huma itself cannot classify.
// Content is validated as CSV (and as UTF-8) by ImportUnits itself.
type UnitImportFile struct {
	File huma.FormFile `form:"file" required:"true"`
}

// CreateUnitImportInput is POST /v1/communities/:id/units/import's
// input (unit-csv-import: Dry-Run Validates Without Writing; Row-By-Row
// Validation Before Any Write). DryRun defaults to false: an
// unqualified POST writes.
type CreateUnitImportInput struct {
	CommunityID string `path:"id" format:"uuid"`
	DryRun      bool   `query:"dry_run"`
	RawBody     huma.MultipartFormFiles[UnitImportFile]
}

// ScopeCommunityID satisfies authz.CommunityScoped.
func (i *CreateUnitImportInput) ScopeCommunityID() uuid.UUID {
	id, _ := uuid.Parse(i.CommunityID)
	return id
}

// UnitImportRowResult is one CSV data row's outcome (unit-csv-import).
// Block/Floor/Door/OwnerName/OwnerDNICIF echo the parsed (and, for the
// owner fields, formula-neutralized) cell values for the caller's
// confirmation UI; UnitID is set only for a row that was actually
// written (never during a dry run, never for a row with Errors).
type UnitImportRowResult struct {
	Row         int        `json:"row"`
	Errors      []string   `json:"errors,omitempty"`
	UnitID      *uuid.UUID `json:"unit_id,omitempty"`
	Block       string     `json:"block,omitempty"`
	Floor       string     `json:"floor,omitempty"`
	Door        string     `json:"door,omitempty"`
	Type        string     `json:"type,omitempty"`
	OwnerName   string     `json:"owner_name,omitempty"`
	OwnerDNICIF string     `json:"owner_dni_cif,omitempty"`
}

// UnitImportResponse is POST /v1/communities/:id/units/import's body.
// Imported is always 0 when DryRun is true or when ANY row carries
// Errors (unit-csv-import: "MUST NOT partially commit when any row
// fails").
type UnitImportResponse struct {
	DryRun   bool                  `json:"dry_run"`
	Imported int                   `json:"imported"`
	Rows     []UnitImportRowResult `json:"rows"`
	Warnings []string              `json:"warnings,omitempty"`
}

type CreateUnitImportOutput struct {
	Body UnitImportResponse
}
