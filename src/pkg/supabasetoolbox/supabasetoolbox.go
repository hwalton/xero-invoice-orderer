package supabasetoolbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func GenerateSignedURL(accessToken, path string) (string, error) {
	apiURL := fmt.Sprintf(
		"%s/storage/v1/object/sign/flashcard-assets/%s",
		os.Getenv("NEXT_PUBLIC_SUPABASE_URL"),
		path,
	)

	expiry := 3600 // URL valid for 1 hour

	body := map[string]interface{}{
		"expiresIn": expiry,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("apikey", os.Getenv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to generate signed URL: status %d", resp.StatusCode)
	}

	var result struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return os.Getenv("NEXT_PUBLIC_SUPABASE_URL") + "/storage/v1" + result.SignedURL, nil
}

// AuthenticateWithSupabase calls Supabase auth REST to exchange email+password for tokens.
func AuthenticateWithSupabase(ctx context.Context, client *http.Client, email string, password string, supabaseURL string, apiKey string) (string, string, string, error) {

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
		log.Printf("AuthenticateWithSupabase: status=%d body=%s", resp.StatusCode, string(body))
		return "", "", "", fmt.Errorf("http error: status %d: %s", resp.StatusCode, string(body))
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

func RefreshAccessToken(r *http.Request, supabaseUrl string, apiKey string) (string, string, error) {
	refreshCookie, err := r.Cookie("refresh_token")
	if err != nil {
		return "", "", fmt.Errorf("refresh token missing: %w", err)
	}
	refreshToken := refreshCookie.Value

	payload := map[string]string{
		"refresh_token": refreshToken,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", supabaseUrl+"/auth/v1/token?grant_type=refresh_token", bytes.NewReader(payloadBytes))
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to refresh access token: %w", err)
	}
	defer resp.Body.Close()

	var result loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode refreshed token: %w", err)
	}

	return result.AccessToken, result.RefreshToken, nil
}
