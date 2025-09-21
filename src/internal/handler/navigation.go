package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"time"

	"github.com/hwalton/freeride-campervans/internal/frontend"
	mid "github.com/hwalton/freeride-campervans/internal/middleware"
	"github.com/hwalton/freeride-campervans/internal/service"
	"github.com/hwalton/freeride-campervans/internal/utils"
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
	if b, err := frontend.TemplatesFS.ReadFile("templates/login.html"); err == nil {
		_, _ = w.Write(b)
		return
	}
	http.Error(w, "template error", http.StatusInternalServerError)
}

// logoutHandler clears auth cookies and redirects to /login
func (h *Handler) logoutHandler(w http.ResponseWriter, r *http.Request) {
	names := []string{"access_token", "refresh_token", "current_card_id", "review_ahead_days", "max_new_cards_per_day"}
	for _, n := range names {
		utils.ClearCookie(w, r, n)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) homeHandler(w http.ResponseWriter, r *http.Request) {
	// ensure middleware helper populates ctxUserID when possible
	if h.auth != nil {
		r = mid.EnsureUserIDInContext(r, h.auth)
	}
	userID, _ := r.Context().Value(mid.CtxUserID).(string)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

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

	// read invoice views from cookies
	var invoiceNumber string
	if c, err := r.Cookie("xero_invoice_number"); err == nil && c.Value != "" {
		invoiceNumber = c.Value
		utils.ClearCookie(w, r, "xero_invoice_number")
	}

	decode := func(name string, dst any) {
		if c, err := r.Cookie(name); err == nil && c.Value != "" {
			if decoded, derr := base64.StdEncoding.DecodeString(c.Value); derr == nil {
				_ = json.Unmarshal(decoded, dst)
			}
			utils.ClearCookie(w, r, name)
		}
	}

	var perAssyBOM []service.BOMNode
	type leafTotal struct {
		PartID   string  `json:"part_id"`
		Name     string  `json:"name"`
		Quantity float64 `json:"quantity"`
	}
	var leafTotals []leafTotal

	decode("xero_perassy_bom", &perAssyBOM)
	decode("xero_leaf_totals", &leafTotals)

	data := map[string]interface{}{
		"Title":             "Home",
		"UserID":            userID,
		"HasXeroConnection": hasXeroConn,
		"XeroTenantID":      tenantID,
		"XeroCreatedAt":     createdAt,
		"XeroSyncMessage":   xeroSyncMsg,

		// new view data
		"PerAssemblyBOM": perAssyBOM,
		"LeafTotals":     leafTotals,
		"InvoiceNumber":  invoiceNumber,
	}

	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "home.html", data); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
		return
	}

	// fallback: render embedded template file if parsed templates not provided
	if b, err := frontend.TemplatesFS.ReadFile("templates/home.html"); err == nil {
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
