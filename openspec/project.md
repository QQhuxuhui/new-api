# Project Context

## Purpose

**New API** is a next-generation AI gateway and asset management system built on top of [One API](https://github.com/songquanpeng/one-api). It serves as a unified interface for multiple AI model providers (OpenAI, Claude, Gemini, etc.) with advanced features including:

- Multi-model API gateway with intelligent routing and load balancing
- Format conversion between different AI APIs (OpenAI ↔ Claude ↔ Gemini)
- User and token management with quota control
- Payment integration (Stripe, Alipay) and billing system
- Multi-language support (Chinese, English, French, Japanese)
- Real-time analytics and monitoring dashboard

**Target Users**: Developers, enterprises, and organizations needing centralized AI API management.

## Tech Stack

### Backend
- **Language**: Go 1.25.1
- **Web Framework**: Gin (gin-gonic/gin v1.9.1)
- **ORM**: GORM with multiple database support
- **Databases**:
  - SQLite (default, via glebarez/sqlite)
  - MySQL ≥ 5.7.8
  - PostgreSQL ≥ 9.6
- **Cache**: Redis (go-redis/redis v8.11.5)
- **Authentication**: JWT (golang-jwt/jwt v5.3.0), WebAuthn, OIDC
- **Session Management**: gin-contrib/sessions
- **WebSocket**: gorilla/websocket v1.5.0

### Frontend
- **Framework**: React 18.2.0
- **Build Tool**: Vite
- **UI Library**: Semi Design (@douyinfe/semi-ui v2.69.1)
- **Routing**: React Router DOM v6
- **HTTP Client**: Axios 1.12.0
- **Charts**: VChart (visactor/react-vchart)
- **i18n**: i18next + react-i18next
- **Styling**: Tailwind CSS

### Infrastructure
- **Containerization**: Docker, Docker Compose
- **Reverse Proxy**: Nginx (recommended for production)
- **Cloud Providers**: AWS SDK (Bedrock), Azure OpenAI

## Project Conventions

### Code Style

#### Go Backend
- **Package Naming**: Lowercase, single-word package names (`model`, `controller`, `service`, `middleware`)
- **File Naming**: Snake_case for multi-word files (e.g., `channel_select.go`, `token_counter.go`)
- **Variable Naming**: camelCase for private, PascalCase for exported
- **Struct Tags**: Use JSON tags for API serialization, GORM tags for database mapping
  ```go
  type Channel struct {
      Id     int    `json:"id"`
      Name   string `json:"name" gorm:"index"`
      Status int    `json:"status" gorm:"default:1"`
  }
  ```
- **Error Handling**: Always check errors explicitly, use descriptive error messages
- **Comments**: Use GoDoc-style comments for exported functions and types

#### Frontend (React)
- **Component Naming**: PascalCase for components
- **File Naming**: Match component name (e.g., `UserProfile.jsx`)
- **Hooks**: Use custom hooks prefix with `use` (e.g., `useAuth`, `useChannel`)
- **Styling**: Tailwind utility classes + Semi Design components
- **i18n**: Use translation keys, organize by feature domain

### Architecture Patterns

#### Backend Architecture (MVC-inspired)
```
├── model/          # Data models and database schemas (GORM models)
├── controller/     # HTTP request handlers (Gin controllers)
├── service/        # Business logic layer
├── middleware/     # Authentication, CORS, rate limiting
├── router/         # Route definitions and grouping
├── relay/          # AI model provider adapters (OpenAI, Claude, etc.)
├── dto/            # Data Transfer Objects for API requests/responses
├── types/          # Type definitions and enums
├── constant/       # Application constants and configuration
├── common/         # Shared utilities and helpers
└── logger/         # Centralized logging
```

#### Key Patterns
- **Repository Pattern**: Models handle database operations via GORM
- **Service Layer**: Business logic separated from controllers
- **Adapter Pattern**: `relay/` packages adapt different AI provider APIs to unified interface
- **Middleware Chain**: Authentication → CORS → Rate Limiting → Logging
- **Session-based Auth**: Gin sessions + JWT for API tokens
- **Channel System**: Weighted random selection + automatic failover for AI providers

#### Database Design
- **Multi-tenancy**: User-level isolation with group-based access control
- **Quota System**: Pre-consumption validation, real-time usage tracking
- **Audit Logging**: All API requests logged with detailed metadata
- **Soft Deletes**: GORM soft delete for data retention

### Testing Strategy

#### Current State
- **Unit Tests**: Minimal coverage (no `*_test.go` files found)
- **Manual Testing**: Primarily through web UI and API clients
- **Channel Testing**: Built-in channel test functionality in controller (`channel-test.go`)

#### Recommended Approach
- **Unit Tests**: Focus on service layer and business logic
- **Integration Tests**: Test database operations and API endpoints
- **E2E Tests**: Critical user flows (authentication, API proxying, billing)
- **Load Testing**: Channel performance and failover mechanisms

### Git Workflow

#### Branching Strategy
- **main**: Production-ready stable branch
- **dev**: Development integration branch (current branch)
- **feature/[name]**: Feature development branches
- **fix/[name]**: Bug fix branches
- **hotfix/[name]**: Critical production fixes

#### Commit Conventions
- Use conventional commits format:
  - `feat:` New features
  - `fix:` Bug fixes
  - `refactor:` Code refactoring
  - `docs:` Documentation updates
  - `chore:` Maintenance tasks
  - `perf:` Performance improvements
  - `test:` Test additions/updates

Example: `feat(relay): add support for Claude 3.7 Sonnet thinking mode`

## Domain Context

### AI Gateway Domain
- **Channel**: An AI provider configuration (API key, base URL, model list)
- **Token**: User-generated API key for accessing the gateway
- **Quota**: Usage credits (measured in tokens or API calls)
- **Model Mapping**: Rename models for client compatibility (e.g., `gpt-4` → `custom-gpt-4`)
- **Relay**: Proxying requests from clients to upstream AI providers
- **Reasoning Effort**: OpenAI o-series models' thinking complexity level
- **Rerank**: Text re-ranking models (Cohere, Jina) for search applications

### Billing & Payment
- **Balance**: User account credit (measured in USD equivalent)
- **Redemption Code**: Prepaid vouchers for quota top-up
- **Billing Group**: Model-specific pricing tiers
- **Cache Billing**: Token cost calculation for cached prompts (OpenAI, Claude, etc.)

### User Management
- **User Roles**:
  - `普通用户` (Regular User): Standard API access
  - `管理员` (Admin): Full system access
  - `Root`: Super admin with unrestricted access
- **Group System**: User groups for model access control
- **Token Groups**: Organize API tokens by project/use-case

## Important Constraints

### Legal & Compliance
- **China AI Regulations**: Cannot provide generative AI services to Chinese public without proper filing (根据《生成式人工智能服务管理暂行办法》)
- **OpenAI Terms**: Users must comply with OpenAI's Terms of Service
- **Educational Use Only**: Project disclaimer states "for personal learning use only"

### Technical Constraints
- **Database**: MySQL ≥ 5.7.8 (for JSON column support)
- **Go Version**: Requires Go 1.25.1 for build
- **Session Secret**: Multi-instance deployment REQUIRES `SESSION_SECRET` environment variable
- **Crypto Secret**: Redis-backed deployments REQUIRE `CRYPTO_SECRET` for encryption
- **SQLite Limitations**: Not recommended for high-concurrency production (use MySQL/PostgreSQL)

### Performance Considerations
- **Streaming Timeout**: Default 300s, configurable via `STREAMING_TIMEOUT`
- **Connection Pooling**: Configurable via `SQL_MAX_IDLE_CONNS`, `SQL_MAX_OPEN_CONNS`
- **Rate Limiting**: User-level and token-level rate limits
- **Channel Weight**: Distribute load across multiple provider channels

### Security Requirements
- **API Key Encryption**: Sensitive keys encrypted at rest (requires `CRYPTO_SECRET`)
- **Rate Limiting**: Protect against abuse and DoS
- **CORS**: Configurable via `FRONTEND_BASE_URL`
- **Two-Factor Auth**: Optional 2FA support (via `twofa.go`)
- **Passkey/WebAuthn**: Modern authentication option

## External Dependencies

### AI Model Providers
- **OpenAI**: GPT-4, GPT-3.5, DALL-E, Whisper, TTS, Realtime API
- **Anthropic Claude**: Claude 3.x series, Messages API
- **Google Gemini**: Gemini 2.5 Pro/Flash with thinking mode
- **Azure OpenAI**: Microsoft-hosted OpenAI models
- **AWS Bedrock**: Amazon's managed AI service
- **Cohere**: Rerank models
- **Jina AI**: Embedding and rerank models
- **DeepSeek**: Chinese AI models
- **Midjourney**: Image generation (via midjourney-proxy)
- **Suno**: Music generation

### Third-Party Services
- **Payment Processors**:
  - Stripe (via `topup_stripe.go`)
  - Creem (via `topup_creem.go`)
  - EPay (Chinese payment gateway)
- **Authentication Providers**:
  - GitHub OAuth
  - Telegram Login
  - LinuxDO (community platform)
  - OIDC (generic OAuth2/OpenID Connect)
  - WeChat (Chinese social platform)
- **Monitoring**:
  - Uptime Kuma integration (via `uptime_kuma.go`)
- **Video Processing**:
  - Gemini video proxy for video understanding

### Infrastructure Services
- **Redis**: Session storage, caching, rate limiting
- **Docker Hub / GHCR**: Container image registry (`calciumion/new-api:latest`)
- **Database**: External MySQL/PostgreSQL for production deployments

## Development Guidelines

### Adding New AI Provider
1. Create adapter in `relay/[provider-name]/` following existing patterns
2. Add provider constants to `constant/api_type.go`
3. Implement request/response transformation logic
4. Add model mapping and pricing configuration
5. Update frontend UI for channel configuration
6. Test with provider's official API

### Adding New Feature
1. **Follow OpenSpec workflow**: Create change proposal in `openspec/changes/`
2. Define requirements and scenarios in spec deltas
3. Update `tasks.md` with implementation checklist
4. Implement backend logic (model → service → controller)
5. Add frontend UI components
6. Update documentation
7. Validate and archive proposal after deployment

### Configuration Management
- **Environment Variables**: Use `.env` file (see `.env.example`)
- **Default Values**: Set sensible defaults in code, override via env vars
- **Database Config**: Stored in `option` table, cached in Redis
- **Feature Flags**: Use environment variables or database options

### Logging & Debugging
- **Structured Logging**: Use `logger/` package for consistent log format
- **Error Logs**: Enable via `ERROR_LOG_ENABLED=true`
- **Debug Mode**: Enable via `DEBUG=true` (verbose logging)
- **Pprof**: Enable performance profiling via `ENABLE_PPROF=true`

---

**Last Updated**: 2025-11-13
**Project Version**: Based on One API with extensive enhancements
**Maintainer**: QuantumNous Team
