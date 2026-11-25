/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { ReactNode } from 'react'
import type { ParsedNode } from 'stream-markdown-parser'

import { isFootnoteNode } from './response-node-guards'
import type { ParsedResponseContent } from './response-types'

const FENCE_START_PATTERN = /^(`{3,}|~{3,})([^\n]*)$/
const FENCE_END_PATTERN = /^(`{3,}|~{3,})\s*$/
const SECTION_HEADING_PATTERN = /^#{2,6}\s+\d+\.\s+/
const MARKDOWN_EXAMPLE_LANGUAGES = new Set(['markdown', 'md', 'mdx'])

type MarkdownExampleFence = {
  contentLines: string[]
  fenceChar: string
  language: string
  nestedFence: boolean
}

function getFenceRunLength(line: string, fenceChar: string): number {
  let length = 0

  for (const char of line) {
    if (char !== fenceChar) {
      break
    }

    length++
  }

  return length
}

function getMarkdownExampleFenceLength(block: MarkdownExampleFence): number {
  let maxFenceLength = 3

  for (const line of block.contentLines) {
    if (!line.startsWith(block.fenceChar)) {
      continue
    }

    maxFenceLength = Math.max(
      maxFenceLength,
      getFenceRunLength(line, block.fenceChar) + 1
    )
  }

  return maxFenceLength
}

function appendMarkdownExampleFence(
  output: string[],
  block: MarkdownExampleFence
): void {
  const fence = block.fenceChar.repeat(getMarkdownExampleFenceLength(block))

  output.push(`${fence}${block.language}`)
  output.push(...block.contentLines)
  output.push(fence)
}

function normalizeMarkdownExampleFences(input: string): string {
  const lines = input.split('\n')
  const output: string[] = []
  let exampleFence: MarkdownExampleFence | null = null

  for (const line of lines) {
    if (!exampleFence) {
      const match = line.match(FENCE_START_PATTERN)

      if (!match) {
        output.push(line)
        continue
      }

      const language = match[2].trim().toLowerCase()
      if (MARKDOWN_EXAMPLE_LANGUAGES.has(language)) {
        exampleFence = {
          contentLines: [],
          fenceChar: match[1][0],
          language,
          nestedFence: false,
        }
        continue
      }

      output.push(line)
      continue
    }

    if (!exampleFence.nestedFence && SECTION_HEADING_PATTERN.test(line)) {
      appendMarkdownExampleFence(output, exampleFence)
      output.push(line)
      exampleFence = null
      continue
    }

    if (exampleFence.nestedFence && FENCE_END_PATTERN.test(line)) {
      exampleFence.contentLines.push(line)
      exampleFence.nestedFence = false
      continue
    }

    if (
      line.startsWith(exampleFence.fenceChar.repeat(3)) &&
      !FENCE_END_PATTERN.test(line)
    ) {
      exampleFence.contentLines.push(line)
      exampleFence.nestedFence = true
      continue
    }

    if (FENCE_END_PATTERN.test(line)) {
      appendMarkdownExampleFence(output, exampleFence)
      exampleFence = null
      continue
    }

    exampleFence.contentLines.push(line)
  }

  if (exampleFence) {
    appendMarkdownExampleFence(output, exampleFence)
  }

  return output.join('\n')
}

export function stripCustomTags(input: unknown): string {
  if (typeof input !== 'string') {
    return String(input ?? '')
  }

  return input
    .replaceAll(
      /<\/?(conversation|conversationcontent|reasoning|reasoningcontent|reasoningtrigger|sources|sourcescontent|sourcestrigger|branch|branchmessages|branchnext|branchpage|branchprevious|branchselector|message|messagecontent)\b[^>]*>/gi,
      ''
    )
    .replaceAll(/<\/?think\b[^>]*>/gi, '')
}

export function getMarkdownContent(children: ReactNode): string {
  if (Array.isArray(children)) {
    return normalizeMarkdownExampleFences(stripCustomTags(children.join('')))
  }

  return normalizeMarkdownExampleFences(stripCustomTags(children))
}

export function getNodeKey(node: ParsedNode, index: number): string {
  const raw = typeof node.raw === 'string' ? node.raw : ''
  return `${node.type}-${index}-${raw.slice(0, 24)}`
}

export function parseResponseContent(
  nodes: ParsedNode[]
): ParsedResponseContent {
  const footnotes: ParsedResponseContent['footnotes'] = []
  const bodyNodes: ParsedNode[] = []

  for (const node of nodes) {
    if (isFootnoteNode(node)) {
      footnotes.push(node)
      continue
    }

    bodyNodes.push(node)
  }

  return { bodyNodes, footnotes }
}

export type DataImage = {
  alt: string
  offset: number
  src: string
}

const DATA_IMAGE_SUBTYPES = new Set(['png', 'jpeg', 'webp', 'gif'])

function isBase64Char(code: number): boolean {
  return (
    (code >= 65 && code <= 90) ||
    (code >= 97 && code <= 122) ||
    (code >= 48 && code <= 57) ||
    code === 43 ||
    code === 47 ||
    code === 61
  )
}

function skipWhitespace(content: string, index: number): number {
  let next = index
  while (next < content.length && /\s/.test(content[next])) {
    next++
  }
  return next
}

/**
 * Returns the images when the content consists only of markdown images with
 * base64 data URLs (as returned by image generation models), otherwise null.
 * Scans linearly so multi-megabyte image payloads never hit regex backtracking.
 */
export function extractDataImageOnlyContent(
  content: string
): DataImage[] | null {
  const images: DataImage[] = []
  let index = skipWhitespace(content, 0)

  while (index < content.length) {
    if (!content.startsWith('![', index)) {
      return null
    }
    const altEnd = content.indexOf('](', index + 2)
    if (altEnd < 0) {
      return null
    }
    const alt = content.slice(index + 2, altEnd)
    if (alt.includes(']') || alt.includes('\n')) {
      return null
    }
    const srcStart = altEnd + 2
    if (!content.startsWith('data:image/', srcStart)) {
      return null
    }
    const subtypeStart = srcStart + 'data:image/'.length
    const subtypeEnd = content.indexOf(';base64,', subtypeStart)
    if (
      subtypeEnd < 0 ||
      !DATA_IMAGE_SUBTYPES.has(content.slice(subtypeStart, subtypeEnd))
    ) {
      return null
    }
    let cursor = subtypeEnd + ';base64,'.length
    const payloadStart = cursor
    while (
      cursor < content.length &&
      isBase64Char(content.charCodeAt(cursor))
    ) {
      cursor++
    }
    if (cursor === payloadStart || content[cursor] !== ')') {
      return null
    }
    images.push({ alt, offset: index, src: content.slice(srcStart, cursor) })
    index = skipWhitespace(content, cursor + 1)
  }

  return images.length > 0 ? images : null
}
