package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyGitHubSignature(t *testing.T) {
	secret := "mysecret"
	body := []byte(`{"action":"push"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyGitHubSignature(secret, body, validSig) {
		t.Error("expected valid signature to pass")
	}
	if verifyGitHubSignature(secret, body, "sha256=badhex") {
		t.Error("expected invalid hex to fail")
	}
	if verifyGitHubSignature(secret, body, "invalid-prefix") {
		t.Error("expected missing prefix to fail")
	}
	if verifyGitHubSignature(secret, body, "") {
		t.Error("expected empty sig to fail")
	}
	if verifyGitHubSignature("wrongsecret", body, validSig) {
		t.Error("expected wrong secret to fail")
	}
}
