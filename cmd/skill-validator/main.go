package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxSkillNameLength   = 64
	maxDescriptionLength = 1024
	maxSkillFileBytes    = 2 << 20
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: skill-validator SKILL_DIRECTORY")
		os.Exit(2)
	}
	if err := validateSkill(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "invalid Skill:", err)
		os.Exit(1)
	}
	fmt.Println("Skill is valid:", filepath.Clean(os.Args[1]))
}

func validateSkill(skillDirectory string) error {
	skillFile := filepath.Join(skillDirectory, "SKILL.md")
	file, err := os.Open(skillFile)
	if err != nil {
		return fmt.Errorf("open SKILL.md: %w", err)
	}
	defer file.Close()

	frontmatter, err := readFrontmatter(io.LimitReader(file, maxSkillFileBytes+1))
	if err != nil {
		return err
	}
	allowed := map[string]bool{
		"name": true, "description": true, "license": true,
		"allowed-tools": true, "metadata": true,
	}
	var unexpected []string
	for key := range frontmatter {
		if !allowed[key] {
			unexpected = append(unexpected, key)
		}
	}
	if len(unexpected) != 0 {
		sort.Strings(unexpected)
		return fmt.Errorf("unexpected frontmatter key(s): %s", strings.Join(unexpected, ", "))
	}

	name, ok := frontmatter["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return errors.New("frontmatter name must be a non-empty string")
	}
	name = strings.TrimSpace(name)
	if len(name) > maxSkillNameLength {
		return fmt.Errorf("frontmatter name is longer than %d characters", maxSkillNameLength)
	}
	if !skillNamePattern.MatchString(name) || strings.HasPrefix(name, "-") ||
		strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
		return errors.New("frontmatter name must use lowercase hyphen-case")
	}

	description, ok := frontmatter["description"].(string)
	if !ok || strings.TrimSpace(description) == "" {
		return errors.New("frontmatter description must be a non-empty string")
	}
	description = strings.TrimSpace(description)
	if len(description) > maxDescriptionLength {
		return fmt.Errorf("frontmatter description is longer than %d characters", maxDescriptionLength)
	}
	if strings.ContainsAny(description, "<>") {
		return errors.New("frontmatter description cannot contain angle brackets")
	}
	return nil
}

func readFrontmatter(reader io.Reader) (map[string]any, error) {
	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, maxSkillFileBytes)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return nil, errors.New("SKILL.md must begin with YAML frontmatter")
	}
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			var frontmatter map[string]any
			if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &frontmatter); err != nil {
				return nil, fmt.Errorf("parse YAML frontmatter: %w", err)
			}
			if frontmatter == nil {
				return nil, errors.New("frontmatter must be a YAML mapping")
			}
			return frontmatter, nil
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}
	return nil, errors.New("unterminated YAML frontmatter")
}
