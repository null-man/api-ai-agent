# Project Memory

## 中文摘要
- 这份文档记录了 2026-03-26 线上 MiniMax / OpenClaw bot 排障全过程。
- 核心问题不是单点故障，而是 3 个因素叠加：
  - `clawhost` Service selector 过宽，曾把 Postgres pod 也选进去。
  - bot Deployment 复用了旧模板，没有按新的数据库配置重新生成。
  - bot PVC 中残留旧的 `openclaw.json`，导致新配置即使正确也不会生效。
- MiniMax 在当前这套接法下，最终验证可用的配置是：
  - provider: `minimax`
  - model: `MiniMax-M2.5-highspeed`
  - baseUrl: `https://api.minimax.io/v1`
  - api: `openai-completions`
- 如果以后 bot 又莫名其妙回退到 Anthropic，优先检查：
  - Deployment 是否重新生成
  - PVC 里旧的 `openclaw.json` 是否被清掉
  - bot API 配置与运行时 `openclaw.json` 是否一致

## 中文结论
- 当前线上已验证跑通。
- 以后如果继续维护这个项目，建议把“PVC 旧配置覆盖新配置”的问题做成永久修复，而不是只靠运维删除文件恢复。

## 2026-03-26 MiniMax / OpenClaw Bot Bring-Up Notes

### Context
- Repo: `/Users/edison/web/clawhost`
- Production server: `root@192.210.135.154`
- Kubernetes: `k3s`
- Namespace: `clawhost`
- API domain: `api.aiagentpricing.dev`
- Bot wildcard domain: `*.aiagentpricing.dev`

### What Broke
- `clawhost` admin was reachable, but created bots did not run correctly.
- Bot subdomain routing worked, but chat flow failed for multiple independent reasons:
  1. `clawhost` service selector was too broad and also matched the Postgres pod.
  2. Bot Deployment templates could be reused without regeneration, so old embedded `openclaw.json` content stayed alive.
  3. Bot PVC retained an old `/home/node/.openclaw/openclaw.json`, and startup logic only rewrote config when the file did not exist.
  4. MiniMax provider initially used `openai-responses`, which caused `HTTP 404: 404 page not found`.
  5. Chat page required the bot gateway token in Control UI before `/chat?session=main` would work.

### Important Root Causes

#### 1. Service selector issue
- `service/clawhost` selected both app and Postgres pods because labels overlapped.
- Symptom: wrong endpoints, `kubectl logs deployment/clawhost` sometimes showed Postgres logs, ingress/backend instability.
- Fix applied:
  - add `app.kubernetes.io/component=server` to the main app pod labels
  - narrow the `clawhost` Service selector to include that label

#### 2. Bot config was not taking effect
- New bot config was correctly stored via API under:
  - `/bot/api/v1/bots/:id/config/models`
  - `/bot/api/v1/bots/:id/config/defaults`
- But scaling an existing bot Deployment did not regenerate the embedded `EOFCONFIG` template.
- Result: the bot kept booting with old Anthropic defaults.

#### 3. Persistent old OpenClaw config
- Even after Deployment template generation was fixed, the bot still loaded Anthropic config because the PVC already contained:
  - `/home/node/.openclaw/openclaw.json`
- Startup script behavior:
  - only writes `openclaw.json` if the file does not exist
- Required operational fix:
  - stop bot
  - delete bot Deployment
  - mount bot PVC subPath in a temporary pod
  - delete stale `openclaw.json` and related generated files
  - start bot again

#### 4. MiniMax API mode mismatch
- Working configuration for this deployment ended up being:
  - provider: `minimax`
  - model: `MiniMax-M2.5-highspeed`
  - base URL: `https://api.minimax.io/v1`
  - api mode: `openai-completions`
- `openai-responses` produced:
  - `HTTP 404: 404 page not found`

### Code Changes Made

#### `/root/clawhost/service/k8s/botconfig.go` on server during fix
- `getAPIOrDefault()` updated so `minimax` defaults to `openai-responses` initially during debugging.
- Later operational config was explicitly set to `openai-completions` through the bot config API.
- `getDefaultModelFromConfig()` was adjusted to prefer:
  1. `AgentDefaults.PrimaryModel`
  2. first configured provider/model
  3. legacy fields
  4. Anthropic fallback only as last resort

Note:
- The more important runtime behavior was not just code edits, but forcing bot Deployment regeneration and deleting stale PVC config files.

### Known Good Runtime State
- Bot ID: `d472b6a7-8a2b-4f7e-b064-976669010da7`
- Bot slug: `d472b6a7`
- Bot name: `jarvis`
- Expected runtime model log line:
  - `agent model: minimax/MiniMax-M2.5-highspeed`
- Final verified `openclaw.json` values:
  - `agents.defaults.model.primary = minimax/MiniMax-M2.5-highspeed`
  - `models.providers.minimax.api = openai-completions`
  - `models.providers.minimax.baseUrl = https://api.minimax.io/v1`

### Operational Recovery Procedure
If a bot is stuck on old config again:

1. Stop the bot through API.
2. Delete the bot Deployment in Kubernetes.
3. Mount the shared PVC subPath for that bot in a temporary pod.
4. Delete:
   - `openclaw.json`
   - `openclaw.json.bak`
   - `agents/main/agent/models.json`
   - `agents/main/agent/auth-profiles.json`
5. Start the bot again through API.
6. Verify:
   - new Deployment template contains correct `EOFCONFIG`
   - new pod `openclaw.json` matches desired provider
   - runtime log shows the intended model/provider

### Useful Commands

#### App pod
```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl -n clawhost get pods
kubectl -n clawhost logs <clawhost-pod> --tail=200
```

#### Bot config via API
```bash
curl -s "http://api.aiagentpricing.dev/bot/api/v1/bots/<BOT_ID>" \
  -H "Authorization: Bearer <APP_API_TOKEN>"

curl -s "http://api.aiagentpricing.dev/bot/api/v1/bots/<BOT_ID>/config/models" \
  -H "Authorization: Bearer <APP_API_TOKEN>"

curl -s "http://api.aiagentpricing.dev/bot/api/v1/bots/<BOT_ID>/config/defaults" \
  -H "Authorization: Bearer <APP_API_TOKEN>"
```

#### Bot lifecycle
```bash
curl -s -X POST "http://api.aiagentpricing.dev/bot/api/v1/bots/<BOT_ID>/stop" \
  -H "Authorization: Bearer <APP_API_TOKEN>"

curl -s -X POST "http://api.aiagentpricing.dev/bot/api/v1/bots/<BOT_ID>/start" \
  -H "Authorization: Bearer <APP_API_TOKEN>"
```

#### Bot runtime inspection
```bash
kubectl -n clawhost get deploy,pods | grep <BOT_SLUG>
kubectl -n clawhost get deploy <DEPLOY_NAME> -o yaml | sed -n '55,125p'
kubectl -n clawhost exec -it <BOT_POD> -- sh -lc 'cat /home/node/.openclaw/openclaw.json'
kubectl -n clawhost logs <BOT_POD> --tail=200
```

### Security Notes
- Multiple secrets were pasted during debugging.
- Any exposed MiniMax key should be rotated immediately.
- Any exposed admin/app tokens should also be considered for rotation if this environment is shared or untrusted.

### Follow-Up Recommendation
- Permanent product/code fix should ensure:
  - bot restart/start regenerates runtime config deterministically
  - stale PVC config cannot silently override newer database config
  - provider-specific defaults for MiniMax are encoded cleanly
  - service selectors remain narrow enough to avoid Postgres/app overlap
