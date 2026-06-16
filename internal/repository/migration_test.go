package repository

import (
	"testing"
)

func TestAutoMigrateCreatesRequiredPhase5TablesIdempotently(t *testing.T) {
	dbPath := t.TempDir() + "/migration.db"
	db, err := NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("first migration: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("first db handle: %v", err)
	}
	_ = sqlDB.Close()

	db, err = NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("second migration: %v", err)
	}
	sqlDB, err = db.DB()
	if err != nil {
		t.Fatalf("second db handle: %v", err)
	}
	defer sqlDB.Close()

	expected := []string{
		"agents",
		"agent_credentials",
		"agent_commands",
		"vpn_nodes",
		"vpn_credentials",
		"proxies",
		"customers",
		"plans",
		"subscriptions",
		"invoices",
		"abuse_events",
		"routing_policies",
		"proxy_pools",
		"usage_metrics",
	}
	for _, table := range expected {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("expected migrated table %q", table)
		}
	}
}
