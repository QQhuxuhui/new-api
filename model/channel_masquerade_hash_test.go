package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestChannel_GetOrCreateMasqueradeHash_PersistsAndStable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	DB = db

	if err := DB.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	c1 := &Channel{Key: "k1", Name: "c1"}
	if err := DB.Create(c1).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	h1 := c1.GetOrCreateMasqueradeHash()
	if len(h1) != 64 {
		t.Fatalf("expected 64-char hash, got %q", h1)
	}

	reloaded, err := GetChannelById(c1.Id, true)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	h2 := reloaded.GetOrCreateMasqueradeHash()
	if h2 != h1 {
		t.Fatalf("expected stable hash, got %q vs %q", h1, h2)
	}
}

func TestChannel_GetOrCreateMasqueradeHash_UniqueAcrossChannels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	DB = db

	if err := DB.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	a := &Channel{Key: "ka", Name: "a"}
	b := &Channel{Key: "kb", Name: "b"}
	if err := DB.Create(a).Error; err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := DB.Create(b).Error; err != nil {
		t.Fatalf("create b: %v", err)
	}

	ha := a.GetOrCreateMasqueradeHash()
	hb := b.GetOrCreateMasqueradeHash()
	if ha == "" || hb == "" {
		t.Fatalf("expected non-empty hashes")
	}
	if ha == hb {
		t.Fatalf("expected different hashes, got %q", ha)
	}
}
