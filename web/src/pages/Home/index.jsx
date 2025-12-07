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

import React, { useContext, useEffect, useState, useRef } from 'react';
import { Button } from '@douyinfe/semi-ui';
import { API, showError } from '../../helpers';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { StatusContext } from '../../context/Status';
import { useActualTheme } from '../../context/Theme';
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import { useTranslation } from 'react-i18next';
import { IconPlay, IconFile, IconGithubLogo } from '@douyinfe/semi-icons';
import { Link, useNavigate } from 'react-router-dom';
import NoticeModal from '../../components/layout/NoticeModal';
import OnboardingWizard from '../../components/onboarding/OnboardingWizard';
import './Home.css';

const Home = () => {
  const { t, i18n } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const actualTheme = useActualTheme();
  const [homePageContentLoaded, setHomePageContentLoaded] = useState(false);
  const [homePageContent, setHomePageContent] = useState('');
  const [noticeVisible, setNoticeVisible] = useState(false);
  const [onboardingVisible, setOnboardingVisible] = useState(false);
  const iframeRef = useRef(null);
  const isMobile = useIsMobile();
  const isDemoSiteMode = statusState?.status?.demo_site_enabled || false;
  const docsLink = statusState?.status?.docs_link || '';
  const xianyuShopLink = statusState?.status?.xianyu_shop_link || '';
  const isChinese = i18n.language.startsWith('zh');
  const navigate = useNavigate();

  // Check if user is logged in
  const isLoggedIn = !!localStorage.getItem('user');

  const displayHomePageContent = async () => {
    setHomePageContent(localStorage.getItem('home_page_content') || '');
    try {
      const res = await API.get('/api/home_page_content');
      const { success, message, data } = res.data;
      if (success) {
        let content = data;
        if (!data.startsWith('https://')) {
          content = marked.parse(data);
        }
        setHomePageContent(content);
        localStorage.setItem('home_page_content', content);
      } else {
        showError(message);
        setHomePageContent('加载首页内容失败...');
      }
    } catch (error) {
      console.error('获取首页内容失败:', error);
      setHomePageContent('');
    } finally {
      setHomePageContentLoaded(true);
    }
  };

  // 处理 iframe 消息发送
  const handleIframeLoad = () => {
    if (iframeRef.current?.contentWindow) {
      iframeRef.current.contentWindow.postMessage(
        { themeMode: actualTheme },
        '*',
      );
      iframeRef.current.contentWindow.postMessage({ lang: i18n.language }, '*');
    }
  };

  useEffect(() => {
    const checkNoticeAndShow = async () => {
      const lastCloseDate = localStorage.getItem('notice_close_date');
      const today = new Date().toDateString();
      if (lastCloseDate !== today) {
        try {
          const res = await API.get('/api/notice');
          const { success, data } = res.data;
          if (success && data && data.trim() !== '') {
            setNoticeVisible(true);
          }
        } catch (error) {
          console.error('获取公告失败:', error);
        }
      }
    };

    checkNoticeAndShow();
  }, []);

  useEffect(() => {
    displayHomePageContent().then();
  }, []);

  // 监听主题和语言变化，实时同步给 iframe
  useEffect(() => {
    if (
      homePageContent.startsWith('https://') &&
      iframeRef.current?.contentWindow
    ) {
      try {
        iframeRef.current.contentWindow.postMessage(
          { themeMode: actualTheme },
          '*',
        );
        iframeRef.current.contentWindow.postMessage(
          { lang: i18n.language },
          '*',
        );
      } catch (error) {
        // 忽略跨域错误
        console.debug('Cannot post message to iframe:', error);
      }
    }
  }, [homePageContent, actualTheme, i18n.language]);

  return (
    <div className='home-container'>
      <NoticeModal
        visible={noticeVisible}
        onClose={() => setNoticeVisible(false)}
        isMobile={isMobile}
      />
      <OnboardingWizard
        visible={onboardingVisible}
        onClose={() => setOnboardingVisible(false)}
      />
      {homePageContentLoaded && homePageContent === '' ? (
        <div className='home-wrapper'>
          {/* Dynamic Background Blobs */}
          <div className='home-blob home-blob-indigo' />
          <div className='home-blob home-blob-teal' />
          <div className='home-blob home-blob-purple' />

          {/* Hero Section */}
          <main className='home-hero'>
            <div className='home-hero-content'>
              {/* Badge */}
              <div className='home-animate-fade-in'>
                <span className='home-badge'>
                  <span className='home-badge-dot'>
                    <span className='home-badge-dot-ping' />
                    <span className='home-badge-dot-inner' />
                  </span>
                  {t('全新架构 · 极速体验')}
                </span>
              </div>

              {/* Headline */}
              <h1
                className={`home-headline home-animate-fade-in home-delay-100 ${isChinese ? 'tracking-wide' : ''}`}
              >
                {t('连接未来的')} <br className='hidden md:block' />
                <span className='home-gradient-text'>{t('智能网关')}</span>
              </h1>

              {/* Subheadline */}
              <p className='home-subheadline home-animate-fade-in home-delay-200'>
                {t('Claude Code 与 Codex 的最佳中转服务。')}
                <br />
                {t('更低的价格，更稳定的连接，为您的开发之旅保驾护航。')}
              </p>

              {/* Buttons */}
              <div className='home-buttons home-animate-fade-in home-delay-300'>
                <Link to='/login' className='w-full sm:w-auto'>
                  <Button
                    theme='solid'
                    type='primary'
                    size='large'
                    className='home-btn-primary'
                  >
                    <span>{t('快速开始')}</span>
                    <svg
                      className='home-btn-arrow'
                      fill='none'
                      stroke='currentColor'
                      viewBox='0 0 24 24'
                    >
                      <path
                        strokeLinecap='round'
                        strokeLinejoin='round'
                        strokeWidth='2'
                        d='M13 7l5 5m0 0l-5 5m5-5H6'
                      />
                    </svg>
                  </Button>
                </Link>
                <Button
                  size='large'
                  className='home-btn-secondary'
                  icon={<IconPlay />}
                  onClick={() => {
                    if (xianyuShopLink) {
                      window.open(xianyuShopLink, '_blank');
                    } else {
                      navigate('/console');
                    }
                  }}
                >
                  {t('获取密钥')}
                </Button>
              </div>

              {/* Additional Buttons */}
              <div className='home-extra-buttons home-animate-fade-in home-delay-300'>
                <Button
                  size={isMobile ? 'default' : 'large'}
                  className='home-btn-tertiary'
                  icon={<IconFile />}
                  onClick={() => {
                    if (isLoggedIn) {
                      setOnboardingVisible(true);
                    } else {
                      window.location.href = '/login';
                    }
                  }}
                >
                  {t('使用教程')}
                </Button>
                {isDemoSiteMode && statusState?.status?.version && (
                  <Button
                    size={isMobile ? 'default' : 'large'}
                    className='home-btn-tertiary'
                    icon={<IconGithubLogo />}
                    onClick={() =>
                      window.open(
                        'https://github.com/QQhuxuhui/new-api',
                        '_blank',
                      )
                    }
                  >
                    {statusState.status.version}
                  </Button>
                )}
                {docsLink && (
                  <Button
                    size={isMobile ? 'default' : 'large'}
                    className='home-btn-tertiary'
                    onClick={() => window.open(docsLink, '_blank')}
                  >
                    {t('查看文档')}
                  </Button>
                )}
              </div>
            </div>

            {/* Features / Stats Cards */}
            <div className='home-features home-animate-fade-in home-delay-400'>
              {/* Card 1 */}
              <div className='home-feature-card'>
                <div className='home-feature-icon home-feature-icon-indigo'>
                  <svg
                    className='w-6 h-6'
                    fill='none'
                    stroke='currentColor'
                    viewBox='0 0 24 24'
                  >
                    <path
                      strokeLinecap='round'
                      strokeLinejoin='round'
                      strokeWidth='2'
                      d='M13 10V3L4 14h7v7l9-11h-7z'
                    />
                  </svg>
                </div>
                <div className='home-feature-value'>99.9%</div>
                <div className='home-feature-label'>{t('服务可用性')}</div>
              </div>

              {/* Card 2 */}
              <div className='home-feature-card'>
                <div className='home-feature-icon home-feature-icon-teal'>
                  <svg
                    className='w-6 h-6'
                    fill='none'
                    stroke='currentColor'
                    viewBox='0 0 24 24'
                  >
                    <path
                      strokeLinecap='round'
                      strokeLinejoin='round'
                      strokeWidth='2'
                      d='M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z'
                    />
                  </svg>
                </div>
                <div className='home-feature-value'>{t('Low Latency')}</div>
                <div className='home-feature-label'>{t('全球加速节点')}</div>
              </div>

              {/* Card 3 */}
              <div className='home-feature-card'>
                <div className='home-feature-icon home-feature-icon-purple'>
                  <svg
                    className='w-6 h-6'
                    fill='none'
                    stroke='currentColor'
                    viewBox='0 0 24 24'
                  >
                    <path
                      strokeLinecap='round'
                      strokeLinejoin='round'
                      strokeWidth='2'
                      d='M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z'
                    />
                  </svg>
                </div>
                <div className='home-feature-value'>{t('Secure')}</div>
                <div className='home-feature-label'>{t('企业级安全防护')}</div>
              </div>
            </div>
          </main>

          {/* Footer */}
          <footer className='home-footer'>
            &copy; 2025 Spark Code. All rights reserved.
          </footer>
        </div>
      ) : (
        <div className='overflow-x-hidden w-full'>
          {homePageContent.startsWith('https://') ? (
            <iframe
              ref={iframeRef}
              src={homePageContent}
              onLoad={handleIframeLoad}
              className='w-full h-screen border-none'
            />
          ) : (
            <div
              className='mt-[60px]'
              dangerouslySetInnerHTML={{
                __html: DOMPurify.sanitize(homePageContent),
              }}
            />
          )}
        </div>
      )}
    </div>
  );
};

export default Home;
