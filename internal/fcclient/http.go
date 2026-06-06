package fcclient

import (
	"context"
	"net/http"
)

// httpNewGETWithAuth builds an authenticated GET request. Factored out so
// tests can mock at the request level if needed.
func httpNewGETWithAuth(ctx context.Context, url, token, version string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json;version="+version+";charset=UTF-8")
	if token != "" {
		req.Header.Set("X-Auth-Token", token)
	}
	return req, nil
}
