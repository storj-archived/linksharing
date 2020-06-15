// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package testsuite

import (
	"net/http"
	"net/http/httptest"
	"path"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"storj.io/common/testcontext"
	"storj.io/linksharing/linksharing"
	"storj.io/storj/private/testplanet"
)

func TestNewHandler(t *testing.T) {
	ctx := testcontext.New(t)
	defer ctx.Cleanup()

	testCases := []struct {
		name   string
		config linksharing.HandlerConfig
		err    string
	}{
		{
			name: "URL base must be http or https",
			config: linksharing.HandlerConfig{
				URLBase: "gopher://chunks",
			},
			err: "URL base must be http:// or https://",
		},
		{
			name: "URL base must contain host",
			config: linksharing.HandlerConfig{
				URLBase: "http://",
			},
			err: "URL base must contain host",
		},
		{
			name: "URL base can have a port",
			config: linksharing.HandlerConfig{
				URLBase: "http://host:99",
			},
		},
		{
			name: "URL base can have a path",
			config: linksharing.HandlerConfig{
				URLBase: "http://host/gopher",
			},
		},
		{
			name: "URL base must not contain user info",
			config: linksharing.HandlerConfig{
				URLBase: "http://joe@host",
			},
			err: "URL base must not contain user info",
		},
		{
			name: "URL base must not contain query values",
			config: linksharing.HandlerConfig{
				URLBase: "http://host/?gopher=chunks",
			},
			err: "URL base must not contain query values",
		},
		{
			name: "URL base must not contain a fragment",
			config: linksharing.HandlerConfig{
				URLBase: "http://host/#gopher-chunks",
			},
			err: "URL base must not contain a fragment",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {

			handler, err := linksharing.NewHandler(zaptest.NewLogger(t), testCase.config)
			if testCase.err != "" {
				require.EqualError(t, err, testCase.err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, handler)
		})
	}
}

func TestHandlerRequests(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount:   2,
		StorageNodeCount: 1,
		UplinkCount:      1,
	}, testHandlerRequests)
}

func testHandlerRequests(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
	err := planet.Uplinks[0].Upload(ctx, planet.Satellites[0], "testbucket", "test/foo", []byte("FOO"))
	require.NoError(t, err)

	access := planet.Uplinks[0].Access[planet.Satellites[0].ID()]
	serializedAccess, err := access.Serialize()
	require.NoError(t, err)

	testCases := []struct {
		name   string
		method string
		path   string
		status int
		header http.Header
		body   string
	}{
		{
			name:   "invalid method",
			method: "PUT",
			status: http.StatusMethodNotAllowed,
			body:   "method not allowed\n",
		},
		{
			name:   "GET missing access",
			method: "GET",
			status: http.StatusBadRequest,
			body:   "invalid request: missing access\n",
		},
		{
			name:   "GET malformed access",
			method: "GET",
			path:   path.Join("BADACCESS", "testbucket", "test/foo"),
			status: http.StatusBadRequest,
			body:   "invalid request: uplink: invalid access grant format\n",
		},
		{
			name:   "GET missing bucket",
			method: "GET",
			path:   serializedAccess,
			status: http.StatusBadRequest,
			body:   "invalid request: missing bucket\n",
		},
		{
			name:   "GET object not found",
			method: "GET",
			path:   path.Join(serializedAccess, "testbucket", "test/bar"),
			status: http.StatusNotFound,
			body:   "object not found\n",
		},
		{
			name:   "GET success",
			method: "GET",
			path:   path.Join(serializedAccess, "testbucket", "test/foo"),
			status: http.StatusOK,
			body:   "FOO",
		},
		{
			name:   "GET bucket listing success",
			method: "GET",
			path:   path.Join(serializedAccess, "testbucket/", ""),
			status: http.StatusOK,
			body:   "test/",
		},
		{
			name:   "GET prefix listing success",
			method: "GET",
			path:   path.Join(serializedAccess, "testbucket", "test") + "/",
			status: http.StatusOK,
			body:   "foo",
		},
		{
			name:   "HEAD missing access",
			method: "HEAD",
			status: http.StatusBadRequest,
			body:   "invalid request: missing access\n",
		},
		{
			name:   "HEAD malformed access",
			method: "HEAD",
			path:   path.Join("BADACCESS", "testbucket", "test/foo"),
			status: http.StatusBadRequest,
			body:   "invalid request: uplink: invalid access grant format\n",
		},
		{
			name:   "HEAD missing bucket",
			method: "HEAD",
			path:   serializedAccess,
			status: http.StatusBadRequest,
			body:   "invalid request: missing bucket\n",
		},
		{
			name:   "HEAD object not found",
			method: "HEAD",
			path:   path.Join(serializedAccess, "testbucket", "test/bar"),
			status: http.StatusNotFound,
			body:   "object not found\n",
		},
		{
			name:   "HEAD success",
			method: "HEAD",
			path:   path.Join(serializedAccess, "testbucket", "test/foo"),
			status: http.StatusFound,
			header: http.Header{
				"Location": []string{"http://localhost/" + path.Join(serializedAccess, "testbucket", "test/foo")},
			},
			body: "",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {

			handler, err := linksharing.NewHandler(zaptest.NewLogger(t), linksharing.HandlerConfig{
				URLBase: "http://localhost",
			})
			require.NoError(t, err)

			url := "http://localhost/" + testCase.path
			w := httptest.NewRecorder()
			r, err := http.NewRequest(testCase.method, url, nil)
			require.NoError(t, err)
			handler.ServeHTTP(w, r)

			assert.Equal(t, testCase.status, w.Code, "status code does not match")
			for h, v := range testCase.header {
				assert.Equal(t, v, w.Header()[h], "%q header does not match", h)
			}
			assert.Contains(t, w.Body.String(), testCase.body, "body does not match")
		})
	}
}
