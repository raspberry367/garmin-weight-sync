package garmin

import (
	"encoding/json"
	"fmt"
	"os"
)

// loadOAuth1Token reads a cached OAuth1 token pair from path. A missing file
// is not an error — it just means no cached credentials exist yet.
func loadOAuth1Token(path string) (*oauth1Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read token cache: %w", err)
	}

	var tok oauth1Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parse token cache: %w", err)
	}
	return &tok, nil
}

// saveOAuth1Token persists the long-lived OAuth1 token pair so future runs
// can skip the SSO login (mechanism doc §3.4).
func saveOAuth1Token(path string, tok *oauth1Token) error {
	data, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("marshal token cache: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write token cache: %w", err)
	}
	return nil
}
