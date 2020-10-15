// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/oschwald/maxminddb-golang"
	"github.com/spf13/cobra"
	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"golang.org/x/crypto/acme/autocert"

	"storj.io/common/fpath"
	"storj.io/linksharing"
	"storj.io/linksharing/handler"
	"storj.io/linksharing/httpserver"
	"storj.io/linksharing/objectmap"
	"storj.io/private/cfgstruct"
	"storj.io/private/process"
)

// LinkSharing defines link sharing configuration.
type LinkSharing struct {
	Address       string `user:"true" help:"public address to listen on" devDefault:"localhost:8080" releaseDefault:":8443"`
	LetsEncrypt   bool   `user:"true" help:"use lets-encrypt to handle TLS certificates" default:"false"`
	CertFile      string `user:"true" help:"server certificate file" devDefault:"" releaseDefault:"server.crt.pem"`
	KeyFile       string `user:"true" help:"server key file" devDefault:"" releaseDefault:"server.key.pem"`
	PublicURL     string `user:"true" help:"public url for the server" devDefault:"http://localhost:8080" releaseDefault:""`
	GeoLocationDB string `user:"true" help:"maxmind database file path" devDefault:"" releaseDefault:""`
}

var (
	rootCmd = &cobra.Command{
		Use:   "link sharing service",
		Short: "Link Sharing Service",
	}
	runCmd = &cobra.Command{
		Use:   "run",
		Short: "Run the link sharing service",
		RunE:  cmdRun,
	}
	setupCmd = &cobra.Command{
		Use:         "setup",
		Short:       "Create config files",
		RunE:        cmdSetup,
		Annotations: map[string]string{"type": "setup"},
	}

	runCfg   LinkSharing
	setupCfg LinkSharing

	confDir string
)

func init() {
	defaultConfDir := fpath.ApplicationDir("storj", "handler")
	cfgstruct.SetupFlag(zap.L(), rootCmd, &confDir, "config-dir", defaultConfDir, "main directory for link sharing configuration")
	defaults := cfgstruct.DefaultsFlag(rootCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(setupCmd)
	process.Bind(runCmd, &runCfg, defaults, cfgstruct.ConfDir(confDir))
	process.Bind(setupCmd, &setupCfg, defaults, cfgstruct.ConfDir(confDir), cfgstruct.SetupMode())
}

func cmdRun(cmd *cobra.Command, args []string) (err error) {
	ctx, _ := process.Ctx(cmd)
	log := zap.L()

	var tlsConfig *tls.Config
	if runCfg.LetsEncrypt {
		tlsConfig, err = configureLetsEncrypt(runCfg.PublicURL)
		if err != nil {
			return err
		}
	} else {
		tlsConfig, err = configureTLS(runCfg.CertFile, runCfg.KeyFile)
		if err != nil {
			return err
		}
	}

	reader, err := maxminddb.Open(runCfg.GeoLocationDB)
	if err != nil {
		return err
	}
	mapper := objectmap.NewIPDB(reader)
	defer func() {
		// mapper.Close() closes reader.
		err = errs.Combine(err, mapper.Close())
	}()

	peer, err := linksharing.New(log, mapper, linksharing.Config{
		Server: httpserver.Config{
			Name:            "Link Sharing",
			Address:         runCfg.Address,
			TLSConfig:       tlsConfig,
			ShutdownTimeout: -1,
			GeoLocationDB:   runCfg.GeoLocationDB,
		},
		Handler: handler.HandlerConfig{
			URLBase: runCfg.Address,
		},
	})
	if err != nil {
		return err
	}

	runError := peer.Run(ctx)
	closeError := peer.Close()

	return errs.Combine(runError, closeError)
}

func cmdSetup(cmd *cobra.Command, args []string) (err error) {
	setupDir, err := filepath.Abs(confDir)
	if err != nil {
		return err
	}

	valid, _ := fpath.IsValidSetupDir(setupDir)
	if !valid {
		return fmt.Errorf("link sharing configuration already exists (%v)", setupDir)
	}

	err = os.MkdirAll(setupDir, 0700)
	if err != nil {
		return err
	}

	return process.SaveConfig(cmd, filepath.Join(setupDir, "config.yaml"))
}

func configureTLS(certFile, keyFile string) (*tls.Config, error) {
	switch {
	case certFile != "" && keyFile != "":
	case certFile == "" && keyFile == "":
		return nil, nil
	case certFile != "" && keyFile == "":
		return nil, errs.New("key file must be provided with cert file")
	case certFile == "" && keyFile != "":
		return nil, errs.New("cert file must be provided with key file")
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errs.New("unable to load server keypair: %v", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil
}

func configureLetsEncrypt(publicURL string) (tlsConfig *tls.Config, err error) {
	parsedURL, err := url.Parse(publicURL)
	if err != nil {
		return nil, err
	}
	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(parsedURL.Host),
		Cache:      autocert.DirCache(".certs"),
	}
	tlsConfig = &tls.Config{
		GetCertificate: certManager.GetCertificate,
	}

	// run HTTP Endpoint as redirect and challenge handler
	go func() {
		_ = http.ListenAndServe(":http", certManager.HTTPHandler(nil))
	}()

	return tlsConfig, nil
}

func main() {
	process.Exec(rootCmd)
}
