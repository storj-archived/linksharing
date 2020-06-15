// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package linksharing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"text/template"

	"github.com/spacemonkeygo/monkit/v3"
	"go.uber.org/zap"

	"storj.io/common/ranger"
	"storj.io/common/ranger/httpranger"
	"storj.io/uplink"
)

var (
	mon = monkit.Package()

	header = `
	<html lang="en">
	<head>
  		<meta charset="utf-8">
  		<title>Tardigrade Linksharing</title>
  		<meta name="description" content="Tardigrade Linksharing">
	</head>
	`
	footer = "</html>"

	prefixTmpl = template.Must(template.New("prefix").Parse(`
	<body>
	<h3>Bucket: {{.Bucket}}</h3>
	<h3>Prefix: {{.Prefix }}</h3>
    <ul>
        {{range .Items}}
            <li><a href="{{.}}">{{.}}</a></li>
        {{end}}
    </ul>
	</body>
	`))
)

// HandlerConfig specifies the handler configuration
type HandlerConfig struct {
	// URLBase is the base URL of the link sharing handler. It is used
	// to construct URLs returned to clients. It should be a fully formed URL.
	URLBase string
}

// Handler implements the link sharing HTTP handler
type Handler struct {
	log     *zap.Logger
	urlBase *url.URL
}

// NewHandler creates a new link sharing HTTP handler
func NewHandler(log *zap.Logger, config HandlerConfig) (*Handler, error) {

	urlBase, err := parseURLBase(config.URLBase)
	if err != nil {
		return nil, err
	}

	return &Handler{
		log:     log,
		urlBase: urlBase,
	}, nil
}

// ServeHTTP handles link sharing requests
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

	access, bucket, key, err := parseRequestPath(r.URL.Path)
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
		return handler.servePrefix(ctx, w, p, bucket, key)
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

	httpranger.ServeContent(ctx, w, r, key, o.System.Created, newObjectRanger(p, o, bucket))
	return nil
}

func (handler *Handler) servePrefix(ctx context.Context, w http.ResponseWriter, project *uplink.Project, bucket, prefix string) (err error) {
	_, err = w.Write([]byte(header))
	if err != nil {
		return err
	}

	var input struct {
		Bucket string
		Prefix string
		Items  []string
	}
	input.Bucket = bucket
	input.Prefix = prefix
	input.Items = make([]string, 0)

	objects := project.ListObjects(ctx, bucket, &uplink.ListObjectsOptions{
		Prefix: prefix,
	})

	// TODO add paging
	for objects.Next() {
		item := objects.Item().Key[len(prefix):]
		input.Items = append(input.Items, item)
	}
	if objects.Err() != nil {
		return objects.Err()
	}

	err = prefixTmpl.Execute(w, input)
	if err != nil {
		return err
	}

	_, err = w.Write([]byte(footer))
	return err
}

func (handler *Handler) handleUplinkErr(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, uplink.ErrBucketNotFound):
		http.Error(w, "bucket not found", http.StatusNotFound)
	case errors.Is(err, uplink.ErrObjectNotFound):
		http.Error(w, "object not found", http.StatusNotFound)
	default:
		handler.log.Error("unable to handle request", zap.String("action", action), zap.Error(err))
		http.Error(w, "unable to handle request", http.StatusInternalServerError)
	}
}

func parseRequestPath(p string) (_ *uplink.Access, bucket string, key string, err error) {
	// Drop the leading slash, if necessary
	p = strings.TrimPrefix(p, "/")

	// Split the request path
	segments := strings.SplitN(p, "/", 3)
	if len(segments) == 1 {
		if segments[0] == "" {
			return nil, "", "", errors.New("missing access")
		}
		return nil, "", "", errors.New("missing bucket")
	}

	scopeb58 := segments[0]
	bucket = segments[1]

	if len(segments) == 3 {
		key = segments[2]
	}

	access, err := uplink.ParseAccess(scopeb58)
	if err != nil {
		return nil, "", "", err
	}
	return access, bucket, key, nil
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
