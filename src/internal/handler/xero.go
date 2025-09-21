package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	mid "github.com/hwalton/freeride-campervans/internal/middleware"
	"github.com/hwalton/freeride-campervans/internal/service"
	"github.com/hwalton/freeride-campervans/internal/utils"
	"github.com/hwalton/freeride-campervans/pkg/xero"
)

// generateState returns a secure random hex string of length 2*n (n bytes).
func generateState(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// xeroConnect redirects to Xero auth URL
func (h *Handler) xeroConnectHandler(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil {
		r = mid.EnsureUserIDInContext(r, h.auth)
	}
	ownerID, _ := r.Context().Value(mid.CtxUserID).(string)
	if ownerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	clientID := utils.GetEnv("XERO_CLIENT_ID", "")
	redirect := utils.GetEnv("REDIRECT", "http://localhost:8080/xero/callback")

	// generate secure state and persist mapping -> ownerID (use DB-backed store with TTL)
	state, err := generateState(16)
	if err != nil {
		http.Error(w, "failed to generate state", http.StatusInternalServerError)
		return
	}
	ttl := 300 // seconds (5 minutes) — adjust as needed
	if err := service.CreateOAuthState(r.Context(), h.dbURL, state, ownerID, ttl); err != nil {
		// Log the underlying error for debugging (do not expose internal details to clients).
		// Use server logs to inspect permission/constraint/connection issues.
		log.Printf("xeroConnect: CreateOAuthState failed: %v", err)
		http.Error(w, "failed to persist state", http.StatusInternalServerError)
		return
	}

	authURL := xero.BuildAuthURL(clientID, redirect, state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// xeroCallback exchanges code for tokens and persists connection(s)
func (h *Handler) xeroCallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" {
		http.Error(w, "code missing", http.StatusBadRequest)
		return
	}
	if state == "" {
		http.Error(w, "state missing", http.StatusBadRequest)
		return
	}

	// lookup ownerID by state (one-time use) via DB
	ownerID, found, err := service.ConsumeOAuthState(ctx, h.dbURL, state)
	if err != nil {
		http.Error(w, "state lookup failed", http.StatusInternalServerError)
		return
	}
	if !found || ownerID == "" {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	clientID := os.Getenv("XERO_CLIENT_ID")
	clientSecret := os.Getenv("XERO_CLIENT_SECRET")
	redirect := utils.GetEnv("REDIRECT", "http://localhost:8080/xero/callback")

	tr, err := xero.ExchangeCodeForToken(ctx, h.client, clientID, clientSecret, code, redirect)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	conns, err := xero.GetConnections(ctx, h.client, tr.AccessToken)
	if err != nil {
		http.Error(w, "failed to get connections: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// persist connections for the ownerID
	for _, c := range conns {
		expires := tr.ExpiresIn
		if expires == 0 {
			expires = 3600
		}
		if err := service.UpsertConnection(ctx, h.dbURL, ownerID, c.TenantID, tr.AccessToken, tr.RefreshToken, expires); err != nil {
			http.Error(w, "persist connection failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// xeroConnections lists stored Xero connections for the current user.
func (h *Handler) xeroConnectionsHandler(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil {
		r = mid.EnsureUserIDInContext(r, h.auth)
	}
	ownerID, _ := r.Context().Value(mid.CtxUserID).(string)
	if ownerID == "" {
		http.Error(w, "owner id missing", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	conns, err := service.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
	if err != nil {
		http.Error(w, "failed to load connections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(conns)
}

// getInvoiceHandler POSTs a form with invoice_id, queries Xero for that invoice's line item codes,
// resolves BOM, then prepares:
//   - PerAssemblyBOM: tree showing "Qty required (for each Assy)"
//   - LeafTotals: flat list aggregating total required for purchasable leaves
//
// These are stored in cookies for home page rendering.
func (h *Handler) getInvoiceHandler(w http.ResponseWriter, r *http.Request) {
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
	invoiceNumber := strings.TrimSpace(r.FormValue("invoice_id"))
	if invoiceNumber == "" {
		http.Error(w, "invoice number required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// load Xero connection for the owner (first one)
	conns, err := service.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
	if err != nil || len(conns) == 0 {
		http.Error(w, "no xero connection found for owner", http.StatusNotFound)
		return
	}
	found := &conns[0]

	// refresh token if near expiry (<= 60s)
	now := time.Now().UTC()
	if found.ExpiresAt <= now.Unix()+60 {
		clientID := os.Getenv("XERO_CLIENT_ID")
		clientSecret := os.Getenv("XERO_CLIENT_SECRET")
		tr, rerr := xero.RefreshToken(ctx, h.client, clientID, clientSecret, found.RefreshToken)
		if rerr != nil {
			http.Error(w, "refresh token failed: "+rerr.Error(), http.StatusInternalServerError)
			return
		}
		if err := service.UpsertConnection(ctx, h.dbURL, ownerID, found.TenantID, tr.AccessToken, tr.RefreshToken, tr.ExpiresIn); err != nil {
			http.Error(w, "persist refreshed token failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		found.AccessToken = tr.AccessToken
		secs := tr.ExpiresIn
		if secs == 0 {
			secs = 3600
		}
		found.ExpiresAt = time.Now().Unix() + secs
	}

	client := h.client
	if client == nil {
		client = http.DefaultClient
	}

	// 1) Fetch invoice lines (roots)
	lines, err := xero.GetInvoiceItemCodes(ctx, client, found.AccessToken, found.TenantID, invoiceNumber)
	if err != nil {
		http.Error(w, "fetch invoice items failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(lines) == 0 {
		utils.SetCookie(w, r, "xero_sync_msg", "No items found on invoice "+invoiceNumber, time.Now().Add(3*time.Minute))
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	roots := make([]service.RootItem, 0, len(lines))
	for _, li := range lines {
		roots = append(roots, service.RootItem{
			PartID:   li.ItemCode,
			Name:     li.Name,
			Quantity: li.Quantity,
		})
	}

	// 2) Resolve BOM (effective totals for all nodes)
	bom, errMsg, err := service.ResolveInvoiceBOM(ctx, h.dbURL, roots, 12, client, found.AccessToken, found.TenantID)
	if err != nil {
		http.Error(w, "resolve bom failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if errMsg != "" {
		utils.SetCookie(w, r, "xero_sync_msg", errMsg, time.Now().Add(3*time.Minute))
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// 3) Build PerAssemblyBOM (children quantities divided by root qty; root keeps its invoice qty)
	type bomNode = service.BOMNode
	var norm func(node bomNode, rootQty float64, isRoot bool) bomNode
	norm = func(node bomNode, rootQty float64, isRoot bool) bomNode {
		q := node.Quantity
		if !isRoot && rootQty > 0 {
			q = q / rootQty
		}
		out := bomNode{
			PartID:     node.PartID,
			Name:       node.Name,
			Quantity:   q,
			IsAssembly: node.IsAssembly,
		}
		for _, ch := range node.Children {
			out.Children = append(out.Children, norm(ch, rootQty, false))
		}
		return out
	}
	perAssy := make([]bomNode, 0, len(bom))
	for i, root := range bom {
		rootQty := roots[i].Quantity
		perAssy = append(perAssy, norm(root, rootQty, true))
	}

	// 4) Aggregate leaf totals across all roots (sum effective totals only for leaves)
	type leafTotal struct {
		PartID   string  `json:"part_id"`
		Name     string  `json:"name"`
		Quantity float64 `json:"quantity"`
	}
	agg := map[string]*leafTotal{}
	var walk func(node bomNode)
	walk = func(node bomNode) {
		if !node.IsAssembly {
			if lt, ok := agg[node.PartID]; ok {
				lt.Quantity += node.Quantity
			} else {
				agg[node.PartID] = &leafTotal{PartID: node.PartID, Name: node.Name, Quantity: node.Quantity}
			}
			return
		}
		for _, ch := range node.Children {
			walk(ch)
		}
	}
	for _, root := range bom {
		walk(root)
	}
	leafTotals := make([]leafTotal, 0, len(agg))
	for _, v := range agg {
		// round to nearest int for form defaults
		v.Quantity = math.Round(v.Quantity)
		leafTotals = append(leafTotals, *v)
	}

	// 5) Store cookies for home page rendering
	setJSONCookie := func(name string, v any) {
		b, _ := json.Marshal(v)
		utils.SetCookie(w, r, name, base64.StdEncoding.EncodeToString(b), time.Now().Add(5*time.Minute))
	}
	setJSONCookie("xero_perassy_bom", perAssy)
	setJSONCookie("xero_leaf_totals", leafTotals)
	utils.SetCookie(w, r, "xero_invoice_number", invoiceNumber, time.Now().Add(5*time.Minute))

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// createPurchaseOrdersHandler reads unordered shopping_list rows, groups by contact (AccountNumber),
// creates a purchase order per contact via pkg/xero, marks rows ordered, and sets a message.
func (h *Handler) createPurchaseOrdersHandler(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil {
		r = mid.EnsureUserIDInContext(r, h.auth)
	}
	ownerID, _ := r.Context().Value(mid.CtxUserID).(string)
	if ownerID == "" {
		http.Error(w, "owner id missing", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// load Xero connection for owner
	conns, err := service.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
	if err != nil {
		http.Error(w, "failed to load connections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(conns) == 0 {
		http.Error(w, "no xero connection found for owner", http.StatusNotFound)
		return
	}
	found := &conns[0]

	// refresh token if near expiry
	now := time.Now().UTC()
	if found.ExpiresAt <= now.Unix()+60 {
		clientID := os.Getenv("XERO_CLIENT_ID")
		clientSecret := os.Getenv("XERO_CLIENT_SECRET")
		tr, err := xero.RefreshToken(ctx, h.client, clientID, clientSecret, found.RefreshToken)
		if err != nil {
			http.Error(w, "refresh token failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := service.UpsertConnection(ctx, h.dbURL, ownerID, found.TenantID, tr.AccessToken, tr.RefreshToken, tr.ExpiresIn); err != nil {
			http.Error(w, "failed to persist refreshed token: "+err.Error(), http.StatusInternalServerError)
			return
		}
		found.AccessToken = tr.AccessToken
		secs := tr.ExpiresIn
		if secs == 0 {
			secs = 3600
		}
		found.ExpiresAt = time.Now().Unix() + secs
	}

	// 1) load unordered shopping list rows
	rows, err := service.GetUnorderedShoppingRows(ctx, h.dbURL)
	if err != nil {
		http.Error(w, "failed to read shopping list: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(rows) == 0 {
		utils.SetCookie(w, r, "xero_sync_msg", "No unordered shopping list items found.", time.Now().Add(5*time.Minute))
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// 2) group rows by contact (and aggregate quantities).
	grouped, err := service.GroupShoppingItemsByContact(ctx, h.dbURL, rows)
	if err != nil {
		utils.SetCookie(w, r, "xero_sync_msg", "Failed to group items by contact: "+err.Error(), time.Now().Add(5*time.Minute))
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// 3) create POs per contact and collect list IDs to mark ordered
	var allListIDs []int
	created := 0

	// caches to reduce Xero calls
	contactIDCache := make(map[string]string) // AccountNumber -> ContactID
	nameCache := make(map[string]string)      // ItemCode -> Name

	for accountNumber, items := range grouped { // accountNumber is Xero Contact.AccountNumber
		// resolve ContactID once per accountNumber
		contactID := contactIDCache[accountNumber]
		if contactID == "" {
			var err error
			contactID, err = xero.GetContactIDByAccountNumber(ctx, h.client, found.AccessToken, found.TenantID, accountNumber)
			if err != nil {
				utils.SetCookie(w, r, "xero_sync_msg", "Contact lookup failed for "+accountNumber+": "+err.Error(), time.Now().Add(5*time.Minute))
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			if contactID == "" {
				utils.SetCookie(w, r, "xero_sync_msg", "No ContactID found for "+accountNumber+" in Xero", time.Now().Add(5*time.Minute))
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			contactIDCache[accountNumber] = contactID
		}

		var poItems []xero.POItem
		for _, it := range items {
			code := it.ItemID // ItemID in DB = Xero Item Code
			desc := code
			if nm, ok := nameCache[code]; ok && nm != "" {
				desc = nm
			} else {
				if nm, ok, err := xero.GetItemNameByCode(ctx, h.client, found.AccessToken, found.TenantID, code); err == nil && ok && nm != "" {
					nameCache[code] = nm
					desc = nm
				}
			}

			poItems = append(poItems, xero.POItem{
				ItemCode:    code,
				Quantity:    it.Quantity,
				Description: desc, // use Name where possible
			})
			allListIDs = append(allListIDs, it.ListIDs...)
		}

		if _, err := xero.CreatePurchaseOrder(ctx, h.client, found.AccessToken, found.TenantID, contactID, poItems); err != nil {
			utils.SetCookie(w, r, "xero_sync_msg", "Failed to create PO for contact "+accountNumber+": "+err.Error(), time.Now().Add(5*time.Minute))
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		created++
	}

	// 4) mark rows ordered
	if len(allListIDs) > 0 {
		if err := service.MarkShoppingListOrdered(ctx, h.dbURL, allListIDs); err != nil {
			http.Error(w, "failed to mark shopping list items ordered: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	msg := fmt.Sprintf("Created %d purchase order(s), %d shopping list rows marked ordered", created, len(allListIDs))
	utils.SetCookie(w, r, "xero_sync_msg", msg, time.Now().Add(5*time.Minute))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// // dumpContactsHandler fetches all Contacts from Xero (pages) and writes the combined
// // JSON to contact.json in the repo root (development helper).
// func (h *Handler) dumpContactsHandler(w http.ResponseWriter, r *http.Request) {
// 	if h.auth != nil {
// 		r = mid.EnsureUserIDInContext(r, h.auth)
// 	}
// 	ownerID, _ := r.Context().Value(mid.CtxUserID).(string)
// 	if ownerID == "" {
// 		http.Error(w, "owner id missing", http.StatusUnauthorized)
// 		return
// 	}

// 	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
// 	defer cancel()

// 	// load Xero connection for the owner
// 	conns, err := service.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
// 	if err != nil {
// 		http.Error(w, "failed to load connections: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}
// 	if len(conns) == 0 {
// 		http.Error(w, "no xero connection found for owner", http.StatusNotFound)
// 		return
// 	}
// 	found := &conns[0]

// 	// refresh token if near expiry
// 	now := time.Now().UTC()
// 	if found.ExpiresAt <= now.Unix()+60 {
// 		clientID := os.Getenv("XERO_CLIENT_ID")
// 		clientSecret := os.Getenv("XERO_CLIENT_SECRET")
// 		tr, err := xero.RefreshToken(ctx, h.client, clientID, clientSecret, found.RefreshToken)
// 		if err != nil {
// 			http.Error(w, "refresh token failed: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		if err := service.UpsertConnection(ctx, h.dbURL, ownerID, found.TenantID, tr.AccessToken, tr.RefreshToken, tr.ExpiresIn); err != nil {
// 			http.Error(w, "failed to persist refreshed token: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		found.AccessToken = tr.AccessToken
// 		secs := tr.ExpiresIn
// 		if secs == 0 {
// 			secs = 3600
// 		}
// 		found.ExpiresAt = time.Now().Unix() + secs
// 	}

// 	// fetch contacts page-by-page and aggregate
// 	allContacts := make([]interface{}, 0)
// 	client := h.client
// 	if client == nil {
// 		client = http.DefaultClient
// 	}

// 	for page := 1; page <= 50; page++ { // safety cap at 50 pages
// 		u := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Contacts?page=%d", page)
// 		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
// 		if err != nil {
// 			http.Error(w, "failed to build request: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		req.Header.Set("Authorization", "Bearer "+found.AccessToken)
// 		req.Header.Set("Xero-tenant-id", found.TenantID)
// 		req.Header.Set("Accept", "application/json")

// 		resp, err := client.Do(req)
// 		if err != nil {
// 			http.Error(w, "contacts fetch failed: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		if resp.Body != nil {
// 			defer resp.Body.Close()
// 		}
// 		if resp.StatusCode >= 300 {
// 			// read body for debugging
// 			var b []byte
// 			_ = json.NewDecoder(resp.Body).Decode(&b) // best-effort
// 			http.Error(w, fmt.Sprintf("contacts fetch failed: status=%d", resp.StatusCode), http.StatusInternalServerError)
// 			return
// 		}

// 		var pageShape struct {
// 			Contacts []interface{} `json:"Contacts"`
// 		}
// 		if err := json.NewDecoder(resp.Body).Decode(&pageShape); err != nil {
// 			http.Error(w, "failed to decode contacts response: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		if len(pageShape.Contacts) == 0 {
// 			break
// 		}
// 		allContacts = append(allContacts, pageShape.Contacts...)
// 		// continue to next page
// 	}

// 	out := map[string]interface{}{
// 		"Contacts":  allContacts,
// 		"FetchedAt": time.Now().UTC().Format(time.RFC3339),
// 	}

// 	b, err := json.MarshalIndent(out, "", "  ")
// 	if err != nil {
// 		http.Error(w, "failed to marshal contacts: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	if err := os.WriteFile("contact.json", b, 0644); err != nil {
// 		http.Error(w, "failed to write contact.json: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
// 	_, _ = w.Write([]byte(fmt.Sprintf("wrote %d contacts to contact.json\n", len(allContacts))))
// }

// // dumpItemsHandler fetches all Items from Xero (pages) and writes the combined
// // JSON to parts.json (development helper).
// func (h *Handler) dumpItemsHandler(w http.ResponseWriter, r *http.Request) {
// 	if h.auth != nil {
// 		r = mid.EnsureUserIDInContext(r, h.auth)
// 	}
// 	ownerID, _ := r.Context().Value(mid.CtxUserID).(string)
// 	if ownerID == "" {
// 		http.Error(w, "owner id missing", http.StatusUnauthorized)
// 		return
// 	}

// 	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
// 	defer cancel()

// 	// load Xero connection for the owner
// 	conns, err := service.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
// 	if err != nil {
// 		http.Error(w, "failed to load connections: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}
// 	if len(conns) == 0 {
// 		http.Error(w, "no xero connection found for owner", http.StatusNotFound)
// 		return
// 	}
// 	found := &conns[0]

// 	// refresh token if near expiry
// 	now := time.Now().UTC()
// 	if found.ExpiresAt <= now.Unix()+60 {
// 		clientID := os.Getenv("XERO_CLIENT_ID")
// 		clientSecret := os.Getenv("XERO_CLIENT_SECRET")
// 		tr, err := xero.RefreshToken(ctx, h.client, clientID, clientSecret, found.RefreshToken)
// 		if err != nil {
// 			http.Error(w, "refresh token failed: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		if err := service.UpsertConnection(ctx, h.dbURL, ownerID, found.TenantID, tr.AccessToken, tr.RefreshToken, tr.ExpiresIn); err != nil {
// 			http.Error(w, "failed to persist refreshed token: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		found.AccessToken = tr.AccessToken
// 		secs := tr.ExpiresIn
// 		if secs == 0 {
// 			secs = 3600
// 		}
// 		found.ExpiresAt = time.Now().Unix() + secs
// 	}

// 	// fetch items page-by-page and aggregate with rate-limit retry
// 	allItems := make([]interface{}, 0)
// 	seen := make(map[string]struct{}) // track unique ItemID/Code

// 	// ensure http client is set (fixes "undeclared client")
// 	client := h.client
// 	if client == nil {
// 		client = http.DefaultClient
// 	}

// 	const maxPages = 1
// 	const maxAttempts = 5

// 	for page := 1; page <= maxPages; page++ {
// 		var pageShape struct {
// 			Items []interface{} `json:"Items"`
// 		}

// 		// attempt with retries on 429
// 		attempt := 0
// 		for {
// 			attempt++
// 			u := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Items?page=%d", page)
// 			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
// 			if err != nil {
// 				http.Error(w, "failed to build request: "+err.Error(), http.StatusInternalServerError)
// 				return
// 			}
// 			req.Header.Set("Authorization", "Bearer "+found.AccessToken)
// 			req.Header.Set("Xero-tenant-id", found.TenantID)
// 			req.Header.Set("Accept", "application/json")

// 			resp, err := client.Do(req)
// 			if err != nil {
// 				// network error, retry a few times
// 				if attempt >= maxAttempts {
// 					http.Error(w, "items fetch failed: "+err.Error(), http.StatusInternalServerError)
// 					return
// 				}
// 				// small backoff then retry
// 				backoff := time.Duration(1<<attempt) * time.Second
// 				select {
// 				case <-time.After(backoff):
// 					continue
// 				case <-ctx.Done():
// 					http.Error(w, "request cancelled", http.StatusRequestTimeout)
// 					return
// 				}
// 			}

// 			bodyBytes, _ := io.ReadAll(resp.Body)
// 			resp.Body.Close()

// 			if resp.StatusCode == http.StatusTooManyRequests {
// 				// Xero rate limited. Respect Retry-After if provided.
// 				retryAfter := 0
// 				if ra := resp.Header.Get("Retry-After"); ra != "" {
// 					if secs, perr := strconv.Atoi(ra); perr == nil {
// 						retryAfter = secs
// 					}
// 				}
// 				if retryAfter <= 0 {
// 					// exponential backoff with small jitter
// 					retryAfter = int((1 << attempt))
// 				}
// 				if attempt >= maxAttempts {
// 					http.Error(w, fmt.Sprintf("items fetch failed: status=429 body=%s", string(bodyBytes)), http.StatusTooManyRequests)
// 					return
// 				}
// 				// wait respecting context
// 				wait := time.Duration(retryAfter) * time.Second
// 				select {
// 				case <-time.After(wait):
// 					continue
// 				case <-ctx.Done():
// 					http.Error(w, "request cancelled", http.StatusRequestTimeout)
// 					return
// 				}
// 			}

// 			if resp.StatusCode >= 300 {
// 				// surface body for debugging
// 				http.Error(w, fmt.Sprintf("items fetch failed: status=%d body=%s", resp.StatusCode, string(bodyBytes)), http.StatusInternalServerError)
// 				return
// 			}

// 			// decode page
// 			if err := json.Unmarshal(bodyBytes, &pageShape); err != nil {
// 				http.Error(w, "failed to decode items response: "+err.Error(), http.StatusInternalServerError)
// 				return
// 			}
// 			break
// 		} // retry loop

// 		if len(pageShape.Items) == 0 {
// 			break
// 		}

// 		// dedupe items by ItemID or Code before appending
// 		for _, it := range pageShape.Items {
// 			m, ok := it.(map[string]interface{})
// 			if !ok {
// 				// unknown shape, append to result
// 				allItems = append(allItems, it)
// 				continue
// 			}

// 			var key string
// 			if v, ok := m["ItemID"].(string); ok && v != "" {
// 				key = "id:" + v
// 			} else if v, ok := m["Code"].(string); ok && v != "" {
// 				key = "code:" + v
// 			} else {
// 				// fallback: try Name (less reliable)
// 				if v, ok := m["Name"].(string); ok && v != "" {
// 					key = "name:" + v
// 				}
// 			}

// 			if key == "" {
// 				allItems = append(allItems, it)
// 				continue
// 			}
// 			if _, found := seen[key]; found {
// 				continue
// 			}
// 			seen[key] = struct{}{}
// 			allItems = append(allItems, it)
// 		}
// 	}

// 	out := map[string]interface{}{
// 		"Items":     allItems,
// 		"FetchedAt": time.Now().UTC().Format(time.RFC3339),
// 	}

// 	b, err := json.MarshalIndent(out, "", "  ")
// 	if err != nil {
// 		http.Error(w, "failed to marshal items: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	if err := os.WriteFile("items.json", b, 0644); err != nil {
// 		http.Error(w, "failed to write items.json: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
// 	_, _ = w.Write([]byte(fmt.Sprintf("wrote %d items to items.json\n", len(allItems))))
// }
