// Package security provides secret detection and redaction for code indexing.
package security

import (
	"regexp"
	"strings"
)

// Secret represents a detected secret.
type Secret struct {
	Type     string `json:"type"`
	Line     int    `json:"line"`
	StartPos int    `json:"start_pos"`
	EndPos   int    `json:"end_pos"`
	Redacted string `json:"redacted"` // What to replace with
}

// SecretDetector detects secrets in code.
type SecretDetector struct {
	patterns     []secretPattern
	placeholders []string
}

type secretPattern struct {
	name   string
	regex  *regexp.Regexp
	redact func(match string) string
}

// NewSecretDetector creates a new detector with default patterns.
func NewSecretDetector() *SecretDetector {
	return &SecretDetector{
		patterns: []secretPattern{
			{
				name:  "api_key",
				regex: regexp.MustCompile(`(?i)(api[_-]?key|apikey|api_secret)\s*[=:]\s*["']([a-zA-Z0-9_\-]{20,})["']`),
				redact: func(match string) string {
					return regexp.MustCompile(`["'][^"']+["']`).ReplaceAllString(match, `"[REDACTED]"`)
				},
			},
			{
				name:  "aws_access_key",
				regex: regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
				redact: func(match string) string {
					return "[REDACTED_AWS_KEY]"
				},
			},
			{
				name:  "password",
				regex: regexp.MustCompile(`(?i)(password|passwd|pwd|secret)\s*[=:]\s*["']([^\s"']{8,})["']`),
				redact: func(match string) string {
					return regexp.MustCompile(`["'][^"']+["']`).ReplaceAllString(match, `"[REDACTED]"`)
				},
			},
			{
				name:  "connection_string",
				regex: regexp.MustCompile(`(?i)(mongodb|postgres|mysql|redis|amqp):\/\/[^\s"']+`),
				redact: func(match string) string {
					// Keep protocol and host, redact credentials
					re := regexp.MustCompile(`(://[^:]+:)[^@]+(@)`)
					return re.ReplaceAllString(match, "${1}[REDACTED]${2}")
				},
			},
			{
				name:  "private_key",
				regex: regexp.MustCompile(`-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----`),
				redact: func(match string) string {
					return "[REDACTED_PRIVATE_KEY]"
				},
			},
			{
				name:  "jwt_token",
				regex: regexp.MustCompile(`eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*`),
				redact: func(match string) string {
					return "[REDACTED_JWT]"
				},
			},
		},
		placeholders: []string{
			"your-", "example", "placeholder", "xxx", "changeme",
			"TODO", "FIXME", "<", ">", "${", "{{",
		},
	}
}

// Detect finds secrets in content.
func (d *SecretDetector) Detect(content string) []Secret {
	var secrets []Secret

	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		// Skip if line looks like a placeholder
		if d.isPlaceholder(line) {
			continue
		}

		for _, pattern := range d.patterns {
			matches := pattern.regex.FindAllStringIndex(line, -1)
			for _, match := range matches {
				secrets = append(secrets, Secret{
					Type:     pattern.name,
					Line:     lineNum + 1,
					StartPos: match[0],
					EndPos:   match[1],
				})
			}
		}
	}

	return secrets
}

// Redact replaces secrets with redacted versions.
func (d *SecretDetector) Redact(content string, secrets []Secret) string {
	if len(secrets) == 0 {
		return content
	}

	result := content

	for _, pattern := range d.patterns {
		result = pattern.regex.ReplaceAllStringFunc(result, pattern.redact)
	}

	return result
}

func (d *SecretDetector) isPlaceholder(line string) bool {
	lower := strings.ToLower(line)
	for _, p := range d.placeholders {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// HasSecrets checks if content contains secrets.
func (d *SecretDetector) HasSecrets(content string) bool {
	return len(d.Detect(content)) > 0
}
