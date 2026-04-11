/*
Copyright (C) 2025 QuantumNous

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

import { useCallback, useRef, useState } from 'react';
import { generateImage } from '../../services/drawFactory';
import { addHistory } from '../../helpers/drawFactoryStorage';

export function useSingleGeneration() {
  const [state, setState] = useState({
    loading: false,
    image: null,
    error: null,
    elapsed: null,
  });
  const abortRef = useRef(null);

  const run = useCallback(
    async ({ model, token, prompt, refs, size }) => {
      if (!model || !token) {
        setState((s) => ({ ...s, error: 'missing model/token' }));
        return;
      }
      abortRef.current = new AbortController();
      setState({ loading: true, image: null, error: null, elapsed: null });
      try {
        const { image, elapsed, raw } = await generateImage({
          model: model.key,
          apiType: model.apiType,
          token: `sk-${token.key}`,
          prompt,
          refs,
          size,
          signal: abortRef.current.signal,
        });
        if (!image) {
          throw new Error('no image in response');
        }
        setState({ loading: false, image, error: null, elapsed });
        addHistory({
          id: Date.now(),
          model: model.key,
          prompt,
          size,
          image,
          elapsed,
          createdAt: Date.now(),
        });
        return { image, raw };
      } catch (e) {
        if (e.name === 'AbortError') {
          setState({ loading: false, image: null, error: null, elapsed: null });
          return;
        }
        setState({
          loading: false,
          image: null,
          error: e.message || 'failed',
          elapsed: null,
        });
        addHistory({
          id: Date.now(),
          model: model.key,
          prompt,
          size,
          error: e.message || 'failed',
          createdAt: Date.now(),
        });
      }
    },
    [],
  );

  const stop = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  return { ...state, run, stop };
}
