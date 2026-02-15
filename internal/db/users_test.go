package db

import (
	"context"
	"testing"
)

func TestCreateUser(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	id, err := d.CreateUser(ctx, &User{
		Username:     "admin",
		PasswordHash: "$2a$12$fakehashvalue",
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestGetUserByUsername(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	_, err := d.CreateUser(ctx, &User{
		Username:     "admin",
		PasswordHash: "$2a$12$fakehash",
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	user, err := d.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.Username != "admin" {
		t.Errorf("expected username=admin, got %q", user.Username)
	}
	if user.Role != "admin" {
		t.Errorf("expected role=admin, got %q", user.Role)
	}
}

func TestGetUserByUsername_NotFound(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	user, err := d.GetUserByUsername(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if user != nil {
		t.Errorf("expected nil for nonexistent user, got %+v", user)
	}
}

func TestGetUserByID(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	id, err := d.CreateUser(ctx, &User{
		Username:     "testuser",
		PasswordHash: "$2a$12$fakehash",
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	user, err := d.GetUserByID(ctx, id)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID != id {
		t.Errorf("expected id=%d, got %d", id, user.ID)
	}
	if user.Username != "testuser" {
		t.Errorf("expected username=testuser, got %q", user.Username)
	}
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	_, err := d.CreateUser(ctx, &User{
		Username:     "admin",
		PasswordHash: "$2a$12$fakehash",
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}

	_, err = d.CreateUser(ctx, &User{
		Username:     "admin",
		PasswordHash: "$2a$12$anotherhash",
		Role:         "admin",
	})
	if err == nil {
		t.Error("expected error for duplicate username")
	}
}
