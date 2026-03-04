export function hasSplitClaudeCacheCreation(
  cacheCreationTokens5m = 0,
  cacheCreationTokens1h = 0,
) {
  return cacheCreationTokens5m > 0 || cacheCreationTokens1h > 0;
}

export function calculateClaudeEffectiveInputTokens({
  inputTokens = 0,
  cacheTokens = 0,
  cacheRatio = 1.0,
  cacheCreationTokens = 0,
  cacheCreationRatio = 1.0,
  cacheCreationTokens5m = 0,
  cacheCreationRatio5m = 1.0,
  cacheCreationTokens1h = 0,
  cacheCreationRatio1h = 1.0,
}) {
  const hasSplit = hasSplitClaudeCacheCreation(
    cacheCreationTokens5m,
    cacheCreationTokens1h,
  );
  const weightedCacheCreationTokens = hasSplit
    ? cacheCreationTokens5m * cacheCreationRatio5m +
      cacheCreationTokens1h * cacheCreationRatio1h
    : cacheCreationTokens * cacheCreationRatio;

  return inputTokens + cacheTokens * cacheRatio + weightedCacheCreationTokens;
}
