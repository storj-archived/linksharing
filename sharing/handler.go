// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"errors"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spacemonkeygo/monkit/v3"
	"go.uber.org/zap"

	"storj.io/linksharing/objectmap"
	"storj.io/uplink"
)

var mon = monkit.Package()

// Config specifies the handler configuration.
type Config struct {
	// URLBase is the base URL of the link sharing handler. It is used
	// to construct URLs returned to clients. It should be a fully formed URL.
	URLBase string

	// Templates location with html templates.
	Templates string

	// TxtRecordTTL is the duration for which an entry in the txtRecordCache is valid.
	TxtRecordTTL time.Duration

	// AuthServiceConfig contains configuration required to use the auth service to resolve
	// access key ids into access grants.
	AuthServiceConfig AuthServiceConfig

	// DNS Server address, for TXT record lookup
	DNSServer string
}

// Handler implements the link sharing HTTP handler.
//
// architecture: Service
type Handler struct {
	log        *zap.Logger
	urlBase    *url.URL
	templates  *template.Template
	mapper     *objectmap.IPDB
	txtRecords *txtRecords
	authConfig AuthServiceConfig
}

// NewHandler creates a new link sharing HTTP handler.
func NewHandler(log *zap.Logger, mapper *objectmap.IPDB, config Config) (*Handler, error) {
	dns, err := NewDNSClient(config.DNSServer)
	if err != nil {
		return nil, err
	}

	urlBase, err := parseURLBase(config.URLBase)
	if err != nil {
		return nil, err
	}

	if config.Templates == "" {
		config.Templates = "./web/*.html"
	}
	templates, err := template.ParseGlob(config.Templates)
	if err != nil {
		return nil, err
	}

	return &Handler{
		log:        log,
		urlBase:    urlBase,
		templates:  templates,
		mapper:     mapper,
		txtRecords: newTxtRecords(config.TxtRecordTTL, dns, config.AuthServiceConfig),
		authConfig: config.AuthServiceConfig,
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

	equal, err := compareHosts(r.Host, handler.urlBase.Host)
	if err != nil {
		return err
	}

	if !equal {
		return handler.handleHostingService(ctx, w, r)
	}

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

	return handler.handleTraditional(ctx, w, r, locationOnly)
}

func compareHosts(url1, url2 string) (equal bool, err error) {
	host1, _, err1 := net.SplitHostPort(url1)
	host2, _, err2 := net.SplitHostPort(url2)

	if err1 != nil && strings.Contains(err1.Error(), "missing port in address") {
		host1 = url1
	} else if err1 != nil {
		return false, err1
	}

	if err2 != nil && strings.Contains(err2.Error(), "missing port in address") {
		host2 = url2
	} else if err2 != nil {
		return false, err2
	}

	if host1 != host2 {
		return false, nil
	}
	return true, nil
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
