// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package linksharing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestParseRecords(t *testing.T){
	records := []string{"storj_grant-2:grant2", "storj_root:linkshare/test", "storj_grant-1:grant1", }
	serializedAccess, root, err := parseRecords(records)
	require.NoError(t, err)
	assert.Equal(t, "grant1grant2", serializedAccess)
	assert.Equal(t, "linkshare/test", root)
}

//temp test
func TestGetRootAndAccess(t *testing.T){
	handler, err := NewHandler(zaptest.NewLogger(t),HandlerConfig{
		Templates: "../templates/*.html",
		URLBase: "http://link.tardigradeshare.io",
		TxtRecordTTL: time.Second*2})
	require.NoError(t, err)
	access, root, err := handler.getRootAndAccess("ls.jenlij.com")
	require.NoError(t, err)
	assert.Equal(t, "1SHicNbHBVgsDPLxGR1JB1qNcsZHUrtsqhhYXbFgEB8tNTSpQYkHbZhcgJwFWkNvjPy5VkHEmnfezGXVtRLZVYaGAxiQFacrtv1jm28GLTPNqUoVNL2wwJ2y6UppKu7JuEqEA8QxGWVZYYAApM9M9D5BSh5M3knjq81P36tFYAVr5J4CEe7DwUoAzdpQcrDEqLMh1N5zqWnNsybrCuVsJzaKtu91pWCGNuZ6X1rGXSV9dTuuqJVKZpAjZZxFnH78RGcNFsocAb8KD43UkfSYy2QrokfmCPes64NwiMhybmcnZEHopQpJrurCVmXeTjKFCuNWvK9Wa6LC1S5", access)
	assert.Equal(t, "linkshare-test", root)
	x, ok := handler.txtRecords.cache["ls.jenlij.com"]
	assert.True(t, ok)
	assert.NotNil(t,x)
	assert.Equal(t, "1SHicNbHBVgsDPLxGR1JB1qNcsZHUrtsqhhYXbFgEB8tNTSpQYkHbZhcgJwFWkNvjPy5VkHEmnfezGXVtRLZVYaGAxiQFacrtv1jm28GLTPNqUoVNL2wwJ2y6UppKu7JuEqEA8QxGWVZYYAApM9M9D5BSh5M3knjq81P36tFYAVr5J4CEe7DwUoAzdpQcrDEqLMh1N5zqWnNsybrCuVsJzaKtu91pWCGNuZ6X1rGXSV9dTuuqJVKZpAjZZxFnH78RGcNFsocAb8KD43UkfSYy2QrokfmCPes64NwiMhybmcnZEHopQpJrurCVmXeTjKFCuNWvK9Wa6LC1S5", x.access)
	assert.Equal(t, "linkshare-test", x.root)

}