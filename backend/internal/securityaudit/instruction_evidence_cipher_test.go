package securityaudit

import (
	"encoding/hex"
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
	cipher, err := NewInstructionEvidenceCipher(&config.Config{
		Totp: config.TotpConfig{EncryptionKey: strings.Repeat("42", 32), EncryptionKeyConfigured: true},
	})
	require.NoError(t, err)

	evidenceDigest := "ac271b6c16370f9a830d0df6f1b4c12d4e6cbfe3a3474cbe33c7325f643554e5"
	evidenceCiphertext, err := hex.DecodeString("9151d38f200b37b697a5718d99969943d423478e5bb494dd2f35c57516547381d06ed3a595805a278e3a3d1d4a17946939d189")
	require.NoError(t, err)
	plaintext, err := cipher.Decrypt("instructions", evidenceDigest, evidenceCiphertext)
	require.NoError(t, err)
	require.Equal(t, "legacy evidence fixture", plaintext)
	_, err = cipher.Decrypt("input1", evidenceDigest, evidenceCiphertext)
	require.Error(t, err)

	hashDigest := "5beb50036a07c0e28ece6445a3b76b4f720cd29d251f6af2d16e0fd390ae537c"
	hashCiphertext, err := hex.DecodeString("0e3e0fb32f4a95ff9d812f1f7fdfeabf34be5d0b8be4894f196cc69e16c084d7a009d4cfce81b9bb98ce9ab6f89e89")
	require.NoError(t, err)
	plaintext, err = cipher.DecryptHashRaw(hashDigest, hashCiphertext)
	require.NoError(t, err)
	require.Equal(t, "legacy hash fixture", plaintext)
	_, err = cipher.DecryptHashRaw(evidenceDigest, hashCiphertext)
	require.Error(t, err)

	tampered := append([]byte(nil), hashCiphertext...)
	tampered[len(tampered)-1] ^= 0x01
	_, err = cipher.DecryptHashRaw(hashDigest, tampered)
	require.Error(t, err)
}
