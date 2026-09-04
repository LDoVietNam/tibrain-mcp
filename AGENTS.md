# Tibrain Agent Definitions

This document contains the technical specifications for TiBrain agents. All agent definitions, endpoint details, and implementation requirements are documented here.

## Agent Definitions

### tibrain-mcp-hub

- **Description**: MCP hub server for TiBrain, manages MCP connections and protocol handling
- **Tools**: read, write, fs
- **Port**: 3005 (MCP SSE)
- **Protocol**: MCP HTTP/SSE
- **Authentication**: Session-based (via TiBrain session cookies)
- **Key responsibilities**:
  - Manage MCP connection lifecycle
  - Handle protocol version negotiation
  - Route MCP requests to appropriate handlers
  - Manage session state for agent communication

### tibrain-api-server

- **Description**: REST API server for TiBrain services
- **Tools**: rest-api, auth
- **Port**: 3004 (HTTP)
- **Protocol**: HTTP/HTTPS
- **Authentication**: API key or session token based
- **Key endpoints**:
  - `/api/v2/runtime/prompts` - Prompt configuration
  - `/api/v1/prompt/preflight` - Prompt validation
  - `/api/v1/prompt/feedback` - Prompt feedback collection
  - `/api/v2/runtime/prompts` - Runtime prompt configuration
  - `/api/v1/secrets/upsert` - Secret management
  - `/v1/secrets/resolve` - Secret resolution

### tibrain-tirouter-integration

- **Description**: TiRouter integration agent for provider routing and resilience
- **Tools**: read, write, bash, grep
- **Port**: 3004 (TiRouter HTTP API)
- **Responsibilities**:
  - Provider routing management
  - Resilience pattern implementation
  - Health monitoring
  - Configuration management
  - Authentication token rotation

---

## Agent Registration Protocol

1. **Registration**: Agents must register with TiBrain MCP hub on startup
2. **Authentication**: All agents must authenticate using TiBrain credentials
3. **Heartbeat**: Agents must send heartbeat every 30 seconds
4. **Graceful shutdown**: Agents must register for cleanup on SIGTERM
5. **Versioning**: Agents must report version via `/mcp/protocol`

---

## Agent Dispatch Protocol

1. **Classification**: Determine task complexity (1-10 scale)
2. **Agent selection**: Match task to appropriate agent pool
3. **Context budget**: Check memory usage before dispatch
4. **Execution**: Run agents with appropriate resource allocation
5. **Monitoring**: Use loop-status to track execution progress
6. **Merge results**: Synthesize results via execution-plans

---

## Agent Dispatch Patterns

| Pattern | Use Case | Max Agents | Tools |
|---------|----------|------------|-------|
| Fan-out | Independent tasks | 4+ | dispatching-parallel-agents |
| Pipeline | Sequential workflows | 3 | workflow |
| Parallel review | Multi-perspective code review | 8 | parallel |
| Security audit | Sequential security checks | 2 | sequential |

---

## Memory Integration

| Memory Scope | Agent Type | Sync Method |
|--------------|------------|-------------|
| L1 (Hot) | All agents | Session-scoped, auto-save |
| L2 (Warm) | Tibrain-memory-agent | Periodic batch flush |
| L3 (Cold) | All agents | On-demand sync via TiBrain |

---

## Claude 5 Model Selection

| Task Type | Model | Context | Agents |
|-----------|-------|---------|--------|
| Quick fix | Haiku 4.5 | 50% | 2 |
| Feature dev | Sonnet 5 | 70% | 8 |
| Complex system | Opus 5 | 80% | 16 |
| Research | Fable 5 | 60% | 4 |
| Security audit | Opus 5 | 80% | 4 |
| Code review | Opus 5 | 80% | 8 |
| Memory ops | Sonnet 5 | 70% | 8 |
| Multi-provider | Opus 5 | 80% | 8 |

---

## Memory-Aware Agent Design

- **Isolation**: Each agent runs in isolated worktree
- **Memory scoping**: Agents access only their scope (episodic/tiered/global/notes)
- **Context budget**: Monitor and flush at 80% context usage
- **Error recovery**: Auto-retry (max 3) → escalate to reviewer
- **Model switching**: Timeout → cheaper model (Opus → Sonnet → Haiku)

---

## 🛡️ Security Requirements

- **Agent isolation**: Mandatory worktree isolation
- **Credential scoping**: Per-agent least privilege
- **Audit logging**: All agent actions logged to TiBrain episodic
- **Settings.json**: Per-component permissions enforcement
- **MCP scopes**: Component-specific filesystem access

---

## 📌 Verification Gates

- [ ] Agent registration verified via MCP
- [ ] Health check passed (curl localhost:3004/healthz)
- [ ] Memory sync verified (tibain read/write)
- [ ] Agent dispatch tested with parallel tasks
- [ ] Context budget respected (<80% threshold)
- [ ] Security scan passed (security-scan skill)