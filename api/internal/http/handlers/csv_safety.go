package handlers

import (
	"errors"
	"path/filepath"
	"strings"
)

// csvFormulaPrefixes are the cell-leading bytes a spreadsheet
// application may evaluate as a formula (unit-csv-import:
// Formula-Injection Hardening, §5.2/PRD §10.1 gate item 7).
const csvFormulaPrefixes = "=+-@"

// csvSafeCell is the SINGLE function shared by the template/export
// write path and the import ingestion path (task 5.8's "cell-prefix
// escaping shared by template/export and import ingestion"): escaping
// an exported value and neutralizing an ingested one before storage are
// the identical transformation. Prefixing with a leading apostrophe is
// the standard CSV-formula-injection mitigation: every major spreadsheet
// application renders an apostrophe-led cell as literal text rather
// than evaluating it.
func csvSafeCell(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsRune(csvFormulaPrefixes, rune(s[0])) {
		return "'" + s
	}
	return s
}

// errUnsafeFilename is returned when a filename sanitizes to nothing
// usable (e.g. a traversal-only path).
var errUnsafeFilename = errors.New("unit-csv-import: filename is empty or traversal-only after sanitization")

// sanitizeImportFilename implements unit-csv-import: Path-Traversal-
// Safe Filenames -- it is called before ANY storage-key derivation, and
// the raw client-supplied name is NEVER used to write a file (the
// upload body is streamed straight into CSV parsing; nothing is ever
// written under a client-supplied name at all, per design D-7). Both
// forward- and backslash-separated traversal sequences are normalized
// away by taking the final path element only.
func sanitizeImportFilename(name string) (string, error) {
	if name == "" {
		return "import.csv", nil
	}
	normalized := strings.ReplaceAll(name, `\`, "/")
	base := filepath.Base(normalized)
	if base == "." || base == ".." || base == "/" || base == "" {
		return "", errUnsafeFilename
	}
	return base, nil
}
