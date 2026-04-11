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

function demoteRunning(list) {
  return list.map((j) =>
    j.status === STATUS.RUNNING
      ? { ...j, status: STATUS.PENDING, startedAt: undefined }
      : j,
  );
}

export function useBatchQueue() {
  const [jobs, setJobs] = useState(() => demoteRunning(getBatchJobs()));
  const [isRunning, setIsRunning] = useState(false);
  const pauseRef = useRef(false);
  const cancelRef = useRef(false);

  // Keep a ref mirror of jobs so the executor always reads fresh state.
  const jobsRef = useRef(jobs);
  useEffect(() => {
    jobsRef.current = jobs;
  }, [jobs]);

  // Persist on every mutation.
  useEffect(() => {
    saveBatchJobs(jobs);
  }, [jobs]);

  const seed = useCallback((pairs) => {
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

  const run = useCallback(async ({ model, token, prompt, size }) => {
    if (!model || !token) return;
    pauseRef.current = false;
    cancelRef.current = false;
    setIsRunning(true);

    try {
      while (!pauseRef.current && !cancelRef.current) {
        const current = jobsRef.current;
        const job = current.find((j) => j.status === STATUS.PENDING);
        if (!job) break;

        setJobs((prev) =>
          prev.map((j) =>
            j.id === job.id
              ? { ...j, status: STATUS.RUNNING, startedAt: Date.now() }
              : j,
          ),
        );

        try {
          const { image } = await generateImage({
            model: model.key,
            apiType: model.apiType,
            token: `sk-${token.key}`,
            prompt,
            refs: [job.refUrl, job.prodUrl].filter(Boolean),
            size,
          });
          setJobs((prev) =>
            prev.map((j) =>
              j.id === job.id
                ? {
                    ...j,
                    status: image ? STATUS.DONE : STATUS.FAILED,
                    image,
                    error: image ? undefined : 'no image in response',
                    finishedAt: Date.now(),
                  }
                : j,
            ),
          );
        } catch (e) {
          setJobs((prev) =>
            prev.map((j) =>
              j.id === job.id
                ? {
                    ...j,
                    status: STATUS.FAILED,
                    error: e.message || 'failed',
                    finishedAt: Date.now(),
                  }
                : j,
            ),
          );
        }
      }
    } finally {
      setIsRunning(false);
    }
  }, []);

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
