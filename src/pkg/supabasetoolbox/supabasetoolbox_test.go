package supabasetoolbox

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGenerateSignedURL_Success verifies we build the final URL from the signedURL response.
func TestGenerateSignedURL_Success(t *testing.T) {
	// mock Supabase storage sign endpoint
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/storage/v1/object/sign/flashcard-assets/myfile.jpg" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body struct {
			ExpiresIn int `json:"expiresIn"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		resp := map[string]string{"signedURL": "/o/abc?token=xyz"}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	out, err := GenerateSignedURL(ts.URL, "anon-key", "access-token", "myfile.jpg")
	if err != nil {
		t.Fatalf("GenerateSignedURL error: %v", err)
	}
	expected := ts.URL + "/storage/v1" + "/o/abc?token=xyz"
	if out != expected {
		t.Fatalf("unexpected url: got %q want %q", out, expected)
	}
}

// TestGenerateSignedURL_Non200 ensures non-200 responses return an error.
func TestGenerateSignedURL_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()

	if _, err := GenerateSignedURL(ts.URL, "anon", "access", "x"); err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

// TestAuthenticateWithSupabase covers success and non-200 failure.
func TestAuthenticateWithSupabase(t *testing.T) {
	tsOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ensure query grant_type=password present
		if r.URL.RawQuery != "grant_type=password" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		var p map[string]string
		_ = json.NewDecoder(r.Body).Decode(&p)
		if p["email"] != "a" || p["password"] != "b" {
			http.Error(w, "bad creds", http.StatusUnauthorized)
			return
		}
		resp := map[string]interface{}{
			"access_token":  "at",
			"refresh_token": "rt",
			"user": map[string]string{
				"id": "uid-1",
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer tsOK.Close()

	cli := tsOK.Client()
	at, rt, uid, err := AuthenticateWithSupabase(context.Background(), cli, "a", "b", tsOK.URL, "key")
	if err != nil {
		t.Fatalf("AuthenticateWithSupabase failed: %v", err)
	}
	if at != "at" || rt != "rt" || uid != "uid-1" {
		t.Fatalf("unexpected tokens: %s %s %s", at, rt, uid)
	}

	// non-200 case
	tsBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer tsBad.Close()

	_, _, _, err = AuthenticateWithSupabase(context.Background(), tsBad.Client(), "a", "b", tsBad.URL, "key")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

// TestRefreshAccessToken exercises missing cookie, success and non-200 cases.
func TestRefreshAccessToken(t *testing.T) {
	// success server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]string
		_ = json.NewDecoder(r.Body).Decode(&p)
		_ = p
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(loginResponse{
			AccessToken:  "new-at",
			RefreshToken: "new-rt",
		})
	}))
	defer ts.Close()

	// preserve and restore default client
	origClient := http.DefaultClient
	http.DefaultClient = ts.Client()
	defer func() { http.DefaultClient = origClient }()

	// missing cookie
	reqNoCookie := &http.Request{Header: http.Header{}}
	if _, _, err := RefreshAccessToken(reqNoCookie, ts.URL, "k"); err == nil {
		t.Fatal("expected error when refresh cookie missing")
	}

	// with cookie
	req, _ := http.NewRequest("POST", "/", bytes.NewReader(nil))
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "old"})
	at, rt, err := RefreshAccessToken(req, ts.URL, "k")
	if err != nil {
		t.Fatalf("RefreshAccessToken failed: %v", err)
	}
	if at != "new-at" || rt != "new-rt" {
		t.Fatalf("unexpected tokens: %s %s", at, rt)
	}

	// server returns non-200
	tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer tsErr.Close()
	http.DefaultClient = tsErr.Client()
	req2, _ := http.NewRequest("POST", "/", nil)
	req2.AddCookie(&http.Cookie{Name: "refresh_token", Value: "old"})
	if _, _, err := RefreshAccessToken(req2, tsErr.URL, "k"); err == nil {
		t.Fatal("expected error for non-200 refresh response")
	}
}
