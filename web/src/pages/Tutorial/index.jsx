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

import React, { useState, useMemo, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Typography, Button, Toast, Spin } from '@douyinfe/semi-ui';
import { IconCopy, IconTick } from '@douyinfe/semi-icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import DOMPurify from 'dompurify';
import { API } from '../../helpers';
import HardcodedTutorial from './HardcodedTutorial';

const { Title, Text } = Typography;

function Tutorial() {
  const { t } = useTranslation();
  const [tutorialData, setTutorialData] = useState(null);
  const [tutorialEnabled, setTutorialEnabled] = useState(false);
  const [loading, setLoading] = useState(true);

  const baseUrl = useMemo(() => window.location.origin, []);

  useEffect(() => {
    const fetchTutorialData = async () => {
      try {
        setLoading(true);
        const res = await API.get('/api/status');
        if (res.data.success && res.data.data) {
          const tutorialStr = res.data.data['console_setting.tutorial'];
          const tutorialEnabledStr =
            res.data.data['console_setting.tutorial_enabled'];

          if (tutorialStr) {
            try {
              const parsed = JSON.parse(tutorialStr);
              setTutorialData(parsed);
            } catch (err) {
              console.error('Failed to parse tutorial data:', err);
            }
          }
          setTutorialEnabled(
            tutorialEnabledStr === 'true' || tutorialEnabledStr === true,
          );
        }
      } catch (err) {
        console.error('Failed to fetch tutorial data:', err);
      } finally {
        setLoading(false);
      }
    };

    fetchTutorialData();
  }, []);

  const replaceVariables = (content) => {
    if (!content) return '';

    const variables = {
      '{{BASE_URL}}': baseUrl,
      '{{CLAUDE_API_URL}}': baseUrl,
      '{{OPENAI_API_URL}}': `${baseUrl}/v1`,
      '{{SITE_NAME}}': 'New API', // Can be fetched from system options if needed
    };

    let result = content;
    Object.entries(variables).forEach(([key, value]) => {
      result = result.replaceAll(key, value);
    });
    return result;
  };

  const renderSection = (section) => {
    if (!section.enabled) return null;

    const content = replaceVariables(section.content);

    return (
      <div key={section.id} className='mb-8'>
        <Title heading={3} className='mb-6'>
          {section.title}
        </Title>
        <div className='prose prose-sm sm:prose max-w-none dark:prose-invert'>
          {section.format === 'markdown' ? (
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                code: ({ node, inline, className, children, ...props }) => {
                  if (inline) {
                    return (
                      <code className='bg-gray-200 dark:bg-gray-700 px-1 rounded' {...props}>
                        {children}
                      </code>
                    );
                  }
                  return (
                    <div className='overflow-x-auto rounded-lg bg-gray-900 dark:bg-black p-3 sm:p-4 font-mono text-xs sm:text-sm border border-gray-700 dark:border-gray-800'>
                      <pre className='text-green-400'>
                        <code {...props}>{children}</code>
                      </pre>
                    </div>
                  );
                },
                a: ({ href, children }) => (
                  <a
                    href={href}
                    target='_blank'
                    rel='noopener noreferrer'
                    className='text-blue-600 dark:text-blue-400 hover:underline'
                  >
                    {children}
                  </a>
                ),
              }}
            >
              {content}
            </ReactMarkdown>
          ) : (
            <div
              dangerouslySetInnerHTML={{
                __html: DOMPurify.sanitize(content),
              }}
            />
          )}
        </div>
      </div>
    );
  };

  // Show loading spinner while fetching
  if (loading) {
    return (
      <div className='mt-[60px] px-2 flex items-center justify-center min-h-screen'>
        <Spin size='large' />
      </div>
    );
  }

  // Check if admin tutorial is enabled and has content
  const hasAdminTutorial =
    tutorialEnabled &&
    tutorialData?.sections &&
    tutorialData.sections.length > 0;

  // If admin tutorial is available, render it
  if (hasAdminTutorial) {
    return (
      <div className='mt-[60px] px-2'>
        <div className='max-w-4xl mx-auto px-3 sm:px-6 py-6 sm:py-8'>
          <Card className='p-4 sm:p-6'>
            {tutorialData.sections
              .sort((a, b) => a.order - b.order)
              .map(renderSection)}
          </Card>
        </div>
      </div>
    );
  }

  // Otherwise, fallback to hardcoded tutorial
  return <HardcodedTutorial />;
}

export default Tutorial;
