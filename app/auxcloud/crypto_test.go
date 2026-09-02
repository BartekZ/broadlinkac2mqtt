package auxcloud

import (
	"testing"
	"time"
)

func TestLoginCryptoRoundTrip(t *testing.T) {
	payload := `{"email":"user@example.com","password":"deadbeef","companyid":"company","lid":"license"}`
	ts := int64(1700000000)

	encrypted, err := encryptLoginBody(payload, ts)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(encrypted)%16 != 0 {
		t.Fatalf("ciphertext length %d is not block aligned", len(encrypted))
	}

	decrypted, err := decryptAESCBCZeroPad(timestampKey(ts), aesIV, encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decrypted) != payload {
		t.Fatalf("decrypted = %q, want %q", decrypted, payload)
	}
}

func TestHashPasswordAndToken(t *testing.T) {
	if got := hashPassword("secret"); len(got) != 40 {
		t.Fatalf("sha1 hex length = %d, want 40", len(got))
	}
	if got := loginToken(`{"email":"a"}`); len(got) != 32 {
		t.Fatalf("md5 hex length = %d, want 32", len(got))
	}
}

func TestCompactJSONDoesNotEscapeHTML(t *testing.T) {
	payload := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{Email: "a@b.com", Password: "a&b<c>"}

	got, err := compactJSON(payload)
	if err != nil {
		t.Fatalf("compactJSON: %v", err)
	}
	if string(got) != `{"email":"a@b.com","password":"a&b<c>"}` {
		t.Fatalf("compactJSON = %s", got)
	}
}

func TestEncryptAddsFullBlockWhenAligned(t *testing.T) {
	data := make([]byte, 16)
	out, err := encryptAESCBCZeroPad(timestampKey(1), aesIV, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 32 {
		t.Fatalf("padded ciphertext length = %d, want 32", len(out))
	}
}

func TestMinRequestGapConstant(t *testing.T) {
	if minRequestGap != time.Second {
		t.Fatalf("minRequestGap = %s, want 1s", minRequestGap)
	}
}
