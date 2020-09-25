// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package objectmap

import (
	"net"

	"github.com/oschwald/maxminddb-golang"
	"github.com/zeebo/errs"
)

// Error is the default error class for objectmap.
var Error = errs.Class("objectmap error")

// IPInfo represents the geolocation data from maxmind db.
type IPInfo struct {
	Location struct {
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
	Postal struct {
		Code string `maxminddb:"code"`
	} `maxminddb:"postal"`
	Country struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

// Reader is a maxmind database reader interface.
type Reader interface {
	Lookup(ip net.IP, result interface{}) error
	Close() error
}

// IPDB holds the database file path and its reader.
type IPDB struct {
	reader Reader
}

// NewIPDB creates a new IPMapper instance.
func NewIPDB(dbPath string) (*IPDB, error) {
	reader, err := maxminddb.Open(dbPath)
	if err != nil {
		return nil, Error.Wrap(err)
	}

	return &IPDB{
		reader: reader,
	}, nil
}

// Close closes the IPMapper reader.
func (mapper *IPDB) Close() (err error) {
	if mapper.reader != nil {
		return mapper.reader.Close()
	}
	return nil
}

// ValidateIP validate and remove port from IP address.
func ValidateIP(ipAddress string) (net.IP, error) {

	ip, _, err := net.SplitHostPort(ipAddress)
	if err != nil {
		ip = ipAddress // assume it had no port
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, errs.New("invalid IP address: %s", ip)
	}
	return parsed, nil
}

// GetIPInfos returns the geolocation information from an IP address.
func (mapper *IPDB) GetIPInfos(ipAddress string) (_ *IPInfo, err error) {

	var record IPInfo
	parsed, err := ValidateIP(ipAddress)
	if err != nil {
		return nil, Error.Wrap(err)
	}

	err = mapper.reader.Lookup(parsed, &record)

	return &record, Error.Wrap(err)
}
