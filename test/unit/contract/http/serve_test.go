package httpcontract_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	"testing"
	"time"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
)

func testTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"localhost"}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
}

func tlsCapabilities() httpcontract.Capabilities {
	return httpcontract.Capabilities{ContractVersion: httpcontract.ContractVersion, TLS: true}
}

func TestTLSPreflightRejectsInvalidStaticConfiguration(t *testing.T) {
	valid := testTLSConfig(t)
	other := testTLSConfig(t)
	for _, tc := range []struct {
		name   string
		config *tls.Config
	}{
		{"missing-certificate", &tls.Config{}},
		{"missing-private-key", &tls.Config{Certificates: []tls.Certificate{{Certificate: valid.Certificates[0].Certificate}}}},
		{"invalid-chain", &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{[]byte("not a certificate")}, PrivateKey: valid.Certificates[0].PrivateKey}}}},
		{"mismatched-key", &tls.Config{Certificates: []tls.Certificate{{Certificate: valid.Certificates[0].Certificate, PrivateKey: other.Certificates[0].PrivateKey}}}},
		{"invalid-version", &tls.Config{Certificates: valid.Certificates, MinVersion: 1}},
		{"inverted-versions", &tls.Config{Certificates: valid.Certificates, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS12}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := (httpcontract.ServeOptions{TLS: tc.config}).Validate(tlsCapabilities()); !errors.Is(err, httpcontract.ErrInvalidAdapter) {
				t.Fatalf("잘못된 TLS 설정: %v", err)
			}
		})
	}
	if err := (httpcontract.ServeOptions{TLS: valid}).Validate(tlsCapabilities()); err != nil {
		t.Fatal(err)
	}
	if err := (httpcontract.ServeOptions{TLS: valid}).Validate(httpcontract.Capabilities{ContractVersion: httpcontract.ContractVersion}); !errors.Is(err, httpcontract.ErrUnsupportedCapability) {
		t.Fatalf("TLS 미지원 선언: %v", err)
	}
	if err := httpcontract.ValidateFactory(&factory{caps: httpcontract.Capabilities{ContractVersion: httpcontract.ContractVersion}}, httpcontract.Requirements{TLS: true}); !errors.Is(err, httpcontract.ErrUnsupportedCapability) {
		t.Fatal(err)
	}
}

func TestTLSPreflightDoesNotCallCertificateProviders(t *testing.T) {
	calls := 0
	for _, config := range []*tls.Config{
		{GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { calls++; return nil, errors.New("not called") }},
		{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) { calls++; return nil, errors.New("not called") }},
	} {
		if _, err := (httpcontract.ServeOptions{TLS: config}).Resolve(tlsCapabilities()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatal("사전 검사에서 인증서 제공자를 실행하면 안 됩니다")
	}
}

// Sign을 노출하지 않는 RSA 키 제공자입니다.
type decryptionKey struct{ key *rsa.PrivateKey }

func (k decryptionKey) Public() crypto.PublicKey { return k.key.Public() }
func (k decryptionKey) Decrypt(random io.Reader, ciphertext []byte, opts crypto.DecrypterOpts) ([]byte, error) {
	return k.key.Decrypt(random, ciphertext, opts)
}

func TestTLSPreflightAcceptsNativeRSADecrypter(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, leaf, leaf, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	config := &tls.Config{MaxVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: decryptionKey{key}}}}
	if err := (httpcontract.ServeOptions{TLS: config}).Validate(tlsCapabilities()); err != nil {
		t.Fatalf("표준 TLS가 허용하는 RSA 키 제공자: %v", err)
	}
}

func TestServeOptionPrecedenceAndNilRejection(t *testing.T) {
	base, first, last := testTLSConfig(t), testTLSConfig(t), testTLSConfig(t)
	base.ServerName, first.ServerName, last.ServerName = "base", "first", "last"
	defaults := httpcontract.ServeOptions{TLS: base}
	resolved, err := defaults.Resolve(tlsCapabilities(), httpcontract.WithTLS(first), httpcontract.WithTLS(last))
	if err != nil || resolved.TLS.ServerName != "last" || resolved.TLS == last || defaults.TLS != base {
		t.Fatalf("선택 옵션 우선순위: %v", err)
	}
	resolved, err = defaults.Resolve(tlsCapabilities())
	if err != nil || resolved.TLS.ServerName != "base" || resolved.TLS == base {
		t.Fatalf("준비 기본값: %v", err)
	}
	for _, option := range []httpcontract.ServeOption{nil, httpcontract.WithTLS(nil)} {
		if _, err := defaults.Resolve(tlsCapabilities(), option); !errors.Is(err, httpcontract.ErrInvalidAdapter) {
			t.Fatalf("명시적인 nil 옵션: %v", err)
		}
	}
	resolved, err = (httpcontract.ServeOptions{}).Resolve(tlsCapabilities())
	if err != nil || resolved.TLS != nil {
		t.Fatal("TLS가 없으면 평문 기본값을 유지해야 합니다")
	}
}

func TestTLSCloneIsolatesCertificateAndConfigurationBuffers(t *testing.T) {
	config := testTLSConfig(t)
	config.NextProtos = []string{"http/1.1"}
	config.CipherSuites = []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}
	config.CurvePreferences = []tls.CurveID{tls.CurveP256}
	config.Certificates[0].OCSPStaple = []byte("original")
	config.NameToCertificate = map[string]*tls.Certificate{"localhost": &config.Certificates[0]}
	cloned := (httpcontract.ServeOptions{TLS: config}).Clone()
	config.MinVersion = tls.VersionTLS13
	config.NextProtos[0] = "changed"
	config.Certificates[0].Certificate[0][0] = 0
	config.NameToCertificate["localhost"].OCSPStaple[0] = 'x'
	config.CipherSuites[0] = 0
	config.CurvePreferences[0] = 0
	if cloned.TLS.MinVersion != tls.VersionTLS12 || cloned.TLS.NextProtos[0] != "http/1.1" || string(cloned.TLS.NameToCertificate["localhost"].OCSPStaple) != "original" || cloned.TLS.CipherSuites[0] == 0 || cloned.TLS.CurvePreferences[0] == 0 {
		t.Fatal("TLS 설정이 원본 버퍼와 연결되어 있습니다")
	}
	if err := cloned.Validate(tlsCapabilities()); err != nil {
		t.Fatalf("원본 변경 뒤 보존한 인증서: %v", err)
	}
}
