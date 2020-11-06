// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package httpserver

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"storj.io/common/pkcrypto"
	"storj.io/common/testcontext"
	"storj.io/linksharing/objectmap"
	"storj.io/linksharing/sharing"
)

var (
	testKey = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgT8yIof+3qG3wQzXf
eAOcuTgWmgqXRnHVwKJl2g1pCb2hRANCAARWxVAPyT1BRs2hqiDuHlPXr1kVDXuw
7/a1USmgsVWiZ0W3JopcTbTMhvMZk+2MKqtWcc3gHF4vRDnHTeQl4lsx
-----END PRIVATE KEY-----`
	testCert = mustCreateLocalhostCert()
)

func TestServer(t *testing.T) {
	address := "localhost:15001"
	handlerConfig := sharing.Config{
		URLBase:   "https://localhost:15001",
		Templates: "../web/*.html",
	}
	mapper := objectmap.NewIPDB(&objectmap.MockReader{})
	handler, err := sharing.NewHandler(zaptest.NewLogger(t), mapper, handlerConfig)
	require.NoError(t, err)

	tempdir := t.TempDir()
	keyPath := filepath.Join(tempdir, "privkey.pem")
	certPath := filepath.Join(tempdir, "public.pem")

	err = ioutil.WriteFile(keyPath, []byte(testKey), 0644)
	require.NoError(t, err)

	err = ioutil.WriteFile(certPath, pkcrypto.CertToPEM(testCert), 0644)
	require.NoError(t, err)

	tlsConfig := &TLSConfig{
		CertFile:    certPath,
		KeyFile:     keyPath,
		LetsEncrypt: false,
		ConfigDir:   tempdir,
		PublicURL:   address,
	}

	noTLSConfig := &TLSConfig{
		CertFile:    "",
		KeyFile:     "",
		LetsEncrypt: false,
		ConfigDir:   tempdir,
		PublicURL:   address,
	}

	testCases := []serverTestCase{
		{
			Mapper:        mapper,
			HandlerConfig: handlerConfig,
			Name:          "missing address",
			TLSConfig:     noTLSConfig,
			Handler:       handler,
			NewErr:        "server address is required",
		},
		{
			Mapper:        mapper,
			HandlerConfig: handlerConfig,
			Name:          "bad address",
			Address:       "this is no good",
			TLSConfig:     noTLSConfig,
			Handler:       handler,
			NewErr:        "unable to listen on this is no good: listen tcp: address this is no good: missing port in address",
		},
		{
			Mapper:        mapper,
			HandlerConfig: handlerConfig,
			Name:          "missing handler",
			Address:       address,
			TLSConfig:     noTLSConfig,
			NewErr:        "server handler is required",
		},
		{
			Mapper:        mapper,
			HandlerConfig: handlerConfig,
			Name:          "success via HTTP",
			Address:       address,
			TLSConfig:     noTLSConfig,
			Handler:       handler,
		},
		{
			Mapper:        mapper,
			HandlerConfig: handlerConfig,
			Name:          "success via HTTPS",
			Address:       address,
			AddressTLS:    "localhost:15002",
			TLSConfig:     tlsConfig,
			Handler:       handler,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := testcontext.NewWithTimeout(t, time.Minute)
			defer ctx.Cleanup()

			s, ok := testCase.NewServer(t)
			if !ok {
				return
			}

			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			ctx.Go(func() error {
				return s.Run(runCtx)
			})

			testCase.DoGet(t)
		})
	}
}

type serverTestCase struct {
	Mapper        *objectmap.IPDB
	HandlerConfig sharing.Config
	Name          string
	Address       string
	AddressTLS    string
	Handler       *sharing.Handler
	TLSConfig     *TLSConfig
	NewErr        string
}

func (testCase *serverTestCase) NewServer(tb testing.TB) (*Server, bool) {
	s, err := New(zaptest.NewLogger(tb), testCase.Handler, Config{
		Name:       "test",
		Address:    testCase.Address,
		AddressTLS: testCase.AddressTLS,
		TLSConfig:  testCase.TLSConfig,
	})
	if testCase.NewErr != "" {
		require.EqualError(tb, err, testCase.NewErr)
		return nil, false
	}
	require.NoError(tb, err)
	return s, true
}

func (testCase *serverTestCase) DoGet(tb testing.TB) {
	scheme := "http"
	client := &http.Client{}
	addr := testCase.Address
	if testCase.AddressTLS != "" {
		scheme = "https"
		addr = testCase.AddressTLS
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certPoolFromCert(testCert),
			},
		}
	}

	resp, err := client.Get(fmt.Sprintf("%s://%s", scheme, addr))
	require.NoError(tb, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(tb, resp.StatusCode, http.StatusBadRequest)

	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(tb, err)
	assert.Equal(tb, "invalid request: missing access\n", string(body))
}

func mustSignerFromPEM(keyBytes string) crypto.Signer {
	key, err := pkcrypto.PrivateKeyFromPEM([]byte(keyBytes))
	if err != nil {
		panic(err)
	}
	return key.(crypto.Signer)
}

func mustCreateLocalhostCert() *x509.Certificate {
	privateKey := mustSignerFromPEM(testKey)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(0),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, privateKey.Public(), privateKey)
	if err != nil {
		panic(err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		panic(err)
	}
	return cert
}

func certPoolFromCert(cert *x509.Certificate) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return pool
}
