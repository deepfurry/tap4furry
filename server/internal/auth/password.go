package auth

import (
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/gofurry/easyhash"
)

var errPasswordOperation = errors.New("password operation failed")

func NormalizeEmail(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	address, err := mail.ParseAddress(value)
	// ParseAddress accepts display names, comments and angle brackets; plain
	// addresses must survive parsing unchanged. Do not fold dots or plus tags.
	if err != nil || !utf8.ValidString(value) || len(value) > 254 || address.Name != "" || address.Address != value {
		return "", identity.ErrValidation
	}
	return strings.ToLower(value), nil
}

func ValidatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || n < 15 || n > 128 {
		return identity.ErrValidation
	}
	return nil
}

func passwordPolicy() easyhash.Policy {
	policy := easyhash.DefaultPolicy()
	policy.PreferredAlgorithm = easyhash.AlgorithmArgon2id
	policy.Argon2id = easyhash.DefaultArgon2()
	return policy
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := easyhash.Hash(password, easyhash.WithArgon2id())
	if err != nil {
		return "", errPasswordOperation
	}
	return hash, nil
}

func verifyPassword(password, stored string) (bool, string, bool, error) {
	// Enforce the maximum even on legacy credentials before invoking a KDF.
	if err := ValidatePassword(password); err != nil {
		return false, "", false, err
	}
	ok, replacement, upgraded, err := easyhash.VerifyAndUpgrade(password, stored, passwordPolicy())
	if err != nil {
		return false, "", false, errPasswordOperation
	}
	return ok, replacement, upgraded, nil
}
