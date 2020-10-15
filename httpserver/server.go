// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package httpserver

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"storj.io/common/errs2"
	"storj.io/linksharing/handler"
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

	// TLSConfig is the TLS configuration for the server. It is optional.
	TLSConfig *tls.Config

	// ShutdownTimeout controls how long to wait for requests to finish before
	// returning from Run() after the context is canceled. It defaults to
	// 10 seconds if unset. If set to a negative value, the server will be
	// closed immediately.
	ShutdownTimeout time.Duration

	// Maxmind geolocation database path.
	GeoLocationDB string
}

// Server is the HTTP server.
//
// architecture: Endpoint
type Server struct {
	log     *zap.Logger
	handler *handler.Handler
	name    string

	listener        net.Listener
	server          *http.Server
	shutdownTimeout time.Duration
}

// New creates a new URL Service Server.
func New(log *zap.Logger, listener net.Listener, handler *handler.Handler, config Config) (*Server, error) {
	switch {
	case config.Address == "":
		return nil, errs.New("server address is required")
	case handler == nil:
		return nil, errs.New("server handler is required")
	}

	mux := http.NewServeMux()
	// TODO add static folder location to handler configuration
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	mux.Handle("/", handler)

	server := &http.Server{
		Handler:   mux,
		TLSConfig: config.TLSConfig,
		ErrorLog:  zap.NewStdLog(log),
	}

	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = DefaultShutdownTimeout
	}

	if config.Name != "" {
		log = log.With(zap.String("server", config.Name))
	}

	return &Server{
		log:             log,
		name:            config.Name,
		listener:        listener,
		server:          server,
		shutdownTimeout: config.ShutdownTimeout,
		handler:         handler,
	}, nil
}

// Run runs the server until it's either closed or it errors.
func (server *Server) Run(ctx context.Context) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	var group errgroup.Group

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

func shutdownWithTimeout(server *http.Server, timeout time.Duration) error {
	if timeout < 0 {
		return server.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return errs2.IgnoreCanceled(server.Shutdown(ctx))
}
