package securityaudit

import (
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
