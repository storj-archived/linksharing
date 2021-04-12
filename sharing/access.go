// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"net/http"

	"github.com/btcsuite/btcutil/base58"
	"github.com/zeebo/errs"

	"storj.io/uplink"
)

func parseAccess(ctx context.Context, access string, cfg AuthServiceConfig) (_ *uplink.Access, err error) {
	defer mon.Task()(&ctx)(&err)
	wrappedParse := func(access string) (*uplink.Access, error) {
		parsed, err := uplink.ParseAccess(access)
		if err != nil {
			return nil, WithStatus(err, http.StatusBadRequest)
		}
		return parsed, nil
	}

	// production access grants are base58check encoded with version zero.
	if _, version, err := base58.CheckDecode(access); err == nil && version == 0 {
		return wrappedParse(access)
	}

	// otherwise, assume an access key.
	authResp, err := cfg.Resolve(ctx, access)
	if err != nil {
		return nil, err
	}
	if !authResp.Public {
		return nil, WithStatus(errs.New("non-public access key id"), http.StatusForbidden)
	}

	return wrappedParse(authResp.AccessGrant)
}
