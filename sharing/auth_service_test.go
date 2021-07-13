// Copyright (C) 2021 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadUserRetry(t *testing.T) {
	firstAttempt := true
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if firstAttempt {
			firstAttempt = false
			return // writing nothing will cause an http.Client error
		}
		_, err := w.Write([]byte(`{"public":true, "secret_key":"", "access_grant":"ag"}`))
		require.NoError(t, err)
	}))
	asc := AuthServiceConfig{BaseURL: ts.URL, Token: "token"}
	asr, err := asc.Resolve(context.Background(), "fakeUser")
	require.NoError(t, err)
	require.Equal(t, "ag", asr.AccessGrant)
	require.False(t, firstAttempt)
}
