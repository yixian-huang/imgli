package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yixian-huang/imgli/internal/service/albumsvc"
	"github.com/yixian-huang/imgli/internal/service/auth"
)

type AlbumDeps struct{ Alb *albumsvc.Service }

type AlbumHandlers struct{ D AlbumDeps }

func albumViewDTO(v *albumsvc.AlbumView) map[string]any {
	a := v.Album
	return map[string]any{
		"id":                  a.ID,
		"name":                a.Name,
		"visibility":          a.Visibility,
		"default_view":        albumsvc.NormalizeDefaultView(a.DefaultView),
		"click_to_immersive":  a.ClickToImmersive,
		"description":         a.Description,
		"image_count":         v.Count,
		"cover_key":           v.CoverKey,
		"list_in_plaza":       a.ListInPlaza,
		"has_access_password": albumsvc.HasAccessPassword(&a),
		"created_at":          a.CreatedAt.Format(time.RFC3339),
	}
}

// publicAlbumViewDTO 访客页：在相册元数据上附可选 owner（有 public_profile 时可链主页）。
func publicAlbumViewDTO(v *albumsvc.AlbumView, passwordRequired bool) map[string]any {
	out := albumViewDTO(v)
	out["password_required"] = passwordRequired
	if passwordRequired {
		// 未解锁不暴露说明与张数细节可仍展示名；保留 count/name
		out["description"] = ""
	}
	if v.Owner != nil {
		out["owner"] = map[string]any{
			"username":       v.Owner.Username,
			"nickname":       v.Owner.Nickname,
			"public_profile": v.Owner.PublicProfile,
		}
	}
	return out
}

func (h *AlbumHandlers) List(w http.ResponseWriter, r *http.Request) {
	views, err := h.D.Alb.List(PrincipalFrom(r).User.ID)
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	items := make([]map[string]any, 0, len(views))
	for i := range views {
		items = append(items, albumViewDTO(&views[i]))
	}
	OK(w, map[string]any{"items": items})
}

func (h *AlbumHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Visibility string `json:"visibility"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	alb, err := h.D.Alb.Create(PrincipalFrom(r).User.ID, req.Name, req.Visibility)
	switch {
	case err == nil:
		OK(w, map[string]any{
			"id": alb.ID, "name": alb.Name, "visibility": alb.Visibility,
			"default_view":       albumsvc.NormalizeDefaultView(alb.DefaultView),
			"click_to_immersive": alb.ClickToImmersive,
			"list_in_plaza":      alb.ListInPlaza,
		})
	case errors.Is(err, albumsvc.ErrInvalidName), errors.Is(err, albumsvc.ErrInvalidVisibility):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
	default:
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
	}
}

func albumIDParam(r *http.Request) (uint64, bool) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil
}

func (h *AlbumHandlers) Detail(w http.ResponseWriter, r *http.Request) {
	id, ok := albumIDParam(r)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "相册 id 无效")
		return
	}
	v, err := h.D.Alb.Get(PrincipalFrom(r).User.ID, id)
	if errors.Is(err, albumsvc.ErrNotFound) {
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
		return
	}
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	OK(w, albumViewDTO(v))
}

// PublicGet GET /api/v1/a/{id} —— 公开相册元数据（无需登录）。
func (h *AlbumHandlers) PublicGet(w http.ResponseWriter, r *http.Request) {
	id, ok := albumIDParam(r)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "相册 id 无效")
		return
	}
	v, err := h.D.Alb.GetPublic(id)
	if errors.Is(err, albumsvc.ErrNotFound) {
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
		return
	}
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	needPW := albumsvc.HasAccessPassword(&v.Album)
	unlocked := !needPW || albumPasswordOK(r, v.Album.ID, v.Album.AccessPasswordHash)
	// 属主访问视为已解锁
	if p := PrincipalFrom(r); p != nil && p.User != nil && p.User.ID == v.Album.UserID {
		unlocked = true
	}
	if unlocked {
		_ = h.D.Alb.RecordView(v.Album.ID)
	}
	OK(w, publicAlbumViewDTO(v, needPW && !unlocked))
}

// PublicImages GET /api/v1/a/{id}/images
func (h *AlbumHandlers) PublicImages(w http.ResponseWriter, r *http.Request) {
	id, ok := albumIDParam(r)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "相册 id 无效")
		return
	}
	// 先取元数据检查口令
	v, err := h.D.Alb.GetPublic(id)
	if errors.Is(err, albumsvc.ErrNotFound) {
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
		return
	}
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	if albumsvc.HasAccessPassword(&v.Album) {
		unlocked := albumPasswordOK(r, v.Album.ID, v.Album.AccessPasswordHash)
		if p := PrincipalFrom(r); p != nil && p.User != nil && p.User.ID == v.Album.UserID {
			unlocked = true
		}
		if !unlocked {
			Fail(w, http.StatusUnauthorized, CodeUnauthorized, "需要访问口令")
			return
		}
	}
	limit := 24
	if vlim := r.URL.Query().Get("limit"); vlim != "" {
		if n, err := strconv.Atoi(vlim); err == nil && n > 0 {
			limit = n
		}
	}
	items, next, err := h.D.Alb.ListPublicImages(id, r.URL.Query().Get("cursor"), limit)
	if errors.Is(err, albumsvc.ErrNotFound) {
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
		return
	}
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"key": it.Key, "name": it.Name, "ext": it.Ext,
			"width": it.Width, "height": it.Height, "size": it.Size,
			"thumbnail_url": "/t/" + it.Key + ".jpg",
			"url":           "/i/" + it.Key + "." + it.Ext,
			"share_path":    "/s/" + it.Key,
		})
	}
	OK(w, map[string]any{"items": out, "next_cursor": next})
}

// UnlockPublic POST /api/v1/a/{id}/unlock
func (h *AlbumHandlers) UnlockPublic(w http.ResponseWriter, r *http.Request) {
	id, ok := albumIDParam(r)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "相册 id 无效")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	v, err := h.D.Alb.GetPublic(id)
	if errors.Is(err, albumsvc.ErrNotFound) {
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
		return
	}
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	if err := albumsvc.VerifyAccessPassword(&v.Album, req.Password); err != nil {
		if errors.Is(err, albumsvc.ErrBadPassword) {
			Fail(w, http.StatusUnauthorized, CodeUnauthorized, "口令错误")
			return
		}
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	setAlbumPassCookie(w, r, v.Album.ID, v.Album.AccessPasswordHash)
	_ = h.D.Alb.RecordView(v.Album.ID)
	OK(w, publicAlbumViewDTO(v, false))
}

func (h *AlbumHandlers) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := albumIDParam(r)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "相册 id 无效")
		return
	}
	var req struct {
		Name             *string `json:"name"`
		Visibility       *string `json:"visibility"`
		DefaultView      *string `json:"default_view"`
		ClickToImmersive *bool   `json:"click_to_immersive"`
		Description      *string `json:"description"`
		CoverKey         *string `json:"cover_key"`
		AccessPassword   *string `json:"access_password"`
		ListInPlaza      *bool   `json:"list_in_plaza"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	alb, err := h.D.Alb.Update(PrincipalFrom(r).User.ID, id, albumsvc.UpdatePatch{
		Name: req.Name, Visibility: req.Visibility, DefaultView: req.DefaultView,
		ClickToImmersive: req.ClickToImmersive,
		Description:      req.Description, CoverKey: req.CoverKey,
		AccessPassword: req.AccessPassword, ListInPlaza: req.ListInPlaza,
	})
	switch {
	case err == nil:
		OK(w, map[string]any{
			"id": alb.ID, "name": alb.Name, "visibility": alb.Visibility,
			"default_view":        albumsvc.NormalizeDefaultView(alb.DefaultView),
			"click_to_immersive":  alb.ClickToImmersive,
			"description":         alb.Description,
			"list_in_plaza":       alb.ListInPlaza,
			"has_access_password": albumsvc.HasAccessPassword(alb),
		})
	case errors.Is(err, albumsvc.ErrNotFound):
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
	case errors.Is(err, albumsvc.ErrInvalidName), errors.Is(err, albumsvc.ErrInvalidVisibility),
		errors.Is(err, albumsvc.ErrInvalidDefaultView), errors.Is(err, albumsvc.ErrInvalidDescription),
		errors.Is(err, albumsvc.ErrInvalidCover), errors.Is(err, albumsvc.ErrInvalidPassword):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
	default:
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
	}
}

func (h *AlbumHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := albumIDParam(r)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "相册 id 无效")
		return
	}
	withImages := r.URL.Query().Get("with_images") == "true"
	err := h.D.Alb.Delete(PrincipalFrom(r).User.ID, id, withImages)
	switch {
	case err == nil:
		OK(w, map[string]any{"id": id, "deleted": true, "with_images": withImages})
	case errors.Is(err, albumsvc.ErrNotFound):
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
	default:
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
	}
}

// Stats GET /api/v1/albums/{id}/stats
func (h *AlbumHandlers) Stats(w http.ResponseWriter, r *http.Request) {
	id, ok := albumIDParam(r)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "相册 id 无效")
		return
	}
	total, daily, err := h.D.Alb.Stats(PrincipalFrom(r).User.ID, id)
	if errors.Is(err, albumsvc.ErrNotFound) {
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
		return
	}
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	OK(w, map[string]any{"total": total, "daily": daily})
}

// Reorder POST /api/v1/albums/{id}/reorder
func (h *AlbumHandlers) Reorder(w http.ResponseWriter, r *http.Request) {
	id, ok := albumIDParam(r)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "相册 id 无效")
		return
	}
	var req struct {
		Keys []string `json:"keys"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	err := h.D.Alb.Reorder(PrincipalFrom(r).User.ID, id, req.Keys)
	switch {
	case err == nil:
		OK(w, map[string]any{"ok": true, "count": len(req.Keys)})
	case errors.Is(err, albumsvc.ErrNotFound):
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
	case errors.Is(err, albumsvc.ErrInvalidReorder):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
	default:
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
	}
}

// SetImagesVisibility POST /api/v1/albums/{id}/images/visibility
func (h *AlbumHandlers) SetImagesVisibility(w http.ResponseWriter, r *http.Request) {
	id, ok := albumIDParam(r)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "相册 id 无效")
		return
	}
	var req struct {
		Visibility string `json:"visibility"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	n, err := h.D.Alb.SetImagesVisibility(PrincipalFrom(r).User.ID, id, req.Visibility)
	switch {
	case err == nil:
		OK(w, map[string]any{"updated": n, "visibility": req.Visibility})
	case errors.Is(err, albumsvc.ErrNotFound):
		Fail(w, http.StatusNotFound, CodeNotFound, "相册不存在")
	case errors.Is(err, albumsvc.ErrInvalidVisibility), errors.Is(err, albumsvc.ErrAlbumForcesPrivate):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
	default:
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
	}
}

// --- album password cookie (mirror image pass) ---

const (
	albumPassCookiePrefix = "imgli_apw_"
	albumPassCookieMaxAge = 7 * 24 * 3600
)

func albumPassCookieName(id uint64) string {
	return albumPassCookiePrefix + strconv.FormatUint(id, 10)
}

func albumPassToken(hash string, id uint64) string {
	mac := hmac.New(sha256.New, []byte(hash))
	_, _ = mac.Write([]byte("v1|album|" + strconv.FormatUint(id, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func albumPasswordOK(r *http.Request, id uint64, hash string) bool {
	if strings.TrimSpace(hash) == "" {
		return true
	}
	want := albumPassToken(hash, id)
	if c, err := r.Cookie(albumPassCookieName(id)); err == nil && c.Value != "" {
		if hmac.Equal([]byte(c.Value), []byte(want)) {
			return true
		}
	}
	// header fallback
	if pw := strings.TrimSpace(r.Header.Get("X-Album-Password")); pw != "" {
		if auth.VerifyPassword(hash, pw) {
			return true
		}
	}
	return false
}

func setAlbumPassCookie(w http.ResponseWriter, r *http.Request, id uint64, hash string) {
	http.SetCookie(w, &http.Cookie{
		Name:     albumPassCookieName(id),
		Value:    albumPassToken(hash, id),
		Path:     "/",
		MaxAge:   albumPassCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		Expires:  time.Now().Add(time.Duration(albumPassCookieMaxAge) * time.Second),
	})
}
