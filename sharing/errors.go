// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"storj.io/linksharing/sharing/errdata"
)

type errSym int

const (
	errAction     errSym = 1
	errStatusCode errSym = 2
)

// WithAction annotates an error with an action. If err is nil, does nothing.
func WithAction(err error, action string) error {
	return errdata.Annotate(err, errAction, action)
}

// WithStatus annotates an error with a status. If err is nil, does nothing.
func WithStatus(err error, statusCode int) error {
	return errdata.Annotate(err, errStatusCode, statusCode)
}

// GetAction returns the most recent action annotation on the error.
// If none is found, defValue is returned instead.
func GetAction(err error, defValue string) string {
	if v, ok := errdata.Value(err, errAction).(string); ok {
		return v
	}
	return defValue
}

// GetStatus returns the most recent status code annotation on the error.
// If none is found, defValue is returned instead.
func GetStatus(err error, defValue int) int {
	if v, ok := errdata.Value(err, errStatusCode).(int); ok {
		return v
	}
	return defValue
}
