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

import React, { useState, useEffect, useRef } from 'react';
import { Modal } from '@douyinfe/semi-ui';
import {
  useOnboarding,
  useOnboardingProgress,
} from '../../hooks/useOnboarding';
import { trackEvent } from '../../helpers/analytics';
import ProgressBar from './ProgressBar';
import WelcomeStep from './steps/WelcomeStep';
import TopupStep from './steps/TopupStep';
import CreateTokenStep from './steps/CreateTokenStep';
import GetStartedStep from './steps/GetStartedStep';

/**
 * Main onboarding wizard component
 * Guides new users through account setup
 */
const OnboardingWizard = ({ visible, onClose, autoStart = false }) => {
  const { progress, updateProgress, markComplete, markDismissed } =
    useOnboarding();

  const { startStep, endStep, getCompletionRate } = useOnboardingProgress();

  const [currentStep, setCurrentStep] = useState(progress.currentStep || 1);
  const [completedSteps, setCompletedSteps] = useState(
    progress.completedSteps || [],
  );
  const [skippedSteps, setSkippedSteps] = useState(progress.skippedSteps || []);
  const [createdToken, setCreatedToken] = useState(
    progress.createdToken || null,
  );
  const [topupData, setTopupData] = useState(progress.topupData || null);

  // Track session number for analytics
  const sessionCountRef = useRef(progress.sessionCount || 1);

  const totalSteps = 4;

  // Handle all initialization when modal opens (single effect to avoid race conditions)
  useEffect(() => {
    if (visible) {
      // Reset UI state only (do not touch localStorage analytics data)
      setCurrentStep(1);
      setCompletedSteps([]); // UI shows fresh progress bar
      setSkippedSteps([]);   // UI resets skipped state
      setCreatedToken(null); // UI resets token display
      setTopupData(null);    // UI resets topup display

      // Prepare batch update for localStorage
      const progressUpdates = {
        currentStep: 1,
      };

      // Check if this is first open or reopen
      // Double-check localStorage to avoid race condition with useOnboarding hook
      const savedProgress = localStorage.getItem('onboarding_progress');
      let hasExistingStartTime = false;

      if (savedProgress) {
        try {
          const parsed = JSON.parse(savedProgress);
          hasExistingStartTime = !!parsed.startTime;
        } catch (error) {
          // Ignore parse errors
        }
      }

      if (!hasExistingStartTime && !progress.startTime) {
        // First open: initialize startTime
        progressUpdates.startTime = new Date().toISOString();
        progressUpdates.sessionCount = 1;
        sessionCountRef.current = 1;

        // Track wizard start
        trackEvent('onboarding_started', {
          auto_start: autoStart,
        });
      } else {
        // Reopen: increment session count
        sessionCountRef.current = (progress.sessionCount || 1) + 1;
        progressUpdates.sessionCount = sessionCountRef.current;

        // Track reopen event
        trackEvent('onboarding_reopened', {
          session: sessionCountRef.current,
          previous_step: progress.currentStep || 1,
        });
      }

      // Single batch update to localStorage
      updateProgress(progressUpdates);

      // Start timing for step 1
      startStep(1);
    }
  }, [visible]);

  /**
   * Handle moving to next step
   */
  const handleNext = (data = {}) => {
    const timeSpent = endStep(currentStep);

    // Check if this step was completed before (for analytics)
    // Use progress.completedSteps from localStorage (not component state)
    const isRepeat = (progress.completedSteps || []).includes(currentStep);

    // Track step completion
    trackEvent('onboarding_step_completed', {
      step: currentStep,
      time_spent: timeSpent,
      is_repeat: isRepeat,
      ...data,
    });

    // Update state based on step data
    if (currentStep === 1 && data.dontShowAgain) {
      markDismissed();
    }

    // Prepare updates for localStorage (single batch update)
    const progressUpdates = {};

    // Handle step 2: topup data
    if (currentStep === 2 && data.method) {
      const newTopupData = data;
      setTopupData(newTopupData);
      progressUpdates.topupData = newTopupData;
    }

    // Handle step 3: created token
    if (currentStep === 3 && data.createdToken) {
      const newToken = data.createdToken;
      setCreatedToken(newToken);
      progressUpdates.createdToken = newToken;
    }

    // Mark current step as completed in component state (for UI)
    const newCompletedSteps = completedSteps.includes(currentStep)
      ? completedSteps
      : [...completedSteps, currentStep];
    setCompletedSteps(newCompletedSteps);

    // Update localStorage completedSteps for analytics persistence
    const updatedCompletedSteps = [...(progress.completedSteps || [])];
    if (!updatedCompletedSteps.includes(currentStep)) {
      updatedCompletedSteps.push(currentStep);
    }
    progressUpdates.completedSteps = updatedCompletedSteps;

    // Move to next step
    if (currentStep < totalSteps) {
      const nextStep = currentStep + 1;
      setCurrentStep(nextStep);

      // Update currentStep in localStorage for analytics and recovery
      progressUpdates.currentStep = nextStep;

      // Single batch update to localStorage
      updateProgress(progressUpdates);

      startStep(nextStep);
    } else {
      // Single batch update to localStorage (no currentStep change)
      updateProgress(progressUpdates);
    }
  };

  /**
   * Handle going back to previous step
   */
  const handlePrev = () => {
    if (currentStep > 1) {
      endStep(currentStep);
      const prevStep = currentStep - 1;
      setCurrentStep(prevStep);

      // Update currentStep in localStorage
      updateProgress({
        currentStep: prevStep,
      });

      startStep(prevStep);
    }
  };

  /**
   * Handle skipping a step
   */
  const handleSkip = (data = {}) => {
    const timeSpent = endStep(currentStep);

    // Track step skip
    trackEvent('onboarding_step_skipped', {
      step: currentStep,
      time_spent: timeSpent,
    });

    // Mark as skipped in component state (for UI)
    const newSkippedSteps = skippedSteps.includes(currentStep)
      ? skippedSteps
      : [...skippedSteps, currentStep];
    setSkippedSteps(newSkippedSteps);

    // Update localStorage skippedSteps for analytics persistence
    const updatedSkippedSteps = [...(progress.skippedSteps || [])];
    if (!updatedSkippedSteps.includes(currentStep)) {
      updatedSkippedSteps.push(currentStep);
    }

    // Handle "don't show again" on welcome step
    if (currentStep === 1 && data.dontShowAgain) {
      markDismissed();

      // Single batch update to localStorage (keep current step)
      updateProgress({
        skippedSteps: updatedSkippedSteps,
      });

      handleClose();
      return;
    }

    // Move to next step
    if (currentStep < totalSteps) {
      const nextStep = currentStep + 1;
      setCurrentStep(nextStep);

      // Single batch update to localStorage
      updateProgress({
        skippedSteps: updatedSkippedSteps,
        currentStep: nextStep,
      });

      startStep(nextStep);
    } else {
      // Single batch update to localStorage (no currentStep change)
      updateProgress({
        skippedSteps: updatedSkippedSteps,
      });

      handleClose();
    }
  };

  /**
   * Handle completing the wizard
   */
  const handleComplete = () => {
    const timeSpent = endStep(currentStep);

    // Mark final step as completed in component state
    const finalCompletedSteps = completedSteps.includes(currentStep)
      ? completedSteps
      : [...completedSteps, currentStep];
    setCompletedSteps(finalCompletedSteps);

    // Update localStorage for the final step
    const updatedCompletedSteps = [...(progress.completedSteps || [])];
    if (!updatedCompletedSteps.includes(currentStep)) {
      updatedCompletedSteps.push(currentStep);
    }

    // Single batch update to localStorage
    updateProgress({
      completedSteps: updatedCompletedSteps,
    });

    // Combine current session state with persisted analytics data
    const hasCreatedToken = createdToken || progress.createdToken;
    const hasTopupData = topupData || progress.topupData;

    // Track completion
    trackEvent('onboarding_completed', {
      time_spent: timeSpent,
      completion_rate: getCompletionRate(
        finalCompletedSteps,
        totalSteps,
      ),
      created_token: !!hasCreatedToken,
      topped_up: !!hasTopupData,
      total_sessions: sessionCountRef.current,
    });

    // Mark onboarding as complete
    markComplete();

    // Close wizard
    onClose();
  };

  /**
   * Handle closing the wizard
   */
  const handleClose = () => {
    const completionRate = getCompletionRate(completedSteps, totalSteps);

    // Track closure
    trackEvent('onboarding_closed', {
      step: currentStep,
      completion_rate: completionRate,
    });

    // Only reset currentStep in localStorage to avoid flash on refresh
    // Preserve analytics data for is_repeat tracking
    updateProgress({
      currentStep: 1,
    });

    onClose();
  };

  /**
   * Render current step component
   */
  const renderStep = () => {
    switch (currentStep) {
      case 1:
        return <WelcomeStep onNext={handleNext} onSkip={handleSkip} />;
      case 2:
        return (
          <TopupStep
            onNext={handleNext}
            onPrev={handlePrev}
            onSkip={handleSkip}
          />
        );
      case 3:
        return (
          <CreateTokenStep
            onNext={handleNext}
            onPrev={handlePrev}
            onSkip={handleSkip}
          />
        );
      case 4:
        return (
          <GetStartedStep
            createdToken={createdToken}
            onComplete={handleComplete}
          />
        );
      default:
        return null;
    }
  };

  return (
    <Modal
      visible={visible}
      onCancel={handleClose}
      footer={null}
      width={600}
      centered
      bodyStyle={{ padding: '24px 32px' }}
      closeOnEsc={true}
    >
      <ProgressBar
        currentStep={currentStep}
        totalSteps={totalSteps}
        completedSteps={completedSteps}
      />
      {renderStep()}
    </Modal>
  );
};

export default OnboardingWizard;
