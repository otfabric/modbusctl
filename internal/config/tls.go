package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"

	"github.com/otfabric/go-modbus"
)

// TLSConfig holds the client-side TLS settings used with tcp+tls:// URLs (Modbus/TCP
// Security). The device authenticates the client by its certificate, so a certificate and
// key are required; the server certificate is checked against the CA bundle, or against the
// system roots when no bundle is given. The settings are ignored for other URLs.
type TLSConfig struct {
	TLSCert string `env:"MODBUSCTL_TLS_CERT" desc:"Client certificate (PEM) for tcp+tls:// URLs; the file may also hold the key" flag:"tls-cert"`
	TLSKey  string `env:"MODBUSCTL_TLS_KEY" desc:"Client private key (PEM) for tcp+tls:// URLs (default: read from --tls-cert)" flag:"tls-key"`
	TLSCA   string `env:"MODBUSCTL_TLS_CA" desc:"CA certificates (PEM) to verify the server of a tcp+tls:// URL (default: the system roots)" flag:"tls-ca"`
}

// IsTLSURL reports whether modbusURL uses Modbus/TCP Security (tcp+tls://).
func IsTLSURL(modbusURL string) bool {
	scheme, _, ok := splitModbusURL(modbusURL)
	return ok && scheme == "tcp+tls"
}

// Load reads the client key pair and the CA bundle. Without --tls-key the key is read from
// the --tls-cert file; without --tls-ca the system roots are used.
func (c TLSConfig) Load() (*tls.Certificate, *x509.CertPool, error) {
	certFile := strings.TrimSpace(c.TLSCert)
	if certFile == "" {
		return nil, nil, fmt.Errorf("tcp+tls:// needs a client certificate: set --tls-cert (and --tls-key unless the file holds the key)")
	}
	keyFile := strings.TrimSpace(c.TLSKey)
	if keyFile == "" {
		keyFile = certFile
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load TLS client certificate %s: %w", certFile, err)
	}
	var roots *x509.CertPool
	if ca := strings.TrimSpace(c.TLSCA); ca != "" {
		roots, err = modbus.LoadCertPool(ca)
		if err != nil {
			return nil, nil, fmt.Errorf("load TLS CA certificates: %w", err)
		}
	} else if roots, err = x509.SystemCertPool(); err != nil {
		return nil, nil, fmt.Errorf("load system root certificates (set --tls-ca): %w", err)
	}
	return &cert, roots, nil
}

// ValidateTLSConfig checks that the TLS files of a tcp+tls:// URL can be loaded. Other URLs
// ignore the settings and always pass.
func ValidateTLSConfig(modbusURL string, cfg TLSConfig) error {
	if !IsTLSURL(modbusURL) {
		return nil
	}
	_, _, err := cfg.Load()
	return err
}
