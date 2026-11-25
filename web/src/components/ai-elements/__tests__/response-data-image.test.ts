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
import { describe, expect, test } from 'vitest'

import { extractDataImageOnlyContent } from '../response-content'

const PNG_SRC = 'data:image/png;base64,iVBORw0KGgo='
const JPEG_SRC = 'data:image/jpeg;base64,/9j/4AAQ'

describe('extractDataImageOnlyContent', () => {
  test('returns the image when content is a single data URL image', () => {
    expect(extractDataImageOnlyContent(`![image](${PNG_SRC})`)).toEqual([
      { alt: 'image', offset: 0, src: PNG_SRC },
    ])
  })

  test('returns every image when images are separated by whitespace', () => {
    expect(
      extractDataImageOnlyContent(`\n![a](${PNG_SRC})\n\n![b](${JPEG_SRC})\n`)
    ).toMatchObject([
      { alt: 'a', src: PNG_SRC },
      { alt: 'b', src: JPEG_SRC },
    ])
  })

  test('returns null when text accompanies the image', () => {
    expect(extractDataImageOnlyContent(`Here: ![image](${PNG_SRC})`)).toBeNull()
    expect(extractDataImageOnlyContent(`![image](${PNG_SRC}) done`)).toBeNull()
  })

  test('returns null for non data URL or unsupported image types', () => {
    expect(
      extractDataImageOnlyContent('![image](https://example.com/a.png)')
    ).toBeNull()
    expect(
      extractDataImageOnlyContent(
        '![image](data:image/svg+xml;base64,PHN2Zz4=)'
      )
    ).toBeNull()
  })

  test('returns null when the base64 payload contains invalid characters', () => {
    expect(
      extractDataImageOnlyContent(
        '![image](data:image/png;base64,abc"onerror=x)'
      )
    ).toBeNull()
  })

  test('returns null for empty content', () => {
    expect(extractDataImageOnlyContent('   ')).toBeNull()
  })
})
