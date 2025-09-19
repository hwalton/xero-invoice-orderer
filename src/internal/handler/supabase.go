package handler

import (
	"log"
	"net/http"

	"github.com/hwalton/freeride-campervans/internal/utils"
	"github.com/hwalton/freeride-campervans/internal/web"
	"github.com/hwalton/freeride-campervans/pkg/supabasetoolbox"
)

// supabaseConnect handles POST from the login form, authenticates with Supabase,
// sets session cookies on success and redirects to "/".
func (h *Handler) supabaseConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	log.Printf("supabaseConnect: attempt for email=%s", email)

	client := h.client
	if client == nil {
		client = http.DefaultClient
	}

	supabaseURL := utils.GetEnv("NEXT_PUBLIC_SUPABASE_URL", "")
	apiKey := utils.GetEnv("NEXT_PUBLIC_SUPABASE_ANON_KEY", "")

	access, refresh, userID, err := supabasetoolbox.AuthenticateWithSupabase(r.Context(), client, email, password, supabaseURL, apiKey)
	if err != nil {
		log.Printf("supabaseConnect: auth failed: %v", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := map[string]interface{}{
			"Title":   "Login — Freeride",
			"Error":   "Invalid credentials",
			"Code":    0,
			"Message": "",
		}
		if h.templates != nil {
			_ = h.templates.ExecuteTemplate(w, "login.html", data)
			return
		}
		if b, e := web.TemplatesFS.ReadFile("templates/login.html"); e == nil {
			_, _ = w.Write(b)
			return
		}
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	// set auth cookies
	http.SetCookie(w, &http.Cookie{Name: "access_token", Value: access, Path: "/", HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: refresh, Path: "/", HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: "user_id", Value: userID, Path: "/", HttpOnly: true})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
