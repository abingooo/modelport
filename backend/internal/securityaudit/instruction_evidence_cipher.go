package securityaudit

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const instructionEvidenceKeyVersion = "instruction-audit-evidence-v1"

var errInstructionEvidenceEncryptionUnavailable = errors.New("instruction audit evidence encryption unavailable")

type InstructionEvidenceCipher struct {
	aead cipher.AEAD
}

func NewInstructionEvidenceCipher(cfg *config.Config) (*InstructionEvidenceCipher, error) {
	result := &InstructionEvidenceCipher{}
	if cfg == nil || !cfg.Totp.EncryptionKeyConfigured {
		return result, nil
	}
	root, err := hex.DecodeString(cfg.Totp.EncryptionKey)
	if err != nil || len(root) != 32 {
		return nil, fmt.Errorf("invalid fixed encryption key for instruction evidence")
	}
	deriver := hmac.New(sha256.New, root)
	_, _ = deriver.Write([]byte("modelport/instruction-audit/evidence/v1"))
	key := deriver.Sum(nil)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create instruction evidence cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create instruction evidence gcm: %w", err)
	}
	result.aead = aead
	return result, nil
}

func (c *InstructionEvidenceCipher) Available() bool {
	return c != nil && c.aead != nil
}

func (c *InstructionEvidenceCipher) Encrypt(source, digest, plaintext string) ([]byte, error) {
	if !c.Available() {
		return nil, errInstructionEvidenceEncryptionUnavailable
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate instruction evidence nonce: %w", err)
	}
	aad := instructionEvidenceAAD(source, digest)
	return c.aead.Seal(nonce, nonce, []byte(plaintext), aad), nil
}

func (c *InstructionEvidenceCipher) Decrypt(source, digest string, ciphertext []byte) (string, error) {
	if !c.Available() {
		return "", errInstructionEvidenceEncryptionUnavailable
	}
	if len(ciphertext) < c.aead.NonceSize() {
		return "", errors.New("instruction evidence ciphertext too short")
	}
	nonce, encrypted := ciphertext[:c.aead.NonceSize()], ciphertext[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, encrypted, instructionEvidenceAAD(source, digest))
	if err != nil {
		return "", fmt.Errorf("decrypt instruction evidence: %w", err)
	}
	return string(plaintext), nil
}

func instructionEvidenceAAD(source, digest string) []byte {
	return []byte(instructionEvidenceKeyVersion + "\x00" + source + "\x00" + digest)
}
