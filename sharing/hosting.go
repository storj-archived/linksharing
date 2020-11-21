// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"net"
	"net/http"
	"strings"
)

// handleHostingService deals with linksharing via custom URLs.
func (handler *Handler) handleHostingService(ctx context.Context, w http.ResponseWriter, r *http.Request) (err error) {
	defer mon.Task()(&ctx)(&err)

	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		if aerr, ok := err.(*net.AddrError); ok && aerr.Err == "missing port in address" {
			host = r.Host
		} else {
			return WithStatus(err, http.StatusBadRequest)
		}
	}

	access, root, err := handler.txtRecords.fetchAccessForHost(ctx, host)
	if err != nil {
		return WithAction(err, "fetch access")
	}

	bucket, key := determineBucketAndObjectKey(root, r.URL.Path)

	return handler.present(ctx, w, r, &parsedRequest{
		access:      access,
		bucket:      bucket,
		realKey:     key,
		visibleKey:  strings.TrimPrefix(r.URL.Path, "/"),
		title:       host,
		root:        breadcrumb{Prefix: host, URL: "/"},
		wrapDefault: false,
	})
}

// determineBucketAndObjectKey is a helper function to parse storj_root and the url into the bucket and object key.
// For example, we have http://mydomain.com/prefix2/index.html with storj_root:bucket1/prefix1/
// The root path will be [bucket1, prefix1/]. Our bucket is named bucket1.
// Since the url has a path of /prefix2/index.html and the second half of the root path is prefix1,
// we get an object key of prefix1/prefix2/index.html. To make this work, the first (and only the
// first) prefix slash from the URL is stripped. Additionally, to aid security, if there is a non-empty
// prefix, it will have a suffix slash added to it if no trailing slash exists. See
// TestDetermineBucketAndObjectKey for many examples.
func determineBucketAndObjectKey(root, urlPath string) (bucket, key string) {
	parts := strings.SplitN(root, "/", 2)
	bucket = parts[0]
	prefix := ""
	if len(parts) > 1 {
		prefix = parts[1]
	}
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return bucket, prefix + strings.TrimPrefix(urlPath, "/")
}
