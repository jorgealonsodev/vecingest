package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"hash"
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// nullMarker is a single byte distinct from any possible length-prefixed
// field (whose shortest possible encoding is a 4-byte zero-length
// prefix), so a SQL NULL never hashes the same as an empty string or
// empty JSON object (db-access-control: audit_log Hash Chain).
const nullMarker = 0xFF

// computeHash implements D-O's exact byte layout:
//
//	hash = SHA-256(prev ‖ F(id) ‖ F(created_at) ‖ F(user_id) ‖ F(community_id) ‖
//	               F(action) ‖ F(entity) ‖ F(entity_id) ‖ F(before) ‖ F(after) ‖
//	               F(ip) ‖ F(request_id))
//
// where F(x) is a big-endian uint32 length prefix followed by x's
// canonical encoding, and F(NULL) is the single nullMarker byte. The
// length prefixes exist so that no two different field splits can
// produce the same byte stream -- without them, moving a character from
// action to entity would be undetectable.
func computeHash(prev []byte, id uuid.UUID, createdAt time.Time, e Entry) []byte {
	h := sha256.New()
	h.Write(prev)

	writeField(h, []byte(id.String()))
	writeField(h, []byte(createdAt.UTC().Format(time.RFC3339Nano)))
	writeUUIDPtr(h, e.UserID)
	writeUUIDPtr(h, e.CommunityID)
	writeField(h, []byte(e.Action))
	writeField(h, []byte(e.Entity))
	writeUUIDPtr(h, e.EntityID)
	writeJSONField(h, e.Before)
	writeJSONField(h, e.After)
	writeIPField(h, e.IP)
	writeUUIDPtr(h, e.RequestID)

	return h.Sum(nil)
}

// writeField writes F(b): a 4-byte big-endian length prefix, then b.
func writeField(h hash.Hash, b []byte) {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(b))) //nolint:gosec // audit_log field values are bounded well under 4 GiB; this is a length prefix, not a security-sensitive count
	h.Write(lenBuf[:])
	h.Write(b)
}

// writeNull writes F(NULL): the single nullMarker byte.
func writeNull(h hash.Hash) {
	h.Write([]byte{nullMarker})
}

func writeUUIDPtr(h hash.Hash, u *uuid.UUID) {
	if u == nil {
		writeNull(h)
		return
	}
	writeField(h, []byte(u.String()))
}

func writeIPField(h hash.Hash, ip *netip.Addr) {
	if ip == nil || !ip.IsValid() {
		writeNull(h)
		return
	}
	writeField(h, []byte(ip.String()))
}

// canonicalJSON re-serializes raw with sorted object keys and no
// insignificant whitespace. encoding/json already sorts map keys when
// marshaling, so decoding into a generic interface{} and re-marshaling
// is sufficient and needs no third-party dependency.
func canonicalJSON(raw []byte) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte{}, nil
	}
	// Decode-then-re-encode for canonical byte ordering only; the generic
	// value is never type-asserted for an authorization decision.
	var v interface{} // nosemgrep: go.lang.security.deserialization.unsafe-deserialization-interface.go-unsafe-deserialization-interface
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func writeJSONField(h hash.Hash, raw []byte) {
	if raw == nil {
		writeNull(h)
		return
	}
	canon, err := canonicalJSON(raw)
	if err != nil {
		// raw is not valid JSON: hash it verbatim rather than silently
		// dropping the tamper-evidence property. This should never
		// happen for well-formed callers (before/after are always
		// produced by json.Marshal upstream).
		writeField(h, raw)
		return
	}
	writeField(h, canon)
}
