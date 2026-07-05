package garmin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ErrMFARequired is returned by login when Garmin demands a multi-factor
// code before it will issue a service ticket. Callers should prompt for the
// code out-of-band and retry with completeMFA.
var ErrMFARequired = errors.New("garmin: MFA code required")

// These sentinels mark auth failures that a bare retry cannot recover from —
// the account needs manual attention. Client.Sync maps them onto
// domain.ErrSyncAuthRequired so the cron use case can stop and alert.
var (
	ErrAccountLocked     = errors.New("garmin: account locked")
	ErrCloudflareBlocked = errors.New("garmin: blocked by Cloudflare")
	ErrRateLimited       = errors.New("garmin: rate limited")
)

// ssoAuth drives the CAS/SSO login flow (mechanism doc §3.2) and produces a
// CAS service ticket that can be exchanged for OAuth1/OAuth2 tokens.
type ssoAuth struct {
	httpClient *http.Client
	sleep      func(ctx context.Context) error

	mfaCSRF string
}

// newSSOAuth builds an ssoAuth with its own cookie jar (mechanism §3.2 Step 0
// requires the session cookies from /sso/embed to be reused on every later
// call). sleep is invoked between steps to avoid Cloudflare rate-limiting
// (§3.3); pass a no-op in tests.
func newSSOAuth(sleep func(ctx context.Context) error) (*ssoAuth, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}
	return &ssoAuth{
		httpClient: &http.Client{Jar: jar, Timeout: 30 * time.Second},
		sleep:      sleep,
	}, nil
}

// randomAntiBotSleep sleeps 10-16s, mirroring the reference implementation's
// anti-bot delay between SSO steps. It returns early with ctx.Err() if ctx is
// cancelled first, so a shutdown signal can interrupt a login in progress
// instead of waiting out the full delay.
func randomAntiBotSleep(ctx context.Context) error {
	d := 10*time.Second + time.Duration(rand.Intn(6001))*time.Millisecond
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

const ssoWidgetQuery = "id=gauth-widget&embedWidget=true&gauthHost=" + ssoEmbedURLEncoded +
	"&service=" + ssoEmbedURLEncoded + "&source=" + ssoEmbedURLEncoded

const ssoEmbedURLEncoded = "https%3A%2F%2Fsso.garmin.com%2Fsso%2Fembed"

// login runs the full SSO flow: init cookies -> CSRF -> submit credentials.
// It returns a service ticket, or ErrMFARequired if a code is needed (call
// completeMFA next).
func (a *ssoAuth) login(ctx context.Context, username, password string) (string, error) {
	if err := a.initCookies(ctx); err != nil {
		return "", fmt.Errorf("init cookies: %w", err)
	}
	if err := a.sleep(ctx); err != nil {
		return "", err
	}

	csrf, err := a.getCSRF(ctx)
	if err != nil {
		return "", fmt.Errorf("get csrf: %w", err)
	}
	if err := a.sleep(ctx); err != nil {
		return "", err
	}

	return a.sendCredentials(ctx, username, password, csrf)
}

// completeMFA finishes a login that returned ErrMFARequired.
func (a *ssoAuth) completeMFA(ctx context.Context, code string) (string, error) {
	if a.mfaCSRF == "" {
		return "", errors.New("garmin: completeMFA called without a pending MFA challenge")
	}

	form := url.Values{
		"embed":    {"true"},
		"mfa-code": {code},
		"fromPage": {"setupEnterMfaCode"},
		"_csrf":    {a.mfaCSRF},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ssoMFAURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	a.setCommonHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("referer", ssoSigninURL)

	body, err := a.doAndRead(req)
	if err != nil {
		return "", fmt.Errorf("submit mfa code: %w", err)
	}

	ticket, ok := extractTicket(body)
	if !ok {
		return "", fmt.Errorf("mfa response did not contain a service ticket")
	}
	return ticket, nil
}

func (a *ssoAuth) initCookies(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ssoEmbedURL+"?"+ssoWidgetQuery, nil)
	if err != nil {
		return err
	}
	a.setCommonHeaders(req)

	_, err = a.doAndRead(req)
	return err
}

func (a *ssoAuth) getCSRF(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ssoSigninURL+"?"+ssoWidgetQuery, nil)
	if err != nil {
		return "", err
	}
	a.setCommonHeaders(req)

	body, err := a.doAndRead(req)
	if err != nil {
		return "", err
	}

	csrf, ok := extractCSRF(body)
	if !ok {
		return "", fmt.Errorf("csrf token not found in signin page (response snippet: %s)", snippet(body))
	}
	return csrf, nil
}

func (a *ssoAuth) sendCredentials(ctx context.Context, username, password, csrf string) (string, error) {
	form := url.Values{
		"username": {username},
		"password": {password},
		"embed":    {"true"},
		"_csrf":    {csrf},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ssoSigninURL+"?"+ssoWidgetQuery, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	a.setCommonHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("origin", "https://sso.garmin.com")
	req.Header.Set("referer", ssoSigninURL)
	req.Header.Set("NK", "NT")

	body, err := a.doAndRead(req)
	if err != nil {
		return "", err
	}

	if strings.Contains(body, "/sso/verifyMFA/loginEnterMfaCode") {
		mfaCSRF, ok := extractCSRF(body)
		if !ok {
			return "", errors.New("mfa redirect without a csrf token")
		}
		a.mfaCSRF = mfaCSRF
		return "", ErrMFARequired
	}

	ticket, ok := extractTicket(body)
	if !ok {
		// Garmin embeds a status var in the response JS (e.g.
		// ACCOUNT_LOCKED, INVALID_USERNAME_PASSWORD). Surface it verbatim so
		// the failure reason is obvious instead of a generic parse error.
		if status, ok := extractStatus(body); ok {
			if strings.Contains(status, "LOCKED") {
				return "", fmt.Errorf("%w: status %q", ErrAccountLocked, status)
			}
			return "", fmt.Errorf("garmin login rejected: status %q", status)
		}
		return "", fmt.Errorf("service ticket not found; check credentials (response snippet: %s)", snippet(body))
	}
	return ticket, nil
}

var statusRegexp = regexp.MustCompile(`var\s+status\s*=\s*"([^"]+)"`)

func extractStatus(html string) (string, bool) {
	m := statusRegexp.FindStringSubmatch(html)
	if len(m) < 2 || m[1] == "" {
		return "", false
	}
	return m[1], true
}

// errorMarkers are substrings Garmin's login page uses to report a rejected
// login (invalid creds, lockout, captcha) rather than a genuine parsing bug.
// The head of the page is boilerplate every time, so when one of these is
// present, snippet centers the preview on it instead.
var errorMarkers = []string{
	"invalid", "incorrect", "locked", "captcha", "too many", "not recognized", "blocked",
}

// snippet returns a short, single-line preview of a response body for error
// messages, so login failures are debuggable without dumping full HTML. If
// the body contains a known error marker, the preview centers on it instead
// of the (always-identical) page head.
func snippet(body string) string {
	flat := strings.Join(strings.Fields(body), " ")
	const window = 300

	lower := strings.ToLower(flat)
	for _, marker := range errorMarkers {
		if idx := strings.Index(lower, marker); idx != -1 {
			start := idx - 100
			if start < 0 {
				start = 0
			}
			end := idx + 200
			if end > len(flat) {
				end = len(flat)
			}
			return "..." + flat[start:end] + "..."
		}
	}

	if len(flat) > window {
		return flat[:window] + "..."
	}
	return flat
}

func (a *ssoAuth) setCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
}

// doAndRead performs the request and returns the body, translating
// Cloudflare's block/rate-limit responses into distinct errors (§3.3).
func (a *ssoAuth) doAndRead(req *http.Request) (string, error) {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	text := string(body)

	if resp.StatusCode == http.StatusForbidden && strings.Contains(text, "error code: 1020") {
		return "", fmt.Errorf("%w (1020)", ErrCloudflareBlocked)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("%w (429)", ErrRateLimited)
	}
	if strings.Contains(text, "Just a moment") || strings.Contains(text, "cf-chl") {
		return "", fmt.Errorf("%w (challenge page, status %d)", ErrCloudflareBlocked, resp.StatusCode)
	}
	return text, nil
}

func extractCSRF(html string) (string, bool) {
	m := csrfRegexp.FindStringSubmatch(html)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

func extractTicket(html string) (string, bool) {
	m := ticketRegexp.FindStringSubmatch(html)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}
