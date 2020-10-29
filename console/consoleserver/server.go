// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package consoleserver

import (
	"context"
	"crypto/tls"
	"errors"
	"html/template"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"storj.io/common/errs2"
	"storj.io/linksharing/console"
	"storj.io/linksharing/console/consoleapi"
)

const (
	// DefaultShutdownTimeout is the default ShutdownTimeout (see Config).
	DefaultShutdownTimeout = time.Second * 10
)

// Config holds the HTTP server configuration.
type Config struct {
	// Name is the name of the server. It is only used for logging. It can
	// be empty.
	Name string

	// Address is the address to bind the server to. It must be set.
	Address string

	// URLBase is the base URL of the link sharing handler. It is used
	// to construct URLs returned to clients. It should be a fully formed URL.
	URLBase string

	// TLSConfig is the TLS configuration for the server. It is optional.
	TLSConfig *tls.Config

	// ShutdownTimeout controls how long to wait for requests to finish before
	// returning from Run() after the context is canceled. It defaults to
	// 10 seconds if unset. If set to a negative value, the server will be
	// closed immediately.
	ShutdownTimeout time.Duration

	// Maxmind geolocation database path.
	GeoLocationDB string

	// path to static resources.
	StaticDir string
}

// Server is the HTTP server.
//
// architecture: Endpoint
type Server struct {
	log  *zap.Logger
	name string

	service         *console.Service
	listener        net.Listener
	shutdownTimeout time.Duration
	server          *http.Server

	templates consoleapi.SharingTemplates
}

// New creates a new URL Service Server.
func New(log *zap.Logger, listener net.Listener, service *console.Service, config Config) (*Server, error) {
	switch {
	case config.Address == "":
		return nil, errs.New("server address is required")
	case service == nil:
		return nil, errs.New("server service is required")
	}

	linksharingServer := &Server{}

	err := linksharingServer.initializeTemplates()
	if err != nil {
		return nil, err
	}

	sharingController := consoleapi.NewSharing(log, service, linksharingServer.templates)

	router := mux.NewRouter()
	fs := http.FileServer(http.Dir(config.StaticDir))

	router.HandleFunc("/{serialized-access}/{bucket-name}", sharingController.BucketFiles).Methods(http.MethodGet)
	router.HandleFunc("/{serialized-access}/{bucket-name}/{file-name}", sharingController.File).Methods(http.MethodGet)
	router.HandleFunc("/raw/{serialized-access}/{bucket-name}/{file-name}", sharingController.OpenFile).Methods(http.MethodGet)

	if config.StaticDir != "" {
		router.PathPrefix("/static/").Handler(http.StripPrefix("/static", fs))
	}

	server := &http.Server{
		Handler:   router,
		TLSConfig: config.TLSConfig,
		ErrorLog:  zap.NewStdLog(log),
	}

	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = DefaultShutdownTimeout
	}

	if config.Name != "" {
		log = log.With(zap.String("server", config.Name))
	}

	linksharingServer.log = log
	linksharingServer.name = config.Name
	linksharingServer.listener = listener
	linksharingServer.server = server
	linksharingServer.shutdownTimeout = config.ShutdownTimeout
	linksharingServer.service = service

	return linksharingServer, nil
}

// Run runs the server until it's either closed or it errors.
func (server *Server) Run(ctx context.Context) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	var group errgroup.Group

	err = server.initializeTemplates()
	if err != nil {
		defer cancel()
		return err
	}

	group.Go(func() error {
		<-ctx.Done()
		server.log.Info("Server shutting down")
		return shutdownWithTimeout(server.server, server.shutdownTimeout)
	})
	group.Go(func() (err error) {
		defer cancel()
		server.log.With(zap.String("addr", server.Addr())).Sugar().Info("Server started")
		if server.server.TLSConfig == nil {
			err = server.server.Serve(server.listener)
		} else {
			err = server.server.ServeTLS(server.listener, "", "")
		}
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		server.log.With(zap.Error(err)).Error("Server closed unexpectedly")
		return err
	})

	return group.Wait()
}

// Addr returns the public address.
func (server *Server) Addr() string {
	return server.listener.Addr().String()
}

// Close closes server and underlying listener.
func (server *Server) Close() error {
	return errs.Combine(server.server.Close(), server.listener.Close())
}

// initializeTemplates is used to initialize all templates.
func (server *Server) initializeTemplates() (err error) {
	server.templates.List, err = template.ParseFiles("./static/templates/prefix-listing.html", "./static/templates/header.html", "./static/templates/footer.html")
	if err != nil {
		return err
	}

	server.templates.SingleObject, err = template.ParseFiles("./static/templates/single-object.html", "./static/templates/header.html", "./static/templates/footer.html")
	if err != nil {
		return err
	}

	server.templates.NotFound, err = template.ParseFiles("./static/templates/404.html", "./static/templates/header.html", "./static/templates/footer.html")
	if err != nil {
		return err
	}

	return nil
}

func shutdownWithTimeout(server *http.Server, timeout time.Duration) error {
	if timeout < 0 {
		return server.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return errs2.IgnoreCanceled(server.Shutdown(ctx))
}
