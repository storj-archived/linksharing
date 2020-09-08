// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package linksharing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"text/template"

	"github.com/spacemonkeygo/monkit/v3"
	"go.uber.org/zap"

	"storj.io/common/memory"
	"storj.io/common/ranger"
	"storj.io/common/ranger/httpranger"
	"storj.io/uplink"
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

// Handler implements the link sharing HTTP handler.
type Handler struct {
	log       *zap.Logger
	urlBase   *url.URL
	templates *template.Template
	txtRecordCache map[string]map[string]string
}

// NewHandler creates a new link sharing HTTP handler.
func NewHandler(log *zap.Logger, config HandlerConfig) (*Handler, error) {

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
	var cache = map[string]map[string]string{}

	return &Handler{
		log:       log,
		urlBase:   urlBase,
		templates: templates,
		txtRecordCache: cache,
	}, nil
}

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
	path := strings.TrimPrefix(r.URL.Path, "/")
	segments := strings.SplitN(r.URL.Path, "/", 3)
	if segments[1] == "host" { // TODO: unsure if this will be segments[0] when deployed and pointing to linkshare.tardigrade.io/host
		err = handler.handleHostingService(ctx,w,r,segments)
	} else {
		err = handler.handleTraditional(ctx, w, r, path, locationOnly)
	}
	return err
}

func (handler *Handler) handleHostingService(ctx context.Context, w http.ResponseWriter, r *http.Request, segments []string) error {
	// http://ls.jenlij.com:8080/host/

	var serializedAccess, root string
	var grant1, grant2 string
	// check cache for access and directory
	record := handler.txtRecordCache["ls.jenlij.com"] // TODO update with r.Host
	if record != nil {
		serializedAccess = record["access"]
		root = record["dir"]
	} else {
		records, _ := net.LookupTXT("ls.jenlij.com") // TODO update with r.Host
		for _, record := range records {
			key := strings.SplitN(record, ":", 2)
			switch key[0] {
			case "storj-grant1":
				grant1 = key[1]
			case "storj-grant2":
				grant2 = key[1]
			case "storj-path1":
				root = key[1]
			default:
				continue
			}
		}
		if grant1 == "" || grant2 == "" || root == "" {
			return errors.New("missing access or root path in txt record")
		}
		serializedAccess = grant1 + grant2
		// update cache
		handler.txtRecordCache["ls.jenlij.com"] = map[string]string {
			"access": serializedAccess,
			"dir": root,
		}
	}
	file := segments[2] // TODO: unsure if this will be segments[1]
	access, err := uplink.ParseAccess(serializedAccess)
	if err != nil {
		return err
	}
	project, err := uplink.OpenProject(ctx, access)
	if err != nil {
		handler.handleUplinkErr(w, "open project", err)
		return err
	}
	defer func() {
		if err := project.Close(); err != nil {
			handler.log.With(zap.Error(err)).Warn("unable to close project")
		}
	}()
	if file == "" || strings.HasSuffix(file, "/") {
		file = "index.html"
	}
	o, err := project.StatObject(ctx, root, file)
	if err != nil {
		handler.handleUplinkErr(w, "stat object", err)
		return err
	}
	httpranger.ServeContent(ctx, w, r, file, o.System.Created, newObjectRanger(project, o, root))
	return nil
}

func (handler *Handler) handleTraditional(ctx context.Context, w http.ResponseWriter, r *http.Request, path string, locationOnly bool) error{
	access, serializedAccess, bucket, key, err := parseRequestPath(path)
	if err != nil {
		err = fmt.Errorf("invalid request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return err
	}
	project, err := uplink.OpenProject(ctx, access)
	if err != nil {
		handler.handleUplinkErr(w, "open project", err)
		return err
	}
	defer func() {
		if err := project.Close(); err != nil {
			handler.log.With(zap.Error(err)).Warn("unable to close project")
		}
	}()

	if key == "" || strings.HasSuffix(key, "/") {
		err = handler.servePrefix(ctx, w, project, serializedAccess, bucket, key)
		if err != nil {
			handler.handleUplinkErr(w, "list prefix", err)
		}
		return nil
	}

	o, err := project.StatObject(ctx, bucket, key)
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
		var input struct {
			Name string
			Size string
		}
		input.Name = bucket + "/" + o.Key
		input.Size = memory.Size(o.System.ContentLength).Base10String()

		return handler.templates.ExecuteTemplate(w, "single-object.html", input)
	}

	if download {
		segments := strings.Split(key, "/")
		object := segments[len(segments)-1]
		w.Header().Set("Content-Disposition", "attachment; filename=\""+object+"\"")
	}
	httpranger.ServeContent(ctx, w, r, key, o.System.Created, newObjectRanger(project, o, bucket))
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
