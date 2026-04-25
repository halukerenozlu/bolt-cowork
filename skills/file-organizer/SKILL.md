---
name: file-organizer
description: Organize files by type extension into directories
auto_trigger: true
---
When this skill is active, organize files in the target directory
by their file extensions. Group files into subdirectories:
- documents/ (.pdf, .docx, .txt, .md)
- images/ (.jpg, .jpeg, .png, .gif, .svg, .webp)
- code/ (.go, .py, .js, .ts, .html, .css)
- data/ (.json, .yaml, .yml, .csv, .xml)
- archives/ (.zip, .tar, .gz, .rar)
- other/ (everything else)

Rules:
- Do not move hidden files (starting with .)
- Do not move directories
- Create subdirectories only when needed
- Show a summary of moved files after completion
