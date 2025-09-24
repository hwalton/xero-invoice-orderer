package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestResolveInvoiceBOM_EmptyDBURL(t *testing.T) {
	t.Parallel()
	roots := []RootItem{{PartID: "X", Name: "X", Quantity: 1}}
	_, msg, err := ResolveInvoiceBOM(context.Background(), "", roots, 5, nil, "", "")
	if err == nil {
		t.Fatalf("expected error for empty db url")
	}
	if !strings.Contains(err.Error(), "db url missing") && msg == "" {
		t.Fatalf("expected db url missing error, got err=%v msg=%q", err, msg)
	}
}

func TestLoadParts_EmptyDBURL(t *testing.T) {
	t.Parallel()
	_, err := LoadParts(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error for empty db url")
	}
	if !strings.Contains(err.Error(), "db url missing") {
		t.Fatalf("expected db url missing error, got %v", err)
	}
}

func TestResolveInvoiceBOM_InvalidDBURL(t *testing.T) {
	t.Parallel()
	// use a clearly invalid/unreachable Postgres URL to provoke a connect error
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	badURL := "postgres://invalid:invalid@localhost:54321/nope?sslmode=disable"
	roots := []RootItem{{PartID: "X", Name: "X", Quantity: 1}}
	_, msg, err := ResolveInvoiceBOM(ctx, badURL, roots, 3, nil, "", "")
	if err == nil {
		t.Fatalf("expected connect error for invalid db url, got nil (msg=%q)", msg)
	}
	// Be permissive: depending on driver behavior and HTTP calls to Xero this function
	// may return a DB connection error or an upstream Xero/http error (e.g. 401).
	if !(strings.Contains(err.Error(), "connect db") ||
		strings.Contains(err.Error(), "dial") ||
		strings.Contains(err.Error(), "get item by code failed") ||
		strings.Contains(err.Error(), "Unauthorized") ||
		strings.Contains(err.Error(), "AuthenticationUnsuccessful")) {
		t.Fatalf("expected connection-related or xero auth error, got: %v", err)
	}
}

func TestLoadParts_InvalidDBURL(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	badURL := "postgres://invalid:invalid@localhost:54321/nope?sslmode=disable"
	_, err := LoadParts(ctx, badURL)
	if err == nil {
		t.Fatalf("expected connect error for invalid db url, got nil")
	}
	if !strings.Contains(err.Error(), "connect db") && !strings.Contains(err.Error(), "dial") {
		t.Fatalf("expected connection-related error, got: %v", err)
	}
}
