package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/hwalton/freeride-campervans/internal/utils"
	"github.com/hwalton/freeride-campervans/internal/web"
)

// performLogin handles POST from /perform-login (login.html form).
// It calls Supabase auth, sets auth cookies on success and redirects to "/".
// On failure it re-renders the login template with an error message.
func (h *Handler) performLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	cli := http.DefaultClient
	if h != nil && h.client != nil {
		cli = h.client
	}

	access, refresh, userID, err := AuthenticateWithSupabase(r.Context(), cli, email, password)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := map[string]interface{}{
			"Error":   "Invalid credentials",
			"Code":    0,
			"Message": "",
			"Title":   "Login — Freeride",
		}
		if h != nil && h.templates != nil {
			_ = h.templates.ExecuteTemplate(w, "login.html", data)
			return
		}
		if b, e := web.TemplatesFS.ReadFile("templates/login.html"); e == nil {
			_, _ = io.WriteString(w, string(b))
			return
		}
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	// set cookies
	http.SetCookie(w, &http.Cookie{Name: "access_token", Value: access, Path: "/", HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: refresh, Path: "/", HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: "user_id", Value: userID, Path: "/", HttpOnly: true})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// AuthenticateWithSupabase calls Supabase auth REST to exchange email+password for tokens.
func AuthenticateWithSupabase(ctx context.Context, client *http.Client, email, password string) (string, string, string, error) {
	supabaseURL := utils.GetEnv("NEXT_PUBLIC_SUPABASE_URL", "")
	apiKey := utils.GetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY", "")

	payload := map[string]string{
		"email":    email,
		"password": password,
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, supabaseURL+"/auth/v1/token?grant_type=password", bytes.NewReader(b))
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Content-Type", "application/json")

	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", "", &httpError{Status: resp.StatusCode, Body: string(body)}
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", "", err
	}
	return result.AccessToken, result.RefreshToken, result.User.ID, nil
}

// logoutHandler clears auth cookies and redirects to /login
func (h *Handler) logoutHandler(w http.ResponseWriter, r *http.Request) {
	// clear cookies by expiring them
	names := []string{"access_token", "refresh_token", "user_id", "current_card_id", "review_ahead_days", "max_new_cards_per_day"}
	for _, n := range names {
		http.SetCookie(w, &http.Cookie{
			Name:     n,
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
		})
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// simple error type to bubble up API body.
type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string {
	return "supabase auth error"
}
