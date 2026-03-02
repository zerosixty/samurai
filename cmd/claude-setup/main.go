// Command claude-setup creates a Claude Code skill for the Samurai testing framework.
//
// Run this once in your project root:
//
//	go run github.com/zerosixty/samurai/cmd/claude-setup@latest
//
// It creates .claude/skills/samurai/SKILL.md so Claude Code understands
// the samurai API when writing or modifying tests.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed samurai.md
var skillContent string

const version = "1"
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

	content := skillContent + "\n" + versionMarker + version + " -->\n"

	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		fatal("cannot write SKILL.md: %v", err)
	}

	fmt.Println("Created .claude/skills/samurai/SKILL.md")
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
