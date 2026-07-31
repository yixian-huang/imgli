# 多策略存储：迁移与同步（设计草案）

**状态**: draft · 2026-07-31  
**关联**: 现有 `imgli storage-migrate`、`internal/service/storagesvc.MigrateFiles`、存储 Caps、用户组 `allowed_policy_ids`  
**非目标**: 把核心能力做成 Open Core；多租户控制面；FTP first-class 热路径  

---

## 0. 问题陈述

站长常见诉求：

1. **搬迁（migrate）**：local → S3/R2，或 S3 A → S3 B，对象与元数据最终只落在目标策略。  
2. **多策略并存（multi-policy）**：新上传走新盘，旧图仍在旧盘；按用户组/显式 policy 分流。  
3. **同步 / 双写（sync，更重）**：同一对象在两处都有副本（灾备、灰度 CDN），读可 failover。

当前产品：**已有 1 的运维向 MVP**；**2 的上传选择部分具备**；**3 未做**。

---

## 1. 现状（已落地）

### 1.1 数据模型

| 实体 | 要点 |
|------|------|
| `storage_policies` | 多条策略；driver=local/s3/webdav/ftp；`cdn_domain` / config |
| `files.storage_policy_id` + `files.path` | **对象归属以 File 行为准**；path 在策略间可「同键」复用 |
| `images` → `file_id` | 多图可共享同一 content-hash file（秒传） |
| 用户组 `allowed_policy_ids` | 上传可选策略白名单 |
| 用户偏好 `default_policy_id` | 在白名单内默认策略 |

### 1.2 上传路由

`upload.resolvePolicy`：显式 policy → 用户默认 → 组 `AllowedPolicyIDs[0]` → 全局第一个 enabled。  
**没有**「按负载均衡 / 按剩余配额自动选策略」。

### 1.3 跨策略搬迁（CLI MVP）

```text
imgli storage-migrate -from <id> -to <id> [-dry-run] [-delete-source] [-limit N]
```

实现：`storagesvc.MigrateFiles`

| 行为 | 说明 |
|------|------|
| 范围 | `files` 表中 `storage_policy_id = from` |
| 复制 | 同 `path` 键 src→dst `Open`+`Put`；缩略图 best-effort |
| 改库 | 成功后 `files.storage_policy_id = to` |
| 删源 | 可选 `-delete-source` |
| 语义 | **搬迁 / cutover**，不是持续双写 |

**缺口（相对站长预期）**：无 Admin UI；无按「用户/相册/时间」过滤；无进度任务；无校验 checksum；无只读副本/双写；无回滚一键；无与 CDN purge 联动。

### 1.4 Caps /  tier

`migrate_only` tier 已在 Caps 设计中预留（导入专用后端），与「热读策略」分层可复用。

---

## 2. 概念分层（建议产品话术）

| 模式 | 一句话 | 读路径 | 写路径 | 实现难度 |
|------|--------|--------|--------|----------|
| **A. 多策略分流** | 新图进新策略，旧图不动 | 各读各的 policy | 仅目标 | 已有 + 运营配置 |
| **B. 一次性搬迁** | 把 from 上的对象搬到 to | 搬完只读 to | cutover 时切换 | CLI 已有 → 加强 |
| **C. 后台搬迁任务** | 同 B，可暂停/续跑/看进度 | 同 B | 同 B | 中 |
| **D. 双写同步** | 上传同时写主+备 | 主优先，失败试备 | 双 Put | 高 |
| **E. 异步复制副本** | 主写完后任务复制到备 | 主；副本仅灾备/校验 | 主 + 异步 | 中高 |

**推荐默认路径**：站长先 **A**，需要腾空旧盘时用 **B/C**；**D/E** 单独立项，不与 0.5 混谈。

---

## 3. 推荐产品路线

### P0 — 文档与可发现性（可立刻）

- README / 运维文档增加 **`storage-migrate` 专节**（dry-run → limit 小批 → 全量 → delete-source）。  
- Admin 策略页脚注：链接文档；说明「搬迁用 CLI，不在此 UI」。  
- doctor：可选检测「仅一条 enabled 策略」或「组允许策略已禁用」。

### P1 — 搬迁任务化（C）

在现有 `MigrateFiles` 上包一层 **Task**（复用 `internal/task`）：

| 字段 / 能力 | 说明 |
|-------------|------|
| `from_policy_id` / `to_policy_id` | 必填 |
| `dry_run` / `delete_source` / `limit` / `cursor` | 续跑用 last file id |
| `filter`（可选） | `created_before`、`user_id`、`surface` |
| 进度 | scanned/copied/skipped/failed 写 task result 或 settings/作业表 |
| 互斥 | 同 from 同时只跑一个 migrate job |
| Admin API | `POST /admin/storage/migrate` + `GET` 状态（管理员 only） |
| 前端 | 策略页「搬迁向导」：选 from/to、dry-run 预览、确认、进度条 |

**校验增强（P1 或 P1.1）**

- 复制后可选 `HEAD`/size 比对；失败不改 `storage_policy_id`。  
- 秒传：同一 hash 多 file 行仍按 **file 行** 搬（当前语义保留）。

### P2 — 分流体验（A 增强）

- 上传 UI：多 allowed policy 时展示「存储策略」选择（偏好已有 default）。  
- Admin：组策略说明「新上传默认走列表第一项」。  
- 可选：`upload_weight` / `prefer_new_for_public` 等**简单**规则（避免复杂调度器）。

### P3 — 副本 / 同步（D/E，单独立项）

**仅当**有明确灾备客户或自托管「双桶」需求再开：

1. **异步复制（E 优先于 D）**  
   - `files` 增加可选 `replica_policy_id` 或旁表 `file_replicas(file_id, policy_id, state)`。  
   - 上传成功后 enqueue `replicate_file`；serve **仍只读主 policy**（简单、正确）。  
2. **双写（D）**  
   - 上传事务内双 Put，失败策略：主成功备失败 → 任务补齐 vs 整单失败（需产品裁决）。  
3. **读 failover**  
   - 主 404 时试副本；与 CDN 302 交互复杂，**最后做**。

**明确不做（除非商业）**

- 跨实例多机「存储集群协议」。  
- 任意格式双向 sync 冲突合并（图床不是网盘同步盘）。

---

## 4. 搬迁作业状态机（P1 草案）

```text
pending → running → completed
                 ↘ failed (可 retry from cursor)
                 ↘ cancelled
```

- `running` 中进程崩溃：凭 `cursor` 续跑（已改 policy 的行自动 skip）。  
- `delete_source=true` 时：仅在 **Update 成功后** 删源；中断不删未改库对象。  
- dry-run 永不改库、不删源。

---

## 5. 与 Caps / CDN 的关系

| 场景 | 注意 |
|------|------|
| local → S3 + cdn_domain | 搬完后公图可 302；旧 CDN 缓存需运营 purge |
| 私有图 + presign | 目标策略需 `PrivatePresignReady`；否则私有外链行为变差 |
| compat/FTP 作目标 | 允许但 doctor/Caps 应 WARN「不宜作热读」 |
| migrate_only 源 | 适合「导入盘」：只进不出或只出不进，与 tier 对齐 |

---

## 6. 安全与权限

- 仅管理员 / CLI 本机运维；禁止普通用户触发全站 migrate。  
- API 不回显存储密钥；进度只含计数与脱敏 path 样例。  
- 大站 limit 分批 + 带宽/磁盘监控（现有 health-check 可挂钩）。

---

## 7. 建议里程碑切分

| 版本意向 | 交付 |
|----------|------|
| **文档补丁（随时）** | storage-migrate 专节 + 策略页链接 — ✅ v0.5.1 / v0.6 |
| **v0.6 候选 A** | Admin 搬迁向导 + task 进度 + cursor 续跑 — ✅ **v0.6.0 已交付**（#54） |
| **v0.6 候选 B** | 过滤条件 + size 校验 — ✅ **v0.6.0 已交付**（#58） |
| **更晚** | file_replicas / 异步复制；双写与读 failover 另 epic |

与「单实例自托管」一致：**先做可靠搬迁，再谈同步**。v0.6 已交付 cutover 任务化；同步仍属更晚 epic。

---

## 8. 验收草图（P1）

1. dry-run：scanned>0，库与对象不变。  
2. limit=10：恰 10 条改 policy，对象在 to 可读。  
3. 全量 + delete-source：from 上空（或仅 skipped 缺失），to 上 serve 正常。  
4. 中断后续跑：无重复错误、无双删。  
5. 目标 disabled：拒绝启动。  
6. 私有图搬迁后口令/过期/带宽计量仍走现有 gate。

---

## 9. 开放问题（需产品拍板时再开写）

1. 搬迁时是否冻结 from 上的新上传？（建议：Admin 先把组 allowed 改为只含 to）  
2. 秒传命中「源策略旧对象」时是否自动触发复制到默认策略？（建议：P2 可选）  
3. 副本是否计入用户存储配额？（建议：主对象计费，副本不计或系数 0）  

---

## 10. 实现触点（代码地图）

| 层 | 路径 |
|----|------|
| CLI | `cmd/imgli/main.go` → `storage-migrate` |
| 核心 | `internal/service/storagesvc/migrate.go` |
| 驱动 | `internal/storage/*` Driver Put/Open/Exists/Delete |
| 任务 | `internal/task`（P1 包装） |
| Admin API | 待增 `handler` + `adminsvc` |
| UI | `web/src/pages/admin/policies` 向导 |
| 文档 | `docs/backup.md` 旁或新建 `docs/storage-migrate.md` |

---

*本文为规划草案，不构成已排期承诺。*
