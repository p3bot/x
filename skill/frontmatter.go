package skill

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/goccy/go-yaml"
)

// frontmatterName returns the name in the YAML frontmatter that opens data:
// the lines between a first line of --- and the next line of ---. A missing
// name reads as "".
func frontmatterName(data []byte) (string, error) {
	lines := bytes.SplitAfter(data, []byte("\n"))
	if !isFence(lines[0]) {
		return "", errors.New("frontmatter: first line is not ---")
	}
	for i := 1; i < len(lines); i++ {
		if !isFence(lines[i]) {
			continue
		}
		var meta struct {
			Name string `yaml:"name"`
		}
		if err := yaml.Unmarshal(bytes.Join(lines[1:i], nil), &meta); err != nil {
			return "", fmt.Errorf("frontmatter: %w", err)
		}
		return meta.Name, nil
	}
	return "", errors.New("frontmatter: no closing ---")
}

func isFence(line []byte) bool {
	return string(bytes.TrimRight(line, "\r\n")) == "---"
}
