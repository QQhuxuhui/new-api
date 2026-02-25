import { marked } from 'marked';
import DOMPurify from 'dompurify';

/**
 * 预处理原始内容，修复 Markdown 解析器将缩进 HTML 误判为代码块的问题。
 * 当 HTML 标签前有 4 个以上空格或 Tab 时，marked 会将其转为 <pre><code>，
 * 导致 HTML 标签被转义为纯文本而无法渲染。
 *
 * 此函数会移除 HTML 标签行首的多余空白，同时保留非 HTML 行的缩进
 * （如真正的代码块）。
 */
function preprocessForMarked(raw) {
  if (!raw) return '';
  // 移除以 4+ 空格或 Tab 开头、紧跟 HTML 标签的行首空白
  return raw.replace(/^[ \t]{4,}(<\/?[a-zA-Z][^>]*>)/gm, '$1');
}

/**
 * 将 Markdown / HTML 混合内容渲染为安全的 HTML 字符串。
 * 统一处理：预处理 → marked 解析 → DOMPurify 清洗。
 *
 * @param {string} raw - 原始 Markdown / HTML 文本
 * @param {object} [options] - 可选配置
 * @param {boolean} [options.sanitize=true] - 是否使用 DOMPurify 清洗
 * @returns {string} 安全的 HTML 字符串
 */
export function renderMarkdown(raw, options = {}) {
  if (!raw) return '';
  const { sanitize = true } = options;
  try {
    const preprocessed = preprocessForMarked(raw);
    const html = marked.parse(preprocessed, { breaks: true, gfm: true });
    if (!sanitize) return html;
    return DOMPurify.sanitize(html, {
      ADD_TAGS: ['iframe'],
      ADD_ATTR: ['target', 'rel', 'style'],
    });
  } catch (err) {
    console.error('Markdown 内容解析失败:', err);
    if (!sanitize) return raw;
    return DOMPurify.sanitize(raw);
  }
}
