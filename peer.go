// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package linksharing

import (
	"context"
	"errors"
	"net"

	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"storj.io/linksharing/handler"
	"storj.io/linksharing/httpserver"
	"storj.io/linksharing/objectmap"
)

// Config contains configurable values for sno registration Peer.
type Config struct {
	Server  httpserver.Config
	Handler handler.Config
}

// Peer is the representation of a Linksharing service itself.
//
// architecture: Peer
type Peer struct {
	Log      *zap.Logger
	Mapper   *objectmap.IPDB
	Listener net.Listener
	Server   *httpserver.Server
}

// New is a constructor for Linksharing Peer.
func New(log *zap.Logger, mapper *objectmap.IPDB, config Config) (_ *Peer, err error) {
	peer := &Peer{
		Log:    log,
		Mapper: mapper,
	}

	handler, err := handler.NewHandler(log, mapper, config.Handler)
	if err != nil {
		return nil, err
	}

	peer.Listener, err = net.Listen("tcp", config.Server.Address)
	if err != nil {
		return nil, errs.New("unable to listen on %s: %v", config.Server.Address, err)
	}

	peer.Server, err = httpserver.New(log, peer.Listener, handler, config.Server)
	if err != nil {
		return nil, err
	}

	return peer, nil
}

// Run runs SNO registration service until it's either closed or it errors.
func (peer *Peer) Run(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)

	// start SNO registration service as a separate goroutine.
	group.Go(func() error {
		return ignoreCancel(peer.Server.Run(ctx))
	})

	return group.Wait()
}

// Close closes all underlying resources.
func (peer *Peer) Close() error {
	errlist := errs.Group{}

	if peer.Server != nil {
		errlist.Add(peer.Server.Close())
	}

	if peer.Listener != nil {
		errlist.Add(peer.Listener.Close())
	}

	return errlist.Err()
}

// we ignore cancellation and stopping errors since they are expected.
func ignoreCancel(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
