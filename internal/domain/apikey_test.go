package domain_test

import (
	"bytes"
	"testing"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

func TestHashAPIKeyDeterministic(t *testing.T) {
	t.Parallel()

	first := domain.HashAPIKey("demo-restaurant-key")
	second := domain.HashAPIKey("demo-restaurant-key")
	other := domain.HashAPIKey("another-key")

	if !bytes.Equal(first, second) {
		t.Error("хеш одного и того же ключа должен совпадать")
	}
	if bytes.Equal(first, other) {
		t.Error("хеши разных ключей не должны совпадать")
	}
	if len(first) != 32 {
		t.Errorf("длина sha256-хеша = %d, want 32", len(first))
	}
}

func TestGenerateSecretsAreUnique(t *testing.T) {
	t.Parallel()

	firstKey, err := domain.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	secondKey, err := domain.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if firstKey == secondKey {
		t.Error("сгенерированные ключи не должны совпадать")
	}

	secret, err := domain.GenerateWebhookSecret()
	if err != nil {
		t.Fatalf("GenerateWebhookSecret: %v", err)
	}
	if secret == "" {
		t.Error("секрет не должен быть пустым")
	}
}

func TestValidSlug(t *testing.T) {
	t.Parallel()

	valid := []string{"demo-bistro", "abc", "shop-123"}
	invalid := []string{"ab", "Demo-Bistro", "с-кириллицей", "with_underscore", "with space", ""}

	for _, slug := range valid {
		if !domain.ValidSlug(slug) {
			t.Errorf("ValidSlug(%q) = false, want true", slug)
		}
	}
	for _, slug := range invalid {
		if domain.ValidSlug(slug) {
			t.Errorf("ValidSlug(%q) = true, want false", slug)
		}
	}
}
