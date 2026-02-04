# security package

Secret detection and redaction for code indexing.

## Purpose

Detect and redact secrets (API keys, passwords, connection strings) before storing code chunks in the vector database.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `SecretDetector` | Pattern-based detector | `secrets.go:18-23` |
| `Secret` | Detected secret info | `secrets.go:9-15` |

## Detected Secret Types

| Type | Pattern | Example |
|------|---------|---------|
| `api_key` | `api[_-]?key.*=.*["']...["']` | `API_KEY = "sk-abc..."` |
| `aws_access_key` | `AKIA[0-9A-Z]{16}` | `AKIAIOSFODNN7...` |
| `password` | `password.*=.*["']...["']` | `password = "secret"` |
| `connection_string` | `mongodb://...`, `postgres://...` | `mongodb://user:pass@host` |
| `private_key` | `-----BEGIN.*PRIVATE KEY-----` | RSA/EC/DSA keys |
| `jwt_token` | `eyJ...\.eyJ...\..*` | JWT tokens |

## Placeholder Detection

Lines containing these patterns are **not** flagged as secrets:
- `your-`, `example`, `placeholder`, `xxx`, `changeme`
- `TODO`, `FIXME`, `<`, `>`, `${`, `{{`

## Usage

```go
detector := security.NewSecretDetector()

// Check for secrets
if detector.HasSecrets(content) {
    secrets := detector.Detect(content)
    redacted := detector.Redact(content, secrets)
}
```

## Integration

Called in `chunk/extractor.go:102-106` after chunk creation:
```go
if e.secretDetector.HasSecrets(chunk.Content) {
    secrets := e.secretDetector.Detect(chunk.Content)
    chunk.Content = e.secretDetector.Redact(chunk.Content, secrets)
    chunk.HasSecrets = true
}
```

## Redaction Format

| Secret Type | Redaction |
|-------------|-----------|
| API key | `"[REDACTED]"` |
| AWS key | `[REDACTED_AWS_KEY]` |
| Password | `"[REDACTED]"` |
| Connection string | `mongodb://user:[REDACTED]@host` |
| Private key | `[REDACTED_PRIVATE_KEY]` |
| JWT | `[REDACTED_JWT]` |

## Gotchas

1. **Placeholder detection**: Case-insensitive, prevents false positives on example code
2. **Connection strings**: Credentials redacted but host preserved for context
3. **Line-by-line**: Detection operates per-line, placeholders skip entire line
4. **HasSecrets flag**: Set on chunk for downstream filtering if needed
