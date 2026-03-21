import assert from 'node:assert/strict';
import {
  calculateNonCachedPromptTokens,
  getDisplayPromptTokens,
} from './log.js';

function testCalculateNonCachedPromptTokensSubtractsCacheReadAndCreation() {
  const promptTokens = calculateNonCachedPromptTokens(194067, 159104, 0);

  assert.equal(promptTokens, 34963);
}

function testCalculateNonCachedPromptTokensFloorsAtZero() {
  const promptTokens = calculateNonCachedPromptTokens(100, 90, 20);

  assert.equal(promptTokens, 0);
}

function testNonClaudeCachedLogUsesNonCachedInputDisplay() {
  const promptTokens = getDisplayPromptTokens({
    prompt_tokens: 194067,
    other: JSON.stringify({
      cache_tokens: 159104,
    }),
  });

  assert.equal(promptTokens, 34963);
}

function testClaudeLogKeepsStoredPromptTokens() {
  const promptTokens = getDisplayPromptTokens({
    prompt_tokens: 34963,
    other: JSON.stringify({
      claude: true,
      cache_tokens: 159104,
      cache_creation_tokens: 12000,
    }),
  });

  assert.equal(promptTokens, 34963);
}

function testNonClaudeCachedCreationAlsoReducesVisibleInput() {
  const promptTokens = getDisplayPromptTokens({
    prompt_tokens: 1000,
    other: JSON.stringify({
      cache_tokens: 200,
      cache_creation_tokens: 300,
    }),
  });

  assert.equal(promptTokens, 500);
}

function testPlainLogKeepsOriginalPromptTokens() {
  const promptTokens = getDisplayPromptTokens({
    prompt_tokens: 777,
    other: '{}',
  });

  assert.equal(promptTokens, 777);
}

testNonClaudeCachedLogUsesNonCachedInputDisplay();
testClaudeLogKeepsStoredPromptTokens();
testNonClaudeCachedCreationAlsoReducesVisibleInput();
testPlainLogKeepsOriginalPromptTokens();
testCalculateNonCachedPromptTokensSubtractsCacheReadAndCreation();
testCalculateNonCachedPromptTokensFloorsAtZero();
console.log('log helper tests passed');
