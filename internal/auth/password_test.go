package auth

import "testing"

func TestHashPassword_ProducesValidHash(t *testing.T) {
	hash, err := HashPassword("secret", 4) // cost 4 for fast tests
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if hash == "" {
		t.Error("hash is empty")
	}
	if hash == "secret" {
		t.Error("hash equals plaintext — not hashed")
	}
}

func TestCheckPassword_Match(t *testing.T) {
	hash, _ := HashPassword("correct-horse", 4)
	if err := CheckPassword("correct-horse", hash); err != nil {
		t.Errorf("CheckPassword returned error for matching password: %v", err)
	}
}

func TestCheckPassword_Mismatch(t *testing.T) {
	hash, _ := HashPassword("correct-horse", 4)
	if err := CheckPassword("wrong-password", hash); err == nil {
		t.Error("CheckPassword returned nil for mismatched password")
	}
}
