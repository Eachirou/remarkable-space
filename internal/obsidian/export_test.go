package obsidian

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportNotebookPreservesPagesAndTypedText(t *testing.T) {
	source := t.TempDir()
	vault := t.TempDir()
	id := "doc-1"
	page := "page-1"
	write := func(name, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(source, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(id+".metadata", `{"visibleName":"Daily notes","type":"DocumentType"}`)
	write(id+".content", `{"fileType":"notebook","cPages":{"pages":[{"id":"page-1"}]}}`)
	write(filepath.Join(id, page+".rm"), string(testRMFile(t)))

	result, err := (Exporter{Source: source, Vault: vault}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if result.Documents != 1 || result.Pages != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	body, err := os.ReadFile(filepath.Join(vault, "Daily notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Embedded text") {
		t.Fatalf("typed text missing: %s", body)
	}
	if _, err := os.Stat(filepath.Join(vault, "_remarkable", id, "page-001.rm")); err != nil {
		t.Fatal(err)
	}
}

func testRMFile(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "rmscene", "testdata", "typed_text_zh.rm"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
