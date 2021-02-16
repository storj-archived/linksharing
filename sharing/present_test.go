// Copyright (C) 2021 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"storj.io/linksharing/objectmap"
	"storj.io/uplink"
)

func TestDownloadContentTypeHeader(t *testing.T) {
	cfg := Config{
		URLBase:   "http://test.test",
		Templates: "../web",
	}

	handler, err := NewHandler(&zap.Logger{}, &objectmap.IPDB{}, cfg)
	require.NoError(t, err)

	ctx := context.Background()
	w := httptest.NewRecorder()
	r, err := http.NewRequest("GET", "http://test.test?download", nil)
	require.NoError(t, err)

	pr := &parsedRequest{}
	project := &uplink.Project{}
	object := &uplink.Object{
		Key: "test.jpg",
	}

	err = handler.showObject(ctx, w, r, pr, project, object)
	require.NoError(t, err)

	ctypes, haveType := w.Header()["Content-Type"]
	require.True(t, haveType)
	require.Equal(t, "image/jpeg", ctypes[0])

	object.Key = "test"

	err = handler.showObject(ctx, w, r, pr, project, object)
	require.NoError(t, err)

	ctypes, haveType = w.Header()["Content-Type"]
	require.True(t, haveType)
	require.Equal(t, "application/octet-stream", ctypes[0])
}
