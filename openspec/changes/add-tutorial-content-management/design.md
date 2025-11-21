# Design: Tutorial Content Management System

## Architecture Overview

This design follows the proven **console settings pattern** used by FAQ, Announcements, and API Info management. The system uses the existing `option` table for storage with JSON-serialized content.

### System Components

```
┌─────────────────────────────────────────────────────────────┐
│                     Admin Dashboard                          │
│  ┌────────────────────────────────────────────────────────┐ │
│  │         SettingsTutorial.jsx                           │ │
│  │  - List tutorial sections                              │ │
│  │  - Add/Edit/Delete sections                            │ │
│  │  - Rich text editor (Markdown/HTML)                    │ │
│  │  - Enable/disable toggle                               │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ PUT /api/option/update
                            │ { key: "console_setting.tutorial", value: {...} }
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Backend (Go)                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  controller/option.go                                  │ │
│  │  - Validate tutorial JSON structure                    │ │
│  │  - Call console_setting.ValidateTutorial()             │ │
│  └────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  setting/console_setting/tutorial.go                   │ │
│  │  - JSON schema validation                              │ │
│  │  - Ensure sections have required fields               │ │
│  └────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  model/option.go                                       │ │
│  │  - Update option.value in database                     │ │
│  │  - Invalidate cache                                    │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ Database: options table
                            │ key='console_setting.tutorial'
                            │ value='{"sections":[...]}'
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Public Tutorial Page                      │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  web/src/pages/Tutorial/index.jsx                      │ │
│  │  - Fetch from /api/status (includes tutorial data)     │ │
│  │  - Parse JSON sections                                 │ │
│  │  - Replace {{variables}} with actual values            │ │
│  │  - Render Markdown/HTML content                        │ │
│  │  - Fallback to hardcoded if empty                      │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Data Model

### Database Schema (No Changes Required)
Uses existing `options` table:
```sql
-- Existing table structure
CREATE TABLE options (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL
);

-- New record
INSERT INTO options (key, value) VALUES (
    'console_setting.tutorial',
    '{"sections": [...]}'
);
```

### JSON Structure for `console_setting.tutorial`

```json
{
  "sections": [
    {
      "id": "string",           // Unique section identifier (e.g., "claude-code")
      "title": "string",        // Display title (e.g., "Claude Code 配置教程")
      "order": "integer",       // Display order (1, 2, 3...)
      "enabled": "boolean",     // Whether section is visible
      "content": "string",      // Tutorial content (Markdown or HTML)
      "format": "string"        // Content format: "markdown" | "html"
    }
  ]
}
```

**Example Data**:
```json
{
  "sections": [
    {
      "id": "claude-code",
      "title": "🤖 Claude Code 配置教程",
      "order": 1,
      "enabled": true,
      "format": "markdown",
      "content": "## 步骤 1: 安装 Node.js\n\nClaude Code 需要 Node.js 环境。\n\n### Windows 安装\n\n访问 https://nodejs.org/ 下载 LTS 版本。\n\n### 配置说明\n\nAPI Base URL: `{{CLAUDE_API_URL}}`\nAPI Key: 从您的账户获取"
    },
    {
      "id": "openai-codex",
      "title": "💻 OpenAI Codex (Cursor/Windsurf) 配置教程",
      "order": 2,
      "enabled": true,
      "format": "markdown",
      "content": "## Cursor 配置\n\n在设置中配置 Base URL:\n\n```\n{{OPENAI_API_URL}}\n```\n\n输入您的 API Key 即可开始使用。"
    }
  ]
}
```

### Validation Rules

**console_setting.tutorial validation** (in `setting/console_setting/tutorial.go`):
1. Must be valid JSON
2. Must have `sections` array (can be empty)
3. Each section must have:
   - `id` (string, non-empty, alphanumeric + hyphens)
   - `title` (string, non-empty)
   - `order` (integer, >= 0)
   - `enabled` (boolean)
   - `content` (string, can be empty)
   - `format` (string, enum: "markdown" | "html")
4. Section IDs must be unique
5. Maximum 20 sections (configurable)

## Component Design

### Backend Components

#### 1. `controller/option.go` Changes
```go
// Add new validation case in UpdateOption function
case "console_setting.tutorial":
    err = console_setting.ValidateConsoleSettings(option.Value.(string), "Tutorial")
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": false,
            "message": "教程内容设置失败: " + err.Error(),
        })
        return
    }
```

#### 2. `setting/console_setting/tutorial.go` (New File)
```go
package console_setting

import (
    "encoding/json"
    "errors"
    "regexp"
)

type TutorialSection struct {
    Id      string `json:"id"`
    Title   string `json:"title"`
    Order   int    `json:"order"`
    Enabled bool   `json:"enabled"`
    Content string `json:"content"`
    Format  string `json:"format"` // "markdown" or "html"
}

type Tutorial struct {
    Sections []TutorialSection `json:"sections"`
}

func ValidateTutorial(jsonStr string) error {
    var tutorial Tutorial

    if err := json.Unmarshal([]byte(jsonStr), &tutorial); err != nil {
        return errors.New("无效的 JSON 格式")
    }

    if len(tutorial.Sections) > 20 {
        return errors.New("教程章节数量不能超过 20 个")
    }

    idMap := make(map[string]bool)
    idPattern := regexp.MustCompile(`^[a-z0-9-]+$`)

    for i, section := range tutorial.Sections {
        // Validate ID
        if section.Id == "" {
            return errors.New("教程章节 ID 不能为空")
        }
        if !idPattern.MatchString(section.Id) {
            return errors.New("教程章节 ID 只能包含小写字母、数字和连字符")
        }
        if idMap[section.Id] {
            return errors.New("教程章节 ID 重复: " + section.Id)
        }
        idMap[section.Id] = true

        // Validate title
        if section.Title == "" {
            return errors.New("教程章节标题不能为空")
        }

        // Validate order
        if section.Order < 0 {
            return errors.New("教程章节顺序不能为负数")
        }

        // Validate format
        if section.Format != "markdown" && section.Format != "html" {
            return errors.New("教程内容格式必须是 'markdown' 或 'html'")
        }
    }

    return nil
}
```

#### 3. `setting/console_setting/validate.go` (Modify)
```go
// Add Tutorial case to ValidateConsoleSettings function
func ValidateConsoleSettings(value string, settingType string) error {
    switch settingType {
    case "Tutorial":
        return ValidateTutorial(value)
    // ... existing cases
    }
}
```

### Frontend Components

#### 1. `web/src/pages/Setting/Dashboard/SettingsTutorial.jsx` (New File)

**Key Features**:
- Tutorial section list table with columns: ID, Title, Order, Status, Actions
- Add/Edit modal with form fields:
  - Section ID (text input, validation pattern)
  - Title (text input)
  - Order (number input)
  - Enabled (switch)
  - Content (textarea with preview)
  - Format (radio: Markdown/HTML)
- Delete confirmation modal
- Enable/disable global tutorial toggle
- Save button to persist changes
- Preview button to show rendered content

**State Management**:
```jsx
const [tutorialSections, setTutorialSections] = useState([]);
const [showModal, setShowModal] = useState(false);
const [editingSection, setEditingSection] = useState(null);
const [modalForm, setModalForm] = useState({
  id: '',
  title: '',
  order: 1,
  enabled: true,
  content: '',
  format: 'markdown'
});
const [panelEnabled, setPanelEnabled] = useState(true);
```

**Key Functions**:
- `handleAddSection()`: Open modal for new section
- `handleEditSection(section)`: Open modal with existing section data
- `handleDeleteSection(id)`: Delete section with confirmation
- `handleSaveSection()`: Validate and add/update section
- `handleSaveAll()`: Save entire tutorial configuration to backend
- `handlePreview(content, format)`: Show preview of rendered content

**Pattern**: Follows `SettingsFAQ.jsx` structure exactly

#### 2. `web/src/components/settings/DashboardSetting.jsx` (Modify)

Add tutorial option and component:
```jsx
let [inputs, setInputs] = useState({
  // ... existing options
  'console_setting.tutorial': '',
  'console_setting.tutorial_enabled': '',
});

// In render
<Card style={{ marginTop: '10px' }}>
  <SettingsTutorial options={inputs} refresh={onRefresh} />
</Card>
```

#### 3. `web/src/pages/Tutorial/index.jsx` (Modify)

**Changes**:
1. Add state for loading tutorial data:
   ```jsx
   const [tutorialData, setTutorialData] = useState(null);
   const [tutorialEnabled, setTutorialEnabled] = useState(false);
   const [loading, setLoading] = useState(true);
   ```

2. Fetch tutorial data from `/api/status`:
   ```jsx
   useEffect(() => {
     const fetchTutorialData = async () => {
       try {
         const res = await API.get('/api/status');
         if (res.data.success && res.data.data) {
           const tutorialStr = res.data.data['console_setting.tutorial'];
           const tutorialEnabledStr = res.data.data['console_setting.tutorial_enabled'];

           if (tutorialStr) {
             setTutorialData(JSON.parse(tutorialStr));
           }
           setTutorialEnabled(tutorialEnabledStr === 'true');
         }
       } catch (err) {
         console.error('Failed to fetch tutorial data:', err);
       } finally {
         setLoading(false);
       }
     };

     fetchTutorialData();
   }, []);
   ```

3. Replace dynamic variables:
   ```jsx
   const replaceVariables = (content) => {
     const baseUrl = window.location.origin;
     const variables = {
       '{{BASE_URL}}': baseUrl,
       '{{CLAUDE_API_URL}}': baseUrl,
       '{{OPENAI_API_URL}}': `${baseUrl}/v1`,
       '{{SITE_NAME}}': '站点名称' // from system options
     };

     let result = content;
     Object.entries(variables).forEach(([key, value]) => {
       result = result.replaceAll(key, value);
     });
     return result;
   };
   ```

4. Render tutorial sections:
   ```jsx
   const renderSection = (section) => {
     if (!section.enabled) return null;

     const content = replaceVariables(section.content);

     return (
       <div key={section.id} className="mb-8">
         <Title heading={3} className="mb-6">{section.title}</Title>
         {section.format === 'markdown' ? (
           <ReactMarkdown>{content}</ReactMarkdown>
         ) : (
           <div dangerouslySetInnerHTML={{ __html: content }} />
         )}
       </div>
     );
   };
   ```

5. Conditional rendering:
   ```jsx
   return (
     <div className="mt-[60px] px-2">
       {loading ? (
         <Spin />
       ) : tutorialEnabled && tutorialData?.sections?.length > 0 ? (
         // Render admin-managed tutorial
         tutorialData.sections
           .sort((a, b) => a.order - b.order)
           .map(renderSection)
       ) : (
         // Fallback to hardcoded tutorial
         <HardcodedTutorial />
       )}
     </div>
   );
   ```

## Dynamic Variable Replacement

### Supported Variables

| Variable | Value Source | Example |
|----------|-------------|---------|
| `{{BASE_URL}}` | `window.location.origin` | `https://api.example.com` |
| `{{CLAUDE_API_URL}}` | `window.location.origin` | `https://api.example.com` |
| `{{OPENAI_API_URL}}` | `window.location.origin + '/v1'` | `https://api.example.com/v1` |
| `{{SITE_NAME}}` | From system options | `"AI Gateway"` |

### Replacement Implementation
```javascript
const replaceVariables = (content) => {
  const variables = {
    '{{BASE_URL}}': window.location.origin,
    '{{CLAUDE_API_URL}}': window.location.origin,
    '{{OPENAI_API_URL}}': `${window.location.origin}/v1`,
    '{{SITE_NAME}}': siteOptions?.siteName || 'New API'
  };

  let result = content;
  Object.entries(variables).forEach(([key, value]) => {
    result = result.replaceAll(key, value);
  });
  return result;
};
```

## Markdown Rendering

### Library Selection
Use `react-markdown` with plugins:
```bash
npm install react-markdown remark-gfm rehype-raw
```

### Component Usage
```jsx
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeRaw from 'rehype-raw';

<ReactMarkdown
  remarkPlugins={[remarkGfm]}
  rehypePlugins={[rehypeRaw]}
  components={{
    code: CodeBlock, // Custom code block with syntax highlighting
    a: ({ href, children }) => (
      <a href={href} target="_blank" rel="noopener noreferrer">
        {children}
      </a>
    )
  }}
>
  {content}
</ReactMarkdown>
```

## Error Handling

### Backend Validation Errors
```go
// Return structured error messages
return errors.New("教程章节 ID 重复: " + section.Id)
```

### Frontend Error Display
```jsx
try {
  await updateOption('console_setting.tutorial', tutorialJson);
  showSuccess('教程内容保存成功');
} catch (err) {
  showError('保存失败: ' + (err.response?.data?.message || err.message));
}
```

### Fallback Handling
```jsx
// If admin content fails to load or is empty
if (!tutorialData || !tutorialData.sections || tutorialData.sections.length === 0) {
  // Render hardcoded tutorial as fallback
  return <HardcodedTutorial />;
}
```

## Performance Considerations

1. **Option Caching**: Backend caches `console_setting.tutorial` in memory/Redis
2. **Lazy Loading**: Tutorial page only fetches data when visited
3. **Markdown Parsing**: Use memoization for parsed content
4. **Preview Throttling**: Debounce preview updates in editor (500ms)

## Security Considerations

1. **XSS Prevention**:
   - Sanitize HTML content using `DOMPurify`
   - Markdown is safer but still use `rehype-sanitize`
2. **Input Validation**: Backend validates all fields (length, format, pattern)
3. **Admin-Only Access**: Tutorial management requires admin role
4. **JSON Injection**: Validate JSON structure to prevent malformed data

## Migration Strategy

### Default Tutorial Content
Provide migration utility to convert existing hardcoded tutorial:
```bash
# CLI command to generate default tutorial JSON
go run scripts/migrate_tutorial.go
```

### Migration Script
```go
// scripts/migrate_tutorial.go
func generateDefaultTutorial() string {
    tutorial := Tutorial{
        Sections: []TutorialSection{
            {
                Id:      "claude-code",
                Title:   "🤖 Claude Code 配置教程",
                Order:   1,
                Enabled: true,
                Format:  "markdown",
                Content: "...", // Extract from current Tutorial/index.jsx
            },
            {
                Id:      "openai-codex",
                Title:   "💻 OpenAI Codex 配置教程",
                Order:   2,
                Enabled: true,
                Format:  "markdown",
                Content: "...", // Extract from current Tutorial/index.jsx
            },
        },
    }

    jsonData, _ := json.MarshalIndent(tutorial, "", "  ")
    return string(jsonData)
}
```

## Testing Strategy

### Backend Testing
1. Unit tests for `ValidateTutorial()`:
   - Valid JSON with multiple sections
   - Invalid JSON format
   - Duplicate section IDs
   - Invalid format values
   - Empty sections array

### Frontend Testing
1. Manual testing:
   - Add/edit/delete sections
   - Markdown rendering
   - HTML rendering
   - Variable replacement
   - Enable/disable toggle
   - Fallback to hardcoded content

2. Integration testing:
   - Save tutorial configuration
   - Fetch tutorial on page load
   - Preview functionality

### User Acceptance Testing
1. Admin can manage tutorial content without code changes
2. Tutorial page displays correctly with admin content
3. Dynamic variables are replaced correctly
4. Fallback works when admin content is empty

## Future Enhancements (Out of Scope)

1. **Multi-language Support**: Store tutorial content per language
2. **Version History**: Track tutorial content changes over time
3. **Template Library**: Pre-built tutorial templates
4. **Media Upload**: Support images/videos in tutorial content
5. **Search Functionality**: Search within tutorial content
6. **Analytics**: Track tutorial page views and section engagement
