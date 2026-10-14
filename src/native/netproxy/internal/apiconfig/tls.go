package apiconfig

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/netip"
	"time"
)

type Connection struct {
	Address           string `json:"address"`
	Secret            string `json:"secret"`
	CertificateSHA256 string `json:"certificate_sha256"`
}

func (config Config) Connection() Connection {
	block, _ := pem.Decode([]byte(config.Certificate))
	fingerprint := ""
	if block != nil {
		sum := sha256.Sum256(block.Bytes)
		fingerprint = hex.EncodeToString(sum[:])
	}
	return Connection{Address: "https://" + config.Listen, Secret: config.Secret, CertificateSHA256: fingerprint}
}

func (config Config) validateCertificate(address netip.Addr) error {
	key, err := tls.X509KeyPair([]byte(config.Certificate), []byte(config.PrivateKey))
	if err != nil || len(key.Certificate) != 1 {
		return errors.New("API TLS 证书或私钥无效")
	}
	certificate, err := x509.ParseCertificate(key.Certificate[0])
	if err != nil || certificate.VerifyHostname(address.String()) != nil {
		return errors.New("API TLS 证书与监听地址不匹配")
	}
	if certificate.CheckSignature(certificate.SignatureAlgorithm, certificate.RawTBSCertificate, certificate.Signature) != nil {
		return errors.New("API TLS 证书必须是有效的自签名证书")
	}
	return nil
}

func (config Config) ServerTLS() (*tls.Config, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	key, err := tls.X509KeyPair([]byte(config.Certificate), []byte(config.PrivateKey))
	if err != nil {
		return nil, errors.New("API TLS 凭据无效")
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{key}}, nil
}

func (config Config) ClientTLS() *tls.Config {
	expected, _ := pem.Decode([]byte(config.Certificate))
	address, _ := netip.ParseAddrPort(config.Listen)
	return &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, VerifyConnection: func(state tls.ConnectionState) error {
		// 安装证书是精确钉扎的本机身份，不依赖公网 CA 或设备时钟；主机名仍须匹配。
		if expected == nil || len(state.PeerCertificates) != 1 || !bytes.Equal(state.PeerCertificates[0].Raw, expected.Bytes) {
			return errors.New("API TLS 证书身份不匹配")
		}
		host := state.ServerName
		if host == "" {
			host = address.Addr().Unmap().String()
		}
		return state.PeerCertificates[0].VerifyHostname(host)
	}}
}

func (config *Config) generateCertificate() error {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	address, err := netip.ParseAddrPort(config.Listen)
	if err != nil {
		return err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "NetProxy local management"},
		NotBefore: now.Add(-24 * time.Hour), NotAfter: now.AddDate(10, 0, 0),
		IPAddresses: []net.IP{address.Addr().AsSlice(), net.ParseIP("::1")}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
	if err != nil {
		return err
	}
	encodedKey, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return err
	}
	config.Certificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	config.PrivateKey = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encodedKey}))
	return nil
}
