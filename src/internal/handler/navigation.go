package handler

import (
	"context"
	"html/template"
	"io"
	"net/http"
	"time"

	mid "github.com/hwalton/freeride-campervans/internal/middleware"
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
	userID, _ := r.Context().Value(ctxUserID).(string)
	var ok bool

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// if no userID in context, try to extract from cookie/header using middleware helper
	if userID == "" && h.auth != nil {
		if userID, ok = mid.GetUserIDFromRequest(r, h.auth); ok && userID != "" {
			// store into request context so downstream/template sees it
			r = r.WithContext(context.WithValue(r.Context(), ctxUserID, userID))
			// set user_id cookie for client (30 days)
			utils.SetCookie(w, r, "user_id", userID, time.Now().Add(30*24*time.Hour))
		}
	}

	// load connections for the user so template can render them server-side
	var conns []service.XeroConnection
	if userID != "" && h.dbURL != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if c, err := service.GetConnectionsForOwner(ctx, h.dbURL, userID); err == nil {
			conns = c
		}
	}

	// For simplified UI: only one tenant per owner. If any row exists use the first.
	var hasXeroConn bool
	var tenantID string
	var createdAt interface{}
	if len(conns) > 0 {
		hasXeroConn = true
		tenantID = conns[0].TenantID
		createdAt = conns[0].CreatedAt
	}

	// Build sync message from short-lived cookie (set after successful sync)
	var xeroSyncMsg string
	if c, err := r.Cookie("xero_sync_msg"); err == nil && c.Value != "" {
		xeroSyncMsg = c.Value
		// clear the cookie so the message is shown only once
		utils.ClearCookie(w, r, "xero_sync_msg")
	}

	data := map[string]interface{}{
		"Title":             "Home",
		"UserID":            userID,
		"HasXeroConnection": hasXeroConn,
		"XeroTenantID":      tenantID,
		"XeroCreatedAt":     createdAt,
		"XeroSyncMessage":   xeroSyncMsg,
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
		if err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		if err := t.Execute(w, data); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		return
	}

	http.Error(w, "template error", http.StatusInternalServerError)
}
