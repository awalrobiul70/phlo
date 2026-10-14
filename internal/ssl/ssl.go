package ssl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Paths are the certificate and key files Phlo writes for local HTTPS.
func Paths(dir string) (cert, key string) {
	return filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
}

// Ensure writes a self-signed certificate for local hosts when it is missing or does not cover
// every address in ips. 127.0.0.1 is always covered.
func Ensure(dir string, hosts, ips []string) (cert, key string, err error) {
	certPath, keyPath := Paths(dir)
	addresses := uniqueIPs(ips)
	if Ready(dir) && covers(certPath, addresses) {
		return certPath, keyPath, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"Phlo Local"}, CommonName: "Phlo Local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  addresses,
		DNSNames:     uniqueHosts(hosts),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}
	certFile, err := os.OpenFile(certPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", "", err
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		certFile.Close()
		return "", "", err
	}
	if err := certFile.Close(); err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	keyFile, err := os.OpenFile(keyPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", "", err
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		keyFile.Close()
		return "", "", err
	}
	if err := keyFile.Close(); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

func uniqueIPs(ips []string) []net.IP {
	out := []net.IP{net.ParseIP("127.0.0.1")}
	for _, value := range ips {
		ip := net.ParseIP(value)
		if ip == nil || slices.ContainsFunc(out, ip.Equal) {
			continue
		}
		out = append(out, ip)
	}
	return out
}

// covers reports whether the certificate at path lists every address in ips.
func covers(path string, ips []net.IP) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(body)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if !slices.ContainsFunc(cert.IPAddresses, ip.Equal) {
			return false
		}
	}
	return true
}

func uniqueHosts(hosts []string) []string {
	seen := map[string]bool{}
	out := []string{"localhost"}
	seen["localhost"] = true
	for _, host := range hosts {
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, host)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Ready reports whether both certificate files exist.
func Ready(dir string) bool {
	cert, key := Paths(dir)
	ci, err := os.Stat(cert)
	if err != nil || ci.IsDir() {
		return false
	}
	ki, err := os.Stat(key)
	return err == nil && !ki.IsDir()
}
