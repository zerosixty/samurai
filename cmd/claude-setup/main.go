// Command claude-setup creates a Claude Code skill for the Samurai testing framework.
//
// Run this once in your project root:
//
//	go run github.com/zerosixty/samurai/cmd/claude-setup@latest
//
// It creates .claude/skills/samurai/ with SKILL.md, api.md and pitfalls.md
// so Claude Code understands the samurai API when writing or modifying tests.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed skill/SKILL.md
var skillContent string

//go:embed skill/api.md
var apiContent string

//go:embed skill/pitfalls.md
var pitfallsContent string

const version = "2"
const versionMarker = "<!-- samurai-skill-v"

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fatal("cannot determine working directory: %v", err)
	}

	if isSamuraiRepo(dir) {
		fatal("you are inside the samurai repository itself.\nRun this in your project directory:\n  go run github.com/zerosixty/samurai/cmd/claude-setup@latest")
	}

	skillDir := filepath.Join(dir, ".claude", "skills", "samurai")
	skillFile := filepath.Join(skillDir, "SKILL.md")

	// Check if already installed and up to date.
	if existing, err := os.ReadFile(skillFile); err == nil {
		if strings.Contains(string(existing), versionMarker+version+" -->") {
			fmt.Println("samurai skill is already up to date (v" + version + ")")
			return
		}
		fmt.Println("Updating samurai skill...")
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		fatal("cannot create .claude/skills/samurai/: %v", err)
	}

	// Write all skill files.
	files := map[string]string{
		"SKILL.md":    skillContent + "\n" + versionMarker + version + " -->\n",
		"api.md":      apiContent,
		"pitfalls.md": pitfallsContent,
	}

	for name, content := range files {
		path := filepath.Join(skillDir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			fatal("cannot write %s: %v", name, err)
		}
	}

	// Clean up old single-file layout (v1).
	oldSamurai := filepath.Join(skillDir, "samurai.md")
	os.Remove(oldSamurai) // ignore error — may not exist

	fmt.Println("Created .claude/skills/samurai/ (SKILL.md, api.md, pitfalls.md)")
	fmt.Println("Claude Code will use samurai context when working with tests.")
	fmt.Println("Invoke /samurai to load the reference manually.")
}

func isSamuraiRepo(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "module github.com/zerosixty/samurai\n")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "claude-setup: "+format+"\n", args...)
	os.Exit(1)
}
