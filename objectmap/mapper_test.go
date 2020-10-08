// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package objectmap

import (
	"errors"
	"github.com/zeebo/assert"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

type MockReader struct{}

func (mr *MockReader) Lookup(ip net.IP, result interface{}) error {

	// Valid geolocation case
	if ip.Equal(net.IPv4(172, 146, 10, 1)) {
		result.(*IPInfo).Location = mockIPInfo(-19.456, 20.123).Location
		return nil
	}
	// Location not found
	if ip.Equal(net.IPv4(1, 1, 1, 1)) {
		return errors.New("Not found")
	}
	return nil
}

func (mr *MockReader) Close() error {
	return nil
}

func mockIPInfo(latitude, longitude float64) *IPInfo {
	return &IPInfo{
		Location: struct {
			Latitude  float64 `maxminddb:"latitude"`
			Longitude float64 `maxminddb:"longitude"`
		}{
			Latitude:  latitude,
			Longitude: longitude,
		},
	}
}
func TestIPDB_GetIPInfos(t *testing.T) {

	mockReader := &MockReader{}

	tests := []struct {
		name        string
		reader      *MockReader
		ipAddress   string
		expected    *IPInfo
		expectedErr bool
	}{
		{"invalid IP", mockReader, "999.999.999.999", nil, true},
		{"invalid (IP:PORT)", mockReader, "999.999.999.999:42", nil, true},
		{"valid IP found geolocation", mockReader, "172.146.10.1", mockIPInfo(-19.456, 20.123), false},
		{"valid (IP:PORT) found geolocation", mockReader, "172.146.10.1:4545", mockIPInfo(-19.456, 20.123), false},
		{"valid IP geolocation not found", mockReader, "1.1.1.1", &IPInfo{}, true},
		{"valid (IP:PORT) geolocation not found", mockReader, "1.1.1.1:1000", &IPInfo{}, true},
	}
	for _, tt := range tests {
		mapper := &IPDB{
			reader: tt.reader,
		}
		testCase := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapper.GetIPInfos(testCase.ipAddress)

			if testCase.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.EqualValues(t, testCase.expected, got)
		})
	}
}

func Test_ASD(t *testing.T) {
	asd, err := net.LookupHost("storj2.finnet.co.uk")
	assert.NoError(t, err)
	assert.NotNil(t, asd)
}
