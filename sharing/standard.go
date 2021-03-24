// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/uplink/private/object"
)

func (handler *Handler) handleStandard(ctx context.Context, w http.ResponseWriter, r *http.Request) (err error) {
	defer mon.Task()(&ctx)(&err)

	var pr parsedRequest
	path := strings.TrimPrefix(r.URL.Path, "/")
	switch {
	case strings.HasPrefix(path, "raw/"): // raw - just render the file
		path = path[len("raw/"):]
		pr.wrapDefault = false
	case strings.HasPrefix(path, "s/"): // wrap the file with a nice frame
		path = path[len("s/"):]
		pr.wrapDefault = true
	default: // backwards compatibility
		// preserve query params
		destination := (&url.URL{Path: "/s/" + path, RawQuery: r.URL.RawQuery}).String()
		http.Redirect(w, r, destination, http.StatusSeeOther)
		return
	}

	var serializedAccess string
	parts := strings.SplitN(path, "/", 3)
	switch len(parts) {
	case 0:
		return errs.New("unreachable")
	case 1:
		if parts[0] == "" {
			return WithStatus(errs.New("missing access"), http.StatusBadRequest)
		}
		return WithStatus(errs.New("missing bucket"), http.StatusBadRequest)
	case 2:
		serializedAccess = parts[0]
		pr.bucket = parts[1]
	default:
		serializedAccess = parts[0]
		pr.bucket = parts[1]
		pr.realKey = parts[2]
	}

	access, err := parseAccess(ctx, serializedAccess, handler.authConfig)
	if err != nil {
		return err
	}

	pr.access = access

	q := r.URL.Query()

	if queryFlagLookup(q, "map", false) {
		return handler.serveMap(ctx, w, pr, queryIntLookup(q, "width", 800))
	}

	pr.visibleKey = pr.realKey
	pr.title = pr.bucket
	pr.root = breadcrumb{Prefix: pr.bucket, URL: "/s/" + serializedAccess + "/" + pr.bucket + "/"}

	return handler.present(ctx, w, r, &pr)
}

type location struct {
	Latitude  float64
	Longitude float64
}

func (handler *Handler) getLocations(ctx context.Context, pr parsedRequest) (locs []location, pieceCount int64, err error) {
	defer mon.Task()(&ctx)(&err)

	ipSummary, err := object.GetObjectIPSummary(ctx, *handler.uplink, pr.access, pr.bucket, pr.realKey)
	if err != nil {
		return nil, 0, WithAction(err, "get locations")
	}

	// we explicitly don't want locations to be nil, so it doesn't
	// render as null when we plop it into the output javascript.
	locations := make([]location, 0, len(ipSummary.IPPorts))
	if handler.mapper != nil {
		for _, ip := range ipSummary.IPPorts {
			info, err := handler.mapper.GetIPInfos(ctx, string(ip))
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

	return locations, ipSummary.PieceCount, nil
}
