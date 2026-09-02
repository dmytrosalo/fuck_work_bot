package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

// signInitData builds a validly signed initData payload for testing.
func signInitData(token string, fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+fields[k])
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(parts, "\n")))

	q := url.Values{}
	for k, v := range fields {
		q.Set(k, v)
	}
	q.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return q.Encode()
}

func validFields() map[string]string {
	return map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      `{"id":460670583,"first_name":"Danya","username":"Dany_ro"}`,
	}
}

func TestVerifyInitDataAcceptsValidSignature(t *testing.T) {
	data := signInitData("test-token", validFields())
	id, firstName, username, err := verifyInitData(data, "test-token", 24*time.Hour)
	if err != nil {
		t.Fatalf("expected valid signature, got %v", err)
	}
	if id != 460670583 {
		t.Errorf("user id = %d, want 460670583", id)
	}
	if firstName != "Danya" {
		t.Errorf("first name = %q, want Danya", firstName)
	}
	if username != "Dany_ro" {
		t.Errorf("username = %q, want Dany_ro", username)
	}
}

func TestVerifyInitDataRejectsTamperedPayload(t *testing.T) {
	data := signInitData("test-token", validFields())
	tampered := strings.Replace(data, "460670583", "111111111", 1)
	if _, _, _, err := verifyInitData(tampered, "test-token", 24*time.Hour); err == nil {
		t.Fatal("expected tampered payload to be rejected")
	}
}

func TestVerifyInitDataRejectsWrongToken(t *testing.T) {
	data := signInitData("test-token", validFields())
	if _, _, _, err := verifyInitData(data, "different-token", 24*time.Hour); err == nil {
		t.Fatal("expected signature from another token to be rejected")
	}
}

func TestVerifyInitDataRejectsStaleAuthDate(t *testing.T) {
	f := validFields()
	f["auth_date"] = fmt.Sprintf("%d", time.Now().Add(-48*time.Hour).Unix())
	data := signInitData("test-token", f)
	if _, _, _, err := verifyInitData(data, "test-token", 24*time.Hour); err == nil {
		t.Fatal("expected stale auth_date to be rejected")
	}
}
