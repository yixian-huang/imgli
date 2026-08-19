package mail

import (
	"html"
	"html/template"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yixian-huang/imgli/internal/apperr"
)

// Kind 可覆盖文案的邮件种类。版式（壳/按钮/可复制链接）内置，不开放 HTML。
type Kind string

const (
	KindWelcome     Kind = "welcome"
	KindVerify      Kind = "verify"
	KindReset       Kind = "reset"
	KindChangeEmail Kind = "change_email"
	KindReject      Kind = "reject"
)

func ParseKind(s string) (Kind, bool) {
	switch Kind(s) {
	case KindWelcome, KindVerify, KindReset, KindChangeEmail, KindReject:
		return Kind(s), true
	default:
		return "", false
	}
}

// LocaleText 站长文案的中英对照。空串表示该语种走内置。
type LocaleText struct {
	ZH string `json:"zh"`
	EN string `json:"en"`
}

// KindCopy 一封信的主题+正文覆盖。
type KindCopy struct {
	Subject LocaleText `json:"subject"`
	Body    LocaleText `json:"body"`
}

// Templates settings `mail_templates` 形状。全空 = 全部走内置。
type Templates struct {
	Welcome     KindCopy `json:"welcome"`
	Verify      KindCopy `json:"verify"`
	Reset       KindCopy `json:"reset"`
	ChangeEmail KindCopy `json:"change_email"`
	Reject      KindCopy `json:"reject"`
}

func DefaultTemplates() Templates { return Templates{} }

func (t Templates) Copy(k Kind) KindCopy {
	switch k {
	case KindWelcome:
		return t.Welcome
	case KindVerify:
		return t.Verify
	case KindReset:
		return t.Reset
	case KindChangeEmail:
		return t.ChangeEmail
	case KindReject:
		return t.Reject
	default:
		return KindCopy{}
	}
}

func (t *Templates) SetCopy(k Kind, c KindCopy) {
	switch k {
	case KindWelcome:
		t.Welcome = c
	case KindVerify:
		t.Verify = c
	case KindReset:
		t.Reset = c
	case KindChangeEmail:
		t.ChangeEmail = c
	case KindReject:
		t.Reject = c
	}
}

func (c KindCopy) Normalize() KindCopy {
	trim := func(l LocaleText) LocaleText {
		return LocaleText{ZH: strings.TrimSpace(l.ZH), EN: strings.TrimSpace(l.EN)}
	}
	return KindCopy{Subject: trim(c.Subject), Body: trim(c.Body)}
}

func (t Templates) Normalize() Templates {
	return Templates{
		Welcome:     t.Welcome.Normalize(),
		Verify:      t.Verify.Normalize(),
		Reset:       t.Reset.Normalize(),
		ChangeEmail: t.ChangeEmail.Normalize(),
		Reject:      t.Reject.Normalize(),
	}
}

const (
	maxSubjectRunes = 120
	maxBodyRunes    = 2000
)

var (
	ErrCopyTooLong            = apperr.New("邮件文案过长：主题最多 120 字，正文最多 2000 字")
	ErrCopyUnknownPlaceholder = apperr.New("邮件文案含未知占位符。可用 {{site_name}} {{link}} {{image_name}} {{image_key}}")
	ErrCopyKindInvalid        = apperr.New("未知的邮件种类")
	ErrCopyInvalid            = apperr.New("邮件文案格式无效")
	placeholderRe             = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)
	allowedPlaceholders       = map[string]bool{"site_name": true, "link": true, "image_name": true, "image_key": true}
)

func ValidateTemplates(t Templates) error {
	for _, k := range []Kind{KindWelcome, KindVerify, KindReset, KindChangeEmail, KindReject} {
		if err := ValidateCopy(k, t.Copy(k)); err != nil {
			return err
		}
	}
	return nil
}

func ValidateCopy(_ Kind, c KindCopy) error {
	c = c.Normalize()
	for _, s := range []string{c.Subject.ZH, c.Subject.EN} {
		if utf8.RuneCountInString(s) > maxSubjectRunes {
			return ErrCopyTooLong
		}
		if err := checkPlaceholders(s); err != nil {
			return err
		}
	}
	for _, s := range []string{c.Body.ZH, c.Body.EN} {
		if utf8.RuneCountInString(s) > maxBodyRunes {
			return ErrCopyTooLong
		}
		if err := checkPlaceholders(s); err != nil {
			return err
		}
	}
	return nil
}

func checkPlaceholders(s string) error {
	for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
		if !allowedPlaceholders[m[1]] {
			return ErrCopyUnknownPlaceholder
		}
	}
	return nil
}

// Vars 渲染时替换占位符。
type Vars struct {
	SiteName  string
	Link      string
	ImageName string
	ImageKey  string
}

func expand(s string, v Vars) string {
	return placeholderRe.ReplaceAllStringFunc(s, func(raw string) string {
		name := placeholderRe.FindStringSubmatch(raw)
		if len(name) < 2 {
			return raw
		}
		switch name[1] {
		case "site_name":
			return v.SiteName
		case "link":
			return v.Link
		case "image_name":
			return v.ImageName
		case "image_key":
			return v.ImageKey
		default:
			return raw
		}
	})
}

func pickLocale(lang, zh, en string) string {
	if isEN(lang) {
		return en
	}
	return zh
}

func merge(over KindCopy, lang, defSub, defBody string) (subject, body string) {
	over = over.Normalize()
	sub, bod := over.Subject.ZH, over.Body.ZH
	if isEN(lang) {
		sub, bod = over.Subject.EN, over.Body.EN
	}
	if sub == "" {
		sub = defSub
	}
	if bod == "" {
		bod = defBody
	}
	return sub, bod
}

func imageLabel(key, name string) string {
	if name != "" {
		return name + " (" + key + ")"
	}
	return key
}

// BuiltinDefaults 给管理端「填入默认」：带占位符、未展开站名。
func BuiltinDefaults() Templates {
	return Templates{
		Welcome: KindCopy{
			Subject: LocaleText{ZH: "欢迎使用 {{site_name}}", EN: "Welcome to {{site_name}}"},
			Body: LocaleText{
				ZH: "欢迎使用 {{site_name}}。请到「设置」创建 API Token，即可对接 PicGo / ShareX / imgli CLI。需要长期稳定请自托管（见文档与 GitHub Release）。公共试用实例可能有存储与月流量限额。",
				EN: "Welcome to {{site_name}}. Create an API token under Settings to use PicGo, ShareX, or the imgli CLI. Self-hosting: see the project docs and GitHub releases. Public trial instances may enforce storage and bandwidth caps.",
			},
		},
		Verify: KindCopy{
			Subject: LocaleText{ZH: "验证你的 {{site_name}} 邮箱", EN: "Verify your {{site_name}} email"},
			Body: LocaleText{
				ZH: "感谢注册!点击下方按钮完成邮箱验证。链接 24 小时内有效。",
				EN: "Thanks for signing up! Click the button below to verify your email. This link is valid for 24 hours.",
			},
		},
		Reset: KindCopy{
			Subject: LocaleText{ZH: "重置你的 {{site_name}} 密码", EN: "Reset your {{site_name}} password"},
			Body: LocaleText{
				ZH: "我们收到了你的密码重置请求,点击下方按钮设置新密码。链接 1 小时内有效。",
				EN: "We received a request to reset your password. Click the button below to set a new password. This link is valid for 1 hour.",
			},
		},
		ChangeEmail: KindCopy{
			Subject: LocaleText{ZH: "确认你的 {{site_name}} 新邮箱", EN: "Confirm your new {{site_name}} email"},
			Body: LocaleText{
				ZH: "点击下方按钮确认将此邮箱绑定到你的账号。链接 1 小时内有效。",
				EN: "Click the button below to confirm this email address for your account. This link is valid for 1 hour.",
			},
		},
		Reject: KindCopy{
			Subject: LocaleText{ZH: "[{{site_name}}] 图片审核通知", EN: "[{{site_name}}] Image review update"},
			Body: LocaleText{
				ZH: "你上传的图片未通过审核，已不可公开访问：{{image_name}}。",
				EN: "An image you uploaded did not pass review and is no longer publicly available: {{image_name}}.",
			},
		},
	}
}

func chrome(kind Kind, lang string) (button, hint, ignore string) {
	if isEN(lang) {
		switch kind {
		case KindWelcome:
			return "Open settings", "If the button does not work, copy this link", "This is an automated message."
		case KindVerify:
			return "Verify email", "If the button does not work, copy this link into your browser", "If you did not request this, you can ignore this email."
		case KindReset:
			return "Reset password", "If the button does not work, copy this link into your browser", "If you did not request this, you can ignore this email."
		case KindChangeEmail:
			return "Confirm email", "If the button does not work, copy this link into your browser", "If you did not request this, you can ignore this email."
		default:
			return "Open site", "You can sign in to manage your library", "This is an automated message."
		}
	}
	switch kind {
	case KindWelcome:
		return "打开设置", "若按钮无法点击，复制链接到浏览器", "本邮件由系统自动发送。"
	case KindVerify:
		return "验证邮箱", "若按钮无法点击,复制链接到浏览器打开", "若这不是你发起的操作,忽略本邮件即可。"
	case KindReset:
		return "重置密码", "若按钮无法点击,复制链接到浏览器打开", "若这不是你发起的操作,忽略本邮件即可。"
	case KindChangeEmail:
		return "确认邮箱", "若按钮无法点击,复制链接到浏览器打开", "若这不是你发起的操作,忽略本邮件即可。"
	default:
		return "打开站点", "可登录后管理图库", "本邮件由系统自动发送。"
	}
}

// 极简品牌邮件壳:站点名头 + 正文 + 按钮链接。版式内置；主题/正文可由 mail_templates 覆盖。
const shellTpl = `<!doctype html><html><body style="margin:0;padding:32px;background:#f5f5f4;font-family:sans-serif">
<div style="max-width:480px;margin:0 auto;background:#fff;border:1px solid #e5e5e3;padding:32px">
<div style="font-weight:800;font-size:18px;margin-bottom:20px">{{.SiteName}}</div>
<p style="font-size:14px;line-height:1.7;color:#333">{{.Body}}</p>
<p style="margin:28px 0"><a href="{{.Link}}" style="background:{{.Accent}};color:#fff;padding:10px 22px;text-decoration:none;font-size:14px">{{.Button}}</a></p>
<p style="font-size:12px;color:#999">{{.LinkHint}}:<br>{{.Link}}</p>
<p style="font-size:12px;color:#999">{{.Ignore}}</p>
</div></body></html>`

var shell = template.Must(template.New("shell").Parse(shellTpl))

type shellData struct {
	SiteName, Link, Button, LinkHint, Ignore, Accent string
	Body                                             template.HTML
}

func formatBody(body string) template.HTML {
	esc := html.EscapeString(body)
	esc = strings.ReplaceAll(esc, "\n", "<br>")
	return template.HTML(esc)
}

func render(siteName, body, link, button, linkHint, ignore, accent string) string {
	if accent == "" {
		accent = "#111111"
	}
	if link == "" {
		link = "#"
	}
	var b strings.Builder
	_ = shell.Execute(&b, shellData{
		SiteName: siteName, Body: formatBody(body), Link: link,
		Button: button, LinkHint: linkHint, Ignore: ignore, Accent: accent,
	})
	return b.String()
}

func isEN(lang string) bool { return lang == "en" }

// Render 统一渲染：覆盖为空则用内置；{{placeholders}} 展开后套壳。按钮/链接条始终内置。
func Render(kind Kind, lang string, vars Vars, over KindCopy, accent string) (subject, html string, err error) {
	if _, ok := ParseKind(string(kind)); !ok {
		return "", "", ErrCopyKindInvalid
	}
	def := BuiltinDefaults().Copy(kind)
	defSub := pickLocale(lang, def.Subject.ZH, def.Subject.EN)
	defBody := pickLocale(lang, def.Body.ZH, def.Body.EN)
	sub, body := merge(over, lang, defSub, defBody)
	sub, body = expand(sub, vars), expand(body, vars)
	btn, hint, ign := chrome(kind, lang)
	return sub, render(vars.SiteName, body, vars.Link, btn, hint, ign, accent), nil
}

func SampleVars(siteName, baseURL string) Vars {
	link := strings.TrimRight(baseURL, "/")
	if link == "" {
		link = "https://example.com"
	}
	return Vars{
		SiteName:  siteName,
		Link:      link + "/settings",
		ImageName: "photo.jpg (xxxxxxxxxxxx)",
		ImageKey:  "xxxxxxxxxxxx",
	}
}

func welcomeLink(baseURL string) string {
	if baseURL == "" {
		return "#"
	}
	return strings.TrimRight(baseURL, "/") + "/settings"
}

func siteLink(baseURL string) string {
	if baseURL == "" {
		return "#"
	}
	return strings.TrimRight(baseURL, "/")
}

// RenderResetPassword 重置密码邮件(链接 1 小时有效)。
func RenderResetPassword(siteName, link, lang string, over KindCopy) (subject, html string) {
	sub, h, _ := Render(KindReset, lang, Vars{SiteName: siteName, Link: link}, over, "")
	return sub, h
}

// RenderImageRejected 内容审核未通过通知（克制文案，不写违规细节）。
func RenderImageRejected(siteName, imageKey, imageName, lang string, over KindCopy) (subject, html string) {
	label := imageLabel(imageKey, imageName)
	sub, h, _ := Render(KindReject, lang, Vars{
		SiteName: siteName, Link: "#", ImageName: label, ImageKey: imageKey,
	}, over, "")
	return sub, h
}

// RenderImageRejectedAt 同上，按钮指向站点首页。
func RenderImageRejectedAt(siteName, imageKey, imageName, lang, baseURL string, over KindCopy) (subject, html string) {
	label := imageLabel(imageKey, imageName)
	sub, h, _ := Render(KindReject, lang, Vars{
		SiteName: siteName, Link: siteLink(baseURL), ImageName: label, ImageKey: imageKey,
	}, over, "")
	return sub, h
}

// RenderChangeEmail 换绑邮箱确认（链接 1 小时有效）。
func RenderChangeEmail(siteName, link, lang string, over KindCopy) (subject, html string) {
	sub, h, _ := Render(KindChangeEmail, lang, Vars{SiteName: siteName, Link: link}, over, "")
	return sub, h
}

// RenderVerifyEmail 邮箱验证邮件(链接 24 小时有效)。
func RenderVerifyEmail(siteName, link, lang string, over KindCopy) (subject, html string) {
	sub, h, _ := Render(KindVerify, lang, Vars{SiteName: siteName, Link: link}, over, "")
	return sub, h
}

// RenderWelcome 注册欢迎邮件。
func RenderWelcome(siteName, baseURL, lang string, over KindCopy) (subject, html string) {
	sub, h, _ := Render(KindWelcome, lang, Vars{SiteName: siteName, Link: welcomeLink(baseURL)}, over, "")
	return sub, h
}
