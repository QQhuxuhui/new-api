/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
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
