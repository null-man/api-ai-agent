# Runbook

## 中文说明
- 这份文档是给线上排障时直接照着执行的，不是背景分析文档。
- 如果你只想快速恢复服务，优先看：
  - `Basic Health Checks`
  - `Bot API Inspection`
  - `Recreate a Bot Deployment Cleanly`
  - `Clear Stale PVC Config`
  - `Verify Bot Runtime Config`
- 当前 MiniMax 已验证可用的关键参数：
  - `api = openai-completions`
  - `primary_model = minimax/MiniMax-M2.5-highspeed`
- 如果聊天页提示 `gateway token missing`，说明不是模型挂了，而是前端没带 bot 的 `access_token`。

## ClawHost / OpenClaw Quick Ops

### Environment
- Repo: `/Users/edison/web/clawhost`
- Server: `root@192.210.135.154`
- Kubeconfig: `/etc/rancher/k3s/k3s.yaml`
- Namespace: `clawhost`

### Core Domains
- Admin/API: `api.aiagentpricing.dev`
- Bot wildcard: `*.aiagentpricing.dev`

### 1. Basic Health Checks

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

kubectl -n clawhost get pods
kubectl -n clawhost get svc,endpoints
kubectl -n clawhost get ingress
```

### 2. Main App Logs

Use the actual app pod, not `kubectl logs deployment/clawhost`, because label overlap previously caused confusing results.

```bash
kubectl -n clawhost get pods -l app.kubernetes.io/name=clawhost,app.kubernetes.io/component=server
kubectl -n clawhost logs <clawhost-pod> --tail=200
```

### 3. Bot Status

```bash
kubectl -n clawhost get deploy,pods | grep <bot-slug>
```

Example:

```bash
kubectl -n clawhost get deploy,pods | grep d472b6a7
```

### 4. Bot API Inspection

```bash
curl -s "http://api.aiagentpricing.dev/bot/api/v1/bots/<BOT_ID>" \
  -H "Authorization: Bearer <APP_API_TOKEN>"
```

Check for:
- `status`
- `deployment_status`
- `config.agents.defaults.model.primary`
- `config.models.providers`

### 5. Update Model Provider

MiniMax known-good shape:

```bash
curl -s -X PUT "http://api.aiagentpricing.dev/bot/api/v1/bots/<BOT_ID>/config/models/minimax" \
  -H "Authorization: Bearer <APP_API_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "minimax",
    "baseUrl": "https://api.minimax.io/v1",
    "apiKey": "<NEW_MINIMAX_KEY>",
    "auth": "api-key",
    "api": "openai-completions",
    "models": [
      {
        "id": "MiniMax-M2.5-highspeed",
        "name": "MiniMax-M2.5-highspeed"
      }
    ]
  }'
```

Then set defaults:

```bash
curl -s -X PUT "http://api.aiagentpricing.dev/bot/api/v1/bots/<BOT_ID>/config/defaults" \
  -H "Authorization: Bearer <APP_API_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "primary_model": "minimax/MiniMax-M2.5-highspeed"
  }'
```

### 6. Recreate a Bot Deployment Cleanly

If bot config changed but runtime still uses old Anthropic config:

```bash
curl -s -X POST "http://api.aiagentpricing.dev/bot/api/v1/bots/<BOT_ID>/stop" \
  -H "Authorization: Bearer <APP_API_TOKEN>"

kubectl -n clawhost delete deployment <deploy-name> --ignore-not-found=true
```

### 7. Clear Stale PVC Config

If bot still keeps loading old `openclaw.json`, clean the PVC subPath.

Temporary pod:

```bash
cat >/tmp/fix-bot.yaml <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: fix-bot-config
  namespace: clawhost
spec:
  restartPolicy: Never
  containers:
    - name: shell
      image: alpine:3.19
      command: ["sh", "-c", "sleep 3600"]
      volumeMounts:
        - name: data
          mountPath: /data
          subPath: <BOT_ID>
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: clawhost-shared-data
