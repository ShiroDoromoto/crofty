// Package id generates ULIDs — lexicographically sortable identifiers used for
// stable post ids (spec 06 §3-2) and workspace ids. A ULID is a 48-bit
// millisecond timestamp plus 80 bits of randomness, encoded as 26 Crockford
// base32 characters. The alphabet matches the validator's ULID rule, so any id
// crofty generates also passes `crofty validate`.
package id

import (
	"crypto/rand"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewULID returns a new ULID string, or an error if the system RNG fails.
func NewULID() (string, error) {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	if _, err := rand.Read(b[6:]); err != nil {
		return "", err
	}
	return encode(b), nil
}

// encode renders the canonical 26-char ULID string from a 16-byte value.
func encode(b [16]byte) string {
	e := crockford
	d := make([]byte, 26)
	d[0] = e[(b[0]&224)>>5]
	d[1] = e[b[0]&31]
	d[2] = e[(b[1]&248)>>3]
	d[3] = e[((b[1]&7)<<2)|((b[2]&192)>>6)]
	d[4] = e[(b[2]&62)>>1]
	d[5] = e[((b[2]&1)<<4)|((b[3]&240)>>4)]
	d[6] = e[((b[3]&15)<<1)|((b[4]&128)>>7)]
	d[7] = e[(b[4]&124)>>2]
	d[8] = e[((b[4]&3)<<3)|((b[5]&224)>>5)]
	d[9] = e[b[5]&31]
	d[10] = e[(b[6]&248)>>3]
	d[11] = e[((b[6]&7)<<2)|((b[7]&192)>>6)]
	d[12] = e[(b[7]&62)>>1]
	d[13] = e[((b[7]&1)<<4)|((b[8]&240)>>4)]
	d[14] = e[((b[8]&15)<<1)|((b[9]&128)>>7)]
	d[15] = e[(b[9]&124)>>2]
	d[16] = e[((b[9]&3)<<3)|((b[10]&224)>>5)]
	d[17] = e[b[10]&31]
	d[18] = e[(b[11]&248)>>3]
	d[19] = e[((b[11]&7)<<2)|((b[12]&192)>>6)]
	d[20] = e[(b[12]&62)>>1]
	d[21] = e[((b[12]&1)<<4)|((b[13]&240)>>4)]
	d[22] = e[((b[13]&15)<<1)|((b[14]&128)>>7)]
	d[23] = e[(b[14]&124)>>2]
	d[24] = e[((b[14]&3)<<3)|((b[15]&224)>>5)]
	d[25] = e[b[15]&31]
	return string(d)
}
