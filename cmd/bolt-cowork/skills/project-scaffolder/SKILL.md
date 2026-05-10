---
name: project-scaffolder
description: Create project structure and boilerplate files
tags: ["scaffold-project", "project-init", "boilerplate", "project-template", "project-setup"]
category: development
version: "1.0.0"
auto_trigger: true
---
When this skill is active, create project scaffolding.
Follow these rules:
- Ask for language/framework if not specified
- Create standard directory structure for the chosen stack
- Include essential config files (.gitignore, README.md, LICENSE)
- For Go projects: go.mod, cmd/, internal/, pkg/ layout
- For Node.js: package.json, src/, .eslintrc
- For Python: pyproject.toml, src/, tests/
- Add placeholder test files
- Do not include actual application logic, only structure
- Explain the purpose of each created directory
