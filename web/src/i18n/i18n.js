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

import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

import enTranslation from './locales/en.json';
import frTranslation from './locales/fr.json';
import zhTranslation from './locales/zh.json';
import ruTranslation from './locales/ru.json';
import jaTranslation from './locales/ja.json';

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    load: 'languageOnly',
    resources: {
      en: enTranslation,
      zh: zhTranslation,
      fr: frTranslation,
      ru: ruTranslation,
      ja: jaTranslation,
    },
    fallbackLng: 'zh',
    // 默认语言策略：探测顺序移除浏览器语言(navigator)，使无历史选择的新访客
    // 一律回退到 <html lang="zh"> = 中文；用户手动切换仍由 localStorage 记住，
    // URL 查询参数 ?lng=xx 也依然有效。避免英文浏览器首次访问被探测成英文。
    detection: {
      order: ['querystring', 'localStorage', 'cookie', 'htmlTag'],
      lookupQuerystring: 'lng',
      lookupLocalStorage: 'i18nextLng',
      lookupCookie: 'i18next',
      caches: ['localStorage'],
    },
    interpolation: {
      escapeValue: false,
    },
  });

export default i18n;
