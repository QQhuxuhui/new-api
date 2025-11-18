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

import React from 'react';
import { Space, Typography } from '@douyinfe/semi-ui';
import { IconTickCircle } from '@douyinfe/semi-icons';

const { Text } = Typography;

/**
 * Progress indicator for onboarding wizard
 * Shows current step and completion status
 */
const ProgressBar = ({ currentStep, totalSteps = 4, completedSteps = [] }) => {
  const steps = Array.from({ length: totalSteps }, (_, i) => i + 1);

  return (
    <div style={{ marginBottom: 24 }}>
      {/* Step indicator text */}
      <div style={{ textAlign: 'center', marginBottom: 16 }}>
        <Text type='tertiary'>
          步骤 {currentStep} / {totalSteps}
        </Text>
      </div>

      {/* Visual step indicators */}
      <Space
        spacing='medium'
        style={{
          width: '100%',
          justifyContent: 'center',
          flexWrap: 'wrap',
        }}
      >
        {steps.map((step, index) => {
          const isCompleted = completedSteps.includes(step);
          const isCurrent = step === currentStep;
          const isPast = step < currentStep;

          return (
            <React.Fragment key={step}>
              {/* Step circle */}
              <div
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                  gap: 4,
                }}
              >
                {isCompleted || isPast ? (
                  <IconTickCircle
                    size='large'
                    style={{ color: 'var(--semi-color-success)' }}
                  />
                ) : isCurrent ? (
                  <div
                    style={{
                      width: 32,
                      height: 32,
                      borderRadius: '50%',
                      border: '2px solid var(--semi-color-primary)',
                      backgroundColor:
                        'var(--semi-color-primary-light-default)',
                    }}
                  />
                ) : (
                  <div
                    style={{
                      width: 32,
                      height: 32,
                      borderRadius: '50%',
                      border: '2px solid var(--semi-color-border)',
                    }}
                  />
                )}
                <Text
                  size='small'
                  type={
                    isCurrent
                      ? 'primary'
                      : isPast || isCompleted
                        ? 'secondary'
                        : 'tertiary'
                  }
                  style={{ fontSize: 12 }}
                >
                  {step}
                </Text>
              </div>

              {/* Connecting line (except after last step) */}
              {index < steps.length - 1 && (
                <div
                  style={{
                    width: 40,
                    height: 2,
                    backgroundColor:
                      isPast || isCompleted
                        ? 'var(--semi-color-success)'
                        : 'var(--semi-color-border)',
                    alignSelf: 'center',
                    marginTop: -20,
                  }}
                />
              )}
            </React.Fragment>
          );
        })}
      </Space>
    </div>
  );
};

export default ProgressBar;
