// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package linksharing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecords(t *testing.T) {
	records := []string{"storj_grant-2:grant2", "storj_root:linkshare/test", "storj_grant-1:grant1"}
	serializedAccess, root, err := parseRecords(records)
	require.NoError(t, err)
	assert.Equal(t, "grant1grant2", serializedAccess)
	assert.Equal(t, "linkshare/test", root)
}
