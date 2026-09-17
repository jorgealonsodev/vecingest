// unit_csv_import.go implements unit-csv-import (design D-7): the
// downloadable template, and the dry-run/write CSV import for
// POST /v1/communities/:id/units/import. This is the largest
// untrusted-input surface in M1 (an uploaded file), so every row is
// validated BEFORE any write, cell values are formula-neutralized, and
// the uploaded filename is sanitized before it is ever echoed back --
// never used to derive a storage path, since the upload is streamed
// straight into CSV parsing and nothing is ever written to disk under a
// client-supplied name.
//
// Scope decision (documented, not silently improvised): this milestone
// creates UNIT rows only. owner_name/owner_dni_cif are template/import
// columns the spec's "columns required for unit and owner import"
// names, and are parsed, validated and formula-neutralized per row, but
// are NOT persisted to any column -- the current schema (D-5, frozen;
// no migration is authorized for this work unit) has no column for
// either, and unit_members.user_id is a NOT NULL FK requiring a real
// users row, which the documented column set has no email to resolve
// or create one from. Owner-account linkage for a CSV-imported unit is
// Phase 6/WU-4's invitation flow, not this one; CreateUnit (units.go)
// already covers linking an EXISTING account's email to a unit at
// single-unit creation time.
package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/authz/scoped"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// unitImportCSVHeader is the documented column set (unit-csv-import:
// Downloadable Import Template): "portal" is the Spanish term for the
// building entrance/block PRD_go.md uses interchangeably with "block".
var unitImportCSVHeader = []string{"portal", "floor", "door", "type", "coefficient", "owner_name", "owner_dni_cif"}

// maxImportFileBytes bounds the upload so a hostile multi-gigabyte file
// cannot be buffered into memory unbounded.
const maxImportFileBytes = 5 << 20 // 5 MiB

// maxImportRows bounds the row count for the same reason, independent
// of raw byte size (a file of many short rows could otherwise stay
// under the byte cap while still exhausting memory/CPU row by row).
const maxImportRows = 5000

// validUnitTypes mirrors the migration's own CHECK constraint
// (units.type IN ('flat','premises','garage','storage')) so an invalid
// value is reported as a row error, never a raw 500 from a constraint
// violation.
var validUnitTypes = map[string]bool{"flat": true, "premises": true, "garage": true, "storage": true}

// GetUnitImportTemplate implements GET
// /v1/communities/:id/units/import/template (unit-csv-import:
// Downloadable Import Template). Registered via scoped.Community with
// unitManageRoles: scoped to the caller's community membership, exactly
// as CreateUnit is.
func (d *Deps) GetUnitImportTemplate(_ context.Context, _ *dto.GetUnitImportTemplateInput, _ authz.Membership) (*huma.StreamResponse, error) {
	return &huma.StreamResponse{
		Body: func(hctx huma.Context) {
			hctx.SetHeader("Content-Type", "text/csv; charset=utf-8")
			hctx.SetHeader("Content-Disposition", `attachment; filename="units-import-template.csv"`)
			w := csv.NewWriter(hctx.BodyWriter())
			header := make([]string, len(unitImportCSVHeader))
			for i, c := range unitImportCSVHeader {
				header[i] = csvSafeCell(c)
			}
			_ = w.Write(header)
			w.Flush()
		},
	}, nil
}

// importRow is one CSV data row's parsed, validated state.
type importRow struct {
	rowNum      int
	block       string
	floor       string
	door        string
	unitType    string
	coefficient string // decimal string, "" when the cell was empty
	ownerName   string
	ownerDNI    string
	errs        []string
}

// ImportUnits implements POST /v1/communities/:id/units/import (unit-
// csv-import: Dry-Run Validates Without Writing; Row-By-Row Validation
// Before Any Write; Formula-Injection Hardening; Path-Traversal-Safe
// Filenames). Registered via scoped.Community with unitManageRoles.
func (d *Deps) ImportUnits(ctx context.Context, in *dto.CreateUnitImportInput, membership authz.Membership) (*dto.CreateUnitImportOutput, error) {
	file := in.RawBody.Data().File
	if !file.IsSet {
		return nil, apperr.New(400, apperr.CodeValidation, "file is required", nil)
	}

	filename, err := sanitizeImportFilename(file.Filename)
	if err != nil {
		return nil, apperr.New(400, apperr.CodeValidation, "unsafe or unusable filename", nil)
	}

	raw, err := io.ReadAll(io.LimitReader(file, maxImportFileBytes+1))
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if len(raw) > maxImportFileBytes {
		return nil, apperr.New(400, apperr.CodeValidation, "import file exceeds the size limit", nil)
	}
	if !utf8.Valid(raw) {
		return nil, apperr.New(400, apperr.CodeValidation, "import file must be valid UTF-8", nil)
	}

	reader := csv.NewReader(bytes.NewReader(raw))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, apperr.New(400, apperr.CodeValidation, "malformed CSV", nil)
	}
	if len(records) == 0 {
		return nil, apperr.New(400, apperr.CodeValidation, "empty import file", nil)
	}

	dataRows := records
	if isImportHeaderRow(records[0]) {
		dataRows = records[1:]
	}
	if len(dataRows) > maxImportRows {
		return nil, apperr.New(400, apperr.CodeValidation, fmt.Sprintf("import file exceeds the %d row limit", maxImportRows), nil)
	}

	q := db.New(d.DB.Read)
	existingUnits, err := q.ListUnitsByCommunityID(ctx, membership.CommunityID())
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	existingKeys := make(map[string]bool, len(existingUnits))
	for _, u := range existingUnits {
		existingKeys[unitDedupKey(u.Block.String, u.Floor.String, u.Door.String)] = true
	}

	rows, hasErrors := parseImportRows(dataRows, existingKeys)

	if in.DryRun || hasErrors {
		return &dto.CreateUnitImportOutput{Body: dto.UnitImportResponse{
			DryRun: in.DryRun, Imported: 0, Rows: importRowResults(rows),
		}}, nil
	}

	return d.writeImportedUnits(ctx, membership, filename, rows)
}

// writeImportedUnits performs the actual write half of ImportUnits: ALL
// rows insert inside ONE transaction (unit-csv-import: "MUST NOT
// partially commit when any row fails") -- any failure at this stage
// (e.g. a race against a concurrent writer creating the same
// block/floor/door between the pre-check and this transaction) rolls
// back every row already inserted in this batch, never a partial
// import.
func (d *Deps) writeImportedUnits(ctx context.Context, membership authz.Membership, filename string, rows []importRow) (*dto.CreateUnitImportOutput, error) {
	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	results := make([]dto.UnitImportRowResult, len(rows))
	unitIDs := make([]uuid.UUID, 0, len(rows))
	for i, r := range rows {
		coefficient, cerr := parseOptionalNumeric(r.coefficient)
		if cerr != nil {
			// Unreachable in practice (parseImportRows already validated
			// the cell), but never silently insert an unparsable value.
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}

		unit, ierr := q.InsertUnit(ctx, db.InsertUnitParams{
			ID:                       uuid.New(),
			CommunityID:              membership.CommunityID(),
			Block:                    optionalText(r.block),
			Floor:                    optionalText(r.floor),
			Door:                     optionalText(r.door),
			Type:                     r.unitType,
			ParticipationCoefficient: coefficient,
		})
		if ierr != nil {
			if isUniqueViolation(ierr) {
				return nil, apperr.New(409, apperr.CodeConflict, fmt.Sprintf("row %d: a unit with this block/floor/door already exists", r.rowNum), nil)
			}
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}

		unitID := unit.ID
		unitIDs = append(unitIDs, unitID)
		results[i] = dto.UnitImportRowResult{
			Row: r.rowNum, UnitID: &unitID, Block: r.block, Floor: r.floor, Door: r.door,
			Type: r.unitType, OwnerName: r.ownerName, OwnerDNICIF: r.ownerDNI,
		}
	}

	sum, err := q.SumParticipationCoefficientByCommunityID(ctx, membership.CommunityID())
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	callerID := membership.UserID()
	communityID := membership.CommunityID()
	after, _ := json.Marshal(map[string]any{"filename": filename, "imported": len(unitIDs), "unit_ids": unitIDs})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, CommunityID: &communityID, Action: "unit.import", Entity: "unit", After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.CreateUnitImportOutput{Body: dto.UnitImportResponse{
		DryRun: false, Imported: len(unitIDs), Rows: results, Warnings: coefficientWarnings(sum),
	}}, nil
}

// parseImportRows validates every row (format, type, coefficient,
// in-file duplicate block/floor/door, AND duplicate against units
// already persisted for this community) BEFORE any write is even
// attempted (unit-csv-import: Row-By-Row Validation Before Any Write).
// existingKeys is queried once, up front, so a row colliding with an
// ALREADY-persisted unit is reported exactly like an in-file collision,
// never surfacing only as a late unique-violation during the write
// transaction.
func parseImportRows(dataRows [][]string, existingKeys map[string]bool) (rows []importRow, hasErrors bool) {
	seenInFile := map[string]int{}
	rows = make([]importRow, 0, len(dataRows))

	for i, record := range dataRows {
		rowNum := i + 1
		r := importRow{rowNum: rowNum}

		if len(record) != len(unitImportCSVHeader) {
			r.errs = append(r.errs, fmt.Sprintf("expected %d columns, got %d", len(unitImportCSVHeader), len(record)))
			rows = append(rows, r)
			hasErrors = true
			continue
		}

		r.block = strings.TrimSpace(record[0])
		r.floor = strings.TrimSpace(record[1])
		r.door = strings.TrimSpace(record[2])
		r.unitType = strings.TrimSpace(record[3])
		coefficientCell := strings.TrimSpace(record[4])
		r.ownerName = csvSafeCell(strings.TrimSpace(record[5]))
		r.ownerDNI = csvSafeCell(strings.TrimSpace(record[6]))

		if !validUnitTypes[r.unitType] {
			r.errs = append(r.errs, "invalid type: must be one of flat, premises, garage, storage")
		}

		if coefficientCell != "" {
			if _, cerr := parseOptionalNumeric(coefficientCell); cerr != nil {
				r.errs = append(r.errs, "invalid coefficient: not a decimal number")
			} else {
				r.coefficient = coefficientCell
			}
		}

		dedupKey := unitDedupKey(r.block, r.floor, r.door)
		if firstRow, dup := seenInFile[dedupKey]; dup {
			r.errs = append(r.errs, fmt.Sprintf("duplicate block/floor/door, already used on row %d", firstRow))
		} else {
			seenInFile[dedupKey] = rowNum
		}
		if existingKeys[dedupKey] {
			r.errs = append(r.errs, "a unit with this block/floor/door already exists in this community")
		}

		if len(r.errs) > 0 {
			hasErrors = true
		}
		rows = append(rows, r)
	}

	return rows, hasErrors
}

// importRowResults renders every parsed row for a dry-run/error
// response (never sets UnitID: nothing was written).
func importRowResults(rows []importRow) []dto.UnitImportRowResult {
	out := make([]dto.UnitImportRowResult, len(rows))
	for i, r := range rows {
		out[i] = dto.UnitImportRowResult{
			Row: r.rowNum, Errors: r.errs, Block: r.block, Floor: r.floor, Door: r.door,
			Type: r.unitType, OwnerName: r.ownerName, OwnerDNICIF: r.ownerDNI,
		}
	}
	return out
}

// unitDedupKey is units' own uniqueness key (Unit Uniqueness Per
// Community).
func unitDedupKey(block, floor, door string) string { return block + "|" + floor + "|" + door }

// isImportHeaderRow reports whether record is the documented header row
// (case-insensitive), so an uploaded file that includes it is not
// itself treated as a data row.
func isImportHeaderRow(record []string) bool {
	if len(record) != len(unitImportCSVHeader) {
		return false
	}
	for i, want := range unitImportCSVHeader {
		if !strings.EqualFold(strings.TrimSpace(record[i]), want) {
			return false
		}
	}
	return true
}

// RegisterUnitCSVImport wires unit-csv-import's two operations into api.
func RegisterUnitCSVImport(api huma.API, d *Deps) {
	scoped.Community(api, huma.Operation{
		OperationID: "getUnitImportTemplate",
		Method:      "GET",
		Path:        "/v1/communities/{id}/units/import/template",
		Summary:     "Download the unit/owner CSV import template",
		Tags:        []string{"units"},
	}, unitManageRoles, d.GetUnitImportTemplate)

	scoped.Community(api, huma.Operation{
		OperationID: "importUnits",
		Method:      "POST",
		Path:        "/v1/communities/{id}/units/import",
		Summary:     "Bulk-import units via CSV (?dry_run=true validates without writing)",
		Tags:        []string{"units"},
	}, unitManageRoles, d.ImportUnits)
}
