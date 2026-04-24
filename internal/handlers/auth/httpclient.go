package auth

import (
	"net/http"
	"time"
)

// newHTTPClient returns an http.Client configured for proxying requests to OAuth2/OIDC providers.
//
// Redirect following is disabled intentionally: we never want the server to
// silently follow a redirect loop issued by a misconfigured or malicious IDP.
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
