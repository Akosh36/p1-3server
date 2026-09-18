package auth

import "golang.org/x/crypto/bcrypt"

// MinPasswordLength is enforced everywhere an admin password is set —
// panel-created accounts (internal/httpapi) and the bootstrap super_admin
// (cmd/api) alike — so there's exactly one place this policy is decided.
// Length over forced complexity rules matches current password-policy
// guidance (e.g. NIST SP 800-63B) better than mandating symbols/digits.
const MinPasswordLength = 8

// HashPassword hashes a plaintext admin password for storage.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether plain matches the stored bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
