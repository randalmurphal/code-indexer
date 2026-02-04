package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectSecrets(t *testing.T) {
	detector := NewSecretDetector()

	tests := []struct {
		name     string
		content  string
		expected int // number of secrets
	}{
		{
			name:     "API key",
			content:  `API_KEY = "sk-1234567890abcdefghijklmnop"`,
			expected: 1,
		},
		{
			name:     "AWS access key bare",
			content:  `AWS_ACCESS_KEY=AKIAIOSFODNN7REALKEY1`,
			expected: 1,
		},
		{
			name:     "AWS access key in env",
			content:  `export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7REALKEY1`,
			expected: 1,
		},
		{
			name:     "Connection string",
			content:  `DATABASE_URL = "mongodb://admin:password123@localhost:27017/db"`,
			expected: 1,
		},
		{
			name:     "Private key",
			content:  "-----BEGIN RSA PRIVATE KEY-----\nMIIEpA...\n-----END RSA PRIVATE KEY-----",
			expected: 1,
		},
		{
			name:     "Password in code",
			content:  `password = "supersecret123"`,
			expected: 1,
		},
		{
			name:     "No secrets",
			content:  "def hello():\n    return \"world\"",
			expected: 0,
		},
		{
			name:     "Placeholder (not secret)",
			content:  `API_KEY = "your-api-key-here"`,
			expected: 0,
		},
		{
			name:     "JWT token",
			content:  `token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"`,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secrets := detector.Detect(tt.content)
			assert.Len(t, secrets, tt.expected, "content: %s", tt.content)
		})
	}
}

func TestRedactSecrets(t *testing.T) {
	detector := NewSecretDetector()

	content := `
DATABASE_URL = "mongodb://admin:supersecret@prod.db.com:27017/mydb"
API_KEY = "sk-abcdef1234567890abcdef"
`

	secrets := detector.Detect(content)
	require.Len(t, secrets, 2)

	redacted := detector.Redact(content, secrets)

	assert.Contains(t, redacted, "[REDACTED]")
	assert.NotContains(t, redacted, "supersecret")
	assert.NotContains(t, redacted, "sk-abcdef")
}

func TestHasSecrets(t *testing.T) {
	detector := NewSecretDetector()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"with secret", `password = "secret123"`, true},
		{"without secret", `name = "john"`, false},
		{"placeholder", `password = "your-password-here"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, detector.HasSecrets(tt.content))
		})
	}
}
