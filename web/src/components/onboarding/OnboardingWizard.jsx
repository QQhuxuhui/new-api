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

import React, { useState, useEffect } from 'react';
import { Modal } from '@douyinfe/semi-ui';
import { useOnboarding, useOnboardingProgress } from '../../hooks/useOnboarding';
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
  const {
    progress,
    updateProgress,
    markComplete,
    markDismissed,
  } = useOnboarding();

  const {
    startStep,
    endStep,
    getCompletionRate,
  } = useOnboardingProgress();

  const [currentStep, setCurrentStep] = useState(progress.currentStep || 1);
  const [completedSteps, setCompletedSteps] = useState(progress.completedSteps || []);
  const [skippedSteps, setSkippedSteps] = useState(progress.skippedSteps || []);
  const [createdToken, setCreatedToken] = useState(progress.createdToken || null);
  const [topupData, setTopupData] = useState(progress.topupData || null);

  const totalSteps = 4;

  // Restore progress from localStorage when hook hydrates
  useEffect(() => {
    if (visible && progress.startTime) {
      // Only restore if there's actual saved progress
      setCurrentStep(progress.currentStep || 1);
      setCompletedSteps(progress.completedSteps || []);
      setSkippedSteps(progress.skippedSteps || []);
      setCreatedToken(progress.createdToken || null);
      setTopupData(progress.topupData || null);
    }
  }, [visible, progress]);

  // Track wizard start
  useEffect(() => {
    if (visible) {
      trackEvent('onboarding_started', {
        auto_start: autoStart,
      });
      startStep(currentStep);
    }
  }, [visible, autoStart]);

  // Save progress whenever state changes
  useEffect(() => {
    if (visible) {
      updateProgress({
        currentStep,
        completedSteps,
        skippedSteps,
        createdToken,
        topupData,
        startTime: progress.startTime || new Date().toISOString(),
      });
    }
  }, [currentStep, completedSteps, skippedSteps, createdToken, topupData]);

  /**
   * Handle moving to next step
   */
  const handleNext = (data = {}) => {
    const timeSpent = endStep(currentStep);

    // Track step completion
    trackEvent('onboarding_step_completed', {
      step: currentStep,
      time_spent: timeSpent,
      ...data,
    });

    // Update state based on step data
    if (currentStep === 1 && data.dontShowAgain) {
      markDismissed();
    }
    if (currentStep === 2 && data.method) {
      // Record topup data if user used any topup method
      setTopupData(data);
    }
    if (currentStep === 3 && data.createdToken) {
      setCreatedToken(data.createdToken);
    }

    // Mark current step as completed
    if (!completedSteps.includes(currentStep)) {
      setCompletedSteps([...completedSteps, currentStep]);
    }

    // Move to next step
    if (currentStep < totalSteps) {
      setCurrentStep(currentStep + 1);
      startStep(currentStep + 1);
    }
  };

  /**
   * Handle going back to previous step
   */
  const handlePrev = () => {
    if (currentStep > 1) {
      endStep(currentStep);
      setCurrentStep(currentStep - 1);
      startStep(currentStep - 1);
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

    // Mark as skipped
    if (!skippedSteps.includes(currentStep)) {
      setSkippedSteps([...skippedSteps, currentStep]);
    }

    // Handle "don't show again" on welcome step
    if (currentStep === 1 && data.dontShowAgain) {
      markDismissed();
      handleClose();
      return;
    }

    // Move to next step
    if (currentStep < totalSteps) {
      setCurrentStep(currentStep + 1);
      startStep(currentStep + 1);
    } else {
      handleClose();
    }
  };

  /**
   * Handle completing the wizard
   */
  const handleComplete = () => {
    const timeSpent = endStep(currentStep);

    // Mark final step as completed
    if (!completedSteps.includes(currentStep)) {
      setCompletedSteps([...completedSteps, currentStep]);
    }

    // Track completion
    trackEvent('onboarding_completed', {
      time_spent: timeSpent,
      completion_rate: getCompletionRate([...completedSteps, currentStep], totalSteps),
      created_token: !!createdToken,
      topped_up: !!topupData,
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
        return <TopupStep onNext={handleNext} onPrev={handlePrev} onSkip={handleSkip} />;
      case 3:
        return <CreateTokenStep onNext={handleNext} onPrev={handlePrev} onSkip={handleSkip} />;
      case 4:
        return <GetStartedStep createdToken={createdToken} onComplete={handleComplete} />;
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
