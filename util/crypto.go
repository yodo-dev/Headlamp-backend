package util

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

// GenerateKeyPair creates a new RSA public/private key pair.
// It returns the keys in PEM-encoded []byte format.
func GenerateKeyPair() (publicKeyPEM []byte, privateKeyPEM []byte, err error) {
	// Generate a new RSA key pair with a 2048-bit key size.
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	publicKey := &privateKey.PublicKey

	// Encode the private key to the PKCS#8 format, which is a standard format.
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	privateKeyPEMBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	privateKeyPEM = pem.EncodeToMemory(privateKeyPEMBlock)

	// Encode the public key to the PKIX format, also a standard.
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, nil, err
	}
	publicKeyPEMBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	publicKeyPEM = pem.EncodeToMemory(publicKeyPEMBlock)

	return publicKeyPEM, privateKeyPEM, nil
}

// EncryptWithPublicKey encrypts data with public key
func EncryptWithPublicKey(msg []byte, pub []byte) ([]byte, error) {
	block, _ := pem.Decode(pub)
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the public key")
	}

	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaPub, ok := pubInterface.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("key type is not RSA")
	}

	return rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaPub, msg, nil)
}

// DecryptWithPrivateKey decrypts data with private key
func DecryptWithPrivateKey(ciphertext []byte, priv []byte) ([]byte, error) {
	block, _ := pem.Decode(priv)
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the private key")
	}

	privInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaPriv, ok := privInterface.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("key type is not RSA")
	}

	return rsa.DecryptOAEP(sha256.New(), rand.Reader, rsaPriv, ciphertext, nil)
}
