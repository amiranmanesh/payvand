package cryptox_test

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/amiranmanesh/payvand/internal/cryptox"
)

func TestPKCS7PadFillsWholeBlocks(t *testing.T) {
	padded := cryptox.PKCS7Pad([]byte("payvand"), 8)
	if len(padded)%8 != 0 {
		t.Fatalf("length = %d, want a multiple of 8", len(padded))
	}
	if got := padded[len(padded)-1]; got != 1 {
		t.Fatalf("padding byte = %d, want 1", got)
	}
}

func TestTripleDESEncryptECBIsDeterministic(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef01234567"))

	first, err := cryptox.TripleDESEncryptECB([]byte("12345678;1001;150000"), key)
	if err != nil {
		t.Fatalf("TripleDESEncryptECB() error = %v", err)
	}
	second, _ := cryptox.TripleDESEncryptECB([]byte("12345678;1001;150000"), key)
	if first != second {
		t.Fatal("ECB mode must produce the same cipher text for the same input")
	}
	if _, err := base64.StdEncoding.DecodeString(first); err != nil {
		t.Fatalf("the result is not base64: %v", err)
	}
}

func TestTripleDESRejectsBadKey(t *testing.T) {
	if _, err := cryptox.TripleDESEncryptECB([]byte("data"), "not base64!"); err == nil {
		t.Fatal("a malformed key must be rejected")
	}
}

func TestAESCBCEncryptRoundTrip(t *testing.T) {
	key, _ := cryptox.RandomBytes(16)
	iv, _ := cryptox.RandomBytes(aes.BlockSize)
	plain := []byte("payvand aes payload")

	encrypted, err := cryptox.AESCBCEncrypt(key, iv, plain)
	if err != nil {
		t.Fatalf("AESCBCEncrypt() error = %v", err)
	}

	block, _ := aes.NewCipher(key)
	decrypted := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(decrypted, encrypted)
	if !bytes.HasPrefix(decrypted, plain) {
		t.Fatalf("decrypted = %q, want it to start with %q", decrypted, plain)
	}
}

func TestRSAKeysAndSignatures(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating the key: %v", err)
	}

	publicDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))
	privatePEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))

	parsedPublic, err := cryptox.ParseRSAPublicKey(publicPEM)
	if err != nil {
		t.Fatalf("ParseRSAPublicKey() error = %v", err)
	}
	sealed, err := cryptox.EncryptPKCS1v15(parsedPublic, []byte("session key"))
	if err != nil {
		t.Fatalf("EncryptPKCS1v15() error = %v", err)
	}
	opened, err := rsa.DecryptPKCS1v15(rand.Reader, key, sealed)
	if err != nil || string(opened) != "session key" {
		t.Fatalf("the envelope did not round trip: %v / %q", err, opened)
	}

	parsedPrivate, err := cryptox.ParseRSAPrivateKey(privatePEM)
	if err != nil {
		t.Fatalf("ParseRSAPrivateKey() error = %v", err)
	}
	signature, err := cryptox.SignPKCS1v15SHA1(parsedPrivate, []byte("body"))
	if err != nil {
		t.Fatalf("SignPKCS1v15SHA1() error = %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(signature)
	digest := sha1.Sum([]byte("body")) //nolint:gosec // the gateway protocol mandates SHA-1
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA1, digest[:], raw); err != nil {
		t.Fatalf("the signature does not verify: %v", err)
	}
}

func TestParseRSAPrivateKeyAcceptsDotNetXML(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating the key: %v", err)
	}
	encode := func(value []byte) string { return base64.StdEncoding.EncodeToString(value) }

	xmlKey := "<RSAKeyValue>" +
		"<Modulus>" + encode(key.N.Bytes()) + "</Modulus>" +
		"<Exponent>" + encode([]byte{1, 0, 1}) + "</Exponent>" +
		"<P>" + encode(key.Primes[0].Bytes()) + "</P>" +
		"<Q>" + encode(key.Primes[1].Bytes()) + "</Q>" +
		"<D>" + encode(key.D.Bytes()) + "</D>" +
		"</RSAKeyValue>"

	parsed, err := cryptox.ParseRSAPrivateKey(xmlKey)
	if err != nil {
		t.Fatalf("ParseRSAPrivateKey() error = %v", err)
	}
	if parsed.N.Cmp(key.N) != 0 {
		t.Fatal("the modulus was not restored")
	}
}

func TestParseRSAPrivateKeyRejectsGarbage(t *testing.T) {
	if _, err := cryptox.ParseRSAPrivateKey("definitely not a key"); err == nil {
		t.Fatal("garbage must be rejected")
	}
}
