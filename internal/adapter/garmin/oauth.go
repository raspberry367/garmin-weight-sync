package garmin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// oauth1Token is the long-lived OAuth1 token pair obtained by exchanging a
// CAS service ticket. It can be cached and reused to skip the SSO login on
// later runs (mechanism doc §3.4).
type oauth1Token struct {
	Token  string
	Secret string
}

// oauth2Token is the short-lived (~1h) bearer token used to authorize
// uploads. Derived from an oauth1Token via the exchange endpoint.
type oauth2Token struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int       `json:"expires_in"`
	ExpiresAt   time.Time `json:"-"`
}

func (t *oauth2Token) valid() bool {
	return t != nil && t.AccessToken != "" && time.Now().Before(t.ExpiresAt)
}

// signOAuth1 builds the OAuth1 "Authorization" header value for method+rawURL.
// When token/tokenSecret are empty, it signs as a request-token call (Step 3,
// preauthorized); when present, as a protected-resource call (Step 4, exchange).
func signOAuth1(method, rawURL, token, tokenSecret string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}

	nonce, err := randomNonce()
	if err != nil {
		return "", err
	}

	params := map[string]string{
		"oauth_consumer_key":     consumerKey,
		"oauth_nonce":            nonce,
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        strconv.FormatInt(time.Now().Unix(), 10),
		"oauth_version":          "1.0",
	}
	if token != "" {
		params["oauth_token"] = token
	}

	// The signature base string must include query params from the URL too.
	for key, values := range parsed.Query() {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	baseURL := fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, parsed.Path)
	signature := computeHMACSHA1Signature(method, baseURL, params, consumerSecret, tokenSecret)
	params["oauth_signature"] = signature

	return buildAuthorizationHeader(params), nil
}

func computeHMACSHA1Signature(method, baseURL string, params map[string]string, consumerSecret, tokenSecret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%s", rfc3986Escape(k), rfc3986Escape(params[k])))
	}
	paramString := strings.Join(pairs, "&")

	baseString := strings.Join([]string{
		method,
		rfc3986Escape(baseURL),
		rfc3986Escape(paramString),
	}, "&")

	signingKey := rfc3986Escape(consumerSecret) + "&" + rfc3986Escape(tokenSecret)

	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(baseString))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func buildAuthorizationHeader(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if strings.HasPrefix(k, "oauth_") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf(`%s="%s"`, k, rfc3986Escape(params[k])))
	}
	return "OAuth " + strings.Join(pairs, ", ")
}

// rfc3986Escape percent-encodes s per RFC 3986 unreserved characters
// (ALPHA / DIGIT / "-" / "." / "_" / "~"), which is what OAuth1 signing
// (RFC 5849 §3.6) requires. url.QueryEscape is the wrong tool here: it follows
// application/x-www-form-urlencoded rules and encodes a space as '+' rather
// than '%20', so any signed value containing one would diverge from the
// signature Garmin computes on its end.
func rfc3986Escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// getOAuth1 exchanges a CAS service ticket for a long-lived OAuth1 token pair
// (mechanism doc §3.2 Step 3).
func getOAuth1(ctx context.Context, httpClient *http.Client, ticket string) (*oauth1Token, error) {
	reqURL := fmt.Sprintf("%s?ticket=%s&login-url=%s&accepts-mfa-tokens=true",
		oauth1PreauthorizedURL, url.QueryEscape(ticket), url.QueryEscape(ssoEmbedURL))

	authHeader, err := signOAuth1(http.MethodGet, reqURL, "", "")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth1 request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read oauth1 response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth1 exchange failed: status %d: %s", resp.StatusCode, string(body))
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse oauth1 response: %w", err)
	}
	token := values.Get("oauth_token")
	secret := values.Get("oauth_token_secret")
	if token == "" || secret == "" {
		return nil, fmt.Errorf("oauth1 response missing token/secret: %s", string(body))
	}

	return &oauth1Token{Token: token, Secret: secret}, nil
}

// getOAuth2 exchanges the OAuth1 token pair for a short-lived OAuth2 bearer
// token (mechanism doc §3.2 Step 4).
func getOAuth2(ctx context.Context, httpClient *http.Client, tok *oauth1Token) (*oauth2Token, error) {
	authHeader, err := signOAuth1(http.MethodPost, oauth2ExchangeURL, tok.Token, tok.Secret)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauth2ExchangeURL, strings.NewReader(""))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth2 request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read oauth2 response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth2 exchange failed: status %d: %s", resp.StatusCode, string(body))
	}

	var parsed oauth2Token
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse oauth2 response: %w", err)
	}
	if parsed.AccessToken == "" {
		return nil, fmt.Errorf("oauth2 response missing access_token: %s", string(body))
	}
	parsed.ExpiresAt = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)

	return &parsed, nil
}
