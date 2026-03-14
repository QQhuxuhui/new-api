package ratio_setting

const CompactModelSuffix = "-openai-compact"

const CompactWildcardModelKey = "*" + CompactModelSuffix

// WithCompactModelSuffix appends the compact suffix to a model name
func WithCompactModelSuffix(modelName string) string {
	return modelName + CompactModelSuffix
}
