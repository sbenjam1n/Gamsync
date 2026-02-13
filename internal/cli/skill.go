package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var skillCmd = &cobra.Command{
	Use:   "skill [name]",
	Short: "Load an agent skill prompt",
	Long: `Load and display an agent skill prompt for use as a system prompt.

Available skills:
  base-agent    Base agent prompt (common to all roles)
  memorizer     Memorizer skill (auditor, validator, plan manager)
  researcher    Researcher skill (coder, proposal emitter)
  gardener      Gardener skill (entropy sweep, quality enforcement)

Usage:
  gam skill memorizer              Print the Memorizer skill prompt
  gam skill researcher --full      Print base-agent + Researcher skill combined
  gam skill list                   List available skills`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("specify a skill name, or use 'gam skill list'")
		}

		name := args[0]
		if name == "list" {
			return listSkills()
		}

		full, _ := cmd.Flags().GetBool("full")
		return printSkill(name, full)
	},
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available skills",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listSkills()
	},
}

func listSkills() error {
	skillsDir := findSkillsDir()
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return fmt.Errorf("read skills directory: %w\nExpected skills/ in project root", err)
	}

	fmt.Println("Available skills:")
	fmt.Println()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")

		// Read first non-empty, non-heading line as description
		data, _ := os.ReadFile(filepath.Join(skillsDir, e.Name()))
		desc := extractDescription(string(data))

		fmt.Printf("  %-15s %s\n", name, desc)
	}

	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  gam skill <name>          Print the skill prompt")
	fmt.Println("  gam skill <name> --full   Print base-agent + skill combined")
	return nil
}

func printSkill(name string, full bool) error {
	skillsDir := findSkillsDir()

	if full && name != "base-agent" {
		// Print base-agent first, then the requested skill
		basePath := filepath.Join(skillsDir, "base-agent.md")
		baseContent, err := os.ReadFile(basePath)
		if err != nil {
			return fmt.Errorf("read base-agent skill: %w", err)
		}
		fmt.Println(string(baseContent))
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
	}

	skillPath := filepath.Join(skillsDir, name+".md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("skill '%s' not found. Run 'gam skill list' to see available skills", name)
	}

	fmt.Println(string(content))
	return nil
}

func findSkillsDir() string {
	// Check project root first
	dir := filepath.Join(projectRoot(), "skills")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}

	// Check relative to executable
	exe, _ := os.Executable()
	dir = filepath.Join(filepath.Dir(exe), "skills")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}

	// Fallback to project root
	return filepath.Join(projectRoot(), "skills")
}

func extractDescription(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Return first paragraph line
		if len(line) > 80 {
			return line[:77] + "..."
		}
		return line
	}
	return ""
}

func init() {
	skillCmd.Flags().Bool("full", false, "Combine base-agent prompt with the requested skill")

	skillCmd.AddCommand(skillListCmd)
}
