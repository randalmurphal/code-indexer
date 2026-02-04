# pattern package

Code pattern detection via method signature clustering.

## Purpose

Detect repeating code patterns (e.g., Importer, Handler, Service) by clustering classes with similar method signatures.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Detector` | Pattern clustering engine | `detector.go:20-25` |
| `DetectorConfig` | Thresholds | `detector.go:27-30` |
| `Pattern` | Detected pattern | `detector.go:32-38` |

## Detection Algorithm

1. Group symbols by file
2. Extract method sets per class
3. Compute Jaccard similarity between classes
4. Cluster classes with similarity > threshold (default 0.8)
5. Infer pattern name from common class name suffix

## Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `MinClusterSize` | 5 | Minimum classes to form pattern |
| `SimilarityThreshold` | 0.8 | Jaccard similarity threshold |

## Usage

```go
detector := pattern.NewDetector(pattern.DetectorConfig{
    MinClusterSize:      5,
    SimilarityThreshold: 0.8,
})

patterns := detector.Detect(symbols)
// patterns[0].Name = "Importer"
// patterns[0].Methods = ["extract", "transform", "validate"]
```

## Pattern Name Inference

Finds longest common suffix among clustered class names:
- `AWSImporter`, `GCSImporter`, `S3Importer` → `Importer`
- `UserHandler`, `OrderHandler` → `Handler`

Algorithm in `detector.go:130-150`:
- Requires suffix ≥ 4 chars
- Tracks longest valid suffix across all pairs

## Gotchas

1. **Minimum cluster size**: Prevents false patterns from 2-3 similar classes
2. **Suffix length**: Must be ≥4 chars to avoid matching "er", "or"
3. **Method comparison**: Uses method names only, not signatures
4. **Integration**: Called during indexing, patterns stored as `kind: "pattern"` chunks
