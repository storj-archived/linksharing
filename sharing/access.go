// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"net/http"

	"github.com/btcsuite/btcutil/base58"
	"github.com/zeebo/errs"

	"storj.io/uplink"
)

const versionAccessKeyID = 1 // we don't want to import stargate just for this

func parseAccess(access string, cfg AuthServiceConfig) (*uplink.Access, error) {
	// check if the serializedAccess is actually an access key id
	if _, version, err := base58.CheckDecode(access); err != nil {
		return nil, WithStatus(errs.New("invalid access"), http.StatusBadRequest)
	} else if version == versionAccessKeyID {
		authResp, err := cfg.Resolve(access)
		if err != nil {
			return nil, err
		}
		if !authResp.Public {
			return nil, WithStatus(errs.New("non-public access key id"), http.StatusForbidden)
		}
		access = authResp.AccessGrant
	} else if version == 0 { // 0 could be any number of things, but we just assume an access
	} else {
		return nil, WithStatus(errs.New("invalid access version"), http.StatusBadRequest)
	}

	parsed, err := uplink.ParseAccess(access)
	if err != nil {
		return nil, WithStatus(err, http.StatusBadRequest)
	}

	return parsed, nil
}
