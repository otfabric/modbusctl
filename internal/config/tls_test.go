package config

import (
	"crypto/x509"
	"errors"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/mbtest"
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
