package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
)

// Префиксы выдаваемых секретов
const (
	apiKeyPrefix        = "rk_"
	webhookSecretPrefix = "whsec_"
	secretBytes         = 24
)

var slugPattern = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)

// HashAPIKey возвращает sha256-хеш API-ключа
func HashAPIKey(key string) []byte {
	sum := sha256.Sum256([]byte(key))
	return sum[:]
}

// GenerateAPIKey создаёт новый API-ключ заведения
func GenerateAPIKey() (string, error) {
	return generateSecret(apiKeyPrefix)
}

// GenerateWebhookSecret создаёт секрет для подписи webhook-ов
func GenerateWebhookSecret() (string, error) {
	return generateSecret(webhookSecretPrefix)
}

func generateSecret(prefix string) (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return prefix + hex.EncodeToString(buf), nil
}

// ValidSlug сообщает, пригоден ли slug как идентификатор заведения
func ValidSlug(slug string) bool {
	return slugPattern.MatchString(slug)
}
