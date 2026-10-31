// SPDX-License-Identifier: MIT

package config

import (
	"crypto/x509"
	"errors"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTLSConfigLoad(t *testing.T) {
	pki := mbtest.NewPKI(t)

	// The key read from the certificate file, the CA from --tls-ca.
	cert, roots, err := TLSConfig{TLSCert: pki.ClientPEMFile, TLSCA: pki.CAFile}.Load()
	if err != nil || cert == nil || roots == nil {
		t.Fatalf("Load with a bundle and a CA = %v, %v, %v", cert, roots, err)
	}

	for _, tc := range []struct {
		name string
		cfg  TLSConfig
		want string
	}{
		{"no certificate", TLSConfig{}, "needs a client certificate"},
		{"missing certificate file", TLSConfig{TLSCert: "/no/such/cert.pem"}, "load TLS client certificate"},
		{"key that does not match", TLSConfig{TLSCert: pki.ClientCertFile, TLSKey: pki.ServerKeyFile}, "load TLS client certificate"},
		{"missing CA file", TLSConfig{TLSCert: pki.ClientCertFile, TLSKey: pki.ClientKeyFile, TLSCA: "/no/such/ca.pem"}, "load TLS CA certificates"},
	} {
		if _, _, err := tc.cfg.Load(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", tc.name, err, tc.want)
		}
	}

	// Without --tls-ca the system roots are used; when they cannot be read, the error says
	// to set --tls-ca.
	orig := systemCertPool
	t.Cleanup(func() { systemCertPool = orig })
	systemCertPool = func() (*x509.CertPool, error) { return nil, errors.New("no system roots") }
	if _, _, err := (TLSConfig{TLSCert: pki.ClientCertFile, TLSKey: pki.ClientKeyFile}).Load(); err == nil ||
		!strings.Contains(err.Error(), "set --tls-ca") {
		t.Errorf("Load without system roots = %v", err)
	}
}

func TestValidateTLSConfig(t *testing.T) {
	if err := ValidateTLSConfig("tcp://127.0.0.1:502", TLSConfig{TLSCert: "/no/such/cert.pem"}); err != nil {
		t.Errorf("a tcp:// URL ignores the TLS settings: %v", err)
	}
	if err := ValidateTLSConfig("tcp+tls://127.0.0.1:802", TLSConfig{}); err == nil {
		t.Error("a tcp+tls:// URL without a certificate passed")
	}
}

func TestServerTLSConfigLoad(t *testing.T) {
	pki := mbtest.NewPKI(t)

	// Without settings the server serves plain TCP.
	var off ServerTLSConfig
	assert.False(t, off.Enabled())
	cert, pool, err := off.Load()
	require.NoError(t, err)
	assert.Nil(t, cert)
	assert.Nil(t, pool)

	on := ServerTLSConfig{TLSCert: pki.ServerCertFile, TLSKey: pki.ServerKeyFile, TLSCA: pki.CAFile}
	assert.True(t, on.Enabled())
	cert, pool, err = on.Load()
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.NotNil(t, pool)

	for _, tc := range []struct {
		name string
		cfg  ServerTLSConfig
		want string
	}{
		{"CA only", ServerTLSConfig{TLSCA: pki.CAFile}, "needs a server certificate"},
		{"blank certificate", ServerTLSConfig{TLSCert: " ", TLSKey: pki.ServerKeyFile}, "needs a server certificate"},
		{"no client CA", ServerTLSConfig{TLSCert: pki.ServerCertFile, TLSKey: pki.ServerKeyFile}, "set --tls-ca"},
		{"key that does not match", ServerTLSConfig{TLSCert: pki.ServerCertFile, TLSKey: pki.ClientKeyFile, TLSCA: pki.CAFile}, "load TLS server certificate"},
		{"missing CA file", ServerTLSConfig{TLSCert: pki.ServerCertFile, TLSKey: pki.ServerKeyFile, TLSCA: "/no/such/ca.pem"}, "load TLS client CA certificates"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, tc.cfg.Enabled())
			_, _, err := tc.cfg.Load()
			require.ErrorContains(t, err, tc.want)
		})
	}
}
