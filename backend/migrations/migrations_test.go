package migrations

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"
)

const legacyModelPortMigrationRoot = "modelport_legacy/v0.1.176.2"

type legacyMigrationHashes struct {
	raw     string
	trimmed string
}

func TestLegacyModelPortMigrationArchiveMatchesManifest(t *testing.T) {
	manifest, err := LegacyFS.ReadFile(legacyModelPortMigrationRoot + "/manifest.tsv")
	if err != nil {
		t.Fatalf("read legacy migration manifest: %v", err)
	}

	expected := make(map[string]legacyMigrationHashes)
	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if lineNumber == 1 {
			if line != "# filename\traw_sha256\trunner_trimmed_sha256" {
				t.Fatalf("unexpected manifest header: %q", line)
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("manifest line %d has %d fields, want 3", lineNumber, len(fields))
		}
		name, raw, trimmed := fields[0], fields[1], fields[2]
		if !strings.HasSuffix(name, ".sql") || strings.Contains(name, "/") {
			t.Fatalf("manifest line %d has invalid migration name %q", lineNumber, name)
		}
		for label, value := range map[string]string{"raw": raw, "trimmed": trimmed} {
			decoded, decodeErr := hex.DecodeString(value)
			if decodeErr != nil || len(decoded) != sha256.Size {
				t.Fatalf("manifest line %d has invalid %s SHA-256 %q", lineNumber, label, value)
			}
		}
		if _, duplicate := expected[name]; duplicate {
			t.Fatalf("manifest contains duplicate migration %q", name)
		}
		expected[name] = legacyMigrationHashes{raw: raw, trimmed: trimmed}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan legacy migration manifest: %v", err)
	}
	if got, want := len(expected), 38; got != want {
		t.Fatalf("manifest contains %d migrations, want %d", got, want)
	}

	entries, err := fs.ReadDir(LegacyFS, legacyModelPortMigrationRoot)
	if err != nil {
		t.Fatalf("list legacy migration archive: %v", err)
	}
	actual := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		actual[entry.Name()] = struct{}{}
		hashes, ok := expected[entry.Name()]
		if !ok {
			t.Errorf("legacy archive contains unmanifested migration %q", entry.Name())
			continue
		}

		content, readErr := LegacyFS.ReadFile(legacyModelPortMigrationRoot + "/" + entry.Name())
		if readErr != nil {
			t.Errorf("read legacy migration %q: %v", entry.Name(), readErr)
			continue
		}
		rawSum := sha256.Sum256(content)
		trimmedSum := sha256.Sum256([]byte(strings.TrimSpace(string(content))))
		if got := hex.EncodeToString(rawSum[:]); got != hashes.raw {
			t.Errorf("legacy migration %q raw checksum = %s, want %s", entry.Name(), got, hashes.raw)
		}
		if got := hex.EncodeToString(trimmedSum[:]); got != hashes.trimmed {
			t.Errorf("legacy migration %q runner checksum = %s, want %s", entry.Name(), got, hashes.trimmed)
		}
	}

	for name := range expected {
		if _, ok := actual[name]; !ok {
			t.Errorf("manifested legacy migration %q is missing", name)
		}
		if _, err := FS.ReadFile(legacyModelPortMigrationRoot + "/" + name); err == nil {
			t.Errorf("legacy migration %q is embedded in the active migration FS", name)
		}
	}
}
