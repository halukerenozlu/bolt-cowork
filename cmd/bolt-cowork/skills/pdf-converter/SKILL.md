---
name: pdf-converter
description: Merge, split, and convert PDF/document files
tags: ["pdf", "convert", "merge", "split", "word", "docx", "pandoc", "libreoffice", "export", "markdown"]
category: tools
version: "2.0.0"
auto_trigger: true
---
When this skill is active, handle PDF and document conversion tasks.
This skill has two tiers depending on what the user is asking for:

## Tier 1 - Native PDF operations (always available, no installation needed)

For merging multiple PDFs into one, or splitting a PDF into multiple files,
use the merge_pdf / split_pdf plan actions directly. These run natively
inside the agent and never fail due to a missing tool:
- Merging PDFs (e.g. "birleştir", "merge", "combine"): use merge_pdf with
  "sources" set to the input PDF paths in the order they should appear and
  "destination" set to the output file.
- Splitting a PDF (e.g. "böl", "split", "her sayfayı ayır"): use split_pdf
  with "path" set to the input PDF, "destination" set to the output
  directory, and "span" set to the number of pages per output file
  (1 = one PDF per page, the default).

## Tier 2 - Format conversion (requires pandoc/libreoffice on the user's machine)

For converting between PDF and other formats (Markdown, DOCX), use the
run_command action with one of these allowed executables:
- Markdown -> PDF: pandoc with --pdf-engine=xelatex
- DOCX -> PDF: libreoffice --headless --convert-to pdf
- PDF -> DOCX: libreoffice --headless --convert-to docx
- Markdown -> DOCX: pandoc -o output.docx

Rules for Tier 2:
- These tools must already be installed on the user's machine; run_command
  cannot install software. If the command fails because the executable is
  missing, tell the user and point to installation instructions:
  pandoc: https://pandoc.org/installing.html
  libreoffice: https://www.libreoffice.org/download/
- Warn that PDF->DOCX conversion may lose formatting
- Preserve the original file, create a new output file
- Show the exact command before executing it
