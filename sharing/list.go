// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"net/http"
	"strings"

	"storj.io/common/memory"
	"storj.io/uplink"
)

type breadcrumb struct {
	Prefix string
	URL    string
}

func (handler *Handler) servePrefix(ctx context.Context, w http.ResponseWriter, project *uplink.Project, root breadcrumb, title, bucket, realPrefix, visiblePrefix string) (err error) {
	type Object struct {
		Key    string
		Size   string
		Prefix bool
	}

	var input struct {
		Title       string
		Breadcrumbs []breadcrumb
		Objects     []Object
	}
	input.Title = title
	input.Breadcrumbs = append(input.Breadcrumbs, root)
	if visiblePrefix != "" {
		trimmed := strings.TrimRight(visiblePrefix, "/")
		for i, prefix := range strings.Split(trimmed, "/") {
			input.Breadcrumbs = append(input.Breadcrumbs, breadcrumb{
				Prefix: prefix,
				URL:    input.Breadcrumbs[i].URL + prefix + "/",
			})
		}
	}

	input.Objects = make([]Object, 0)

	objects := project.ListObjects(ctx, bucket, &uplink.ListObjectsOptions{
		Prefix: realPrefix,
		System: true,
	})

	// TODO add paging
	for objects.Next() {
		item := objects.Item()
		key := item.Key[len(realPrefix):]
		input.Objects = append(input.Objects, Object{
			Key:    key,
			Size:   memory.Size(item.System.ContentLength).Base10String(),
			Prefix: item.IsPrefix,
		})
	}
	if objects.Err() != nil {
		return objects.Err()
	}

	return handler.templates.ExecuteTemplate(w, "prefix-listing.html", input)
}
