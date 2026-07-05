package garmin

import "regexp"

// Well-known Garmin Connect Mobile OAuth1 consumer credentials.
// Same pair used by python-garminconnect, Garth, and other unofficial clients.
const (
	consumerKey    = "fc3e99d2-118c-44b8-8ae3-03370dde24c0"
	consumerSecret = "E08WAR897WEy2knn7aFBrvegVAf0AFdWBBF"
)

const (
	ssoEmbedURL  = "https://sso.garmin.com/sso/embed"
	ssoSigninURL = "https://sso.garmin.com/sso/signin"
	ssoMFAURL    = "https://sso.garmin.com/sso/verifyMFA/loginEnterMfaCode"

	oauth1PreauthorizedURL = "https://connectapi.garmin.com/oauth-service/oauth/preauthorized"
	oauth2ExchangeURL      = "https://connectapi.garmin.com/oauth-service/oauth/exchange/user/2.0"

	uploadURL = "https://connectapi.garmin.com/upload-service/upload/.fit"

	// userAgent impersonates the Garmin Connect Android app; the SSO/upload
	// endpoints reject requests from a generic Go http.Client User-Agent.
	userAgent = "com.garmin.android.apps.connectmobile"
)

var (
	csrfRegexp   = regexp.MustCompile(`name="_csrf"\s+value="(.+?)"`)
	ticketRegexp = regexp.MustCompile(`embed\?ticket=([^"]+)"`)
)
