package traffic

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

type Certificate struct {
	PEM      string `json:"pem"`
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
}

type authority struct {
	cert   *x509.Certificate
	key    *ecdsa.PrivateKey
	public Certificate
}

func loadCA(path string) (*authority, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, errors.New("无法读取抓包 CA")
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm() != 0o600 {
		return nil, errors.New("抓包 CA 文件必须为权限 0600 的普通文件")
	}
	content, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(content) > 8192 {
		return nil, errors.New("无法读取抓包 CA")
	}
	certBlock, rest := pem.Decode(content)
	keyBlock, tail := pem.Decode(rest)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" || keyBlock == nil ||
		keyBlock.Type != "PRIVATE KEY" || len(bytes.TrimSpace(tail)) != 0 {
		return nil, errors.New("抓包 CA 无效")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil || !cert.IsCA || cert.CheckSignatureFrom(cert) != nil {
		return nil, errors.New("抓包 CA 证书无效")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	key, ok := parsed.(*ecdsa.PrivateKey)
	if err != nil || !ok || !key.PublicKey.Equal(cert.PublicKey) {
		return nil, errors.New("抓包 CA 私钥不匹配")
	}
	// Android 文件名使用 OpenSSL subject_hash_old 的小端 MD5 前四字节，不用于信任校验。
	nameHash := androidSubjectHash(cert.RawSubject)
	digest := sha256.Sum256(cert.Raw)
	return &authority{cert, key, Certificate{
		PEM: string(pem.EncodeToMemory(certBlock)), Filename: nameHash + ".0", SHA256: hex.EncodeToString(digest[:]),
	}}, nil
}

func createCA(path string) (*authority, error) {
	if ca, err := loadCA(path); !errors.Is(err, os.ErrNotExist) {
		return ca, err
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("已有抓包 CA 文件，未覆盖")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "NetProxy Debug " + serial.Text(16)},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(5, 0, 0), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, MaxPathLenZero: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	data = append(data, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})...)
	if err := writeNewCA(path, data); err != nil {
		return nil, errors.New("无法保存抓包 CA")
	}
	return loadCA(path)
}

func writeNewCA(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".traffic-ca-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Link(f.Name(), path)
}

func (ca *authority) leaf(host string, now time.Time) (tls.Certificate, error) {
	if !now.Before(ca.cert.NotAfter) || now.Before(ca.cert.NotBefore) {
		return tls.Certificate{}, errors.New("抓包 CA 不在有效期内")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := randomSerial()
	if err != nil {
		return tls.Certificate{}, err
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: host}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ca.cert.NotAfter.Before(template.NotAfter) {
		template.NotAfter = ca.cert.NotAfter
	}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: template}, nil
}

func randomSerial() (*big.Int, error) {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, err
	}
	return n.Add(n, big.NewInt(1)), nil
}

func androidSubjectHash(subject []byte) string {
	digest := md5.Sum(subject)
	return fmt.Sprintf("%08x", binary.LittleEndian.Uint32(digest[:4]))
}
