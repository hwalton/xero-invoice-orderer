package handler

import (
	"context"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/hwalton/freeride-campervans/internal/service"
	"github.com/hwalton/freeride-campervans/internal/utils"
	"github.com/hwalton/freeride-campervans/internal/web"
)

// login serves the login page via templates
func (h *Handler) loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// render using parsed templates; pass any dynamic data here
	data := map[string]interface{}{
		"Title": "Login — Freeride",
	}
	if h.templates != nil {
		_ = h.templates.ExecuteTemplate(w, "login.html", data)
		return
	}
	// fallback: embedded raw file (if templates not provided)
	if b, err := web.TemplatesFS.ReadFile("templates/login.html"); err == nil {
		_, _ = io.WriteString(w, string(b))
		return
	}
	http.Error(w, "template error", http.StatusInternalServerError)
}

// logoutHandler clears auth cookies and redirects to /login
func (h *Handler) logoutHandler(w http.ResponseWriter, r *http.Request) {
	names := []string{"access_token", "refresh_token", "user_id", "current_card_id", "review_ahead_days", "max_new_cards_per_day"}
	for _, n := range names {
		utils.ClearCookie(w, r, n)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) homeHandler(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(ctxUserID).(string)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// load connections for the user so template can render them server-side
	var conns []service.XeroConnection
	if uid != "" && h.dbURL != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		c, err := service.GetConnectionsForOwner(ctx, h.dbURL, uid)
		if err != nil {
			log.Printf("homeHandler: failed to load xero connections for user %s: %v", uid, err)
			// proceed with empty list so template still renders
		} else {
			conns = c
		}
	}

	data := map[string]interface{}{
		"Title":       "Home",
		"UserID":      uid,
		"Connections": conns,
	}

	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "home.html", data); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
		return
	}

	// fallback: render embedded template file if parsed templates not provided
	if b, err := web.TemplatesFS.ReadFile("templates/home.html"); err == nil {
		t, err := template.New("home").Parse(string(b))
		if err == nil {
			if err := t.Execute(w, data); err == nil {
				return
			}
		}
	}

	// final fallback removed — return an error if no template is available
	http.Error(w, "template error", http.StatusInternalServerError)
}
