package garmin

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rsb/garmin-weight-sync/internal/domain"
)

// Config configures a Client.
type Config struct {
	Username       string
	Password       string
	TokenCachePath string
}

// Client implements domain.MeasurementSyncer by pushing a BodyComposition to
// Garmin Connect as a FIT File.Weight upload (mechanism doc §7).
type Client struct {
	cfg Config

	httpClient *http.Client
	sleep      func(ctx context.Context) error

	mu     sync.Mutex
	oauth1 *oauth1Token
	oauth2 *oauth2Token
}

var _ domain.MeasurementSyncer = (*Client)(nil)

// NewClient builds a Client, loading any cached OAuth1 token pair from
// cfg.TokenCachePath.
func NewClient(cfg Config) (*Client, error) {
	c := &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		sleep:      randomAntiBotSleep,
	}

	if cfg.TokenCachePath != "" {
		tok, err := loadOAuth1Token(cfg.TokenCachePath)
		if err != nil {
			return nil, err
		}
		c.oauth1 = tok
	}

	return c, nil
}

// Sync uploads a single measurement to Garmin Connect (mechanism doc §7).
// ctx is honored across the anti-bot sleeps and network calls a full SSO
// login can require (tens of seconds) — cancelling it (e.g. during graceful
// shutdown) aborts the sync promptly instead of blocking until it finishes.
func (c *Client) Sync(ctx context.Context, m *domain.BodyComposition) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureOAuth2(ctx); err != nil {
		if isUnrecoverableAuth(err) {
			// Signal the caller to stop the batch and alert a human, rather
			// than retrying (which can deepen a lockout).
			return fmt.Errorf("%w: %v", domain.ErrSyncAuthRequired, err)
		}
		return fmt.Errorf("garmin auth: %w", err)
	}

	fitBytes, err := encodeWeightFIT(m)
	if err != nil {
		return fmt.Errorf("encode fit file: %w", err)
	}

	result, err := uploadFIT(ctx, c.httpClient, c.oauth2.AccessToken, fitBytes)
	if err != nil {
		return fmt.Errorf("upload to garmin: %w", err)
	}
	if result.Duplicate {
		log.Printf("garmin: measurement %s already uploaded (duplicate), treating as success", m.AppleHealthID)
	} else {
		log.Printf("garmin: uploaded measurement %s (uploadId=%d)", m.AppleHealthID, result.UploadID)
	}
	return nil
}

// ensureOAuth2 makes sure c.oauth2 holds a valid bearer token, refreshing via
// the cached OAuth1 pair or, failing that, running a full SSO login.
func (c *Client) ensureOAuth2(ctx context.Context) error {
	if c.oauth2.valid() {
		return nil
	}

	if c.oauth1 != nil {
		tok, err := getOAuth2(ctx, c.httpClient, c.oauth1)
		if err == nil {
			c.oauth2 = tok
			return nil
		}
		log.Printf("garmin: cached OAuth1 token rejected, falling back to full login: %v", err)
		c.oauth1 = nil
	}

	return c.fullLogin(ctx)
}

// isUnrecoverableAuth reports whether an auth error needs manual intervention
// (re-login, unlock, or waiting out a block) rather than a bare retry.
func isUnrecoverableAuth(err error) bool {
	return errors.Is(err, ErrMFARequired) ||
		errors.Is(err, ErrAccountLocked) ||
		errors.Is(err, ErrCloudflareBlocked) ||
		errors.Is(err, ErrRateLimited)
}

// login runs the shared SSO/CAS flow (mechanism doc §3.2): submit
// credentials, then — if Garmin demands an MFA code — hand off to onMFA to
// either complete it or report why it can't, before exchanging the resulting
// ticket for a cached OAuth1/OAuth2 token pair. fullLogin and
// LoginInteractive differ only in onMFA.
func (c *Client) login(ctx context.Context, onMFA func(ctx context.Context, auth *ssoAuth) (string, error)) error {
	auth, err := newSSOAuth(c.sleep)
	if err != nil {
		return err
	}

	ticket, err := auth.login(ctx, c.cfg.Username, c.cfg.Password)
	if errors.Is(err, ErrMFARequired) {
		if ticket, err = onMFA(ctx, auth); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("sso login: %w", err)
	}

	return c.exchangeTicket(ctx, ticket)
}

// fullLogin runs the SSO/CAS flow from scratch (mechanism doc §3.2). It
// cannot complete unattended if Garmin demands an MFA code — cron-based sync
// has no one to prompt, so that case surfaces as an error. Run the
// cmd/garmin-login CLI out-of-band to seed cfg.TokenCachePath before that
// ever happens in production.
func (c *Client) fullLogin(ctx context.Context) error {
	return c.login(ctx, func(context.Context, *ssoAuth) (string, error) {
		return "", fmt.Errorf("%w: cron sync cannot complete MFA unattended; run cmd/garmin-login to seed %s with a valid OAuth1 token pair", ErrMFARequired, c.cfg.TokenCachePath)
	})
}

// LoginInteractive runs the SSO/CAS flow, invoking promptMFA if Garmin
// requests a multi-factor code, then caches the resulting OAuth1 token pair.
// It's meant for one-off out-of-band setup (see cmd/garmin-login) so that
// unattended cron sync never has to handle an MFA challenge itself.
func (c *Client) LoginInteractive(ctx context.Context, promptMFA func() (string, error)) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.login(ctx, func(ctx context.Context, auth *ssoAuth) (string, error) {
		code, err := promptMFA()
		if err != nil {
			return "", fmt.Errorf("read mfa code: %w", err)
		}
		if err := auth.sleep(ctx); err != nil {
			return "", err
		}
		return auth.completeMFA(ctx, code)
	})
}

// exchangeTicket turns a CAS service ticket into cached OAuth1/OAuth2 tokens
// (mechanism doc §3.2 Steps 3-4).
func (c *Client) exchangeTicket(ctx context.Context, ticket string) error {
	if err := c.sleep(ctx); err != nil {
		return err
	}
	oauth1Tok, err := getOAuth1(ctx, c.httpClient, ticket)
	if err != nil {
		return fmt.Errorf("oauth1 exchange: %w", err)
	}

	if err := c.sleep(ctx); err != nil {
		return err
	}
	oauth2Tok, err := getOAuth2(ctx, c.httpClient, oauth1Tok)
	if err != nil {
		return fmt.Errorf("oauth2 exchange: %w", err)
	}

	c.oauth1 = oauth1Tok
	c.oauth2 = oauth2Tok

	if c.cfg.TokenCachePath != "" {
		if err := saveOAuth1Token(c.cfg.TokenCachePath, oauth1Tok); err != nil {
			log.Printf("garmin: failed to persist OAuth1 token cache: %v", err)
		}
	}

	return nil
}
