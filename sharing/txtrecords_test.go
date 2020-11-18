// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"storj.io/uplink"
)

func TestParseRecords(t *testing.T) {
	access58 := `12edqwjdy4fmoHasYrxLzmu8Ubv8Hsateq1LPYne6Jzd64qCsYgET53eJzhB4L2pWDKBpqMowxt8vqLCbYxu8Qz6u6dZWTdzbeiohfj46CUU8fTetqPdGcNu7dyNdEzvV6UibnsAhnvEbiKTteKdbJurb8PVGHV4vahsMy6CWm8mZUgXEUVu3jLLpcmQLhgEpqGuEpoygb186MggAseeoimP5pvPPjBZxt9GTTAhiJ9WsGW1NeFMSazSevS`
	expectedAccess, err := uplink.ParseAccess(access58)
	require.NoError(t, err)

	records := []string{
		"storj_grant-2:" + access58[len(access58)/2:],
		"storj_grant-1:" + access58[:len(access58)/2],
		"storj_root:linkshare/test",
	}

	access, serializedAccess, root, err := parseRecords(records)
	require.NoError(t, err)

	assert.Equal(t, expectedAccess, access)
	assert.Equal(t, serializedAccess, access58)
	assert.Equal(t, "linkshare/test", root)
}

func TestParseRecords_Invalid(t *testing.T) {
	access58 := `12edqwjdy4fmoHasYrxLzmu8Ubv8Hsateq1LPYne6Jzd64qCsYgET53eJzhB4L2pWDKBpqMowxt8vqLCbYxu8Qz6u6dZWTdzbeiohfj46CUU8fTetqPdGcNu7dyNdEzvV6UibnsAhnvEbiKTteKdbJurb8PVGHV4vahsMy6CWm8mZUgXEUVu3jLLpcmQLhgEpqGuEpoygb186MggAseeoimP5pvPPjBZxt9GTTAhiJ9WsGW1NeFMSazSevS`

	half1 := "storj_grant-1:" + access58[:len(access58)/2]
	half2 := "storj_grant-2:" + access58[len(access58)/2:]
	invalidRecords := [][]string{
		{"storj_root:linkshare/test", half1},
		{"storj_root:linkshare/test", half2},
		{half1, half2},
		{"storj_root:linkshare/test", half1, half2, "storj_grant-3:xxx"},
		{"storj_root:linkshare/test", half1, half2, "storj_grant-3:"},
	}

	for _, records := range invalidRecords {
		_, _, _, err := parseRecords(records)
		assert.Error(t, err, records)
	}
}
