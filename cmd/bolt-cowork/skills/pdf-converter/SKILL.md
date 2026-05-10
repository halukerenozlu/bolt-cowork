---
name: pdf-converter
description: Convert documents between PDF and other formats using pandoc or libreoffice
tags: ["pdf", "convert", "word", "docx", "pandoc", "libreoffice", "export", "markdown"]
category: tools
version: "1.0.0"
auto_trigger: true
---
When this skill is active, convert documents between formats.
Follow these rules:
- Detect available tools: check if pandoc and/or libreoffice is installed
- Preferred conversion paths:
  Markdown -> PDF: pandoc with --pdf-engine=xelatex
  DOCX -> PDF: libreoffice --headless --convert-to pdf
  PDF -> DOCX: libreoffice --headless --convert-to docx
  Markdown -> DOCX: pandoc -o output.docx
- If neither tool is installed, provide installation instructions:
  pandoc: https://pandoc.org/installing.html
  libreoffice: https://www.libreoffice.org/download/
- Warn that PDF->DOCX conversion may lose formatting
- Preserve original file, create new output file
- Show the exact command before executing
