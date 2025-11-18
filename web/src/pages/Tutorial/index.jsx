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

import React, { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Typography, Button, Toast } from '@douyinfe/semi-ui';
import { IconCopy, IconTick } from '@douyinfe/semi-icons';

const { Title, Text, Paragraph } = Typography;

// 动态获取站点 Base URL
const getBaseUrl = () => {
  const origin = window.location.origin;
  return origin;
};

// 代码块组件（带复制功能）
const CodeBlock = ({ code, language = 'bash' }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      Toast.success('已复制到剪贴板');
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      Toast.error('复制失败');
    }
  };

  return (
    <div className='relative group'>
      <div className='overflow-x-auto rounded-lg bg-gray-900 dark:bg-black p-3 sm:p-4 font-mono text-xs sm:text-sm border border-gray-700 dark:border-gray-800'>
        <pre className='text-green-400'>
          <code>{code}</code>
        </pre>
      </div>
      <Button
        icon={copied ? <IconTick /> : <IconCopy />}
        size='small'
        theme='solid'
        className='absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity'
        onClick={handleCopy}
      >
        {copied ? '已复制' : '复制'}
      </Button>
    </div>
  );
};

// 步骤标题组件
const StepTitle = ({ step, title }) => (
  <div className='mb-3 sm:mb-4 flex items-center'>
    <span className='mr-2 sm:mr-3 flex h-6 w-6 sm:h-8 sm:w-8 items-center justify-center rounded-full bg-blue-500 text-xs sm:text-sm font-bold text-white'>
      {step}
    </span>
    <Text className='text-lg sm:text-xl font-semibold'>{title}</Text>
  </div>
);

// 提示框组件
const NoteBox = ({ type = 'info', title, children }) => {
  const styles = {
    info: 'border-blue-200 bg-blue-50 dark:border-blue-500/40 dark:bg-blue-950/30',
    success:
      'border-green-200 bg-green-50 dark:border-green-500/40 dark:bg-green-950/30',
    warning:
      'border-yellow-200 bg-yellow-50 dark:border-yellow-500/40 dark:bg-yellow-950/30',
  };

  const textStyles = {
    info: 'text-blue-800 dark:text-blue-300',
    success: 'text-green-800 dark:text-green-300',
    warning: 'text-yellow-800 dark:text-yellow-300',
  };

  return (
    <div className={`rounded-lg border p-3 sm:p-4 ${styles[type]}`}>
      {title && (
        <Text
          className={`block mb-2 font-medium text-sm sm:text-base ${textStyles[type]}`}
        >
          {title}
        </Text>
      )}
      <div className={`text-xs sm:text-sm ${textStyles[type]}`}>{children}</div>
    </div>
  );
};

function Tutorial() {
  const { t } = useTranslation();
  const [activeOS, setActiveOS] = useState('windows');

  const baseUrl = useMemo(() => getBaseUrl(), []);
  const claudeApiUrl = useMemo(() => baseUrl, [baseUrl]); // Claude Code 不需要 /v1 后缀
  const openaiApiUrl = useMemo(() => `${baseUrl}/v1`, [baseUrl]); // OpenAI Codex 需要 /v1 后缀

  const osSystems = [
    { key: 'windows', name: 'Windows', icon: '🪟' },
    { key: 'macos', name: 'macOS', icon: '🍎' },
    { key: 'linux', name: 'Linux / WSL2', icon: '🐧' },
  ];

  return (
    <div className='mt-[60px] px-2'>
      <div className='max-w-4xl mx-auto px-3 sm:px-6 py-6 sm:py-8'>
        <Card className='p-4 sm:p-6'>
          {/* 页面标题 */}
          <div className='mb-6 sm:mb-8'>
            <Title heading={2} className='mb-3 sm:mb-4'>
              📚 {t('tutorial.title', 'AI Code 使用教程')}
            </Title>
            <Paragraph className='text-sm sm:text-base text-gray-600 dark:text-gray-400'>
              {t(
                'tutorial.description',
                '跟着这个教程，你可以轻松在自己的电脑上安装并使用 Claude Code 和 OpenAI Codex。',
              )}
            </Paragraph>
          </div>

          {/* 操作系统选择 */}
          <div className='mb-6 sm:mb-8'>
            <div className='flex flex-wrap gap-2 rounded-xl bg-gray-100 dark:bg-gray-800 p-2'>
              {osSystems.map((os) => (
                <button
                  key={os.key}
                  className={`flex flex-1 items-center justify-center gap-2 rounded-lg px-4 py-3 text-sm font-semibold transition-all duration-300 ${
                    activeOS === os.key
                      ? 'bg-white text-blue-600 shadow-sm dark:bg-blue-600 dark:text-white'
                      : 'text-gray-600 hover:bg-white/50 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-gray-700 dark:hover:text-white'
                  }`}
                  onClick={() => setActiveOS(os.key)}
                >
                  <span>{os.icon}</span>
                  <span>{os.name}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Claude Code 教程 */}
          <div className='mb-8 sm:mb-12'>
            <Title heading={3} className='mb-6'>
              🤖 Claude Code 配置教程
            </Title>

            {/* 步骤 1: 安装 Node.js */}
            <div className='mb-6 sm:mb-8'>
              <StepTitle
                step={1}
                title={t('tutorial.step1.title', '安装 Node.js 环境')}
              />
              <Paragraph className='mb-4 text-sm sm:text-base text-gray-600 dark:text-gray-400'>
                {t(
                  'tutorial.step1.description',
                  'Claude Code 需要 Node.js 环境才能运行。',
                )}
              </Paragraph>

              {activeOS === 'windows' && (
                <NoteBox type='info' title='Windows 安装方法'>
                  <p className='mb-2'>方法一：官网下载（推荐）</p>
                  <ol className='list-decimal ml-4 space-y-1'>
                    <li>
                      打开浏览器访问{' '}
                      <code className='bg-gray-200 dark:bg-gray-700 px-1 rounded'>
                        https://nodejs.org/
                      </code>
                    </li>
                    <li>点击 "LTS" 版本进行下载（推荐长期支持版本）</li>
                    <li>
                      下载完成后双击{' '}
                      <code className='bg-gray-200 dark:bg-gray-700 px-1 rounded'>
                        .msi
                      </code>{' '}
                      文件
                    </li>
                    <li>按照安装向导完成安装，保持默认设置即可</li>
                  </ol>
                  <p className='mt-3 mb-2'>方法二：使用包管理器</p>
                  <CodeBlock
                    code={`# 使用 Chocolatey
choco install nodejs

# 或使用 Scoop
scoop install nodejs`}
                  />
                </NoteBox>
              )}

              {activeOS === 'macos' && (
                <NoteBox type='info' title='macOS 安装方法'>
                  <p className='mb-2'>方法一：使用 Homebrew（推荐）</p>
                  <CodeBlock
                    code={`# 如果未安装 Homebrew，先安装
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# 安装 Node.js
brew install node`}
                  />
                  <p className='mt-3 mb-2'>方法二：官网下载</p>
                  <ol className='list-decimal ml-4 space-y-1'>
                    <li>访问 https://nodejs.org/</li>
                    <li>下载 macOS Installer (.pkg)</li>
                    <li>双击安装包，按照向导完成安装</li>
                  </ol>
                </NoteBox>
              )}

              {activeOS === 'linux' && (
                <NoteBox type='info' title='Linux / WSL2 安装方法'>
                  <p className='mb-2'>使用包管理器安装：</p>
                  <CodeBlock
                    code={`# Ubuntu/Debian
curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
sudo apt-get install -y nodejs

# Fedora
sudo dnf install nodejs

# Arch Linux
sudo pacman -S nodejs npm`}
                  />
                </NoteBox>
              )}

              <div className='mt-4'>
                <NoteBox type='success' title='验证安装'>
                  <p className='mb-2'>安装完成后，打开终端输入以下命令验证：</p>
                  <CodeBlock
                    code={`node --version
npm --version`}
                  />
                  <p className='mt-2'>如果显示版本号，说明安装成功！</p>
                </NoteBox>
              </div>
            </div>

            {/* 步骤 2: 安装 Claude Code */}
            <div className='mb-6 sm:mb-8'>
              <StepTitle
                step={2}
                title={t('tutorial.step2.title', '安装 Claude Code')}
              />
              <Paragraph className='mb-4 text-sm sm:text-base text-gray-600 dark:text-gray-400'>
                使用 npm 全局安装 Claude Code：
              </Paragraph>
              <CodeBlock code='npm install -g @anthropic-ai/claude-code' />
              <div className='mt-4'>
                <NoteBox type='success' title='验证安装'>
                  <CodeBlock code='claude --version' />
                  <p className='mt-2'>显示版本号即安装成功！</p>
                </NoteBox>
              </div>
            </div>

            {/* 步骤 3: 配置 Claude Code */}
            <div className='mb-6 sm:mb-8'>
              <StepTitle
                step={3}
                title={t('tutorial.step3.title', '配置 Claude Code')}
              />

              {/* 推荐方法 */}
              <div className='mb-6'>
                <div className='flex items-center mb-3'>
                  <span className='bg-green-500 text-white text-xs font-bold px-2 py-1 rounded mr-2'>
                    推荐
                  </span>
                  <Text className='text-base sm:text-lg font-semibold'>
                    方法一：使用全局配置文件（推荐）
                  </Text>
                </div>
                <Paragraph className='mb-3 text-sm sm:text-base text-gray-600 dark:text-gray-400'>
                  在用户主目录创建配置文件，应用于所有项目，安全且便捷。
                </Paragraph>

                {activeOS === 'windows' && (
                  <>
                    <p className='mb-2 text-sm text-gray-600 dark:text-gray-400'>
                      创建文件：
                      <code className='bg-gray-200 dark:bg-gray-700 px-1 rounded'>
                        %USERPROFILE%\.claude\settings.json
                      </code>
                    </p>
                    <CodeBlock
                      code={`{
  "apiConfiguration": {
    "baseURL": "${claudeApiUrl}",
    "apiKey": "YOUR_API_KEY"
  }
}`}
                      language='json'
                    />
                  </>
                )}

                {(activeOS === 'macos' || activeOS === 'linux') && (
                  <>
                    <p className='mb-2 text-sm text-gray-600 dark:text-gray-400'>
                      创建文件：
                      <code className='bg-gray-200 dark:bg-gray-700 px-1 rounded'>
                        ~/.claude/settings.json
                      </code>
                    </p>
                    <CodeBlock
                      code={`# 创建配置目录和文件
mkdir -p ~/.claude
cat > ~/.claude/settings.json << 'EOF'
{
  "apiConfiguration": {
    "baseURL": "${claudeApiUrl}",
    "apiKey": "YOUR_API_KEY"
  }
}
EOF`}
                    />
                  </>
                )}

                <div className='mt-4'>
                  <NoteBox type='success' title='配置说明'>
                    <ul className='list-disc ml-4 space-y-1'>
                      <li>配置文件存储在用户主目录，不会被提交到 Git</li>
                      <li>所有项目共享此配置，无需重复设置</li>
                      <li>安全可靠，避免 API Key 泄露风险</li>
                    </ul>
                  </NoteBox>
                </div>
              </div>

              {/* 其他方法 */}
              <div className='mb-6'>
                <Text className='text-base sm:text-lg font-semibold mb-3 block'>
                  方法二：使用配置命令
                </Text>
                <Paragraph className='mb-3 text-sm sm:text-base text-gray-600 dark:text-gray-400'>
                  运行交互式配置向导：
                </Paragraph>
                <CodeBlock code='claude configure' />
              </div>

              <div className='mb-6'>
                <Text className='text-base sm:text-lg font-semibold mb-3 block'>
                  方法三：使用环境变量
                </Text>

                {activeOS === 'windows' && (
                  <>
                    <p className='mb-2 text-sm'>临时设置（PowerShell）：</p>
                    <CodeBlock
                      code={`$env:ANTHROPIC_BASE_URL="${claudeApiUrl}"
$env:ANTHROPIC_API_KEY="YOUR_API_KEY"`}
                    />
                  </>
                )}

                {activeOS === 'macos' && (
                  <>
                    <p className='mb-2 text-sm'>
                      添加到 ~/.zshrc 或 ~/.bashrc：
                    </p>
                    <CodeBlock
                      code={`export ANTHROPIC_BASE_URL="${claudeApiUrl}"
export ANTHROPIC_API_KEY="YOUR_API_KEY"`}
                    />
                    <p className='mt-2 text-sm text-gray-600 dark:text-gray-400'>
                      添加后运行: source ~/.zshrc
                    </p>
                  </>
                )}

                {activeOS === 'linux' && (
                  <>
                    <p className='mb-2 text-sm'>
                      添加到 ~/.bashrc 或 ~/.zshrc：
                    </p>
                    <CodeBlock
                      code={`export ANTHROPIC_BASE_URL="${claudeApiUrl}"
export ANTHROPIC_API_KEY="YOUR_API_KEY"`}
                    />
                    <p className='mt-2 text-sm text-gray-600 dark:text-gray-400'>
                      添加后运行: source ~/.bashrc
                    </p>
                  </>
                )}
              </div>

              <div className='mt-4'>
                <NoteBox type='info' title='API 信息'>
                  <ul className='list-disc ml-4 space-y-1'>
                    <li>
                      API Base URL:{' '}
                      <code className='bg-gray-200 dark:bg-gray-700 px-1 rounded'>
                        {claudeApiUrl}
                      </code>
                    </li>
                    <li>API Key: 从您的账户获取（通常以 sk- 开头）</li>
                  </ul>
                </NoteBox>
              </div>
            </div>
          </div>

          {/* OpenAI Codex 教程 */}
          <div className='mb-8 sm:mb-12'>
            <Title heading={3} className='mb-6'>
              💻 OpenAI Codex (Cursor / Windsurf) 配置教程
            </Title>

            {/* Cursor 配置 */}
            <div className='mb-6 sm:mb-8'>
              <StepTitle step={1} title='配置 Cursor 编辑器' />

              {/* 推荐方法 */}
              <div className='mb-6'>
                <div className='flex items-center mb-3'>
                  <span className='bg-green-500 text-white text-xs font-bold px-2 py-1 rounded mr-2'>
                    推荐
                  </span>
                  <Text className='text-base sm:text-lg font-semibold'>
                    方法一：在设置中配置（推荐）
                  </Text>
                </div>
                <Paragraph className='mb-3 text-sm sm:text-base text-gray-600 dark:text-gray-400'>
                  在 Cursor 设置中配置自定义 API 端点，安全便捷：
                </Paragraph>
                <p className='mb-2 text-sm'>
                  打开 Cursor 设置 (Settings → Features → Override OpenAI Base
                  URL)
                </p>
                <NoteBox type='info'>
                  <p className='mb-2'>Base URL:</p>
                  <code className='bg-gray-200 dark:bg-gray-700 px-2 py-1 rounded block'>
                    {openaiApiUrl}
                  </code>
                  <p className='mt-3 mb-2'>API Key:</p>
                  <p className='text-xs'>在设置中输入您从平台获取的 API Key</p>
                </NoteBox>
                <div className='mt-4'>
                  <NoteBox type='success' title='配置说明'>
                    <ul className='list-disc ml-4 space-y-1'>
                      <li>配置存储在 Cursor 应用内部，不会暴露到项目文件</li>
                      <li>所有项目共享此配置，无需重复设置</li>
                      <li>安全可靠，避免 API Key 泄露风险</li>
                    </ul>
                  </NoteBox>
                </div>
              </div>

              {/* 其他方法 */}
              <div className='mb-6'>
                <Text className='text-base sm:text-lg font-semibold mb-3 block'>
                  方法二：使用环境变量
                </Text>
                <Paragraph className='mb-3 text-sm sm:text-base text-gray-600 dark:text-gray-400'>
                  在用户配置文件中设置环境变量：
                </Paragraph>

                {activeOS === 'windows' && (
                  <>
                    <p className='mb-2 text-sm'>
                      在 PowerShell 配置文件中添加：
                    </p>
                    <CodeBlock
                      code={`$env:OPENAI_API_BASE="${openaiApiUrl}"
$env:OPENAI_API_KEY="YOUR_API_KEY"`}
                    />
                  </>
                )}

                {(activeOS === 'macos' || activeOS === 'linux') && (
                  <>
                    <p className='mb-2 text-sm'>
                      添加到 ~/.zshrc 或 ~/.bashrc：
                    </p>
                    <CodeBlock
                      code={`export OPENAI_API_BASE="${openaiApiUrl}"
export OPENAI_API_KEY="YOUR_API_KEY"

# 重新加载配置
source ~/.zshrc  # 或 source ~/.bashrc`}
                    />
                  </>
                )}
              </div>
            </div>

            {/* Windsurf 配置 */}
            <div className='mb-6 sm:mb-8'>
              <StepTitle step={2} title='配置 Windsurf 编辑器' />

              {/* 推荐方法 */}
              <div className='mb-6'>
                <div className='flex items-center mb-3'>
                  <span className='bg-green-500 text-white text-xs font-bold px-2 py-1 rounded mr-2'>
                    推荐
                  </span>
                  <Text className='text-base sm:text-lg font-semibold'>
                    方法一：使用环境变量（推荐）
                  </Text>
                </div>
                <Paragraph className='mb-3 text-sm sm:text-base text-gray-600 dark:text-gray-400'>
                  在用户配置文件中设置环境变量，安全且全局生效：
                </Paragraph>

                {activeOS === 'windows' && (
                  <>
                    <p className='mb-2 text-sm'>
                      在 PowerShell 配置文件中添加：
                    </p>
                    <CodeBlock
                      code={`$env:OPENAI_API_BASE="${openaiApiUrl}"
$env:OPENAI_API_KEY="YOUR_API_KEY"`}
                    />
                  </>
                )}

                {(activeOS === 'macos' || activeOS === 'linux') && (
                  <>
                    <p className='mb-2 text-sm'>
                      添加到 ~/.zshrc 或 ~/.bashrc：
                    </p>
                    <CodeBlock
                      code={`export OPENAI_API_BASE="${openaiApiUrl}"
export OPENAI_API_KEY="YOUR_API_KEY"

# 重新加载配置
source ~/.zshrc  # 或 source ~/.bashrc`}
                    />
                  </>
                )}

                <div className='mt-4'>
                  <NoteBox type='success' title='配置说明'>
                    <ul className='list-disc ml-4 space-y-1'>
                      <li>
                        环境变量存储在用户主目录配置文件中，不会暴露到项目
                      </li>
                      <li>所有项目和终端会话共享此配置</li>
                      <li>安全可靠，避免 API Key 泄露风险</li>
                    </ul>
                  </NoteBox>
                </div>
              </div>
            </div>
          </div>

          {/* 总结 */}
          <div className='mt-8 border-t border-gray-200 dark:border-gray-700 pt-6'>
            <NoteBox type='success' title='🎉 配置完成！'>
              <p>现在您已经完成了所有配置，可以开始使用 AI Code 工具了！</p>
              <ul className='list-disc ml-4 mt-2 space-y-1'>
                <li>
                  Claude Code: 运行{' '}
                  <code className='bg-gray-200 dark:bg-gray-700 px-1 rounded'>
                    claude
                  </code>{' '}
                  命令开始使用
                </li>
                <li>Cursor / Windsurf: 直接在编辑器中使用 AI 功能</li>
              </ul>
            </NoteBox>
          </div>
        </Card>
      </div>
    </div>
  );
}

export default Tutorial;
