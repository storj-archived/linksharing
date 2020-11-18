// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"sync"
	"time"

	"github.com/miekg/dns"

	"storj.io/uplink"
)

type txtRecords struct {
	maxTTL time.Duration
	dns    *DNSClient

	mu    sync.RWMutex
	cache map[string]txtRecord
}

type txtRecord struct {
	// TODO: storing the actual access grant in the cache saves us some
	// work and a request to the auth service, so that's nice. however,
	// by storing the actual access grant in the cache, the dns entry will
	// live until TTL *even though* the access key it used may have gotten
	// revoked sooner. this is a troubling problem for access keys, and
	// implies we should only support revoking access grants and not support
	// revoking access keys due to this confusion.
	access     *uplink.Access
	root       string
	expiration time.Time
}

func newTxtRecords(maxTTL time.Duration, dns *DNSClient) *txtRecords {
	return &txtRecords{maxTTL: maxTTL, dns: dns, cache: make(map[string]txtRecord)}
}

// fetchAccessForHost fetches the root and access grant from the cache or dns server when applicable.
func (records *txtRecords) fetchAccessForHost(ctx context.Context, hostname string) (access *uplink.Access, root string, err error) {
	record, exists := records.fromCache(hostname)
	if exists {
		return record.access, record.root, nil
	}

	access, root, ttl, err := records.queryAccessFromDNS(ctx, hostname)
	if err != nil {
		return access, root, err
	}
	records.updateCache(hostname, root, access, ttl)

	return access, root, err
}

// fromCache checks the txt record cache to see if we have a valid access grant and root path.
func (records *txtRecords) fromCache(hostname string) (record txtRecord, exists bool) {
	records.mu.RLock()
	defer records.mu.RUnlock()

	record, ok := records.cache[hostname]
	if ok && !record.expiration.Before(time.Now()) {
		return record, true
	}
	return record, false
}

// updateCache updates the txtRecord cache with the hostname and corresponding access, root, and time of update.
func (records *txtRecords) updateCache(hostname, root string, access *uplink.Access, ttl time.Duration) {
	records.mu.Lock()
	defer records.mu.Unlock()

	records.cache[hostname] = txtRecord{access: access, root: root, expiration: time.Now().Add(ttl)}
}

// queryAccessFromDNS does an txt record lookup for the hostname on the dns server.
func (records *txtRecords) queryAccessFromDNS(ctx context.Context, hostname string) (access *uplink.Access, root string, ttl time.Duration, err error) {
	r, err := records.dns.Lookup(ctx, "txt-"+hostname, dns.TypeTXT)
	if err != nil {
		return nil, "", 0, err
	}
	set := ResponseToTXTRecordSet(r)

	serializedAccess := set.Lookup("storj-access")
	if serializedAccess == "" {
		// backcompat
		serializedAccess = set.Lookup("storj-grant")
	}
	root = set.Lookup("storj-root")
	if root == "" {
		// backcompat
		root = set.Lookup("storj-path")
	}

	access, err = uplink.ParseAccess(serializedAccess)
	if err != nil {
		return nil, "", 0, err
	}

	return access, root, set.TTL(), nil
}
