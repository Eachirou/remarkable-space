package obsidian

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/boangs/remarkable-space/internal/rmscene"
)

type Exporter struct {
	Source string
	Vault  string
	Now    func() time.Time
}

type Result struct {
	Documents int
	Pages     int
	TypedText int
}

type metadata struct {
	VisibleName string `json:"visibleName"`
	Type        string `json:"type"`
	Deleted     bool   `json:"deleted"`
	Parent      string `json:"parent"`
}

type content struct {
	FileType string `json:"fileType"`
	CPages   struct {
		Pages []struct {
			ID string `json:"id"`
		} `json:"pages"`
	} `json:"cPages"`
}

func (e Exporter) Run() (Result, error) {
	if e.Source == "" || e.Vault == "" {
		return Result{}, errors.New("source and vault are required")
	}
	if e.Now == nil {
		e.Now = time.Now
	}
	if _, err := os.Stat(e.Source); err != nil {
		return Result{}, fmt.Errorf("source: %w", err)
	}
	attachments := filepath.Join(e.Vault, "_remarkable")
	if err := os.MkdirAll(attachments, 0o755); err != nil {
		return Result{}, fmt.Errorf("create vault: %w", err)
	}

	entries, err := os.ReadDir(e.Source)
	if err != nil {
		return Result{}, fmt.Errorf("read source: %w", err)
	}
	var ids []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".metadata") {
			ids = append(ids, strings.TrimSuffix(entry.Name(), ".metadata"))
		}
	}
	sort.Strings(ids)

	var result Result
	for _, id := range ids {
		n, err := e.exportEntry(id, attachments)
		if err != nil {
			return result, fmt.Errorf("export %s: %w", id, err)
		}
		if n.Documents > 0 {
			result.Documents += n.Documents
			result.Pages += n.Pages
			result.TypedText += n.TypedText
		}
	}
	return result, nil
}

func (e Exporter) exportEntry(id, attachments string) (Result, error) {
	metaBytes, err := os.ReadFile(filepath.Join(e.Source, id+".metadata"))
	if err != nil {
		return Result{}, err
	}
	var meta metadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return Result{}, fmt.Errorf("metadata: %w", err)
	}
	if meta.Deleted || meta.Type != "DocumentType" {
		return Result{}, nil
	}
	contentBytes, err := os.ReadFile(filepath.Join(e.Source, id+".content"))
	if err != nil {
		return Result{}, err
	}
	var c content
	if err := json.Unmarshal(contentBytes, &c); err != nil {
		return Result{}, fmt.Errorf("content: %w", err)
	}

	name := cleanName(meta.VisibleName)
	if c.FileType == "pdf" || c.FileType == "epub" {
		ext := "." + c.FileType
		src := filepath.Join(e.Source, id+ext)
		dst := filepath.Join(attachments, id+ext)
		if err := copyFile(src, dst); err != nil {
			return Result{}, err
		}
		body := fmt.Sprintf("# %s\n\n[Open the original %s](%s)\n", name, strings.ToUpper(c.FileType), filepath.ToSlash(filepath.Join("_remarkable", id+ext)))
		if err := writeNote(filepath.Join(e.Vault, name+".md"), body); err != nil {
			return Result{}, err
		}
		return Result{Documents: 1}, nil
	}
	if c.FileType != "notebook" {
		return Result{}, fmt.Errorf("unsupported file type %q", c.FileType)
	}

	dir := filepath.Join(attachments, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}
	var body strings.Builder
	body.WriteString("# " + name + "\n\n")
	body.WriteString("> Handwritten pages are preserved as `.rm` source files. OCR is not run until a local renderer and OCR engine are configured.\n\n")
	result := Result{Documents: 1}
	for page, p := range c.CPages.Pages {
		if p.ID == "" {
			continue
		}
		src := filepath.Join(e.Source, id, p.ID+".rm")
		dst := filepath.Join(dir, fmt.Sprintf("page-%03d.rm", page+1))
		if err := copyFile(src, dst); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return Result{}, err
		}
		result.Pages++
		body.WriteString(fmt.Sprintf("## Page %d\n\n[Open the reMarkable source page](%s)\n\n", page+1,
			filepath.ToSlash(filepath.Join("_remarkable", id, filepath.Base(dst)))))
		text, err := rmscene.ExtractTextFile(src)
		if err != nil {
			return Result{}, fmt.Errorf("page %s: %w", p.ID, err)
		}
		if strings.TrimSpace(text) != "" {
			result.TypedText++
			body.WriteString("### Embedded text\n\n" + text + "\n\n")
		}
	}
	if err := writeNote(filepath.Join(e.Vault, name+".md"), body.String()); err != nil {
		return Result{}, err
	}
	return result, nil
}

func cleanName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Untitled"
	}
	name = strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(name)
	name = strings.Trim(name, ". ")
	if name == "" {
		return "Untitled"
	}
	return name
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".remarkable-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

func writeNote(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}
