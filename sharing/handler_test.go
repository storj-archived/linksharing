// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"storj.io/common/testcontext"
	"storj.io/storj/private/testplanet"
)

func TestParseRecords(t *testing.T) {
	ctx := testcontext.New(t)
	defer ctx.Cleanup()
	testplanet.Run(t, testplanet.Config{
		SatelliteCount:   1,
		StorageNodeCount: 1,
		UplinkCount:      1,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		access := planet.Uplinks[0].Access[planet.Satellites[0].ID()]
		serializedAccess, err := access.Serialize()
		require.NoError(t, err)
		midpoint := len(serializedAccess) / 2
		r1 := serializedAccess[:midpoint]
		r2 := serializedAccess[midpoint:]
		records := []string{"storj_grant-2:" + r2, "storj_root:linkshare/test", "storj_grant-1:" + r1}
		parsedAccess, root, err := parseRecords(records)
		require.NoError(t, err)
		assert.Equal(t, access, parsedAccess)
		assert.Equal(t, "linkshare/test", root)
	})
}