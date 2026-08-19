package adminsvc

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/yixian-huang/imgli/internal/apperr"
	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/mail"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/moderation"
	"github.com/yixian-huang/imgli/internal/service/settings"
	"github.com/yixian-huang/imgli/internal/service/stats"
	"github.com/yixian-huang/imgli/internal/service/upload"
)

// smtpFromRe from 字段：非空时需为形如 local@domain.tld 的邮箱(空值仍允许)。
var smtpFromRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// hotlinkHostRe 防盗链域名(剥掉可选 *. 前缀后的部分):小写字母/数字开头结尾的
// 点分标签,标签内允许连字符——域名字符白名单,兜住黑名单漏网(? 内嵌 * 等)。
var hotlinkHostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

var (
	// ErrUnknownSetting PutSettings 只认已登记 settings 键（含插槽与站长文案）。
	ErrUnknownSetting = apperr.New("未知的设置键")
	// ErrSiteNameInvalid site_name 需 1-64 个字符（TrimSpace 后）。
	ErrSiteNameInvalid = apperr.New("site_name 需 1-64 个字符")
	// ErrRegistrationModeInvalid registration_mode 仅支持 open|invite|closed。
	ErrRegistrationModeInvalid = apperr.New("registration_mode 仅支持 open|invite|closed")
	// ErrModerationInvalid moderation 键的值不是合法 JSON 对象。
	ErrModerationInvalid = apperr.New("moderation 配置格式错误")
	// ErrGuestUploadInvalid guest_upload_enabled 需为布尔值。
	ErrGuestUploadInvalid = apperr.New("guest_upload_enabled 需为布尔值")
	// ErrPlazaEnabledInvalid plaza_enabled 需为布尔值。
	ErrPlazaEnabledInvalid = apperr.New("plaza_enabled 需为布尔值")
	// ErrFaviconURLInvalid favicon_url 须为空或 http(s) URL。
	ErrFaviconURLInvalid = apperr.New("favicon_url 须为空或 http(s) URL")
	// ErrSourceURLInvalid source_url 须为空或 http(s) URL。
	ErrSourceURLInvalid = apperr.New("source_url 须为空或 http(s) URL")
	// ErrOSSCreditInvalid oss_credit 仅 on|off。
	ErrOSSCreditInvalid = apperr.New("oss_credit 仅支持 on|off")
	// ErrAboutBodyInvalid about_body 格式无效。
	ErrAboutBodyInvalid = apperr.New("about_body 格式无效")
	// ErrWelcomeEmailInvalid welcome_email 需为布尔值。
	ErrWelcomeEmailInvalid = apperr.New("welcome_email 需为布尔值")
	// ErrSMTPInvalid smtp JSON 无法解析。字段级错误见下面几条，避免一条文案兜住所有失败。
	ErrSMTPInvalid = apperr.New("smtp 配置无效")
	// ErrSMTPPortInvalid 端口越界。
	ErrSMTPPortInvalid = apperr.New("SMTP 端口需为 1–65535")
	// ErrSMTPEncryptionInvalid encryption 不是 none|starttls|ssl。
	ErrSMTPEncryptionInvalid = apperr.New("加密方式仅支持 无加密 / STARTTLS / SSL")
	// ErrSMTPFromInvalid from 非空且不是邮箱。
	ErrSMTPFromInvalid = apperr.New("发件人需为邮箱地址（如 noreply@example.com），或留空则使用用户名")
	// ErrSMTPNoneWithAuth 明文 + 用户名：net/smtp 会拒发凭据。
	ErrSMTPNoneWithAuth = apperr.New("填了用户名就不能选「无加密」。请改用 STARTTLS 或 SSL")
	// ErrSMTPPasswordReenter 掩码密码回传时改了 host/username：必须重输，避免把旧凭据打到新服务器。
	// 飞书/Lark 公共邮箱用户常先存密码再改用户名为完整邮箱，旧笼统错误会被理解成端口/加密填错。
	ErrSMTPPasswordReenter = apperr.New("改了 SMTP 服务器或用户名，请重新输入密码")
	// ErrHotlinkDomainInvalid 防盗链域名不合法（空/空白/scheme/路径/非法通配）。
	ErrHotlinkDomainInvalid = apperr.New("防盗链域名不合法")
	// theme_accent / theme_bg_* 错误见 theme.go（ErrThemeAccentInvalid 等）。
)

// maskAPIKey 打码 api_key：非空时返回 "****"+尾4字符（长度<=4 时全打码为 "****"），
// 空值原样返回空——GET 响应与 PUT 保留语义（settingWrite 里识别 "****" 前缀）共用此约定。
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

// MaskSecret 供 handler 层给策略 config 里的密钥打码(复用 maskAPIKey:****+尾4)。
func MaskSecret(s string) string { return maskAPIKey(s) }

// GetSettings 返回管理端设置面：{site_name, registration_mode, guest_upload_enabled, plaza_enabled, moderation:{...}, smtp:{...}, hotlink:{...}, processing:{...}}。
// moderation.api_key / moderation.access_key_secret / smtp.password 按 maskAPIKey 打码——明文密钥永不通过本方法对外可见。
// access_key_id 与 region 明文回显。
func (s *Service) GetSettings() (map[string]any, error) {
	st := s.settings()

	var siteName string
	if err := st.Get(model.SettingSiteName, &siteName); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	var regMode string
	if err := st.Get(model.SettingRegistrationMode, &regMode); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	modCfg := moderation.DefaultConfig()
	if err := st.Get(model.SettingModeration, &modCfg); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	moderation.NormalizeConfig(&modCfg)
	var guestUpload bool
	if err := st.Get(model.SettingGuestUpload, &guestUpload); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	var plazaEnabled bool
	if err := st.Get(model.SettingPlazaEnabled, &plazaEnabled); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	smtpCfg := mail.DefaultConfig()
	if err := st.Get(model.SettingSMTP, &smtpCfg); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	smtpCfg.Password = maskAPIKey(smtpCfg.Password)
	hotCfg := stats.DefaultHotlink()
	if err := st.Get(model.SettingHotlink, &hotCfg); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	procCfg := upload.DefaultProcessing()
	if err := st.Get(model.SettingProcessing, &procCfg); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	ann := DefaultAnnouncement()
	if err := st.Get(model.SettingAnnouncement, &ann); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	ann = NormalizeAnnouncement(ann)
	foot := DefaultFooter()
	if err := st.Get(model.SettingFooter, &foot); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	if foot.Groups == nil {
		foot.Groups = []FooterGroup{}
	}
	htmlInj := DefaultHTMLInject()
	if err := st.Get(model.SettingHTMLInject, &htmlInj); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	var helpURL, upgradeURL, shareBrand, faviconURL, sourceURL, ossCredit string
	var themeAccent, themeBgColor, themeBgImageURL string
	var themeBgDim, themeGlass float64
	var regNotice LocaleString
	var aboutEnabled, welcomeEmail bool
	var aboutBody LocaleString
	_ = st.Get(model.SettingHelpURL, &helpURL)
	_ = st.Get(model.SettingUpgradeURL, &upgradeURL)
	_ = st.Get(model.SettingRegisterNotice, &regNotice)
	_ = st.Get(model.SettingShareBranding, &shareBrand)
	_ = st.Get(model.SettingFaviconURL, &faviconURL)
	_ = st.Get(model.SettingSourceURL, &sourceURL)
	_ = st.Get(model.SettingOSSCredit, &ossCredit)
	_ = st.Get(model.SettingAboutEnabled, &aboutEnabled)
	_ = st.Get(model.SettingAboutBody, &aboutBody)
	welcomeEmail = true
	_ = st.Get(model.SettingWelcomeEmail, &welcomeEmail)
	mailTpl := mail.DefaultTemplates()
	if err := st.Get(model.SettingMailTemplates, &mailTpl); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return nil, err
	}
	mailTpl = mailTpl.Normalize()
	_ = st.Get(model.SettingThemeAccent, &themeAccent)
	_ = st.Get(model.SettingThemeBgColor, &themeBgColor)
	_ = st.Get(model.SettingThemeBgImageURL, &themeBgImageURL)
	themeBgDim = DefaultThemeBgDim
	_ = st.Get(model.SettingThemeBgDim, &themeBgDim)
	themeGlass = DefaultThemeGlass
	_ = st.Get(model.SettingThemeGlass, &themeGlass)
	helpURL = NormalizeOptionalURL(helpURL)
	upgradeURL = NormalizeOptionalURL(upgradeURL)
	faviconURL = NormalizeOptionalURL(faviconURL)
	sourceURL = NormalizeOptionalURL(sourceURL)
	themeAccent = NormalizeThemeAccent(themeAccent)
	themeBgColor = NormalizeThemeBgColor(themeBgColor)
	themeBgImageURL = NormalizeOptionalURL(themeBgImageURL)
	if err := ValidateThemeBgDim(themeBgDim); err != nil {
		themeBgDim = DefaultThemeBgDim
	}
	if err := ValidateThemeGlass(themeGlass); err != nil {
		themeGlass = DefaultThemeGlass
	}
	pubStats := DefaultPublicStats()
	_ = st.Get(model.SettingPublicStats, &pubStats)
	pubStats = NormalizePublicStats(pubStats)
	regNotice = regNotice.Normalize()
	aboutBody = aboutBody.Normalize()
	shareBrand = NormalizeShareBranding(shareBrand)
	ossCredit = NormalizeOSSCredit(ossCredit)

	return map[string]any{
		"site_name":            siteName,
		"registration_mode":    regMode,
		"guest_upload_enabled": guestUpload,
		"plaza_enabled":        plazaEnabled,
		"moderation": map[string]any{
			"enabled":           modCfg.Enabled,
			"provider":          modCfg.Provider,
			"endpoint":          modCfg.Endpoint,
			"api_key":           maskAPIKey(modCfg.APIKey),
			"threshold":         modCfg.Threshold,
			"action":            modCfg.Action,
			"access_key_id":     modCfg.AccessKeyID,
			"access_key_secret": maskAPIKey(modCfg.AccessKeySecret),
			"region":            modCfg.Region,
			"login_sample_rate": modCfg.LoginSampleRate,
			"on_plugin_error":   modCfg.OnPluginError,
			"notify_on_reject":  modCfg.NotifyOnReject,
			"ocr_keywords": map[string]any{
				"enabled":  modCfg.OCRKeywords.Enabled,
				"endpoint": modCfg.OCRKeywords.Endpoint,
				"api_key":  maskAPIKey(modCfg.OCRKeywords.APIKey),
				"keywords": modCfg.OCRKeywords.Keywords,
				"on_hit":   modCfg.OCRKeywords.OnHit,
			},
		},
		"smtp": map[string]any{
			"host":       smtpCfg.Host,
			"port":       smtpCfg.Port,
			"username":   smtpCfg.Username,
			"password":   smtpCfg.Password,
			"from":       smtpCfg.From,
			"encryption": smtpCfg.Encryption,
		},
		"hotlink":    hotCfg,
		"processing": procCfg,
		// 构建能力：前端据此禁用「转 WebP」等仅 vips 可用的选项。
		"processing_capabilities": map[string]any{
			"webp_encode": imaging.WebPEncodeAvailable(),
		},
		"announcement":           ann,
		"footer":                 foot,
		"html_inject":            htmlInj,
		"help_url":               helpURL,
		"upgrade_url":            upgradeURL,
		"register_notice":        regNotice,
		"share_branding":         shareBrand,
		"favicon_url":            faviconURL,
		"source_url":             sourceURL,
		"oss_credit":             ossCredit,
		"about_enabled":          aboutEnabled,
		"about_body":             aboutBody,
		"welcome_email":          welcomeEmail,
		"mail_templates":         mailTpl,
		"mail_template_defaults": mail.BuiltinDefaults(),
		"theme_accent":           themeAccent,
		"theme_bg_color":         themeBgColor,
		"theme_bg_image_url":     themeBgImageURL,
		"theme_bg_dim":           themeBgDim,
		"theme_glass":            themeGlass,
		"public_stats":           pubStats,
	}, nil
}

// settingWrite 是校验通过、待落库的单个键值对；PutSettings 先把全部键校验完（收集到
// 一批 settingWrite），全部通过后才真正写库——任一键校验失败，整个请求不落任何键
// （契约「逐键校验，任一键失败整个请求 400 不落库」）。
type settingWrite struct {
	key   string
	value any
}

// PutSettings 部分更新设置面。patch 只认已登记 settings 键（含 help_url/upgrade_url/register_notice/share_branding），
// 未知键返回 ErrUnknownSetting。moderation 按整对象校验（moderation.ValidateConfig）；
// 其 api_key / access_key_secret 若以 "****" 开头，视为前端把打码后的展示值原样回传：
// api_key 仅当 provider 与 endpoint 均未变才沿用库中明文；access_key_secret 仅当
// provider/region/access_key_id 均未变才沿用——改指向即失效，返回 ErrModerationInvalid。
// smtp.password 同样支持 "****" 前缀保留语义（host+username 未变）。
// hotlink 经 normalizeHotlink 规整（小写/去重/域名形态校验）。
// processing 经 upload.ValidateProcessing 校验（坏 JSON 与校验失败均返 upload.ErrProcessingInvalid）。
func (s *Service) PutSettings(patch map[string]json.RawMessage) error {
	// 必须用共享 settings 实例：Set 内 Invalidate 才能打掉 DiscoverHandler 的 plaza 缓存
	st := s.settings()
	writes := make([]settingWrite, 0, len(patch))

	for key, raw := range patch {
		switch key {
		case model.SettingSiteName:
			var name string
			if err := json.Unmarshal(raw, &name); err != nil {
				return ErrSiteNameInvalid
			}
			name = strings.TrimSpace(name)
			if name == "" || len(name) > 64 {
				return ErrSiteNameInvalid
			}
			writes = append(writes, settingWrite{model.SettingSiteName, name})

		case model.SettingRegistrationMode:
			var mode string
			if err := json.Unmarshal(raw, &mode); err != nil {
				return ErrRegistrationModeInvalid
			}
			switch mode {
			case "open", "invite", "closed":
			default:
				return ErrRegistrationModeInvalid
			}
			writes = append(writes, settingWrite{model.SettingRegistrationMode, mode})

		case model.SettingGuestUpload:
			var enabled bool
			if err := json.Unmarshal(raw, &enabled); err != nil {
				return ErrGuestUploadInvalid
			}
			writes = append(writes, settingWrite{model.SettingGuestUpload, enabled})

		case model.SettingPlazaEnabled:
			var enabled bool
			if err := json.Unmarshal(raw, &enabled); err != nil {
				return ErrPlazaEnabledInvalid
			}
			writes = append(writes, settingWrite{model.SettingPlazaEnabled, enabled})

		case model.SettingModeration:
			cfg := moderation.DefaultConfig()
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return ErrModerationInvalid
			}
			moderation.NormalizeConfig(&cfg)
			if strings.HasPrefix(cfg.APIKey, "****") || strings.HasPrefix(cfg.AccessKeySecret, "****") ||
				strings.HasPrefix(cfg.OCRKeywords.APIKey, "****") {
				var cur moderation.Config
				if err := st.Get(model.SettingModeration, &cur); err != nil && !errors.Is(err, settings.ErrNotFound) {
					return err
				}
				if strings.HasPrefix(cfg.APIKey, "****") {
					// 改指向即失效:provider 或 endpoint 变了不得沿用旧 key(收 C-②b 债)
					if cfg.Provider != cur.Provider || cfg.Endpoint != cur.Endpoint {
						return ErrModerationInvalid
					}
					cfg.APIKey = cur.APIKey
				}
				if strings.HasPrefix(cfg.AccessKeySecret, "****") {
					if cfg.Provider != cur.Provider || cfg.Region != cur.Region || cfg.AccessKeyID != cur.AccessKeyID {
						return ErrModerationInvalid
					}
					cfg.AccessKeySecret = cur.AccessKeySecret
				}
				if strings.HasPrefix(cfg.OCRKeywords.APIKey, "****") {
					if cfg.OCRKeywords.Endpoint != cur.OCRKeywords.Endpoint {
						return ErrModerationInvalid
					}
					cfg.OCRKeywords.APIKey = cur.OCRKeywords.APIKey
				}
			}
			if err := moderation.ValidateConfig(cfg); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingModeration, cfg})

		case model.SettingSMTP:
			var cfg mail.Config
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return ErrSMTPInvalid
			}
			prepared, err := s.PrepareSMTP(cfg)
			if err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingSMTP, prepared})

		case model.SettingHotlink:
			var cfg stats.HotlinkConfig
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return ErrHotlinkDomainInvalid
			}
			norm, err := normalizeHotlink(cfg)
			if err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingHotlink, norm})

		case model.SettingProcessing:
			var cfg upload.Processing
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return upload.ErrProcessingInvalid
			}
			if err := upload.ValidateProcessing(cfg); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingProcessing, cfg})

		case model.SettingAnnouncement:
			var cfg Announcement
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return ErrAnnouncementInvalid
			}
			cfg = NormalizeAnnouncement(cfg)
			if err := ValidateAnnouncement(cfg); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingAnnouncement, cfg})

		case model.SettingFooter:
			var cfg Footer
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return ErrFooterInvalid
			}
			if cfg.Groups == nil {
				cfg.Groups = []FooterGroup{}
			}
			if err := ValidateFooter(cfg); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingFooter, cfg})

		case model.SettingHTMLInject:
			var cfg HTMLInject
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return ErrHTMLInjectInvalid
			}
			if err := ValidateHTMLInject(cfg); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingHTMLInject, cfg})

		case model.SettingHelpURL:
			var u string
			if err := json.Unmarshal(raw, &u); err != nil {
				return ErrHelpURLInvalid
			}
			u = NormalizeOptionalURL(u)
			if err := ValidateOptionalURL(u); err != nil {
				return ErrHelpURLInvalid
			}
			writes = append(writes, settingWrite{model.SettingHelpURL, u})

		case model.SettingUpgradeURL:
			var u string
			if err := json.Unmarshal(raw, &u); err != nil {
				return ErrUpgradeURLInvalid
			}
			u = NormalizeOptionalURL(u)
			if err := ValidateOptionalURL(u); err != nil {
				return ErrUpgradeURLInvalid
			}
			writes = append(writes, settingWrite{model.SettingUpgradeURL, u})

		case model.SettingRegisterNotice:
			var n LocaleString
			if err := json.Unmarshal(raw, &n); err != nil {
				return ErrRegisterNoticeInvalid
			}
			n = n.Normalize()
			if err := ValidateRegisterNotice(n); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingRegisterNotice, n})

		case model.SettingShareBranding:
			var mode string
			if err := json.Unmarshal(raw, &mode); err != nil {
				return ErrShareBrandingInvalid
			}
			if err := ValidateShareBranding(mode); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingShareBranding, NormalizeShareBranding(mode)})

		case model.SettingFaviconURL:
			var u string
			if err := json.Unmarshal(raw, &u); err != nil {
				return ErrFaviconURLInvalid
			}
			u = NormalizeOptionalURL(u)
			if err := ValidateOptionalURL(u); err != nil {
				return ErrFaviconURLInvalid
			}
			writes = append(writes, settingWrite{model.SettingFaviconURL, u})

		case model.SettingSourceURL:
			var u string
			if err := json.Unmarshal(raw, &u); err != nil {
				return ErrSourceURLInvalid
			}
			u = NormalizeOptionalURL(u)
			if err := ValidateOptionalURL(u); err != nil {
				return ErrSourceURLInvalid
			}
			writes = append(writes, settingWrite{model.SettingSourceURL, u})

		case model.SettingOSSCredit:
			var mode string
			if err := json.Unmarshal(raw, &mode); err != nil {
				return ErrOSSCreditInvalid
			}
			mode = NormalizeOSSCredit(mode)
			if mode != "on" && mode != "off" {
				return ErrOSSCreditInvalid
			}
			writes = append(writes, settingWrite{model.SettingOSSCredit, mode})

		case model.SettingAboutEnabled:
			var enabled bool
			if err := json.Unmarshal(raw, &enabled); err != nil {
				return ErrAboutBodyInvalid
			}
			writes = append(writes, settingWrite{model.SettingAboutEnabled, enabled})

		case model.SettingAboutBody:
			var n LocaleString
			if err := json.Unmarshal(raw, &n); err != nil {
				return ErrAboutBodyInvalid
			}
			n = n.Normalize()
			if n.MaxRunes() > 4000 {
				return ErrAboutBodyInvalid
			}
			writes = append(writes, settingWrite{model.SettingAboutBody, n})

		case model.SettingWelcomeEmail:
			var enabled bool
			if err := json.Unmarshal(raw, &enabled); err != nil {
				return ErrWelcomeEmailInvalid
			}
			writes = append(writes, settingWrite{model.SettingWelcomeEmail, enabled})

		case model.SettingMailTemplates:
			var tpl mail.Templates
			if err := json.Unmarshal(raw, &tpl); err != nil {
				return mail.ErrCopyInvalid
			}
			tpl = tpl.Normalize()
			if err := mail.ValidateTemplates(tpl); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingMailTemplates, tpl})

		case model.SettingThemeAccent:
			var a string
			if err := json.Unmarshal(raw, &a); err != nil {
				return ErrThemeAccentInvalid
			}
			if err := ValidateThemeAccent(a); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingThemeAccent, NormalizeThemeAccent(a)})

		case model.SettingThemeBgColor:
			var c string
			if err := json.Unmarshal(raw, &c); err != nil {
				return ErrThemeBgColorInvalid
			}
			if err := ValidateThemeBgColor(c); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingThemeBgColor, NormalizeThemeBgColor(c)})

		case model.SettingThemeBgImageURL:
			var u string
			if err := json.Unmarshal(raw, &u); err != nil {
				return ErrThemeBgImageURLInvalid
			}
			u = NormalizeOptionalURL(u)
			if err := ValidateOptionalURL(u); err != nil {
				return ErrThemeBgImageURLInvalid
			}
			writes = append(writes, settingWrite{model.SettingThemeBgImageURL, u})

		case model.SettingThemeBgDim:
			var d float64
			if err := json.Unmarshal(raw, &d); err != nil {
				return ErrThemeBgDimInvalid
			}
			if err := ValidateThemeBgDim(d); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingThemeBgDim, d})

		case model.SettingThemeGlass:
			var g float64
			if err := json.Unmarshal(raw, &g); err != nil {
				return ErrThemeGlassInvalid
			}
			if err := ValidateThemeGlass(g); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingThemeGlass, g})

		case model.SettingPublicStats:
			var cfg PublicStatsConfig
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return ErrPublicStatsInvalid
			}
			cfg = NormalizePublicStats(cfg)
			if err := ValidatePublicStats(cfg); err != nil {
				return err
			}
			writes = append(writes, settingWrite{model.SettingPublicStats, cfg})

		default:
			return ErrUnknownSetting
		}
	}

	for _, w := range writes {
		if err := st.Set(w.key, w.value); err != nil {
			return err
		}
	}
	// 公开统计缓存：配置变更后立即失效
	for _, w := range writes {
		if w.key == model.SettingPublicStats {
			InvalidatePublicStatsCache()
			break
		}
	}
	return nil
}

// PrepareSMTP 校验并规整 SMTP 配置（trim host/username/from；掩码密码按身份解析）。
// 保存与「用表单值测发信」共用，避免测发信绕过改指向不得沿用旧密码的纪律。
func (s *Service) PrepareSMTP(cfg mail.Config) (mail.Config, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.From = strings.TrimSpace(cfg.From)
	if cfg.Port < 1 || cfg.Port > 65535 {
		return cfg, ErrSMTPPortInvalid
	}
	switch cfg.Encryption {
	case "none", "starttls", "ssl":
	default:
		return cfg, ErrSMTPEncryptionInvalid
	}
	// net/smtp PlainAuth / LOGIN 均拒绝非 TLS 连接——none+认证必然发送失败,保存时拒绝。
	if cfg.Encryption == "none" && cfg.Username != "" {
		return cfg, ErrSMTPNoneWithAuth
	}
	if cfg.From != "" && !smtpFromRe.MatchString(cfg.From) {
		return cfg, ErrSMTPFromInvalid
	}
	if strings.HasPrefix(cfg.Password, "****") {
		var cur mail.Config
		if err := s.settings().Get(model.SettingSMTP, &cur); err != nil && !errors.Is(err, settings.ErrNotFound) {
			return cfg, err
		}
		// 防 admin 改指向受控服务器窃取旧凭据:仅 host+username 均未变才保留密码。
		if cfg.Host != cur.Host || cfg.Username != cur.Username {
			return cfg, ErrSMTPPasswordReenter
		}
		cfg.Password = cur.Password
	}
	return cfg, nil
}

// normalizeHotlink 校验并规整防盗链配置:域名逐项 TrimSpace 后须非空、无内部空白、
// 不含 scheme/路径字符(/:);通配仅允许 "*." 前缀且剥掉后仍含 "." 且非空;
// 全部小写化并按序去重。违规返回 ErrHotlinkDomainInvalid。
func normalizeHotlink(cfg stats.HotlinkConfig) (stats.HotlinkConfig, error) {
	out := make([]string, 0, len(cfg.AllowedDomains))
	seen := map[string]bool{}
	for _, d := range cfg.AllowedDomains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			return cfg, ErrHotlinkDomainInvalid
		}
		host := d
		if strings.HasPrefix(d, "*.") {
			host = d[2:]
			if host == "" || !strings.Contains(host, ".") {
				return cfg, ErrHotlinkDomainInvalid
			}
		}
		// 剥掉合法 *. 前缀后不许再出现星号(拒 foo.*.example);字符集白名单拒
		// 空白/scheme/路径/问号等一切非域名字符(codex 终审:此前黑名单漏 ? 与内嵌 *)。
		if !hotlinkHostRe.MatchString(host) {
			return cfg, ErrHotlinkDomainInvalid
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	cfg.AllowedDomains = out
	return cfg, nil
}

// NormalizeOSSCredit oss_credit: empty → on（默认展示「基于 imgli」）。
func NormalizeOSSCredit(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "off", "0", "false":
		return "off"
	default:
		return "on"
	}
}
