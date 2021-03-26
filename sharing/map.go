// Copyright (C) 2021 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"storj.io/dotworld"
	"storj.io/dotworld/reference"
	"storj.io/uplink/private/object"
)

type location struct {
	Latitude  float64
	Longitude float64
}

func (handler *Handler) getLocations(ctx context.Context, pr *parsedRequest) (locs []location, pieceCount int64, err error) {
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

func (handler *Handler) serveMap(ctx context.Context, w http.ResponseWriter, pr *parsedRequest, width int) (err error) {
	defer mon.Task()(&ctx)(&err)

	locations, pieces, err := handler.getLocations(ctx, pr)
	if err != nil {
		return err
	}

	m := reference.WorldMap()

	for _, loc := range locations {
		if nearest := m.Nearest(m.Lookup(dotworld.S2{
			Lat:  float32(loc.Latitude),
			Long: float32(loc.Longitude),
		})); nearest != nil {
			nearest.Load += 1.0 / float32(pieces)
		}
	}

	w.Header().Set("Content-Type", "image/svg+xml")

	var buf bytes.Buffer
	err = m.EncodeSVG(&buf, width, width/2)
	if err != nil {
		return WithAction(err, "svg encode")
	}

	data := buf.Bytes()
	if pieces == 0 {
		data = bytes.Replace(data, []byte("</svg>"), []byte(
			`<text x="50%" y="85%" dominant-baseline="middle" text-anchor="middle"
	    style="font-family:Poppins,sans-serif;font-size:18px;fill:#6c757d;fill-opacity:1;">
	    Files under 4k are stored as metadata with strong encryption.
	  </text>
	</svg>`), 1)
	}

	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	_, err = w.Write(data)
	return err
}
