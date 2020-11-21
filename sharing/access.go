// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"github.com/btcsuite/btcutil/base58"
	"github.com/zeebo/errs"

	"storj.io/uplink"
)

const versionAccessKeyID = 1 // we don't want to import stargate just for this

func parseAccess(access string, cfg AuthServiceConfig) (*uplink.Access, error) {
	// check if the serializedAccess is actually an access key id
	if _, version, err := base58.CheckDecode(access); err != nil {
		return nil, errs.New("invalid access")
	} else if version == versionAccessKeyID {
		authResp, err := cfg.Resolve(access)
		if err != nil {
			return nil, err
		}
		if !authResp.Public {
			return nil, errs.New("non-public access key id")
		}
		access = authResp.AccessGrant
	} else if version == 0 { // 0 could be any number of things, but we just assume an access
	} else {
		return nil, errs.New("invalid access version")
	}

	return uplink.ParseAccess(access)
}
