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
import { Button, Typography } from '@douyinfe/semi-ui';
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

const { Text } = Typography;

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
  const isChinese = i18n.language.startsWith('zh');

  // Check if user is logged in
  const isLoggedIn = !!localStorage.getItem('user');

  // FAQ data from admin-managed backend (console_setting.faq)
  const faqData = statusState?.status?.faq || [];
  const faqEnabled = statusState?.status?.faq_enabled ?? true;

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
    <div className='w-full overflow-x-hidden'>
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
        <div className='w-full overflow-x-hidden'>
          {/* Banner 部分 */}
          <div className='w-full border-b border-semi-color-border min-h-[500px] md:min-h-[600px] lg:min-h-[700px] relative overflow-x-hidden'>
            {/* 背景模糊晕染球 */}
            <div className='blur-ball blur-ball-indigo' />
            <div className='blur-ball blur-ball-teal' />
            <div className='flex items-center justify-center h-full px-4 py-20 md:py-24 lg:py-32 mt-10'>
              {/* 居中内容区 */}
              <div className='flex flex-col items-center justify-center text-center max-w-4xl mx-auto'>
                <div className='flex flex-col items-center justify-center mb-6 md:mb-8'>
                  <h1
                    className={`text-4xl md:text-5xl lg:text-6xl xl:text-7xl font-bold text-semi-color-text-0 leading-tight ${isChinese ? 'tracking-wide md:tracking-wider' : ''}`}
                  >
                    <span className='shine-text'>Claude Code/Codex中转</span>
                  </h1>
                  <p className='text-base md:text-lg lg:text-xl text-semi-color-text-1 mt-4 md:mt-6 max-w-xl'>
                    {t('更好的价格，更好的稳定性')}
                  </p>
                  {/* 主要操作按钮组 */}
                  <div className='flex flex-col items-center justify-center gap-6 mt-8 md:mt-10 w-full max-w-2xl'>
                    {/* 快速开始按钮 - 更大更突出 */}
                    <Link to='/login' className='w-full sm:w-auto'>
                      <Button
                        theme='solid'
                        type='primary'
                        size='large'
                        className='!rounded-3xl w-full sm:w-auto px-16 py-4 font-semibold shadow-lg hover:shadow-xl transition-shadow'
                      >
                        {t('快速开始')}
                      </Button>
                    </Link>

                    {/* 次要按钮组 - 水平排列 */}
                    <div className='flex flex-row gap-3 md:gap-4 justify-center items-center flex-wrap'>
                      <Link to='/console'>
                        <Button
                          theme='solid'
                          type='primary'
                          size={isMobile ? 'default' : 'large'}
                          className='!rounded-3xl px-6 md:px-8 py-2'
                          icon={<IconPlay />}
                        >
                          {t('获取密钥')}
                        </Button>
                      </Link>
                      <Button
                        size={isMobile ? 'default' : 'large'}
                        className='flex items-center !rounded-3xl px-6 py-2'
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
                          className='flex items-center !rounded-3xl px-6 py-2'
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
                    </div>
                  </div>
                </div>

                {/* 常见问答 FAQ - Admin-managed content (displays first 4 items) */}
                {faqEnabled && faqData.length > 0 && (
                  <div className='mt-12 md:mt-16 lg:mt-20 w-full px-4'>
                    <div className='flex items-center mb-6 md:mb-8 justify-center'>
                      <Text
                        type='tertiary'
                        className='text-lg md:text-xl lg:text-2xl font-light'
                      >
                        {t('常见问答')}
                      </Text>
                    </div>
                    <div className='max-w-4xl mx-auto space-y-4'>
                      {faqData.slice(0, 4).map((faq, index) => (
                        <div
                          key={faq.id || index}
                          className='bg-semi-color-bg-1 rounded-2xl p-6 shadow-sm hover:shadow-md transition-shadow'
                        >
                          <h3 className='text-lg md:text-xl font-semibold text-semi-color-text-0 mb-2'>
                            {faq.question}
                          </h3>
                          <div
                            className='text-semi-color-text-2'
                            dangerouslySetInnerHTML={{
                              __html: DOMPurify.sanitize(
                                marked.parse(faq.answer || ''),
                              ),
                            }}
                          />
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* 外部文档链接（独立于FAQ，只要配置了就显示） */}
                {docsLink && (
                  <div className='mt-12 md:mt-16 lg:mt-20 w-full px-4 text-center'>
                    <Button
                      type='tertiary'
                      size='small'
                      onClick={() => window.open(docsLink, '_blank')}
                    >
                      {t('查看更多外部文档')}
                    </Button>
                  </div>
                )}
              </div>
            </div>
          </div>
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
              dangerouslySetInnerHTML={{ __html: homePageContent }}
            />
          )}
        </div>
      )}
    </div>
  );
};

export default Home;
