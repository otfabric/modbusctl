// SPDX-License-Identifier: MIT

package mbtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// PKI is a throwaway certificate authority for tcp+tls tests. It issues one server
// certificate (for 127.0.0.1, ::1 and localhost) and one client certificate, and writes the
// PEM files the CLI flags take into a temporary directory.
type PKI struct {
	// CAFile holds the CA certificate.
	CAFile string
	// ServerCertFile and ServerKeyFile hold the server key pair.
	ServerCertFile, ServerKeyFile string
	// ClientCertFile and ClientKeyFile hold the client key pair.
	ClientCertFile, ClientKeyFile string
	// ClientPEMFile holds the client certificate followed by its key.
	ClientPEMFile string

	// ServerCert is the server key pair, for a server built in the test.
	ServerCert tls.Certificate
	// Pool holds the CA certificate, for verifying either side.
	Pool *x509.CertPool

	t      testing.TB
	dir    string
	ca     *x509.Certificate
	caKey  *ecdsa.PrivateKey
	serial int64
}

// NewPKI creates a CA and issues a server and a client certificate. The files are removed
// when the test ends.
func NewPKI(t testing.TB) *PKI {
	t.Helper()
	p := &PKI{t: t, dir: t.TempDir()}
	p.caKey = newKey(t)
	tmpl := p.template("mbtest CA")
	tmpl.IsCA = true
	tmpl.BasicConstraintsValid = true
	tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &p.caKey.PublicKey, p.caKey)
	if err != nil {
		t.Fatalf("mbtest: create CA: %v", err)
	}
	if p.ca, err = x509.ParseCertificate(der); err != nil {
		t.Fatalf("mbtest: parse CA: %v", err)
	}
	p.Pool = x509.NewCertPool()
	p.Pool.AddCert(p.ca)
	p.CAFile = p.write("ca.pem", pemBlock("CERTIFICATE", der))

	p.ServerCertFile, p.ServerKeyFile = p.Issue("server", true)
	if p.ServerCert, err = tls.LoadX509KeyPair(p.ServerCertFile, p.ServerKeyFile); err != nil {
		t.Fatalf("mbtest: load server key pair: %v", err)
	}
	p.ClientCertFile, p.ClientKeyFile = p.Issue("client", false)
	cert, _ := os.ReadFile(p.ClientCertFile)
	key, _ := os.ReadFile(p.ClientKeyFile)
	p.ClientPEMFile = p.write("client-bundle.pem", append(cert, key...))
	return p
}

// Issue signs a new key pair for name, as a server certificate for the loopback addresses
// or as a client certificate, and returns the certificate and key files.
func (p *PKI) Issue(name string, server bool) (certFile, keyFile string) {
	p.t.Helper()
	key := newKey(p.t)
	tmpl := p.template(name)
	tmpl.KeyUsage = x509.KeyUsageDigitalSignature
	if server {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tmpl.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
		tmpl.DNSNames = []string{"localhost"}
	} else {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.ca, &key.PublicKey, p.caKey)
	if err != nil {
		p.t.Fatalf("mbtest: issue %s: %v", name, err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		p.t.Fatalf("mbtest: marshal key of %s: %v", name, err)
	}
	return p.write(name+".pem", pemBlock("CERTIFICATE", der)), p.write(name+"-key.pem", pemBlock("PRIVATE KEY", keyDER))
}

// ServerConfig returns the TLS configuration of a Modbus/TCP Security server: it presents
// the server certificate and requires a client certificate signed by clientCAs.
func (p *PKI) ServerConfig(clientCAs *x509.CertPool) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{p.ServerCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
		MinVersion:   tls.VersionTLS12,
	}
}

func (p *PKI) template(name string) *x509.Certificate {
	p.serial++
	return &x509.Certificate{
		SerialNumber: big.NewInt(p.serial),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
}

func (p *PKI) write(name string, data []byte) string {
	path := filepath.Join(p.dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		p.t.Fatalf("mbtest: write %s: %v", path, err)
	}
	return path
}

func newKey(t testing.TB) *ecdsa.PrivateKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("mbtest: generate key: %v", err)
	}
	return key
}

func pemBlock(typ string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}

// ServeTLS serves the device as Modbus/TCP Security (MBAP over TLS) on a loopback port
// until the test ends: the server presents the PKI's server certificate and requires a
// client certificate signed by the PKI's CA. It returns the host:port to connect to.
func (d *Device) ServeTLS(t testing.TB, p *PKI) string {
	t.Helper()
	return d.ServeTLSConfig(t, p.ServerConfig(p.Pool))
}

// ServeTLSConfig is ServeTLS with a TLS configuration of the test's choosing.
func (d *Device) ServeTLSConfig(t testing.TB, conf *tls.Config) string {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", conf)
	if err != nil {
		t.Fatalf("mbtest: listen tls: %v", err)
	}
	return serveListener(t, ln, d.serveMBAP)
}
