// Package telegram implements domain.Notifier by sending messages through the
// Telegram Bot API. It's used to alert a human when unattended Garmin sync
// hits a failure it can't recover from on its own (lockout, MFA, block).
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rsb/garmin-weight-sync/internal/domain"
)

// Notifier sends alerts to a single Telegram chat. When token or chatID is
// empty it is a no-op, so the rest of the app doesn't need to branch on
// whether alerting is configured.
type Notifier struct {
	token      string
	chatID     string
	httpClient *http.Client
}

var _ domain.Notifier = (*Notifier)(nil)

// New builds a Notifier. Pass empty token/chatID to disable alerting.
func New(token, chatID string) *Notifier {
	return &Notifier{
		token:      token,
		chatID:     chatID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled reports whether alerts will actually be sent.
func (n *Notifier) Enabled() bool {
	return n.token != "" && n.chatID != ""
}

// Notify sends message to the configured chat. It no-ops silently when
// disabled.
func (n *Notifier) Notify(ctx context.Context, message string) error {
	if !n.Enabled() {
		return nil
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.token)
	form := url.Values{
		"chat_id": {n.chatID},
		"text":    {message},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Description string `json:"description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return fmt.Errorf("telegram API error: status %d: %s", resp.StatusCode, apiErr.Description)
	}
	return nil
}
