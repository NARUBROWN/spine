package httpcontract

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"slices"
)

// ServeOptions는 관리 서버의 실행 기본값입니다. TLS가 nil이면 평문으로 실행합니다.
// 외부 native 서버에 Handler만 연결한 경우 그 서버의 TLS 설정은 외부 소유자에게 있습니다.
type ServeOptions struct {
	TLS *tls.Config
}

// ServeOption은 실행 시 준비한 기본값을 바꾸는 선택 옵션입니다.
// 계약이 제공하는 옵션만 사용하며 사용자 옵션 콜백을 실행하지 않습니다.
type ServeOption interface {
	applyServe(*ServeOptions) error
}

type tlsOption struct{ config *tls.Config }

// WithTLS는 TLS 실행 설정을 지정합니다. 마지막 옵션이 준비한 기본값과 앞선 옵션보다 우선합니다.
// nil은 오류이며, 옵션을 생략하면 준비한 설정을 사용합니다.
func WithTLS(config *tls.Config) ServeOption { return tlsOption{config: config} }

func (o tlsOption) applyServe(options *ServeOptions) error {
	if o.config == nil {
		return fmt.Errorf("%w: WithTLS requires a non-nil config", ErrInvalidAdapter)
	}
	options.TLS = o.config
	return nil
}

// Resolve는 기본값과 명시한 옵션을 합쳐 검증하고 독립적인 설정을 반환합니다.
// 리스너나 인증서 선택 콜백을 실행하지 않습니다.
func (o ServeOptions) Resolve(caps Capabilities, options ...ServeOption) (ServeOptions, error) {
	for _, option := range options {
		if isNil(option) {
			return ServeOptions{}, fmt.Errorf("%w: Serve option is nil", ErrInvalidAdapter)
		}
		if err := option.applyServe(&o); err != nil {
			return ServeOptions{}, err
		}
	}
	o = o.Clone()
	if err := o.Validate(caps); err != nil {
		return ServeOptions{}, err
	}
	return o, nil
}

// Validate는 TLS 지원, 설정 구조와 정적 인증서의 키 쌍을 검사합니다.
// 신뢰 체인, 호스트명, 유효 기간과 동적 인증서의 가용성은 실제 연결에서 검증합니다.
func (o ServeOptions) Validate(caps Capabilities) error {
	if err := validateVersion(caps); err != nil {
		return err
	}
	if o.TLS == nil {
		return nil
	}
	if !caps.TLS {
		return fmt.Errorf("%w: TLS", ErrUnsupportedCapability)
	}
	c := o.TLS
	for _, version := range []uint16{c.MinVersion, c.MaxVersion} {
		if version != 0 && (version < tls.VersionTLS10 || version > tls.VersionTLS13) {
			return fmt.Errorf("%w: invalid TLS version", ErrInvalidAdapter)
		}
	}
	if c.MinVersion != 0 && c.MaxVersion != 0 && c.MinVersion > c.MaxVersion {
		return fmt.Errorf("%w: TLS MinVersion exceeds MaxVersion", ErrInvalidAdapter)
	}
	if len(c.Certificates) == 0 && c.GetCertificate == nil && c.GetConfigForClient == nil {
		return fmt.Errorf("%w: TLS requires a certificate or certificate provider", ErrInvalidAdapter)
	}
	for i, certificate := range c.Certificates {
		if err := validateCertificate(certificate); err != nil {
			return fmt.Errorf("%w: TLS Certificates[%d]: %v", ErrInvalidAdapter, i, err)
		}
	}
	return nil
}

func validateCertificate(certificate tls.Certificate) (err error) {
	// 외부 키 제공자가 Public에서 패닉을 내더라도 비밀 값 없이 설정 오류로 보고합니다.
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("private key public value is unavailable")
		}
	}()
	if len(certificate.Certificate) == 0 || isNil(certificate.PrivateKey) {
		return fmt.Errorf("certificate and private key are required")
	}
	var leaf *x509.Certificate
	for i, data := range certificate.Certificate {
		parsed, err := x509.ParseCertificate(data)
		if err != nil {
			return fmt.Errorf("certificate chain entry %d is invalid", i)
		}
		if i == 0 {
			leaf = parsed
		}
	}
	var key crypto.PublicKey
	switch private := certificate.PrivateKey.(type) {
	case crypto.Signer:
		key = private.Public()
	case crypto.Decrypter:
		// 표준 TLS 서버가 TLS 1.2 이하에서 지원하는 RSA 복호화 키도 허용합니다.
		key = private.Public()
		if _, ok := key.(*rsa.PublicKey); !ok {
			return fmt.Errorf("decryption-only private key must use RSA")
		}
	default:
		return fmt.Errorf("private key must implement crypto.Signer or crypto.Decrypter")
	}
	public, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return fmt.Errorf("private key public value is invalid")
	}
	if !bytes.Equal(public, leaf.RawSubjectPublicKeyInfo) {
		return fmt.Errorf("certificate and private key do not match")
	}
	return nil
}

// Clone은 TLS 설정과 수정 가능한 슬라이스, 인증서 바이트와 CA 풀을 복사합니다.
// 개인 키, 콜백과 I/O 제공자는 복제하지 않으며 호출자가 안전한 동시 사용을 보장합니다.
func (o ServeOptions) Clone() ServeOptions {
	if o.TLS == nil {
		return o
	}
	c := o.TLS.Clone()
	c.NextProtos = slices.Clone(c.NextProtos)
	c.CipherSuites = slices.Clone(c.CipherSuites)
	c.CurvePreferences = slices.Clone(c.CurvePreferences)
	c.EncryptedClientHelloConfigList = slices.Clone(c.EncryptedClientHelloConfigList)
	c.EncryptedClientHelloKeys = slices.Clone(c.EncryptedClientHelloKeys)
	for i := range c.EncryptedClientHelloKeys {
		c.EncryptedClientHelloKeys[i].Config = slices.Clone(c.EncryptedClientHelloKeys[i].Config)
		c.EncryptedClientHelloKeys[i].PrivateKey = slices.Clone(c.EncryptedClientHelloKeys[i].PrivateKey)
	}
	c.Certificates = slices.Clone(c.Certificates)
	for i := range c.Certificates {
		c.Certificates[i] = cloneCertificate(c.Certificates[i])
	}
	if c.NameToCertificate != nil {
		cloned := make(map[string]*tls.Certificate, len(c.NameToCertificate))
		for name, certificate := range c.NameToCertificate {
			if certificate != nil {
				value := cloneCertificate(*certificate)
				cloned[name] = &value
			} else {
				cloned[name] = nil
			}
		}
		c.NameToCertificate = cloned
	}
	if c.RootCAs != nil {
		c.RootCAs = c.RootCAs.Clone()
	}
	if c.ClientCAs != nil {
		c.ClientCAs = c.ClientCAs.Clone()
	}
	o.TLS = c
	return o
}

func cloneCertificate(c tls.Certificate) tls.Certificate {
	c.Certificate = cloneBytes(c.Certificate)
	c.SupportedSignatureAlgorithms = slices.Clone(c.SupportedSignatureAlgorithms)
	c.OCSPStaple = slices.Clone(c.OCSPStaple)
	c.SignedCertificateTimestamps = cloneBytes(c.SignedCertificateTimestamps)
	c.Leaf = nil
	if len(c.Certificate) > 0 {
		c.Leaf, _ = x509.ParseCertificate(c.Certificate[0])
	}
	return c
}

func cloneBytes(values [][]byte) [][]byte {
	copy := slices.Clone(values)
	for i := range copy {
		copy[i] = slices.Clone(copy[i])
	}
	return copy
}
