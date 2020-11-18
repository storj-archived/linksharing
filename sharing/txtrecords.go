// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"storj.io/uplink"
)

type txtRecords struct {
	ttl time.Duration

	mu    sync.Mutex
	cache map[string]txtRecord
}

type txtRecord struct {
	access    *uplink.Access
	root      string
	timestamp time.Time
}

func newTxtRecords(ttl time.Duration) *txtRecords {
	return &txtRecords{ttl: ttl, cache: make(map[string]txtRecord)}
}

// fetchAccessForHost fetches the root and access grant from the cache or dns server when applicable.
func (records *txtRecords) fetchAccessForHost(ctx context.Context, hostname string) (access *uplink.Access, root string, err error) {
	record, exists := records.fromCache(hostname)
	if exists {
		return record.access, record.root, nil
	}

	access, root, err = queryAccessFromDNS(ctx, hostname)
	if err != nil {
		return access, root, err
	}
	records.updateCache(hostname, root, access)

	return access, root, err
}

// fromCache checks the txt record cache to see if we have a valid access grant and root path.
func (records *txtRecords) fromCache(hostname string) (record txtRecord, exists bool) {
	records.mu.Lock()
	defer records.mu.Unlock()

	record, ok := records.cache[hostname]
	if ok && !recordIsExpired(record, records.ttl) {
		return record, true
	}
	return record, false
}

// recordIsExpired checks whether an entry in the txtRecord cache is expired.
// A record is expired if its last timestamp plus the ttl was in the past.
func recordIsExpired(record txtRecord, ttl time.Duration) bool {
	return record.timestamp.Add(ttl).Before(time.Now())
}

// updateCache updates the txtRecord cache with the hostname and corresponding access, root, and time of update.
func (records *txtRecords) updateCache(hostname, root string, access *uplink.Access) {
	records.mu.Lock()
	defer records.mu.Unlock()

	records.cache[hostname] = txtRecord{access: access, root: root, timestamp: time.Now()}
}

// queryAccessFromDNS does an txt record lookup for the hostname on the dns server.
func queryAccessFromDNS(ctx context.Context, hostname string) (access *uplink.Access, root string, err error) {
	records, err := net.DefaultResolver.LookupTXT(ctx, hostname)
	if err != nil {
		return access, root, err
	}
	return parseRecords(records)
}

// parseRecords transforms the data from the hostname's external TXT records.
// For example, a hostname may have the following TXT records: "storj_grant-1:abcd", "storj_grant-2:efgh", "storj_root:mybucket/folder".
// parseRecords then will return serializedAccess="abcdefgh" and root="mybucket/folder".
func parseRecords(records []string) (access *uplink.Access, root string, err error) {
	grants := map[int]string{}
	for _, record := range records {
		r := strings.SplitN(record, ":", 2)
		if strings.HasPrefix(r[0], "storj_grant") {
			section := strings.Split(r[0], "-")
			key, err := strconv.Atoi(section[1])
			if err != nil {
				return access, root, err
			}
			grants[key] = r[1]
		} else if r[0] == "storj_root" {
			root = r[1]
		}
	}

	if root == "" {
		return access, root, errors.New("missing root path in txt record")
	}

	var serializedAccess string
	for i := 1; i <= len(grants); i++ {
		if grants[i] == "" {
			return access, root, errors.New("missing grants")
		}
		serializedAccess += grants[i]
	}
	access, err = uplink.ParseAccess(serializedAccess)
	return access, root, err
}
