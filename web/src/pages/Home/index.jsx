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
import '../../components/common/markdown/markdown.css';
import { IconPlay, IconFile, IconGithubLogo } from '@douyinfe/semi-icons';
import { Link, useNavigate } from 'react-router-dom';
import NoticeModal from '../../components/layout/NoticeModal';
import OnboardingWizard from '../../components/onboarding/OnboardingWizard';
import HomeBento from './HomeBento';
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
        <HomeBento />
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
              className='markdown-body mt-[60px]'
              dangerouslySetInnerHTML={{
                __html: DOMPurify.sanitize(homePageContent, {
                  ADD_TAGS: ['iframe'],
                  ADD_ATTR: ['target', 'rel', 'style'],
                }),
              }}
            />
          )}
        </div>
      )}
    </div>
  );
};

export default Home;
