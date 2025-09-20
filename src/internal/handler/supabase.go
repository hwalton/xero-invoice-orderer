package handler

import (
	"context"
	"log"
	"net/http"
	"time"

	mid "github.com/hwalton/freeride-campervans/internal/middleware"
	"github.com/hwalton/freeride-campervans/internal/utils"
	"github.com/hwalton/freeride-campervans/internal/web"
	"github.com/hwalton/freeride-campervans/pkg/supabasetoolbox"
)

// supabaseConnect handles POST from the login form, authenticates with Supabase,
// sets session cookies on success and redirects to "/".
func (h *Handler) supabaseConnectHandler(w http.ResponseWriter, r *http.Request) {
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

	// set auth cookies with Secure, SameSite, HttpOnly and expiry
	exp := time.Now().Add(time.Duration(3600) * time.Second) // align with access token expiry
	utils.SetCookie(w, r, "access_token", access, exp)
	utils.SetCookie(w, r, "refresh_token", refresh, time.Now().Add(30*24*time.Hour))

	// keep user id in request context instead of a cookie (for this request)
	r = r.WithContext(context.WithValue(r.Context(), mid.CtxUserID, userID))

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
