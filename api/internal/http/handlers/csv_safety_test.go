package handlers

import "testing"

// unit-csv-import: Formula-Injection Hardening (both scenarios,
// classified as a table-test unit test per design.md's Testing
// Strategy). csvSafeCell is the single function shared by the template/
// export write path and the import ingestion path: escaping an
// exported value and neutralizing an ingested one are the identical
// transformation (prefix with a leading apostrophe so a spreadsheet
// application renders the cell as literal text instead of evaluating a
// formula).
func TestCSVSafeCell(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"leading equals escaped", "=SUM(A1:A9)", "'=SUM(A1:A9)"},
		{"leading plus escaped", "+1234", "'+1234"},
		{"leading minus escaped", "-1234", "'-1234"},
		{"leading at escaped", "@cmd", "'@cmd"},
		{"safe value untouched", "Jane Doe", "Jane Doe"},
		{"empty value untouched", "", ""},
		{"equals NOT at the start is untouched", "A=B", "A=B"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := csvSafeCell(c.in)
			if got != c.want {
				t.Fatalf("csvSafeCell(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// unit-csv-import: Path-Traversal-Safe Filenames (table-test unit
// test).
func TestSanitizeImportFilename(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"traversal sequence stripped to base name", "../../etc/passwd.csv", "passwd.csv", false},
		{"plain filename untouched", "units.csv", "units.csv", false},
		{"backslash traversal stripped", `..\..\windows\system32\units.csv`, "units.csv", false},
		{"empty filename defaults", "", "import.csv", false},
		{"traversal-only filename rejected", "../../", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := sanitizeImportFilename(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("sanitizeImportFilename(%q) = %q, want an error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("sanitizeImportFilename(%q) unexpected error: %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("sanitizeImportFilename(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
