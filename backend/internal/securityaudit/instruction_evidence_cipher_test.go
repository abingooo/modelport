package securityaudit

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestInstructionEvidenceCipherRequiresFixedKey(t *testing.T) {
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("42", 32), EncryptionKeyConfigured: false},
	})
	require.NoError(t, err)
	require.False(t, cipher.Available())
	_, err = cipher.Encrypt("instructions", sha256Hex("secret"), "secret")
	require.ErrorIs(t, err, errInstructionEvidenceEncryptionUnavailable)
}

func TestInstructionEvidenceCipherRoundTripAndAADIsolation(t *testing.T) {
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("42", 32), EncryptionKeyConfigured: true},
	})
	require.NoError(t, err)
	require.True(t, cipher.Available())
	digest := sha256Hex("exact\ntext")
	encrypted, err := cipher.Encrypt("instructions", digest, "exact\ntext")
	require.NoError(t, err)
	require.NotContains(t, string(encrypted), "exact")
	plaintext, err := cipher.Decrypt("instructions", digest, encrypted)
	require.NoError(t, err)
	require.Equal(t, "exact\ntext", plaintext)
	_, err = cipher.Decrypt("input1", digest, encrypted)
	require.Error(t, err)
}

func TestInstructionHashRawCipherUsesDedicatedPurposeAndDigestAAD(t *testing.T) {
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("42", 32), EncryptionKeyConfigured: true},
	})
	require.NoError(t, err)
	digest := sha256Hex("standard instruction")
	encrypted, err := cipher.EncryptHashRaw(digest, "standard instruction")
	require.NoError(t, err)
	plaintext, err := cipher.DecryptHashRaw(digest, encrypted)
	require.NoError(t, err)
	require.Equal(t, "standard instruction", plaintext)
	_, err = cipher.DecryptHashRaw(sha256Hex("other"), encrypted)
	require.Error(t, err)
	_, err = cipher.Decrypt("instructions", digest, encrypted)
	require.Error(t, err)
}

func TestInstructionEvidenceCipherReadsLegacyModelPortVectors(t *testing.T) {
	manifest := loadInstructionCryptoCompatibilityManifest(t)
	require.Equal(t, "custom-v0.1.176.2", manifest.Source.Tag)
	require.Equal(t, "b6cb4d0c8b47d7561631ab61418e1b6fdeb379bc", manifest.Source.Commit)
	require.Equal(t, strings.Repeat("42", 32), manifest.Vectors.Evidence.KeyHex)
	require.Equal(t, manifest.Vectors.Evidence.KeyHex, manifest.Vectors.HashRaw.KeyHex)
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: manifest.Vectors.Evidence.KeyHex, EncryptionKeyConfigured: true},
	})
	require.NoError(t, err)
	wrongKeyCipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("43", 32), EncryptionKeyConfigured: true},
	})
	require.NoError(t, err)

	evidence := manifest.Vectors.Evidence
	evidenceCiphertext, err := hex.DecodeString(evidence.CiphertextHex)
	require.NoError(t, err)
	evidenceNonce, err := hex.DecodeString(evidence.NonceHex)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(evidenceCiphertext), len(evidenceNonce))
	require.Equal(t, evidenceNonce, evidenceCiphertext[:len(evidenceNonce)])
	plaintext, err := cipher.Decrypt(evidence.Source, evidence.Digest, evidenceCiphertext)
	require.NoError(t, err)
	require.Equal(t, evidence.Plaintext, plaintext)
	_, err = cipher.Decrypt("input1", evidence.Digest, evidenceCiphertext)
	require.Error(t, err)
	_, err = wrongKeyCipher.Decrypt(evidence.Source, evidence.Digest, evidenceCiphertext)
	require.Error(t, err)

	hashRaw := manifest.Vectors.HashRaw
	hashCiphertext, err := hex.DecodeString(hashRaw.CiphertextHex)
	require.NoError(t, err)
	hashNonce, err := hex.DecodeString(hashRaw.NonceHex)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(hashCiphertext), len(hashNonce))
	require.Equal(t, hashNonce, hashCiphertext[:len(hashNonce)])
	plaintext, err = cipher.DecryptHashRaw(hashRaw.Digest, hashCiphertext)
	require.NoError(t, err)
	require.Equal(t, hashRaw.Plaintext, plaintext)
	_, err = cipher.DecryptHashRaw(evidence.Digest, hashCiphertext)
	require.Error(t, err)
	_, err = wrongKeyCipher.DecryptHashRaw(hashRaw.Digest, hashCiphertext)
	require.Error(t, err)

	tampered := append([]byte(nil), hashCiphertext...)
	tampered[len(tampered)-1] ^= 0x01
	_, err = cipher.DecryptHashRaw(hashRaw.Digest, tampered)
	require.Error(t, err)
}

type instructionCryptoCompatibilityManifest struct {
	Source struct {
		Tag    string `json:"tag"`
		Commit string `json:"commit"`
	} `json:"source"`
	Vectors struct {
		Evidence struct {
			KeyHex        string `json:"key_hex"`
			Source        string `json:"source"`
			Plaintext     string `json:"plaintext"`
			Digest        string `json:"digest"`
			NonceHex      string `json:"nonce_hex"`
			CiphertextHex string `json:"ciphertext_hex"`
		} `json:"evidence"`
		HashRaw struct {
			KeyHex        string `json:"key_hex"`
			Plaintext     string `json:"plaintext"`
			Digest        string `json:"digest"`
			NonceHex      string `json:"nonce_hex"`
			CiphertextHex string `json:"ciphertext_hex"`
		} `json:"hash_raw"`
	} `json:"vectors"`
}

func loadInstructionCryptoCompatibilityManifest(t *testing.T) instructionCryptoCompatibilityManifest {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	manifestPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "testdata", "modelport_crypto_compat_v1.json")
	contents, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest instructionCryptoCompatibilityManifest
	require.NoError(t, json.Unmarshal(contents, &manifest))
	return manifest
}
