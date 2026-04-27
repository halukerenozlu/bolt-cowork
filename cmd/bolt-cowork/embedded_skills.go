package main

import "embed"

// embeddedSkillsFS holds the default bundled skills compiled into the binary.
//
//go:embed skills
var embeddedSkillsFS embed.FS
