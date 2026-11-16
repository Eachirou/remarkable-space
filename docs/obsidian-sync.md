# reMarkable to Obsidian export

`remarkable-sync` creates one Markdown note per reMarkable document. It reads a
local copy of the tablet's `xochitl` directory and writes into an Obsidian
vault:

- PDFs and EPUBs are copied to `_remarkable/` and linked from the note.
- Notebook `.rm` pages are copied unchanged to `_remarkable/<document-id>/`.
- Embedded typed text in v6 `.rm` files is extracted into the Markdown note.
- Handwritten strokes remain available in the original `.rm` files.

The exporter is deliberately local. It does not send notes to Scrybble or any
OCR service. Scrybble has no documented public API, and its website currently
describes handwriting-to-Markdown as an upcoming feature.

## Export from the verified backup

Build and run:

```sh
go build -o remarkable-sync ./cmd/remarkable-sync
./remarkable-sync \
  -source /media/ervan/9C33-6BBD/backup/remarkable-backup/files \
  -vault "$HOME/Documents/MyVault"
```

Run it again after a new backup to refresh the generated notes and attachments.
The source is read-only; the exporter never modifies the tablet or backup.

## Export from the tablet over SSH

Keep the tablet's SSH password out of command arguments. Stage a copy first so
the exporter has the same deterministic input on Linux and Windows:

```sh
mkdir -p /tmp/remarkable-xochitl
scp -r root@remarkable.local:/home/root/.local/share/remarkable/xochitl/. \
  /tmp/remarkable-xochitl/
./remarkable-sync -source /tmp/remarkable-xochitl -vault "$HOME/Documents/MyVault"
```

On Windows, use the same `scp` command in PowerShell with a destination such
as `$HOME\remarkable-xochitl`; OpenSSH is included in current Windows 10/11.

## OCR status

This first milestone does not OCR handwriting. Correct handwriting OCR needs a
renderer for reMarkable stroke scenes and a local OCR engine; extracting typed
text is not equivalent to recognizing handwriting. The next implementation
step can add a local renderer and an opt-in Tesseract pipeline without
uploading note contents.
