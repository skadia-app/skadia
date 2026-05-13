package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultSkillsDir = "config/agent/skills"

type Skill struct {
	Name            string           `yaml:"name"`
	Description     string           `yaml:"description"`
	SchemaExcerpt   string           `yaml:"schema_excerpt"`
	DomainKnowledge string           `yaml:"domain_knowledge"`
	Tools           []string         `yaml:"tools"`
	FewShotExamples []map[string]any `yaml:"few_shot_examples"`
}

func (s *Skill) ToContextString() string {
	lines := []string{
		fmt.Sprintf("## Kontext: %s", s.Description),
		"",
		"### Schema",
		strings.TrimSpace(s.SchemaExcerpt),
		"",
		"### Hinweise",
		strings.TrimSpace(s.DomainKnowledge),
	}
	return strings.Join(lines, "\n")
}

func (s *Skill) FormatFewShots(count int) string {
	if count > len(s.FewShotExamples) {
		count = len(s.FewShotExamples)
	}
	if count < 0 {
		count = 0
	}

	lines := []string{"## Beispiele"}
	for _, ex := range s.FewShotExamples[:count] {
		lines = append(lines, fmt.Sprintf("Frage: %q", stringValue(ex["q"])))
		if steps, ok := asStepSlice(ex["steps"]); ok {
			for _, step := range steps {
				lines = append(lines, formatToolCall(step["tool"], step["args"]))
			}
		} else {
			lines = append(lines, formatToolCall(ex["tool"], ex["args"]))
		}
		if answer, ok := ex["answer"]; ok {
			lines = append(lines, fmt.Sprintf("→ %s", stringValue(answer)))
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func LoadSkills(dir string) (map[string]*Skill, error) {
	skillsDir, err := resolveSkillsDir(dir)
	if err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(filepath.Join(skillsDir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob skills in %s: %w", skillsDir, err)
	}
	sort.Strings(matches)

	skills := make(map[string]*Skill, len(matches))
	for _, yamlFile := range matches {
		data, err := os.ReadFile(yamlFile)
		if err != nil {
			return nil, fmt.Errorf("read skill %s: %w", yamlFile, err)
		}

		var skill Skill
		if err := yaml.Unmarshal(data, &skill); err != nil {
			return nil, fmt.Errorf("parse skill %s: %w", yamlFile, err)
		}
		if skill.Name == "" {
			return nil, fmt.Errorf("parse skill %s: missing name", yamlFile)
		}
		skills[skill.Name] = &skill
	}

	return skills, nil
}

func resolveSkillsDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve default skills dir: %w", err)
	}
	for {
		candidate := filepath.Join(wd, defaultSkillsDir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", fmt.Errorf("default skills dir %s not found from %s", defaultSkillsDir, wd)
		}
		wd = parent
	}
}

func formatToolCall(tool any, args any) string {
	return fmt.Sprintf("→ %s(%s)", stringValue(tool), pythonRepr(args))
}

func asStepSlice(value any) ([]map[string]any, bool) {
	rawSteps, ok := value.([]any)
	if !ok {
		return nil, false
	}

	steps := make([]map[string]any, 0, len(rawSteps))
	for _, raw := range rawSteps {
		step, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		steps = append(steps, step)
	}
	return steps, true
}

func pythonRepr(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s: %s", pythonQuote(key), pythonReprNested(v[key])))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, pythonReprNested(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case bool:
		if v {
			return "True"
		}
		return "False"
	default:
		return fmt.Sprint(v)
	}
}

func pythonReprNested(value any) string {
	if v, ok := value.(string); ok {
		return pythonQuote(v)
	}
	return pythonRepr(value)
}

func pythonQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "\\'") + "'"
}

func stringValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}
