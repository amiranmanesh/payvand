// Package cryptox holds the cryptographic primitives the Iranian PSPs ask for:
// 3DES-ECB signatures (Sadad), AES-CBC plus RSA envelopes (IranKish) and
// RSA signatures over the request body (Pasargad).
//
// Everything is built on the standard library.
package cryptox

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// PKCS7Pad appends PKCS#7 padding so the data fills whole blocks.
func PKCS7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

// TripleDESEncryptECB encrypts data with 3DES in ECB mode and PKCS#7 padding,
// then base64-encodes the result. The key itself is given base64 encoded,
// which is how Sadad hands out terminal keys.
func TripleDESEncryptECB(data []byte, keyBase64 string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return "", fmt.Errorf("payvand: decoding 3des key: %w", err)
	}
	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return "", fmt.Errorf("payvand: building 3des cipher: %w", err)
	}

	padded := PKCS7Pad(data, block.BlockSize())
	out := make([]byte, len(padded))
	for start := 0; start < len(padded); start += block.BlockSize() {
		block.Encrypt(out[start:start+block.BlockSize()], padded[start:start+block.BlockSize()])
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

// AESCBCEncrypt encrypts data with AES-CBC and PKCS#7 padding.
func AESCBCEncrypt(key, iv, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("payvand: building aes cipher: %w", err)
	}
	padded := PKCS7Pad(data, block.BlockSize())
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return out, nil
}

// RandomBytes returns n cryptographically secure random bytes.
func RandomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("payvand: reading random bytes: %w", err)
	}
	return buf, nil
}

// SHA256Sum returns the SHA-256 digest of data.
func SHA256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

// ParseRSAPublicKey accepts a PEM encoded PKIX or PKCS#1 public key, or the
// bare base64 body of one, and returns the RSA key.
func ParseRSAPublicKey(key string) (*rsa.PublicKey, error) {
	der, err := decodePEMOrBase64(strings.TrimSpace(key), "PUBLIC KEY")
	if err != nil {
		return nil, err
	}
	if parsed, err := x509.ParsePKIXPublicKey(der); err == nil {
		rsaKey, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("payvand: public key is not an RSA key")
		}
		return rsaKey, nil
	}
	rsaKey, err := x509.ParsePKCS1PublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("payvand: parsing rsa public key: %w", err)
	}
	return rsaKey, nil
}

// ParseRSAPrivateKey accepts a PEM encoded PKCS#1 or PKCS#8 private key, the
// bare base64 body of one, or a .NET style <RSAKeyValue> XML key, which is the
// format Pasargad still distributes.
func ParseRSAPrivateKey(key string) (*rsa.PrivateKey, error) {
	trimmed := strings.TrimSpace(key)
	if strings.HasPrefix(trimmed, "<") {
		return parseXMLPrivateKey(trimmed)
	}

	der, err := decodePEMOrBase64(trimmed, "PRIVATE KEY")
	if err != nil {
		return nil, err
	}
	if parsed, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return parsed, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("payvand: parsing rsa private key: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("payvand: private key is not an RSA key")
	}
	return rsaKey, nil
}

// EncryptPKCS1v15 encrypts data with the given RSA public key.
func EncryptPKCS1v15(pub *rsa.PublicKey, data []byte) ([]byte, error) {
	out, err := rsa.EncryptPKCS1v15(rand.Reader, pub, data)
	if err != nil {
		return nil, fmt.Errorf("payvand: rsa encryption: %w", err)
	}
	return out, nil
}

// SignPKCS1v15SHA1 signs data with RSASSA-PKCS1-v1_5 over SHA-1 and returns the
// base64 signature. Pasargad's "Sign" header uses this legacy combination.
func SignPKCS1v15SHA1(priv *rsa.PrivateKey, data []byte) (string, error) {
	digest := sha1.Sum(data) //nolint:gosec // the gateway protocol mandates SHA-1
	signature, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA1, digest[:])
	if err != nil {
		return "", fmt.Errorf("payvand: rsa signature: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// SignPKCS1v15SHA256 signs data with RSASSA-PKCS1-v1_5 over SHA-256 and returns
// the base64 signature.
func SignPKCS1v15SHA256(priv *rsa.PrivateKey, data []byte) (string, error) {
	digest := sha256.Sum256(data)
	signature, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("payvand: rsa signature: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// decodePEMOrBase64 returns the DER bytes of a key given either as PEM or as
// the bare base64 body.
func decodePEMOrBase64(key, kind string) ([]byte, error) {
	if block, _ := pem.Decode([]byte(key)); block != nil {
		return block.Bytes, nil
	}
	der, err := base64.StdEncoding.DecodeString(stripWhitespace(key))
	if err != nil {
		return nil, fmt.Errorf("payvand: %s is neither PEM nor base64: %w", strings.ToLower(kind), err)
	}
	return der, nil
}

// stripWhitespace removes the line breaks a copy-pasted key usually carries.
func stripWhitespace(s string) string {
	return strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(s)
}

// xmlRSAKey is the .NET RSAKeyValue serialisation.
type xmlRSAKey struct {
	Modulus  string `xml:"Modulus"`
	Exponent string `xml:"Exponent"`
	P        string `xml:"P"`
	Q        string `xml:"Q"`
	DP       string `xml:"DP"`
	DQ       string `xml:"DQ"`
	InverseQ string `xml:"InverseQ"`
	D        string `xml:"D"`
}

// parseXMLPrivateKey rebuilds an RSA private key from a .NET RSAKeyValue.
func parseXMLPrivateKey(key string) (*rsa.PrivateKey, error) {
	var parsed xmlRSAKey
	if err := xml.Unmarshal([]byte(key), &parsed); err != nil {
		return nil, fmt.Errorf("payvand: parsing xml rsa key: %w", err)
	}
	if parsed.Modulus == "" || parsed.D == "" {
		return nil, errors.New("payvand: xml rsa key is missing Modulus or D")
	}

	number := func(value string) (*big.Int, error) {
		raw, err := base64.StdEncoding.DecodeString(stripWhitespace(value))
		if err != nil {
			return nil, fmt.Errorf("payvand: decoding xml rsa key component: %w", err)
		}
		return new(big.Int).SetBytes(raw), nil
	}

	modulus, err := number(parsed.Modulus)
	if err != nil {
		return nil, err
	}
	exponent, err := number(parsed.Exponent)
	if err != nil {
		return nil, err
	}
	d, err := number(parsed.D)
	if err != nil {
		return nil, err
	}
	p, err := number(parsed.P)
	if err != nil {
		return nil, err
	}
	q, err := number(parsed.Q)
	if err != nil {
		return nil, err
	}

	priv := &rsa.PrivateKey{
		PublicKey: rsa.PublicKey{N: modulus, E: int(exponent.Int64())},
		D:         d,
		Primes:    []*big.Int{p, q},
	}
	priv.Precompute()
	if err := priv.Validate(); err != nil {
		return nil, fmt.Errorf("payvand: invalid xml rsa key: %w", err)
	}
	return priv, nil
}
