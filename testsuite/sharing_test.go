// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package testsuite

import (
	"html/template"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"storj.io/common/testcontext"
	"storj.io/linksharing/console"
	"storj.io/linksharing/console/consoleapi"
	"storj.io/linksharing/objectmap"
)

func TestNewSharing(t *testing.T) {
	ctx := testcontext.New(t)
	defer ctx.Cleanup()

	mapper := objectmap.NewIPDB(&objectmap.MockReader{})

	t.Run("NewSharing test", func(t *testing.T) {
		listTmpl, err := template.ParseFiles("../static/templates/prefix-listing.html")
		require.NoError(t, err)
		require.NotNil(t, listTmpl)

		singleTmpl, err := template.ParseFiles("../static/templates/single-object.html")
		require.NoError(t, err)
		require.NotNil(t, singleTmpl)

		errorTmpl, err := template.ParseFiles("../static/templates/404.html")
		require.NoError(t, err)
		require.NotNil(t, errorTmpl)

		templates := consoleapi.SharingTemplates{
			List:         listTmpl,
			SingleObject: singleTmpl,
			NotFound:     errorTmpl,
		}

		service, err := console.NewService(zaptest.NewLogger(t), mapper)
		require.NoError(t, err)
		require.NotNil(t, service)

		sharing := consoleapi.NewSharing(zaptest.NewLogger(t), service, templates)
		require.NotNil(t, sharing)
	})
}
