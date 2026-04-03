package services_test

import (
	"testing"

	"blackbox/server/internal/models"
	"blackbox/server/internal/services"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestNormalizeService(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.AutoMigrate(&models.ServiceAlias{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: "traefik-proxy"}).Error; err != nil {
		t.Fatalf("seed alias: %v", err)
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "alias match", in: "traefik-proxy", want: "traefik"},
		{name: "unchanged", in: "postgres", want: "postgres"},
		{name: "trimmed empty", in: "   ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := services.NormalizeService(database, tt.in)
			if err != nil {
				t.Fatalf("NormalizeService(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeService(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}

	if err := database.Exec("DROP TABLE service_aliases").Error; err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if _, err := services.NormalizeService(database, "traefik-proxy"); err == nil {
		t.Fatal("expected NormalizeService to return an error when alias lookup fails")
	}
}
