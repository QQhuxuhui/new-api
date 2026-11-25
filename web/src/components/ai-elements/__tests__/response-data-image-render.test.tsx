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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import { Response } from '../response'

afterEach(() => {
  cleanup()
})

describe('Response with oversized content', () => {
  test('renders a generated image when a data URL image exceeds the parse limit', () => {
    const src = `data:image/png;base64,${'A'.repeat(30_000)}`
    render(<Response final>{`![generated](${src})`}</Response>)

    const image = screen.getByRole('img', { name: 'generated' })
    expect(image.getAttribute('src')).toBe(src)
  })

  test('keeps raw text when oversized content is not image-only', () => {
    const text = `intro ${'x'.repeat(30_000)}`
    render(<Response final>{text}</Response>)

    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByText(text)).toBeTruthy()
  })
})
