/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import { useCallback, useEffect, useRef, useState } from 'react';
import { generateImage } from '../../services/drawFactory';
import {
  getBatchJobs,
  saveBatchJobs,
  clearBatchJobs,
} from '../../helpers/drawFactoryStorage';

const STATUS = {
  PENDING: 'pending',
  RUNNING: 'running',
  DONE: 'done',
  FAILED: 'failed',
};

export function useBatchQueue() {
  const [jobs, setJobs] = useState(() => getBatchJobs());
  const [isRunning, setIsRunning] = useState(false);
  const pauseRef = useRef(false);
  const cancelRef = useRef(false);

  // Persist on every mutation.
  useEffect(() => {
    saveBatchJobs(jobs);
  }, [jobs]);

  const seed = useCallback((pairs) => {
    // pairs: [{ refUrl, prodUrl }, ...]
    const seeded = pairs.map((p, i) => ({
      id: `${Date.now()}-${i}`,
      refUrl: p.refUrl,
      prodUrl: p.prodUrl,
      status: STATUS.PENDING,
    }));
    setJobs(seeded);
  }, []);

  const clear = useCallback(() => {
    clearBatchJobs();
    setJobs([]);
  }, []);

  const run = useCallback(
    async ({ model, token, prompt, size }) => {
      if (!model || !token) return;
      pauseRef.current = false;
      cancelRef.current = false;
      setIsRunning(true);

      // Work on a mutable snapshot; commit after each job.
      let snapshot = jobs.slice();

      for (let i = 0; i < snapshot.length; i += 1) {
        if (pauseRef.current || cancelRef.current) break;
        const job = snapshot[i];
        if (job.status !== STATUS.PENDING) continue;

        snapshot = snapshot.slice();
        snapshot[i] = {
          ...job,
          status: STATUS.RUNNING,
          startedAt: Date.now(),
        };
        setJobs(snapshot);

        try {
          const { image } = await generateImage({
            model: model.key,
            apiType: model.apiType,
            token: `sk-${token.key}`,
            prompt,
            refs: [job.refUrl, job.prodUrl].filter(Boolean),
            size,
          });
          snapshot = snapshot.slice();
          snapshot[i] = {
            ...snapshot[i],
            status: image ? STATUS.DONE : STATUS.FAILED,
            image,
            error: image ? undefined : 'no image in response',
            finishedAt: Date.now(),
          };
        } catch (e) {
          snapshot = snapshot.slice();
          snapshot[i] = {
            ...snapshot[i],
            status: STATUS.FAILED,
            error: e.message || 'failed',
            finishedAt: Date.now(),
          };
        }
        setJobs(snapshot);
      }

      setIsRunning(false);
    },
    [jobs],
  );

  const pause = useCallback(() => {
    pauseRef.current = true;
  }, []);

  const cancel = useCallback(() => {
    cancelRef.current = true;
  }, []);

  const retryFailed = useCallback(() => {
    setJobs((prev) =>
      prev.map((j) =>
        j.status === STATUS.FAILED
          ? { ...j, status: STATUS.PENDING, error: undefined }
          : j,
      ),
    );
  }, []);

  const counts = {
    done: jobs.filter((j) => j.status === STATUS.DONE).length,
    failed: jobs.filter((j) => j.status === STATUS.FAILED).length,
    pending: jobs.filter((j) => j.status === STATUS.PENDING).length,
    running: jobs.filter((j) => j.status === STATUS.RUNNING).length,
  };

  return {
    jobs,
    counts,
    isRunning,
    seed,
    clear,
    run,
    pause,
    cancel,
    retryFailed,
  };
}

export { STATUS as BATCH_STATUS };
