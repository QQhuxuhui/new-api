/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

// DrawFactory upstream request builders + response parsing.
// Single source of truth for "what do the request/response bodies look like".

export const API_TYPE = {
  CHAT: 'chat',
  IMAGES: 'images',
};

const ENDPOINTS = {
  [API_TYPE.CHAT]: '/v1/chat/completions',
  [API_TYPE.IMAGES]: '/v1/images/generations',
};

export function buildChatCompletionsBody({
  model,
  prompt,
  refs = [],
  size,
}) {
  const content = [];
  if (prompt) content.push({ type: 'text', text: prompt });
  for (const ref of refs) {
    content.push({
      type: 'image_url',
      image_url: { url: ref },
    });
  }
  const body = {
    model,
    messages: [{ role: 'user', content }],
  };
  if (size) body.size = size;
  return body;
}

export function buildImagesGenerationsBody({ model, prompt, size }) {
  const body = { model, prompt };
  if (size) body.size = size;
  return body;
}

export function extractImageFromResponse(data, apiType) {
  if (!data) return null;
  if (apiType === API_TYPE.IMAGES) {
    const item = data?.data?.[0];
    if (!item) return null;
    if (item.b64_json) return `data:image/png;base64,${item.b64_json}`;
    if (item.url) return item.url;
    return null;
  }
  // chat completions: look for image_url or base64 in message content
  const choice = data?.choices?.[0];
  const msg = choice?.message;
  if (!msg) return null;
  const content = msg.content;
  if (typeof content === 'string') {
    const m = content.match(/!\[.*?\]\((.*?)\)/);
    if (m) return m[1];
    if (content.startsWith('data:image')) return content;
    return null;
  }
  if (Array.isArray(content)) {
    for (const part of content) {
      if (part?.type === 'image_url' && part?.image_url?.url) {
        return part.image_url.url;
      }
      if (part?.type === 'image' && part?.source?.data) {
        const mime = part.source.media_type || 'image/png';
        return `data:${mime};base64,${part.source.data}`;
      }
    }
  }
  return null;
}

export async function generateImage({
  model,
  apiType,
  token,
  prompt,
  refs,
  size,
  signal,
}) {
  const endpoint = ENDPOINTS[apiType];
  if (!endpoint) throw new Error(`unsupported apiType: ${apiType}`);
  const body =
    apiType === API_TYPE.CHAT
      ? buildChatCompletionsBody({ model, prompt, refs, size })
      : buildImagesGenerationsBody({ model, prompt, size });

  const start = Date.now();
  const resp = await fetch(endpoint, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
    signal,
  });
  const elapsed = Date.now() - start;
  const text = await resp.text();
  let data;
  try {
    data = text ? JSON.parse(text) : null;
  } catch (_e) {
    data = null;
  }

  if (!resp.ok) {
    const msg =
      data?.error?.message || data?.message || text || `HTTP ${resp.status}`;
    const err = new Error(msg);
    err.status = resp.status;
    err.raw = data ?? text;
    throw err;
  }

  const image = extractImageFromResponse(data, apiType);
  return { image, elapsed, raw: data };
}
