package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns the bcrypt hash of the password using the given cost.
func HashPassword(password string, cost int) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	return string(hash), err
}

// CheckPassword compares a plaintext password against a bcrypt hash.
// Returns nil on match, non-nil on mismatch.
func CheckPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
