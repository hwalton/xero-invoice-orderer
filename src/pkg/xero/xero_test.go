package xero

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// helper to decode JSON and return map for simple assertions
func mustUnmarshal(t *testing.T, b []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestBuildPOPayload_EmptyContactID(t *testing.T) {
	_, err := buildPOPayload("", []POItem{{ItemCode: "A", Quantity: 1}})
	if err == nil {
		t.Fatalf("expected error for empty contact id")
	}
}

func TestBuildPOPayload_Success(t *testing.T) {
	items := []POItem{
		{ItemCode: "C1", Quantity: 2, Description: "desc"},
	}
	b, err := buildPOPayload("contact-123", items)
	if err != nil {
		t.Fatalf("buildPOPayload failed: %v", err)
	}
	var got map[string][]map[string]any
	mustUnmarshal(t, b, &got)
	if len(got["PurchaseOrders"]) != 1 {
		t.Fatalf("expected one purchase order, got %d", len(got["PurchaseOrders"]))
	}
	po := got["PurchaseOrders"][0]
	contact := po["Contact"].(map[string]any)
	if contact["ContactID"] != "contact-123" {
		t.Fatalf("unexpected contact id: %v", contact["ContactID"])
	}
}

func TestBuildItemsUpsertPayload_CodeToItemID(t *testing.T) {
	parts := []Part{
		{PartID: "P1", Name: "Part 1", SalesPrice: 10, CostPrice: 5},
		{PartID: "P2", Name: "Part 2"},
	}
	codeToID := func(code string) (string, error) {
		if code == "P1" {
			return "existing-id-1", nil
		}
		return "", nil
	}
	b, err := buildItemsUpsertPayload(parts, codeToID)
	if err != nil {
		t.Fatalf("buildItemsUpsertPayload failed: %v", err)
	}
	var payload struct {
		Items []map[string]any `json:"Items"`
	}
	mustUnmarshal(t, b, &payload)
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(payload.Items))
	}
	if payload.Items[0]["ItemID"] != "existing-id-1" {
		t.Fatalf("expected ItemID for P1 to be set, got %v", payload.Items[0]["ItemID"])
	}
}

func TestParseFirstItemName_EmptyAndPresent(t *testing.T) {
	empty, found, err := parseFirstItemName([]byte(`{"Items": []}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if found {
		t.Fatalf("expected not found for empty items")
	}
	if empty != "" {
		t.Fatalf("expected empty name")
	}

	b := []byte(`{"Items":[{"Name":"Widget"}]}`)
	name, found, err := parseFirstItemName(b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !found || name != "Widget" {
		t.Fatalf("unexpected result: %v %v", name, found)
	}
}

func TestParseInvoiceLines_SelectsNameOrDescription(t *testing.T) {
	body := []byte(`{"Invoices":[{"LineItems":[{"ItemCode":"IC1","Description":"desc-only","Quantity":2,"Item":{"Name":""}},{"ItemCode":"IC2","Description":"d2","Quantity":1,"Item":{"Name":"Named"}}]}]}`)
	lines, err := parseInvoiceLines(body)
	if err != nil {
		t.Fatalf("parseInvoiceLines error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0].Name != "desc-only" || lines[0].ItemCode != "IC1" {
		t.Fatalf("unexpected first line: %#v", lines[0])
	}
	if lines[1].Name != "Named" || lines[1].ItemCode != "IC2" {
		t.Fatalf("unexpected second line: %#v", lines[1])
	}
}

func TestBuildAuthURL_Escaping(t *testing.T) {
	clientID := "cid"
	redirect := "https://example.com/cb?a=1"
	state := "s t&x"
	u := BuildAuthURL(clientID, redirect, state)
	// ensure redirect and state are query-escaped
	if !strings.Contains(u, url.QueryEscape(redirect)) {
		t.Fatalf("redirect not escaped in URL: %s", u)
	}
	if !strings.Contains(u, url.QueryEscape(state)) {
		t.Fatalf("state not escaped in URL: %s", u)
	}
}

// ---- HTTP-backed functions ----

func TestGetItemNameByID_NotFoundAndSuccess(t *testing.T) {
	// server that responds to item id path
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api.xro/2.0/Items/notfound" {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api.xro/2.0/Items/") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"Items":[{"Name":"MyItem"}]}`))
			return
		}
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer ts.Close()

	target, _ := url.Parse(ts.URL)
	// wrap the test server client's transport so absolute requests get rewritten
	client := &http.Client{Transport: hostRewriter{base: ts.Client().Transport, target: target}}

	// not found case should return found=false, nil error
	name, found, err := GetItemNameByID(context.Background(), client, "at", "tid", "notfound")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if found {
		t.Fatalf("expected not found")
	}
	// success
	name, found, err = GetItemNameByID(context.Background(), client, "at", "tid", "someid")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !found || name != "MyItem" {
		t.Fatalf("unexpected result: name=%q found=%v", name, found)
	}
}

func TestGetItemNameByCode_ErrorStatus(t *testing.T) {
	// server that returns 500 for Items query
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api.xro/2.0/Items") {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer ts.Close()

	client := ts.Client()
	_, _, err := GetItemNameByCode(context.Background(), client, "at", "tid", "code123")
	if err == nil {
		t.Fatalf("expected error on non-200")
	}
}

// helper transport that rewrites absolute host/scheme to target (httptest) host.
type hostRewriter struct {
	base   http.RoundTripper
	target *url.URL
}

func (h hostRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	// clone request and rewrite scheme/host so requests to api.xero.com go to ts
	n := req.Clone(req.Context())
	n.URL.Scheme = h.target.Scheme
	n.URL.Host = h.target.Host
	n.Host = h.target.Host
	return h.base.RoundTrip(n)
}
