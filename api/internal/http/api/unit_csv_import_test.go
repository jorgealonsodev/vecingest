package api_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// doMultipartCSV posts a single-file multipart/form-data request (field
// "file") to url, returning the parsed JSON response -- there is no
// existing multipart test helper in this codebase (every other M1
// endpoint is plain JSON), so this is unit-csv-import's own.
func doMultipartCSV(t *testing.T, client *http.Client, url, filename, content string, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write form file content: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do multipart request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp, body
}

// unit-csv-import: Downloadable Import Template.
func TestUnitImport_DownloadableTemplate(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "csv-template-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "CSV Template Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/communities/"+communityID.String()+"/units/import/template", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("get template: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	header := buf.String()
	for _, col := range []string{"portal", "floor", "door", "type", "coefficient", "owner_name", "owner_dni_cif"} {
		if !strings.Contains(header, col) {
			t.Fatalf("expected template to contain column %q, got %q", col, header)
		}
	}
}

// unit-csv-import: Dry-Run Validates Without Writing (both scenarios).
func TestUnitImport_DryRunValidatesWithoutWriting(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "csv-dryrun-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "CSV DryRun Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	var csvBuilder strings.Builder
	csvBuilder.WriteString("portal,floor,door,type,coefficient,owner_name,owner_dni_cif\n")
	for i := 0; i < 8; i++ {
		csvBuilder.WriteString("A,1,D" + strconv.Itoa(i) + ",flat,10,Jane Doe,12345678A\n")
	}
	csvBuilder.WriteString("A,2,D0,not-a-type,10,Jane Doe,12345678A\n")     // invalid type
	csvBuilder.WriteString("A,2,D1,flat,not-a-number,Jane Doe,12345678A\n") // invalid coefficient

	resp, body := doMultipartCSV(t, client, srv.URL+"/v1/communities/"+communityID.String()+"/units/import?dry_run=true", "units.csv", csvBuilder.String(), auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	if imported, _ := body["imported"].(float64); imported != 0 {
		t.Fatalf("expected zero rows written on a dry run, got %v", body["imported"])
	}
	rows, _ := body["rows"].([]any)
	invalidCount := 0
	for _, r := range rows {
		rm, _ := r.(map[string]any)
		if errs, ok := rm["errors"].([]any); ok && len(errs) > 0 {
			invalidCount++
		}
	}
	if invalidCount != 2 {
		t.Fatalf("expected exactly 2 invalid rows reported, got %d (rows=%v)", invalidCount, rows)
	}

	q := db.New(handlesDB.Write)
	units, err := q.ListUnitsByCommunityID(t.Context(), communityID)
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	if len(units) != 0 {
		t.Fatalf("expected no unit written by a dry run, got %d", len(units))
	}

	// All-valid dry run: reports success, still writes nothing.
	var validOnly strings.Builder
	validOnly.WriteString("portal,floor,door,type,coefficient,owner_name,owner_dni_cif\n")
	validOnly.WriteString("B,1,D0,flat,50,Jane Doe,12345678A\n")

	resp, body = doMultipartCSV(t, client, srv.URL+"/v1/communities/"+communityID.String()+"/units/import?dry_run=true", "units.csv", validOnly.String(), auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	rows, _ = body["rows"].([]any)
	for _, r := range rows {
		rm, _ := r.(map[string]any)
		if errs, ok := rm["errors"].([]any); ok && len(errs) > 0 {
			t.Fatalf("expected no errors for an all-valid dry run, got %v", rm)
		}
	}
	units, err = q.ListUnitsByCommunityID(t.Context(), communityID)
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	if len(units) != 0 {
		t.Fatalf("expected still no unit written by an all-valid dry run, got %d", len(units))
	}
}

// unit-csv-import: Row-By-Row Validation Before Any Write.
func TestUnitImport_RowByRowValidationBeforeAnyWrite(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "csv-txn-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "CSV Txn Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	var csvBuilder strings.Builder
	csvBuilder.WriteString("portal,floor,door,type,coefficient,owner_name,owner_dni_cif\n")
	for i := 0; i < 9; i++ {
		csvBuilder.WriteString("C,1,D" + strconv.Itoa(i) + ",flat,10,Jane Doe,12345678A\n")
	}
	csvBuilder.WriteString("C,2,D0,invalid-type,10,Jane Doe,12345678A\n")

	resp, body := doMultipartCSV(t, client, srv.URL+"/v1/communities/"+communityID.String()+"/units/import", "units.csv", csvBuilder.String(), auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (validation errors are reported in the body, not a transport failure), got %d body=%v", resp.StatusCode, body)
	}
	if imported, _ := body["imported"].(float64); imported != 0 {
		t.Fatalf("expected zero rows written when any row is invalid, got %v", body["imported"])
	}

	q := db.New(handlesDB.Write)
	units, err := q.ListUnitsByCommunityID(t.Context(), communityID)
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	if len(units) != 0 {
		t.Fatalf("expected NO unit written -- not even the 9 valid ones -- when one row fails, got %d", len(units))
	}
}

// unit-csv-import: Formula-Injection Hardening -- import ingestion half
// (the template/export half is covered by csv_safety_test.go's
// TestCSVSafeCell table test, per design.md's Testing Strategy
// classifying formula escaping as a unit-level table test).
func TestUnitImport_FormulaInjectionNeutralizedOnIngestion(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "csv-formula-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "CSV Formula Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	csvContent := "portal,floor,door,type,coefficient,owner_name,owner_dni_cif\n" +
		"D,1,D0,flat,50,=SUM(A1:A9),12345678A\n"

	resp, body := doMultipartCSV(t, client, srv.URL+"/v1/communities/"+communityID.String()+"/units/import", "units.csv", csvContent, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	rows, _ := body["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("expected exactly one row result, got %v", rows)
	}
	row, _ := rows[0].(map[string]any)
	ownerName, _ := row["owner_name"].(string)
	if ownerName != "'=SUM(A1:A9)" {
		t.Fatalf("expected the formula-prefixed owner name to be neutralized, got %q", ownerName)
	}
}

// unit-csv-import: Path-Traversal-Safe Filenames.
func TestUnitImport_PathTraversalSafeFilename(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "csv-traversal-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "CSV Traversal Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	csvContent := "portal,floor,door,type,coefficient,owner_name,owner_dni_cif\n" +
		"E,1,D0,flat,50,Jane Doe,12345678A\n"

	resp, body := doMultipartCSV(t, client, srv.URL+"/v1/communities/"+communityID.String()+"/units/import", "../../etc/passwd.csv", csvContent, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected the traversal filename to be sanitized rather than crash the request, got %d body=%v", resp.StatusCode, body)
	}
	if imported, _ := body["imported"].(float64); imported != 1 {
		t.Fatalf("expected the import to still succeed using the sanitized filename, got %v", body["imported"])
	}

	// The audit trail records the SANITIZED filename -- never the raw
	// traversal string -- proving sanitization actually ran rather than
	// merely failing to crash by coincidence.
	var afterJSON []byte
	if err := handlesDB.Write.QueryRow(t.Context(),
		"SELECT after FROM audit_log WHERE action = 'unit.import' AND community_id = $1 ORDER BY created_at DESC LIMIT 1",
		communityID,
	).Scan(&afterJSON); err != nil {
		t.Fatalf("query audit_log: %v", err)
	}
	var after map[string]any
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		t.Fatalf("unmarshal audit after: %v", err)
	}
	if after["filename"] != "passwd.csv" {
		t.Fatalf("expected the audit trail to record the sanitized filename %q, got %v", "passwd.csv", after["filename"])
	}
}

// unit-csv-import: a row cannot redirect a write to another tenant --
// there is no community_id column in the CSV at all; every imported
// unit lands in the route's own community regardless of file content.
func TestUnitImport_NeverWritesAcrossTenantBoundary(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "csv-tenant-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "CSV Tenant Community")
	otherOfficeID, _ := seedOfficeWithAdmin(t, handlesDB, "csv-tenant-other-admin@example.com")
	otherCommunityID := seedCommunity(t, handlesDB, otherOfficeID, "CSV Other Tenant Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	csvContent := "portal,floor,door,type,coefficient,owner_name,owner_dni_cif\n" +
		"F,1,D0,flat,50,Jane Doe,12345678A\n" +
		"F,1,D1,flat,0,Jane Doe,12345678A\n"

	resp, body := doMultipartCSV(t, client, srv.URL+"/v1/communities/"+communityID.String()+"/units/import", "units.csv", csvContent, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
	if imported, _ := body["imported"].(float64); imported != 2 {
		t.Fatalf("expected 2 rows imported, got %v", body["imported"])
	}

	q := db.New(handlesDB.Write)
	otherUnits, err := q.ListUnitsByCommunityID(t.Context(), otherCommunityID)
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	if len(otherUnits) != 0 {
		t.Fatalf("expected zero units written into the OTHER community, got %d", len(otherUnits))
	}
	ownUnits, err := q.ListUnitsByCommunityID(t.Context(), communityID)
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	if len(ownUnits) != 2 {
		t.Fatalf("expected 2 units written into the route's own community, got %d", len(ownUnits))
	}
}

// unit-csv-import: hostile-input hardening -- malformed encoding and an
// oversized row count are both rejected rather than processed.
func TestUnitImport_HostileInputHardening(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "csv-hostile-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "CSV Hostile Community")
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	invalidUTF8 := "portal,floor,door,type,coefficient,owner_name,owner_dni_cif\n" +
		"G,1,D0,flat,100,\xff\xfe,12345678A\n"
	resp, body := doMultipartCSV(t, client, srv.URL+"/v1/communities/"+communityID.String()+"/units/import", "units.csv", invalidUTF8, auth)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid UTF-8, got %d body=%v", resp.StatusCode, body)
	}

	var tooManyRows strings.Builder
	tooManyRows.WriteString("portal,floor,door,type,coefficient,owner_name,owner_dni_cif\n")
	for i := 0; i < 5001; i++ {
		tooManyRows.WriteString("H," + strconv.Itoa(i) + ",D0,flat,0,Jane Doe,12345678A\n")
	}
	resp, body = doMultipartCSV(t, client, srv.URL+"/v1/communities/"+communityID.String()+"/units/import", "units.csv", tooManyRows.String(), auth)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for a file exceeding the row cap, got %d body=%v", resp.StatusCode, body)
	}
}
