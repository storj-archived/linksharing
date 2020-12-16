// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"net/url"
	"strings"
)

// queryFlagLookup finds a boolean value in a url.Values struct, returning
// defValue if not found.
//  * no flag is the default value
//  * ?flag is assumed true
//  * ?flag=no (or false or 0 or off) is assumed false (case insensitive)
//  * everything else is true
func queryFlagLookup(q url.Values, name string, defValue bool) bool {
	vals, ok := q[name]
	if !ok || len(vals) == 0 {
		// the flag wasn't specified
		return defValue
	}
	val := vals[0]
	if len(val) == 0 {
		// the flag was specified, but no value was provided. must be form of
		// ?flag or ?flag= but no value. assume that means on.
		return true
	}
	switch strings.ToLower(val) {
	case "no", "false", "0", "off":
		// cases where the flag is false
		return false
	}
	return true
}
