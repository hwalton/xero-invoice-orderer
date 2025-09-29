package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hwalton/xero-invoice-orderer/internal/frontend"
	mid "github.com/hwalton/xero-invoice-orderer/internal/middleware"
	"github.com/hwalton/xero-invoice-orderer/internal/service"
	"github.com/hwalton/xero-invoice-orderer/internal/utils"
	"github.com/hwalton/xero-invoice-orderer/pkg/supabasetoolbox"
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
		if b, e := frontend.TemplatesFS.ReadFile("templates/login.html"); e == nil {
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

func (h *Handler) addShoppingListHandler(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil {
		r = mid.EnsureUserIDInContext(r, h.auth)
	}
	ownerID, _ := r.Context().Value(mid.CtxUserID).(string)
	if ownerID == "" {
		http.Error(w, "owner id missing", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	itemIDs := r.Form["item_code"] // now carries ItemID from BOM
	qtys := r.Form["qty"]
	if len(itemIDs) == 0 || len(qtys) == 0 {
		http.Error(w, "invalid input", http.StatusBadRequest)
		return
	}

	sum := make(map[string]int)
	for i := range itemIDs {
		id := strings.TrimSpace(itemIDs[i])
		if id == "" {
			continue
		}
		qStr := "1"
		if i < len(qtys) && qtys[i] != "" {
			qStr = qtys[i]
		}
		q, err := strconv.Atoi(qStr)
		if err != nil || q <= 0 {
			continue
		}
		sum[id] += q
	}
	if len(sum) == 0 {
		http.Error(w, "no valid items", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	added := 0
	for id, q := range sum {
		if err := service.AddShoppingListEntry(ctx, h.dbURL, id, q, false); err != nil {
			http.Error(w, "failed to add to shopping list: "+err.Error(), http.StatusInternalServerError)
			return
		}
		added++
	}

	msg := fmt.Sprintf("%d items added to shopping list", added)
	utils.SetCookie(w, r, "xero_sync_msg", msg, time.Now().Add(5*time.Minute))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
