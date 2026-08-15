package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yixian-huang/imgli/internal/linkbuilder"
	"github.com/yixian-huang/imgli/internal/service/auth"
	"github.com/yixian-huang/imgli/internal/service/imagesvc"
	"github.com/yixian-huang/imgli/internal/service/stats"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
)

// ImageDeps 图片 handler 依赖。
type ImageDeps struct {
	Img   *imagesvc.Service
	Res   *storagesvc.Resolver
	Stats *stats.Service
}

type ImageHandlers struct{ D ImageDeps }

const maxListLimit = 100

// imageItemDTO 列表项（精简）——类型化 DTO，JSON 字段与历史 map 契约一致。
func (h *ImageHandlers) imageItemDTO(row *imagesvc.Row) ImageItemDTO {
	return imageItemDTOFrom(row, h.D.Res.LinkBase(&row.Policy))
}

// List GET /api/v1/images
func (h *ImageHandlers) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 24
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	f := imagesvc.Filter{
		Q: q.Get("q"), Format: q.Get("format"), Album: q.Get("album"),
		Visibility: q.Get("visibility"), Sort: q.Get("sort"),
	}
	rows, next, err := h.D.Img.List(PrincipalFrom(r).User.ID, f, q.Get("cursor"), limit)
	if err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "列表参数无效")
		return
	}
	items := make([]ImageItemDTO, 0, len(rows))
	for i := range rows {
		items = append(items, h.imageItemDTO(&rows[i]))
	}
	OK(w, map[string]any{"items": items, "next_cursor": next})
}

// imageDetailDTO 详情（含 mime、仅属主 upload_ip）。
func (h *ImageHandlers) imageDetailDTO(row *imagesvc.Row) ImageItemDTO {
	m := h.imageItemDTO(row)
	m.MIME = row.File.MIME
	m.UploadIP = row.Img.UploadIP // 仅属主可达此端点，故直接给出
	return m
}

// Share GET /api/v1/s/{key} —— 公开分享页元数据（无需登录）。
// 仅 public+normal+未过期；其余一律 404，不区分 private 是否存在。
func (h *ImageHandlers) Share(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "key")
	row, err := h.D.Img.GetPublicShare(ref)
	if errors.Is(err, imagesvc.ErrNotFound) {
		Fail(w, http.StatusNotFound, CodeNotFound, "资源不存在")
		return
	}
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	dto := h.imageItemDTO(row)
	// 分享页用稳定 key 预览；外链仍用 slug-aware links
	base := h.D.Res.LinkBase(&row.Policy)
	dto.ShareURL = base + "/s/" + row.Img.Key
	if row.Img.Slug != nil && *row.Img.Slug != "" {
		dto.ShareURL = base + "/s/" + *row.Img.Slug
	}
	// 有口令且未解锁：不给可直出 URL，避免分享页 <img> 触发无口令请求噪声。
	needPW := imgHasPassword(&row.Img) && !imgPasswordOK(r, &row.Img)
	dto.PasswordRequired = &needPW
	if needPW {
		dto.Links = linkbuilder.Links{}
	}
	OK(w, dto)
}

// UnlockShare POST /api/v1/s/{key}/unlock —— 校验访问口令并写 cookie。
func (h *ImageHandlers) UnlockShare(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "key")
	var req struct {
		Password string `json:"password"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	pw := strings.TrimSpace(req.Password)
	if pw == "" || len(pw) > imgPassMaxLen {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "password 不合法")
		return
	}
	row, err := h.D.Img.GetPublicShare(ref)
	if errors.Is(err, imagesvc.ErrNotFound) {
		// 不区分：统一 404，避免枚举
		Fail(w, http.StatusNotFound, CodeNotFound, "资源不存在")
		return
	}
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	if !imgHasPassword(&row.Img) {
		// 无口令不需要解锁
		h.Share(w, r)
		return
	}
	if !auth.VerifyPassword(row.Img.AccessPasswordHash, pw) {
		Fail(w, http.StatusUnauthorized, CodeUnauthorized, "口令错误")
		return
	}
	setImgPassCookie(w, r, row.Img.Key, row.Img.AccessPasswordHash)
	// 复用 Share 组装（此时 cookie 已写，但当前 r 尚无 cookie——手动标已解锁）
	dto := h.imageItemDTO(row)
	base := h.D.Res.LinkBase(&row.Policy)
	dto.ShareURL = base + "/s/" + row.Img.Key
	if row.Img.Slug != nil && *row.Img.Slug != "" {
		dto.ShareURL = base + "/s/" + *row.Img.Slug
	}
	falseV := false
	dto.PasswordRequired = &falseV
	OK(w, dto)
}

// Detail GET /api/v1/images/{key}
func (h *ImageHandlers) Detail(w http.ResponseWriter, r *http.Request) {
	row, err := h.D.Img.Get(PrincipalFrom(r).User.ID, chi.URLParam(r, "key"))
	if err != nil {
		Fail(w, http.StatusNotFound, CodeNotFound, "图片不存在")
		return
	}
	OK(w, h.imageDetailDTO(row))
}

// Stats GET /api/v1/images/{key}/stats——属主访问统计(详情弹窗 ACCESS 区块)。
func (h *ImageHandlers) Stats(w http.ResponseWriter, r *http.Request) {
	total, daily, err := h.D.Stats.ImageStats(PrincipalFrom(r).User.ID, chi.URLParam(r, "key"))
	switch {
	case errors.Is(err, stats.ErrNotFound):
		Fail(w, http.StatusNotFound, CodeNotFound, "图片不存在")
	case err != nil:
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
	default:
		OK(w, map[string]any{"total": total, "daily": daily})
	}
}

// Update PATCH /api/v1/images/{key}
func (h *ImageHandlers) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           *string `json:"name"`
		Visibility     *string `json:"visibility"`
		AlbumID        *int64  `json:"album_id"`
		ExpiresIn      *int    `json:"expires_in"`
		Slug           *string `json:"slug"`
		MaxViews       *int    `json:"max_views"`
		AccessPassword *string `json:"access_password"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	var expAt *time.Time
	setExp := false
	if req.ExpiresIn != nil {
		setExp = true
		if *req.ExpiresIn > MaxExpiresInSec {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, "expires_in 不合法")
			return
		}
		if *req.ExpiresIn > 0 {
			t := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Second)
			expAt = &t
		}
		// <=0 → expAt=nil（清除）
	}
	row, err := h.D.Img.Update(PrincipalFrom(r).User.ID, chi.URLParam(r, "key"), imagesvc.UpdatePatch{
		Name: req.Name, Visibility: req.Visibility, AlbumID: req.AlbumID,
		ExpiresAt: expAt, SetExpires: setExp, Slug: req.Slug,
		MaxViews: req.MaxViews, AccessPassword: req.AccessPassword,
	})
	switch {
	case err == nil:
		if req.AccessPassword != nil && strings.TrimSpace(*req.AccessPassword) == "" {
			clearImgPassCookie(w, chi.URLParam(r, "key"))
		}
		OK(w, h.imageDetailDTO(row))
	case errors.Is(err, imagesvc.ErrNotFound):
		Fail(w, http.StatusNotFound, CodeNotFound, "图片不存在")
	case errors.Is(err, imagesvc.ErrAlbumNotFound):
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
	case errors.Is(err, imagesvc.ErrExpiresOverGroup):
		Fail(w, http.StatusBadRequest, CodeExpiresOverGroup, "有效期超出用户组限制")
	case errors.Is(err, imagesvc.ErrMaxViewsOverGroup):
		Fail(w, http.StatusBadRequest, CodeMaxViewsOverGroup, "访问次数超出用户组限制")
	case errors.Is(err, imagesvc.ErrInvalidVisibility), errors.Is(err, imagesvc.ErrInvalidName),
		errors.Is(err, imagesvc.ErrInvalidSlug), errors.Is(err, imagesvc.ErrSlugTaken),
		errors.Is(err, imagesvc.ErrInvalidMaxViews), errors.Is(err, imagesvc.ErrInvalidAccessPassword),
		errors.Is(err, imagesvc.ErrAlbumForcesPrivate):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
	default:
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
	}
}

// Delete DELETE /api/v1/images/{key} —— 软删进回收站。
func (h *ImageHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	err := h.D.Img.SoftDelete(PrincipalFrom(r).User.ID, chi.URLParam(r, "key"))
	switch {
	case err == nil:
		OK(w, map[string]any{"key": chi.URLParam(r, "key"), "deleted": true})
	case errors.Is(err, imagesvc.ErrNotFound):
		Fail(w, http.StatusNotFound, CodeNotFound, "图片不存在")
	default:
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
	}
}

const maxBatchKeys = 100

// Batch POST /api/v1/images/batch
func (h *ImageHandlers) Batch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action            string   `json:"action"`
		Keys              []string `json:"keys"`
		Visibility        string   `json:"visibility"`
		AlbumID           *int64   `json:"album_id"`
		NamePattern       string   `json:"name_pattern"`
		Find              string   `json:"find"`
		Replace           string   `json:"replace"`
		ReplaceIgnoreCase *bool    `json:"replace_ignore_case"`
		CleanSeparators   *bool    `json:"clean_separators"`
		StartN            *int     `json:"start_n"`
		AlbumName         string   `json:"album_name"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	if len(req.Keys) == 0 || len(req.Keys) > maxBatchKeys {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "keys 数量需为 1-100")
		return
	}
	opts := imagesvc.BatchOpts{
		Visibility:  req.Visibility,
		AlbumID:     req.AlbumID,
		NamePattern: req.NamePattern,
		Find:        req.Find,
		Replace:     req.Replace,
		AlbumName:   req.AlbumName,
	}
	if req.ReplaceIgnoreCase != nil {
		opts.ReplaceIgnoreCase = *req.ReplaceIgnoreCase
	}
	if req.CleanSeparators != nil {
		opts.CleanSeparators = *req.CleanSeparators
	} else if strings.TrimSpace(req.Find) != "" {
		opts.CleanSeparators = true
	}
	if req.StartN != nil {
		opts.StartN = *req.StartN
	}
	results, err := h.D.Img.Batch(PrincipalFrom(r).User.ID, req.Action, req.Keys, opts)
	if errors.Is(err, imagesvc.ErrInvalidAction) {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "action 仅支持 delete|visibility|move|rename")
		return
	}
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	OK(w, map[string]any{"results": results})
}
