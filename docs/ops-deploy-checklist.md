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
# 推荐：仓库脚本（本机 / CI smoke-prod / 部署机均可）
./scripts/ops-smoke-public.sh https://img.li

# 或逐步手敲
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

发版全流程（tag 门禁、Release 时序、baili 升级）见 **[docs/ops-release.md](ops-release.md)**。
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

## 升级到 v0.9（用户组生命周期）

1. 部署带 AutoMigrate 的新二进制/镜像（用户组新列自动加）。
2. **游客组**：若生命周期字段仍全为 `0`，启动 Seed 会补默认 1d / 7d / force 7d。
3. 登录用户组：在后台 **用户组** 按运营策略配置 `max_expires_in` / `force_max_age_days` 等。
4. **存量**：改配置**不**自动改写旧图；需要时用「存量预览 / 钳制」，或依赖小时任务 +
   **系统 / 运维** 清理 kinds（含 `group_retention` / `group_force_age`）。
5. 大规模硬清公开图后按需做 **CDN purge**（见清理文档）。

详见 [`user-groups-lifecycle.md`](user-groups-lifecycle.md)。

## 升级到 v0.9.2（秒传复用 · 站名 · 策略统计）

1. 滚动部署新二进制/镜像即可（无新迁移要求；API 多返回 `reused` 与策略
   `live_image_count` / `trash_image_count`，旧客户端可忽略）。
2. **同用户秒传**：相同内容且可见性/相册/过期/次数/口令一致时返回原 `key`，图库不增行、不二次扣配额；跨用户仍各自 key + 共享物理文件。
3. **站点名称**：后台改 `site_name` 后刷新前台，顶栏字标应显示站名（鲤鱼标仍在）。
4. **存储策略**：列表对象数含回收站仍占用的物理文件；详情有在线/回收站图片拆分——勿与「在线图片列表」直接对表。

验收清单：[`superpowers/plans/2026-08-04-v0.9.2-acceptance.md`](superpowers/plans/2026-08-04-v0.9.2-acceptance.md)。

## 升级到 v0.9.3–v0.9.4（UI · 回收站恢复 · WebP）

1. 滚动部署即可。v0.9.3 为 Tailwind 全站样式；部署后务必做 **SPA 冒烟**（本页「部署后」表），避免旧前端缓存白屏。
2. **v0.9.4 回收站恢复**：后台图片列表/详情/批量支持 restore；无需数据迁移。
3. **原图 WebP**：Docker 镜像默认带 libvips；纯 Go GitHub 二进制无 vips 时 `processing_capabilities.webp_encode` 为 false，后台勿强开 WebP。
4. 系统页可读出 `imaging_backend` / `webp_encode`。

## 升级到 v0.9.14（SMTP · 邮件文案）

1. 滚动部署新二进制/镜像即可。首次启动 Seed 会写入空的 `mail_templates` 键（全空=继续用内置文案），**无 schema / 存储迁移**。
2. 后台 **系统设置 → 邮件 SMTP**：飞书/Lark 公共邮箱用完整地址 + IMAP/SMTP 密码；改用户名须重输密码；测发信用当前表单。其下 **邮件文案** 可改五封信的中英主题/正文，预览/试发不必先保存。
3. `imgli version` 为 **v0.9.14**；SPA 冒烟（本页「部署后」表）。

## 升级到 v0.9.13（上传拖放）

1. 滚动部署新二进制/镜像即可，**无 schema / 存储迁移**。
2. 验收：往上传区丢一张图只出现一条队列、一次上传；拖入后不松手移出页面，overlay 消失且未入队；在投放区文字/图标间移动时内层高亮不闪。
3. `imgli version` 为 **v0.9.13**；SPA 冒烟（本页「部署后」表）。

## 升级到 v0.9.12（私密相册隐私 · 源站缓存）

1. 滚动部署新二进制/镜像。首次启动会跑 **schema v8**（把私密相册里仍标 public 的图改为 private），并 **best-effort 纠偏** `files.surface` 与 `images.visibility` 不一致的对象（`public/` ↔ `private/` 搬迁，可能有额外 I/O）。
2. **隐私行为**：私密相册内的图不进广场；匿名 `/i` 401；父相册私密时 `/s` 与分享 OG 404；不能在私密相册内把图改回公开；公开相册封面必须是公开且无口令的图；首页公开统计 `live_image_count` 只计未过期、无口令的公开图。
3. **源站缓存**（默认开）：公开 `/t` 与未 302 的 `/i` 落在 `{data_dir}/.serve-cache`（默认 512MiB）。关闭：`IMGLI_SERVE_CACHE_DISABLED=true` 或 YAML `serve_cache_disabled: true`。
4. 验收：私密相册抽一张图确认广场不可见、匿名直链 401、分享页 404；公开热链重复请求应命中缓存（S3 GET 下降）；`imgli version` 为 **v0.9.12**；SPA 冒烟（本页「部署后」表）。

## 升级到 v0.9.5（暗色 · 站点外观）

1. 滚动部署新二进制/镜像（settings 缺省键在启动 Seed 写入：`theme_bg_dim=0.72`、`theme_glass=0.78`，accent/背景 URL 默认为空）。
2. 后台 **系统设置 → 外观**：可配强调色、整站背景图、遮罩、面板不透明度；公开 `GET /api/v1/config` 下发同名键。
3. 暗色模式依赖前端 `@theme inline` 语义色——请确认部署的是 **本版本前端 bundle**（hash 变化）。
4. 字段与边界见 [`design/site-customization-ia.md`](design/site-customization-ia.md)。

## 相关运维文档

- 发版全流程 / CI：[`ops-release.md`](ops-release.md)
- 存储搬迁：[`storage-migrate.md`](storage-migrate.md)
- 清理 vs CDN：[`ops-cleanup-cdn-boundary.md`](ops-cleanup-cdn-boundary.md)
- 用户组生命周期 / 有效期策略：[`user-groups-lifecycle.md`](user-groups-lifecycle.md)
- 站点定制 / 外观：[`design/site-customization-ia.md`](design/site-customization-ia.md)
- OIDC 排错：[`oidc-operator.md`](oidc-operator.md)
- 统计/CDN 计量：`deploy/ops/admin-stats-metering.md`
