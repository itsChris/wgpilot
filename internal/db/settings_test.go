package db

import (
	"context"
	"testing"
)

func TestSettings_SetAndGet(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	if err := d.SetSetting(ctx, "setup_complete", "true"); err != nil {
		t.Fatalf("set setting: %v", err)
	}

	val, err := d.GetSetting(ctx, "setup_complete")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if val != "true" {
		t.Errorf("expected %q, got %q", "true", val)
	}
}

func TestSettings_GetMissing(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	val, err := d.GetSetting(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got %q", val)
	}
}

func TestSettings_Upsert(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	if err := d.SetSetting(ctx, "key", "value1"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if err := d.SetSetting(ctx, "key", "value2"); err != nil {
		t.Fatalf("upsert setting: %v", err)
	}

	val, err := d.GetSetting(ctx, "key")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if val != "value2" {
		t.Errorf("expected %q after upsert, got %q", "value2", val)
	}
}

func TestSettings_Delete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	if err := d.SetSetting(ctx, "delete_me", "value"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if err := d.DeleteSetting(ctx, "delete_me"); err != nil {
		t.Fatalf("delete setting: %v", err)
	}

	val, err := d.GetSetting(ctx, "delete_me")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty after delete, got %q", val)
	}
}

func TestSettings_List(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	if err := d.SetSetting(ctx, "a", "1"); err != nil {
		t.Fatalf("set a: %v", err)
	}
	if err := d.SetSetting(ctx, "b", "2"); err != nil {
		t.Fatalf("set b: %v", err)
	}

	settings, err := d.ListSettings(ctx)
	if err != nil {
		t.Fatalf("list settings: %v", err)
	}
	if len(settings) != 2 {
		t.Fatalf("expected 2 settings, got %d", len(settings))
	}
	if settings["a"] != "1" || settings["b"] != "2" {
		t.Errorf("unexpected settings: %v", settings)
	}
}
