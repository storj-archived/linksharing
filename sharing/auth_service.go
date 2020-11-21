// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/zeebo/errs"
)

// AuthServiceConfig describes configuration necessary to interact with the auth service.
type AuthServiceConfig struct {
	// Base url to use for the auth service to resolve access key ids
	BaseURL string

	// Authorization token used for the auth service to resolve access key ids.
	Token string
}

// AuthServiceResponse is the struct representing the response from the auth service.
type AuthServiceResponse struct {
	AccessGrant string `json:"access_grant"`
	SecretKey   string `json:"secret_key"`
	Public      bool   `json:"public"`
}

// AuthServiceError wraps all the errors returned when resolving an access key.
var AuthServiceError = errs.Class("auth service")

// Resolve maps an access key into an auth service response.
func (a AuthServiceConfig) Resolve(ctx context.Context, accessKeyID string) (_ *AuthServiceResponse, err error) {
	reqURL, err := url.Parse(a.BaseURL)
	if err != nil {
		return nil, AuthServiceError.Wrap(err)
	}

	reqURL.Path = path.Join(reqURL.Path, "/v1/access", accessKeyID)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL.String(), nil)
	if err != nil {
		return nil, AuthServiceError.Wrap(err)
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, AuthServiceError.Wrap(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, AuthServiceError.New("invalid status code: %d", resp.StatusCode)
	}

	var authResp AuthServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, AuthServiceError.Wrap(err)
	}

	return &authResp, nil
}
