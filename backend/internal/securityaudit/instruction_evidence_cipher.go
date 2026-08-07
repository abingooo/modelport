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
const instructionHashRawKeyVersion = "instruction-audit-hash-raw-v1"

var errInstructionEvidenceEncryptionUnavailable = errors.New("instruction audit evidence encryption unavailable")

type InstructionEvidenceCipher struct {
	aead        cipher.AEAD
	hashRawAEAD cipher.AEAD
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
	result.aead, err = deriveInstructionAEAD(root, "modelport/instruction-audit/evidence/v1")
	if err != nil {
		return nil, fmt.Errorf("create instruction evidence cipher: %w", err)
	}
	result.hashRawAEAD, err = deriveInstructionAEAD(root, "modelport/instruction-audit/hash-raw/v1")
	if err != nil {
		return nil, fmt.Errorf("create instruction hash raw cipher: %w", err)
	}
	return result, nil
}

func deriveInstructionAEAD(root []byte, purpose string) (cipher.AEAD, error) {
	deriver := hmac.New(sha256.New, root)
	_, _ = deriver.Write([]byte(purpose))
	block, err := aes.NewCipher(deriver.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (c *InstructionEvidenceCipher) Available() bool {
	return c != nil && c.aead != nil && c.hashRawAEAD != nil
}

func (c *InstructionEvidenceCipher) EncryptHashRaw(digest, plaintext string) ([]byte, error) {
	if !c.Available() {
		return nil, errInstructionEvidenceEncryptionUnavailable
	}
	return sealInstructionContent(c.hashRawAEAD, instructionHashRawAAD(digest), plaintext)
}

func (c *InstructionEvidenceCipher) DecryptHashRaw(digest string, ciphertext []byte) (string, error) {
	if !c.Available() {
		return "", errInstructionEvidenceEncryptionUnavailable
	}
	return openInstructionContent(c.hashRawAEAD, instructionHashRawAAD(digest), ciphertext)
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

func instructionHashRawAAD(digest string) []byte {
	return []byte(instructionHashRawKeyVersion + "\x00" + digest)
}

func sealInstructionContent(aead cipher.AEAD, aad []byte, plaintext string) ([]byte, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate instruction content nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, []byte(plaintext), aad), nil
}

func openInstructionContent(aead cipher.AEAD, aad, ciphertext []byte) (string, error) {
	if len(ciphertext) < aead.NonceSize() {
		return "", errors.New("instruction content ciphertext too short")
	}
	nonce, encrypted := ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, encrypted, aad)
	if err != nil {
		return "", fmt.Errorf("decrypt instruction content: %w", err)
	}
	return string(plaintext), nil
}
