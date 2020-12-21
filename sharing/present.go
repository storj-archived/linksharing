// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"storj.io/common/memory"
	"storj.io/common/ranger/httpranger"
	"storj.io/linksharing/objectranger"
	"storj.io/uplink"
	"storj.io/uplink/private/object"
)

type parsedRequest struct {
	access          *uplink.Access
	bucket          string
	realKey         string
	visibleKey      string
	title           string
	root            breadcrumb
	wrapDefault     bool
	downloadDefault bool
}

func (handler *Handler) present(ctx context.Context, w http.ResponseWriter, r *http.Request, pr *parsedRequest) (err error) {
	defer mon.Task()(&ctx)(&err)

	project, err := uplink.OpenProject(ctx, pr.access)
	if err != nil {
		return WithAction(err, "open project")
	}
	defer func() {
		if err := project.Close(); err != nil {
			handler.log.With(zap.Error(err)).Warn("unable to close project")
		}
	}()

	if pr.realKey != "" { // there are no objects with the empty key
		o, err := project.StatObject(ctx, pr.bucket, pr.realKey)
		if err == nil {
			return handler.showObject(ctx, w, r, pr, project, o)
		}
		if !errors.Is(err, uplink.ErrObjectNotFound) {
			return WithAction(err, "stat object")
		}
		if !strings.HasSuffix(pr.realKey, "/") {
			objNotFoundErr := WithAction(err, "stat object")

			// s3 has interesting behavior, which is if the object doesn't exist,
			// but is a prefix, it will issue a redirect to have a trailing slash.
			// we need to do a brief list to find out if this object is a prefix.
			it := project.ListObjects(ctx, pr.bucket, &uplink.ListObjectsOptions{
				Prefix:    pr.realKey + "/",
				Recursive: true, // this is actually easier on the database if we don't page more than once
			})
			isPrefix := it.Next() // are there any objects with this prefix?
			err := it.Err()
			if err != nil {
				return WithAction(err, "prefix determination")
			}

			if isPrefix {
				http.Redirect(w, r, r.URL.Path+"/", http.StatusSeeOther)
				return nil
			}

			return objNotFoundErr
		}
	}

	// due to the above logic, by this point, the key is either exactly "" or ends in a "/"

	o, err := project.StatObject(ctx, pr.bucket, pr.realKey+"index.html")
	if err == nil {
		return handler.showObject(ctx, w, r, pr, project, o)
	}
	if !errors.Is(err, uplink.ErrObjectNotFound) {
		return WithAction(err, "stat object - index.html")
	}

	if !strings.HasSuffix(r.URL.Path, "/") {
		// Call redirect because directories must have a trailing '/' for the listed hyperlinks to generate correctly.
		http.Redirect(w, r, r.URL.Path+"/", http.StatusSeeOther)
		return nil
	}

	return handler.servePrefix(ctx, w, project, pr)
}

func (handler *Handler) showObject(ctx context.Context, w http.ResponseWriter, r *http.Request, pr *parsedRequest, project *uplink.Project, o *uplink.Object) (err error) {
	defer mon.Task()(&ctx)(&err)

	q := r.URL.Query()
	// if someone provides the 'download' flag on or off, we do that, otherwise
	// we do what the downloadDefault was (based on the URL scope).
	download := queryFlagLookup(q, "download", pr.downloadDefault)
	// if we're not downloading, and someone provides the 'wrap' flag on or off,
	// we do that. otherwise, we *don't* wrap if someone provided the view flag
	// on, otherwise we fall back to what wrapDefault was.
	wrap := queryFlagLookup(q, "wrap",
		!queryFlagLookup(q, "view", !pr.wrapDefault))

	if download {
		w.Header().Set("Content-Disposition", "attachment")
	}
	if download || !wrap {
		httpranger.ServeContent(ctx, w, r, o.Key, o.System.Created, objectranger.New(project, o, pr.bucket))
		return nil
	}

	ipBytes, err := object.GetObjectIPs(ctx, uplink.Config{}, pr.access, pr.bucket, o.Key)
	if err != nil {
		return WithAction(err, "get object IPs")
	}

	type location struct {
		Latitude  float64
		Longitude float64
	}

	// we explicitly don't want locations to be nil, so it doesn't
	// render as null when we plop it into the output javascript.
	locations := make([]location, 0, len(ipBytes))
	if handler.mapper != nil {
		for _, ip := range ipBytes {
			info, err := handler.mapper.GetIPInfos(string(ip))
			if err != nil {
				handler.log.Error("failed to get IP info", zap.Error(err))
				continue
			}

			locations = append(locations, location{
				Latitude:  info.Location.Latitude,
				Longitude: info.Location.Longitude,
			})
		}
	}

	var input struct {
		Key       string
		Size      string
		Locations []location
		Pieces    int64
	}
	input.Key = filepath.Base(o.Key)
	input.Size = memory.Size(o.System.ContentLength).Base10String()
	input.Locations = locations
	input.Pieces = int64(len(locations))

	handler.renderTemplate(w, "single-object.html", pageData{
		Data:  input,
		Title: pr.title,
	})
	return nil
}
