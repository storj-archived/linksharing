// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"storj.io/common/ranger/httpranger"
	"storj.io/linksharing/objectranger"
	"storj.io/uplink"
)

// handleHostingService deals with linksharing via custom URLs.
func (handler *Handler) handleHostingService(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil && strings.Contains(err.Error(), "missing port in address") {
		host = r.Host
	} else if err != nil {
		handler.log.Error("unable to handle request", zap.Error(err))
		http.Error(w, "unable to handle request", http.StatusInternalServerError)
		return err
	}

	access, root, err := handler.txtRecords.fetchAccessForHost(ctx, host)
	if err != nil {
		handler.log.Error("unable to handle request", zap.Error(err))
		http.Error(w, "unable to handle request", http.StatusInternalServerError)
		return err
	}

	project, err := uplink.OpenProject(ctx, access)
	if err != nil {
		handler.handleUplinkErr(w, "open project", err)
		return err
	}
	defer func() {
		if err := project.Close(); err != nil {
			handler.log.With(zap.Error(err)).Warn("unable to close project")
		}
	}()

	bucket, key := determineBucketAndObjectKey(root, r.URL.Path)
	if key != "" { // there are no objects with the empty key
		o, err := project.StatObject(ctx, bucket, key)
		if err == nil {
			// the requested key exists
			httpranger.ServeContent(ctx, w, r, key, o.System.Created, objectranger.New(project, o, bucket))
			return nil
		}
		if !strings.HasSuffix(key, "/") || !errors.Is(err, uplink.ErrObjectNotFound) {
			// the requested key does not end in a slash, or there was an unknown error
			handler.handleUplinkErr(w, "stat object", err)
			return err
		}
	}

	// due to the above logic, by this point, the key is either exactly "" or ends in a "/"

	k := key + "index.html"
	o, err := project.StatObject(ctx, bucket, k)
	if err == nil {
		httpranger.ServeContent(ctx, w, r, k, o.System.Created, objectranger.New(project, o, bucket))
		return nil
	}
	if !errors.Is(err, uplink.ErrObjectNotFound) {
		handler.handleUplinkErr(w, "stat object", err)
		return err
	}

	err = handler.servePrefix(ctx, w, project, breadcrumb{Prefix: host, URL: "/"}, host, bucket, key, strings.TrimPrefix(r.URL.Path, "/"))
	if err != nil {
		handler.handleUplinkErr(w, "list prefix", err)
		return err
	}
	return nil
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
