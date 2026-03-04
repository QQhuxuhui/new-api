import assert from 'node:assert/strict';
import { calculateClaudeEffectiveInputTokens } from './claudePrice.js';

function testSplitCacheCreationDoesNotDoubleCount() {
  const effectiveInputTokens = calculateClaudeEffectiveInputTokens({
    inputTokens: 0,
    cacheTokens: 30361,
    cacheRatio: 0.1,
    cacheCreationTokens: 127772,
    cacheCreationRatio: 1.25,
    cacheCreationTokens5m: 127772,
    cacheCreationRatio5m: 1.25,
    cacheCreationTokens1h: 0,
    cacheCreationRatio1h: 2.0,
  });

  // 0 + 30361*0.1 + 127772*1.25
  assert.equal(effectiveInputTokens, 162751.1);
}

function testLegacyCacheCreationStillCountsWhenNoSplit() {
  const effectiveInputTokens = calculateClaudeEffectiveInputTokens({
    inputTokens: 1000,
    cacheTokens: 200,
    cacheRatio: 0.5,
    cacheCreationTokens: 300,
    cacheCreationRatio: 1.25,
    cacheCreationTokens5m: 0,
    cacheCreationRatio5m: 1.25,
    cacheCreationTokens1h: 0,
    cacheCreationRatio1h: 2.0,
  });

  // 1000 + 200*0.5 + 300*1.25
  assert.equal(effectiveInputTokens, 1475);
}

testSplitCacheCreationDoesNotDoubleCount();
testLegacyCacheCreationStillCountsWhenNoSplit();
console.log('claudePrice tests passed');
