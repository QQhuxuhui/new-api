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

import { useState, useEffect } from 'react';

const STORAGE_KEYS = {
  COMPLETED: 'onboarding_completed',
  DISMISSED: 'onboarding_dismissed',
  PROGRESS: 'onboarding_progress',
  LOGIN_COUNT: 'login_count',
};

/**
 * Custom hook for managing onboarding state
 * Handles localStorage persistence and state management
 */
export const useOnboarding = () => {
  const [isCompleted, setIsCompleted] = useState(false);
  const [isDismissed, setIsDismissed] = useState(false);
  const [progress, setProgress] = useState({
    currentStep: 1,
    completedSteps: [],
    skippedSteps: [],
    createdToken: null,
    topupData: null,
    usageModeData: null,
    startTime: null,
  });

  // Load state from localStorage on mount
  useEffect(() => {
    const completed = localStorage.getItem(STORAGE_KEYS.COMPLETED) === 'true';
    const dismissed = localStorage.getItem(STORAGE_KEYS.DISMISSED) === 'true';
    const savedProgress = localStorage.getItem(STORAGE_KEYS.PROGRESS);

    setIsCompleted(completed);
    setIsDismissed(dismissed);

    if (savedProgress) {
      try {
        setProgress(JSON.parse(savedProgress));
      } catch (error) {
        console.error('Failed to parse onboarding progress:', error);
      }
    }
  }, []);

  /**
   * Update onboarding progress
   * @param {object} newProgress - New progress data to merge
   */
  const updateProgress = (newProgress) => {
    // Read latest from localStorage to avoid stale closure issues
    let currentProgress = progress;
    const savedProgress = localStorage.getItem(STORAGE_KEYS.PROGRESS);
    if (savedProgress) {
      try {
        currentProgress = JSON.parse(savedProgress);
      } catch (error) {
        console.error('Failed to parse onboarding progress during update, using in-memory state:', error);
        // Fall back to in-memory progress state if localStorage is corrupted
      }
    }
    const updatedProgress = { ...currentProgress, ...newProgress };
    setProgress(updatedProgress);
    localStorage.setItem(
      STORAGE_KEYS.PROGRESS,
      JSON.stringify(updatedProgress),
    );
  };

  /**
   * Mark onboarding as completed
   */
  const markComplete = () => {
    setIsCompleted(true);
    localStorage.setItem(STORAGE_KEYS.COMPLETED, 'true');
    // Clear progress after completion
    localStorage.removeItem(STORAGE_KEYS.PROGRESS);
  };

  /**
   * Mark onboarding as dismissed (user chose not to complete)
   */
  const markDismissed = () => {
    setIsDismissed(true);
    localStorage.setItem(STORAGE_KEYS.DISMISSED, 'true');
  };

  /**
   * Reset onboarding state (for testing or manual restart)
   */
  const reset = () => {
    setIsCompleted(false);
    setIsDismissed(false);
    setProgress({
      currentStep: 1,
      completedSteps: [],
      skippedSteps: [],
      createdToken: null,
      topupData: null,
      usageModeData: null,
      startTime: new Date().toISOString(),
    });
    localStorage.removeItem(STORAGE_KEYS.COMPLETED);
    localStorage.removeItem(STORAGE_KEYS.DISMISSED);
    localStorage.removeItem(STORAGE_KEYS.PROGRESS);
  };

  /**
   * Check if onboarding should be shown
   * @returns {boolean}
   */
  const shouldShow = () => {
    // Don't show if already completed or dismissed
    if (isCompleted || isDismissed) {
      return false;
    }

    // Check if this is a first-time user
    const loginCount = parseInt(
      localStorage.getItem(STORAGE_KEYS.LOGIN_COUNT) || '0',
      10,
    );

    // Show if login count is 1 (first login) or if there's saved progress
    return loginCount === 1 || progress.startTime !== null;
  };

  /**
   * Increment login count (should be called on successful login)
   */
  const incrementLoginCount = () => {
    const currentCount = parseInt(
      localStorage.getItem(STORAGE_KEYS.LOGIN_COUNT) || '0',
      10,
    );
    const newCount = currentCount + 1;
    localStorage.setItem(STORAGE_KEYS.LOGIN_COUNT, newCount.toString());
  };

  return {
    isCompleted,
    isDismissed,
    progress,
    updateProgress,
    markComplete,
    markDismissed,
    reset,
    shouldShow,
    incrementLoginCount,
  };
};

/**
 * Hook for tracking time spent on each step
 */
export const useOnboardingProgress = () => {
  const [stepTimes, setStepTimes] = useState({});
  const [stepStartTime, setStepStartTime] = useState(null);

  /**
   * Start timing a step
   * @param {number} step - Step number
   */
  const startStep = (step) => {
    setStepStartTime(Date.now());
  };

  /**
   * End timing a step and record duration
   * @param {number} step - Step number
   * @returns {number} Time spent in seconds
   */
  const endStep = (step) => {
    if (!stepStartTime) return 0;

    const duration = (Date.now() - stepStartTime) / 1000; // Convert to seconds
    setStepTimes((prev) => ({
      ...prev,
      [step]: duration,
    }));
    setStepStartTime(null);

    return duration;
  };

  /**
   * Calculate total completion rate
   * @param {number[]} completedSteps - Array of completed step numbers
   * @param {number} totalSteps - Total number of steps
   * @returns {number} Completion rate percentage (0-100)
   */
  const getCompletionRate = (completedSteps, totalSteps = 4) => {
    return Math.round((completedSteps.length / totalSteps) * 100);
  };

  return {
    stepTimes,
    startStep,
    endStep,
    getCompletionRate,
  };
};
