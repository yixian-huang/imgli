# img.li 生产部署检查清单（防 outage）

> 2026-07-30 事故：后端公告/页脚改为 `{zh,en}` locale map 后，前端仍把字段当
> **字符串** 渲染 → React「Objects are not valid as a React child」→ **白屏**。
> 进程与 `/healthz` 仍 200，监控若只盯 healthz 会**漏报**。

## 部署前（必须）

1. **本地完整构建**
   ```bash
   make web          # tsc --noEmit + vite build，禁止 tsc 失败仍上传旧二进制
   make test         # 或至少 go test + npm test
   ```
2. **回归：locale / 插槽 / 详情弹窗**
   ```bash
   cd web && npm test -- --run src/lib/locale.test.ts src/ui/SiteSlots.test.tsx src/pages/images/DetailModal.test.tsx
   ```
3. **e2e 与 guest 落地约定**  
   游客关闭时 **`/` 不硬跳 `/login`**（展示 login-gate）。注册流应 `goto('/login')`，勿 `goto('/')` 再断言 `/login`。
4. **确认产物是新包**  
   构建后的 `web/dist/assets/index-*.js` 文件名应变化；部署后公网 HTML 引用的 hash **必须** 与本次一致。

## 部署后（必须，5 分钟内）

| 检查 | 命令 / 动作 | 期望 |
|------|-------------|------|
| 进程 | `systemctl is-active baili` | active |
| healthz | `curl -sf https://img.li/healthz` | 200 |
| 配置 | `curl -sf https://img.li/api/v1/config \| jq .status` | true |
| **SPA 冒烟** | 浏览器无痕打开 `/`，或 Playwright | **非白屏**；可见上传区或登录门 |
| 公告/页脚 | 首页可见 NOTICE 文案与页脚链接 | 无控制台 React 红错 |
| 资源 | HTML 内 `index-*.js` 与 `curl -I` 该 URL | 200 |

```bash
# 快速公网冒烟（部署机/本机均可）
curl -sfS -o /dev/null -w "root:%{http_code}\n" https://img.li/
curl -sfS -o /dev/null -w "healthz:%{http_code}\n" https://img.li/healthz/
JS=$(curl -sfS https://img.li/ | sed -n 's/.*src="\(\/assets\/index-[^"]*\.js\)".*/\1/p' | head -1)
echo "bundle=$JS"
curl -sfS -o /dev/null -w "js:%{http_code}\n" "https://img.li$JS"
# 配置形状抽检：公告 text 若为 object，前端必须能渲染
curl -sfS https://img.li/api/v1/config | python3 -c "
import sys,json
d=json.load(sys.stdin)['data']
ann=(d.get('announcement') or {})
t=ann.get('text')
print('ann_text_type', type(t).__name__)
if isinstance(t, dict) and not (t.get('zh') or t.get('en')):
    raise SystemExit('empty locale map')
print('ok')
"
```

## 契约变更护栏

- **公开 JSON 字段改类型**（string → object 等）= **破坏性变更**：前端与 e2e **同 PR** 合入，禁止只发后端。
- 新插槽字段：写 `pickLocale` / 兼容旧 string；补 `SiteSlots.test.tsx` 一类「渲染不炸」单测。
- 合并前自问：`/api/v1/config` 多了 object，SPA 会不会当 children 直接 render？

## 监控建议（后续）

| 信号 | 说明 |
|------|------|
| 仅 healthz | **不够**——本次事故 healthz 仍绿 |
| 合成监控 | 定期 GET `/` 断言 HTML 含 `id="root"` 且 JS 200；或 Playwright 断言 `[data-testid=dropzone]` / 登录门 |
| 前端错误 | 可选 Sentry / 简单 `window.onerror` 上报 |
| 部署门禁 | CI：`make web && npm test` 失败禁止 release 产物 |

## 回滚

```bash
# VIP：切回上一版二进制（releases 目录内上一个 hash）
systemctl stop baili
cp -a /opt/baili/releases/<previous> /opt/baili/bin/imgli
systemctl start baili
curl -sfS https://img.li/healthz
# 浏览器确认非白屏
```

## 相关代码

- 前端 locale 解析：`web/src/lib/locale.ts`
- 插槽渲染：`web/src/ui/SiteSlots.tsx`
- 回归测：`web/src/ui/SiteSlots.test.tsx`、`web/src/lib/locale.test.ts`

## 相关运维文档

- 存储搬迁：[`storage-migrate.md`](storage-migrate.md)
- 清理 vs CDN：[`ops-cleanup-cdn-boundary.md`](ops-cleanup-cdn-boundary.md)
- OIDC 排错：[`oidc-operator.md`](oidc-operator.md)
- 统计/CDN 计量：`deploy/ops/admin-stats-metering.md`
