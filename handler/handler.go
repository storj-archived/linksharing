// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package handler

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/spacemonkeygo/monkit/v3"
	"go.uber.org/zap"

	"storj.io/common/memory"
	"storj.io/common/ranger"
	"storj.io/common/ranger/httpranger"
	"storj.io/linksharing/objectmap"
	"storj.io/uplink"
	"storj.io/uplink/private/object"
)

var (
	mon = monkit.Package()
)

// HandlerConfig specifies the handler configuration.
type HandlerConfig struct {
	// URLBase is the base URL of the link sharing handler. It is used
	// to construct URLs returned to clients. It should be a fully formed URL.
	URLBase string

	// Templates location with html templates.
	Templates string
}

// Location represents geographical points
// in the globe.
type Location struct {
	Latitude  float64
	Longitude float64
}

// Handler implements the link sharing HTTP handler.
//
// architecture: Service
type Handler struct {
	log       *zap.Logger
	urlBase   *url.URL
	templates *template.Template
	mapper    *objectmap.IPDB
}

// NewHandler creates a new link sharing HTTP handler.
func NewHandler(log *zap.Logger, mapper *objectmap.IPDB, config HandlerConfig) (*Handler, error) {
	urlBase, err := parseURLBase(config.URLBase)
	if err != nil {
		return nil, err
	}

	if config.Templates == "" {
		config.Templates = "./templates/*.html"
	}
	templates, err := template.ParseGlob(config.Templates)
	if err != nil {
		return nil, err
	}

	return &Handler{
		log:       log,
		urlBase:   urlBase,
		templates: templates,
		mapper:    mapper,
	}, nil
}

// TODO: i could assume that it is a business logic layer, so we should remove transport and server from here.

// ServeHTTP handles link sharing requests.
func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// serveHTTP handles the request in full. the error that is returned can
	// be ignored since it was only added to facilitate monitoring.
	_ = handler.serveHTTP(w, r)
}

func (handler *Handler) serveHTTP(w http.ResponseWriter, r *http.Request) (err error) {
	ctx := r.Context()
	defer mon.Task()(&ctx)(&err)

	locationOnly := false

	switch r.Method {
	case http.MethodHead:
		locationOnly = true
	case http.MethodGet:
	default:
		err = errors.New("method not allowed")
		http.Error(w, err.Error(), http.StatusMethodNotAllowed)
		return err
	}

	access, serializedAccess, bucket, key, err := parseRequestPath(r.URL.Path)
	if err != nil {
		err = fmt.Errorf("invalid request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return err
	}

	p, err := uplink.OpenProject(ctx, access)
	if err != nil {
		handler.handleUplinkErr(w, "open project", err)
		return err
	}
	defer func() {
		if err := p.Close(); err != nil {
			handler.log.With(zap.Error(err)).Warn("unable to close project")
		}
	}()

	if key == "" || strings.HasSuffix(key, "/") {
		if !strings.HasSuffix(r.URL.Path, "/") {
			// redirect because directories must have a trailing '/' for the listed hyperlinks to generate correctly
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return nil
		}
		err = handler.servePrefix(ctx, w, p, serializedAccess, bucket, key)
		if err != nil {
			handler.handleUplinkErr(w, "list prefix", err)
		}
		return nil
	}

	o, err := p.StatObject(ctx, bucket, key)
	if err != nil {
		handler.handleUplinkErr(w, "stat object", err)
		return err
	}

	if locationOnly {
		location := makeLocation(handler.urlBase, r.URL.Path)
		http.Redirect(w, r, location, http.StatusFound)
		return nil
	}

	_, download := r.URL.Query()["download"]
	_, view := r.URL.Query()["view"]
	if !download && !view {
		ipBytes, err := object.GetObjectIPs(ctx, uplink.Config{}, access, bucket, key)
		if err != nil {
			handler.handleUplinkErr(w, "get object IPs", err)
			return err
		}

		var locations []Location
		for _, ip := range ipBytes {
			info, err := handler.mapper.GetIPInfos(string(ip))
			if err != nil {
				handler.log.Error("failed to get IP info", zap.Error(err))
				continue
			}

			location := Location{
				Latitude:  info.Location.Latitude,
				Longitude: info.Location.Longitude,
			}

			locations = append(locations, location)
		}

		var input struct {
			Name      string
			Size      string
			Locations []Location
			Pieces    int64
		}
		input.Name = o.Key
		input.Size = memory.Size(o.System.ContentLength).Base10String()
		input.Locations = locations
		input.Pieces = int64(len(locations))

		return handler.templates.ExecuteTemplate(w, "single-object.html", input)
	}

	if download {
		segments := strings.Split(key, "/")
		object := segments[len(segments)-1]
		w.Header().Set("Content-Disposition", "attachment; filename=\""+object+"\"")
	}
	httpranger.ServeContent(ctx, w, r, key, o.System.Created, newObjectRanger(p, o, bucket))
	return nil
}

func (handler *Handler) servePrefix(ctx context.Context, w http.ResponseWriter, project *uplink.Project, serializedAccess string, bucket, prefix string) (err error) {
	type Item struct {
		Name   string
		Size   string
		Prefix bool
	}

	type Breadcrumb struct {
		Prefix string
		URL    string
	}

	var input struct {
		Bucket      string
		Breadcrumbs []Breadcrumb
		Items       []Item
	}
	input.Bucket = bucket
	input.Breadcrumbs = append(input.Breadcrumbs, Breadcrumb{
		Prefix: bucket,
		URL:    serializedAccess + "/" + bucket + "/",
	})
	if prefix != "" {
		trimmed := strings.TrimRight(prefix, "/")
		for i, prefix := range strings.Split(trimmed, "/") {
			input.Breadcrumbs = append(input.Breadcrumbs, Breadcrumb{
				Prefix: prefix,
				URL:    input.Breadcrumbs[i].URL + "/" + prefix + "/",
			})
		}
	}

	input.Items = make([]Item, 0)

	objects := project.ListObjects(ctx, bucket, &uplink.ListObjectsOptions{
		Prefix: prefix,
		System: true,
	})

	// TODO add paging
	for objects.Next() {
		item := objects.Item()
		name := item.Key[len(prefix):]
		input.Items = append(input.Items, Item{
			Name:   name,
			Size:   memory.Size(item.System.ContentLength).Base10String(),
			Prefix: item.IsPrefix,
		})
	}
	if objects.Err() != nil {
		return objects.Err()
	}

	return handler.templates.ExecuteTemplate(w, "prefix-listing.html", input)
}

func (handler *Handler) handleUplinkErr(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, uplink.ErrBucketNotFound):
		w.WriteHeader(http.StatusNotFound)
		err = handler.templates.ExecuteTemplate(w, "404.html", "Oops! Bucket not found.")
		if err != nil {
			handler.log.Error("error while executing template", zap.Error(err))
		}
	case errors.Is(err, uplink.ErrObjectNotFound):
		w.WriteHeader(http.StatusNotFound)
		err = handler.templates.ExecuteTemplate(w, "404.html", "Oops! Object not found.")
		if err != nil {
			handler.log.Error("error while executing template", zap.Error(err))
		}
	default:
		handler.log.Error("unable to handle request", zap.String("action", action), zap.Error(err))
		http.Error(w, "unable to handle request", http.StatusInternalServerError)
	}
}

func parseRequestPath(p string) (_ *uplink.Access, serializedAccess, bucket, key string, err error) {
	// Drop the leading slash, if necessary
	p = strings.TrimPrefix(p, "/")

	// Split the request path
	segments := strings.SplitN(p, "/", 3)
	if len(segments) == 1 {
		if segments[0] == "" {
			return nil, "", "", "", errors.New("missing access")
		}
		return nil, "", "", "", errors.New("missing bucket")
	}

	serializedAccess = segments[0]
	bucket = segments[1]

	if len(segments) == 3 {
		key = segments[2]
	}

	access, err := uplink.ParseAccess(serializedAccess)
	if err != nil {
		return nil, "", "", "", err
	}
	return access, serializedAccess, bucket, key, nil
}

type objectRanger struct {
	p      *uplink.Project
	o      *uplink.Object
	bucket string
}

func newObjectRanger(p *uplink.Project, o *uplink.Object, bucket string) ranger.Ranger {
	return &objectRanger{
		p:      p,
		o:      o,
		bucket: bucket,
	}
}

func (ranger *objectRanger) Size() int64 {
	return ranger.o.System.ContentLength
}

func (ranger *objectRanger) Range(ctx context.Context, offset, length int64) (_ io.ReadCloser, err error) {
	defer mon.Task()(&ctx)(&err)
	return ranger.p.DownloadObject(ctx, ranger.bucket, ranger.o.Key, &uplink.DownloadOptions{Offset: offset, Length: length})
}

func parseURLBase(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return nil, errors.New("URL base must be http:// or https://")
	case u.Host == "":
		return nil, errors.New("URL base must contain host")
	case u.User != nil:
		return nil, errors.New("URL base must not contain user info")
	case u.RawQuery != "":
		return nil, errors.New("URL base must not contain query values")
	case u.Fragment != "":
		return nil, errors.New("URL base must not contain a fragment")
	}
	return u, nil
}

func makeLocation(base *url.URL, reqPath string) string {
	location := *base
	location.Path = path.Join(location.Path, reqPath)
	return location.String()
}
