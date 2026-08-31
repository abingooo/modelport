package securityaudit_test

import (
	"bytes"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
)

type generatedEvidenceVector struct {
	KeyHex        string `json:"key_hex"`
	Source        string `json:"source"`
	Plaintext     string `json:"plaintext"`
	Digest        string `json:"digest"`
	NonceHex      string `json:"nonce_hex"`
	CiphertextHex string `json:"ciphertext_hex"`
}

type generatedHashRawVector struct {
	KeyHex        string `json:"key_hex"`
	Plaintext     string `json:"plaintext"`
	Digest        string `json:"digest"`
	NonceHex      string `json:"nonce_hex"`
	CiphertextHex string `json:"ciphertext_hex"`
}

type generatedSecretVector struct {
	KeyHex           string `json:"key_hex"`
	Plaintext        string `json:"plaintext"`
	NonceHex         string `json:"nonce_hex"`
	CiphertextBase64 string `json:"ciphertext_base64"`
}

type generatedCompatibilityVectors struct {
	Evidence generatedEvidenceVector `json:"evidence"`
	HashRaw  generatedHashRawVector  `json:"hash_raw"`
	Secret   generatedSecretVector   `json:"secret_encryptor"`
}

func TestGenerateModelPortCryptoCompatibilityVectors(t *testing.T) {
	outputPath := os.Getenv("MODELPORT_CRYPTO_VECTOR_OUTPUT")
	if outputPath == "" {
		t.Fatal("MODELPORT_CRYPTO_VECTOR_OUTPUT is required")
	}
	keyHex := strings.Repeat("42", 32)
	cfg := &config.Config{Totp: config.TotpConfig{
		EncryptionKey: keyHex, EncryptionKeyConfigured: true,
	}}
	evidenceCipher, err := securityaudit.NewInstructionEvidenceCipher(cfg)
	if err != nil {
		t.Fatal(err)
	}
	secretEncryptor, err := repository.NewAESEncryptor(cfg)
	if err != nil {
		t.Fatal(err)
	}

	evidencePlaintext := "legacy evidence fixture"
	evidenceDigest := sha256Hex(evidencePlaintext)
	evidenceNonce := "000102030405060708090a0b"
	var evidenceCiphertext []byte
	withDeterministicRandom(t, evidenceNonce, func() {
		evidenceCiphertext, err = evidenceCipher.Encrypt("instructions", evidenceDigest, evidencePlaintext)
	})
	if err != nil {
		t.Fatal(err)
	}

	hashRawPlaintext := "legacy hash fixture"
	hashRawDigest := sha256Hex(hashRawPlaintext)
	hashRawNonce := "0c0d0e0f1011121314151617"
	var hashRawCiphertext []byte
	withDeterministicRandom(t, hashRawNonce, func() {
		hashRawCiphertext, err = evidenceCipher.EncryptHashRaw(hashRawDigest, hashRawPlaintext)
	})
	if err != nil {
		t.Fatal(err)
	}

	secretPlaintext := "legacy secret fixture"
	secretNonce := "18191a1b1c1d1e1f20212223"
	var secretCiphertext string
	withDeterministicRandom(t, secretNonce, func() {
		secretCiphertext, err = secretEncryptor.Encrypt(secretPlaintext)
	})
	if err != nil {
		t.Fatal(err)
	}

	vectors := generatedCompatibilityVectors{
		Evidence: generatedEvidenceVector{
			KeyHex: keyHex, Source: "instructions", Plaintext: evidencePlaintext,
			Digest: evidenceDigest, NonceHex: evidenceNonce,
			CiphertextHex: hex.EncodeToString(evidenceCiphertext),
		},
		HashRaw: generatedHashRawVector{
			KeyHex: keyHex, Plaintext: hashRawPlaintext, Digest: hashRawDigest,
			NonceHex: hashRawNonce, CiphertextHex: hex.EncodeToString(hashRawCiphertext),
		},
		Secret: generatedSecretVector{
			KeyHex: keyHex, Plaintext: secretPlaintext, NonceHex: secretNonce,
			CiphertextBase64: secretCiphertext,
		},
	}
	encoded, err := json.MarshalIndent(vectors, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if err = os.WriteFile(outputPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func withDeterministicRandom(t *testing.T, nonceHex string, fn func()) {
	t.Helper()
	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		t.Fatal(err)
	}
	previous := crand.Reader
	crand.Reader = bytes.NewReader(nonce)
	defer func() { crand.Reader = previous }()
	fn()
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
