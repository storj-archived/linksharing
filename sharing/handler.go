// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"context"
	"errors"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/spacemonkeygo/monkit/v3"
	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/linksharing/objectmap"
	"storj.io/uplink"
)

var mon = monkit.Package()

// pageData is the type that is passed to the template rendering engine.
type pageData struct {
	Data  interface{} // data to provide to the page
	Title string      // <title> for the page

	// because we are serving data on someone else's domain, for our
	// branded pages like file listing and the map view, all static assets
	// must use an absolute url. this is the base url they are all based off
	// of. automatically filled in by renderTemplate.
	Base string
}

// Config specifies the handler configuration.
type Config struct {
	// URLBase is the base URL of the link sharing handler. It is used
	// to construct URLs returned to clients. It should be a fully formed URL.
	URLBase string

	// Templates location with html templates.
	Templates string

	// StaticSourcesPath is the path to where the web assets are located
	// on disk.
	StaticSourcesPath string

	// TxtRecordTTL is the duration for which an entry in the txtRecordCache is valid.
	TxtRecordTTL time.Duration

	// AuthServiceConfig contains configuration required to use the auth service to resolve
	// access key ids into access grants.
	AuthServiceConfig AuthServiceConfig

	// DNS Server address, for TXT record lookup
	DNSServer string

	// LandingRedirectTarget is the url to redirect empty requests to.
	LandingRedirectTarget string
}

// Handler implements the link sharing HTTP handler.
//
// architecture: Service
type Handler struct {
	log             *zap.Logger
	urlBase         *url.URL
	templates       *template.Template
	mapper          *objectmap.IPDB
	txtRecords      *txtRecords
	authConfig      AuthServiceConfig
	static          http.Handler
	landingRedirect string
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

	templates, err := template.ParseGlob(filepath.Join(config.Templates, "*.html"))
	if err != nil {
		return nil, err
	}

	return &Handler{
		log:             log,
		urlBase:         urlBase,
		templates:       templates,
		mapper:          mapper,
		txtRecords:      newTxtRecords(config.TxtRecordTTL, dns, config.AuthServiceConfig),
		authConfig:      config.AuthServiceConfig,
		static:          http.StripPrefix("/static/", http.FileServer(http.Dir(config.StaticSourcesPath))),
		landingRedirect: config.LandingRedirectTarget,
	}, nil
}

// ServeHTTP handles link sharing requests.
func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	defer mon.Task()(&ctx)(nil)

	handlerErr := handler.serveHTTP(ctx, w, r)
	if handlerErr == nil {
		return
	}

	status := http.StatusInternalServerError
	message := "Internal server error. Please try again later."
	action := GetAction(handlerErr, "unknown")
	skipLog := false
	switch {
	case errors.Is(handlerErr, uplink.ErrBucketNotFound):
		status = http.StatusNotFound
		message = "Oops! Bucket not found."
		skipLog = true
	case errors.Is(handlerErr, uplink.ErrObjectNotFound):
		status = http.StatusNotFound
		message = "Oops! Object not found."
		skipLog = true
	default:
		status = GetStatus(handlerErr, status)
		switch status {
		case http.StatusForbidden:
			message = "Access denied."
			skipLog = true
		case http.StatusNotFound:
			message = "Not found."
			skipLog = true
		case http.StatusBadRequest, http.StatusMethodNotAllowed:
			message = "Malformed request. Please try again."
			skipLog = true
		}
	}

	if !skipLog {
		handler.log.Error("unable to handle request", zap.Error(handlerErr), zap.String("action", action))
	}

	w.WriteHeader(status)
	handler.renderTemplate(w, "error.html", pageData{Data: message, Title: "Error"})
}

func (handler *Handler) renderTemplate(w http.ResponseWriter, template string, data pageData) {
	data.Base = strings.TrimSuffix(handler.urlBase.String(), "/")
	err := handler.templates.ExecuteTemplate(w, template, data)
	if err != nil {
		handler.log.Error("error while executing template", zap.Error(err))
	}
}

func (handler *Handler) serveHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) (err error) {
	defer mon.Task()(&ctx)(&err)

	if r.Method != http.MethodHead && r.Method != http.MethodGet {
		return WithStatus(errs.New("method not allowed"), http.StatusMethodNotAllowed)
	}

	ourDomain, err := compareHosts(r.Host, handler.urlBase.Host)
	if err != nil {
		return err
	}

	if !ourDomain {
		return handler.handleHostingService(ctx, w, r)
	}

	switch {
	case strings.HasPrefix(r.URL.Path, "/static/"):
		handler.static.ServeHTTP(w, r.WithContext(ctx))
		return nil
	case handler.landingRedirect != "" && (r.URL.Path == "" || r.URL.Path == "/"):
		http.Redirect(w, r, handler.landingRedirect, http.StatusSeeOther)
		return nil
	default:
		return handler.handleStandard(ctx, w, r)
	}
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
