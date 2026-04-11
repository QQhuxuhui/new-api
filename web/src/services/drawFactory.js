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

const DEFAULT_MAX_TOKENS = 4096;

export function buildChatCompletionsBody({ model, prompt, refs = [], size }) {
  let content;
  if (refs.length > 0) {
    content = [];
    for (const ref of refs) {
      content.push({ type: 'image_url', image_url: { url: ref } });
    }
    content.push({ type: 'text', text: prompt || '' });
  } else {
    content = prompt || '';
  }

  const body = {
    model,
    messages: [{ role: 'user', content }],
    max_tokens: DEFAULT_MAX_TOKENS,
  };

  if (size) {
    body.extra_body = {
      google: { image_config: { image_size: size } },
    };
  }

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
  // chat completions: look for image in several possible shapes
  const choice = data?.choices?.[0];
  const msg = choice?.message;
  if (!msg) return null;

  const content = msg.content;

  if (typeof content === 'string') {
    // markdown image with base64
    const md = content.match(
      /!\[.*?\]\((data:image\/(?:png|jpeg|jpg|webp|gif);base64,[^)]+)\)/,
    );
    if (md) return md[1];

    // raw data URL anywhere in the string
    const raw = content.match(
      /data:image\/(?:png|jpeg|jpg|webp|gif);base64,[A-Za-z0-9+/=]+/,
    );
    if (raw) return raw[0];
  } else if (Array.isArray(content)) {
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

  // Gemini inline_data in message.parts[]
  const parts = msg.parts;
  if (Array.isArray(parts)) {
    const inline = parts.find((p) => p?.inline_data);
    if (inline) {
      const mime = inline.inline_data.mime_type || 'image/png';
      return `data:${mime};base64,${inline.inline_data.data}`;
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
