// Package mail SMTP 邮件发送。配置存 settings(每次发送时读,改配置免重启);
// host 空视为未配置(ErrNotConfigured)。发送经 sender 函数,测试可注入桩。
package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/settings"
)

// Config settings `smtp` 键形状(与 admin API/前端 JSON 逐字一致)。
type Config struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	From       string `json:"from"`
	Encryption string `json:"encryption"` // none | starttls | ssl
}

// DefaultConfig 播种/缺省值(host 空=未配置)。
func DefaultConfig() Config { return Config{Port: 587, Encryption: "starttls"} }

var ErrNotConfigured = errors.New("mail: SMTP 未配置")

const dialTimeout = 10 * time.Second

type Service struct {
	db      *gorm.DB
	sender  func(cfg Config, to string, msg []byte) error // 测试注入点
	BaseURL string                                        // 欢迎/拒绝预览按钮；server 注入
}

func New(db *gorm.DB) *Service { return &Service{db: db, sender: smtpSend} }

func (s *Service) templates() Templates {
	t := DefaultTemplates()
	if err := settings.New(s.db).Get(model.SettingMailTemplates, &t); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return DefaultTemplates()
	}
	return t
}

func (s *Service) accent() string {
	var a string
	if err := settings.New(s.db).Get(model.SettingThemeAccent, &a); err != nil {
		return ""
	}
	return strings.TrimSpace(a)
}

func (s *Service) config() (Config, error) {
	cfg := DefaultConfig()
	if err := settings.New(s.db).Get(model.SettingSMTP, &cfg); err != nil && !errors.Is(err, settings.ErrNotFound) {
		return cfg, err
	}
	return cfg, nil
}

func (s *Service) siteName() string {
	name := "img.li"
	var v string
	if err := settings.New(s.db).Get(model.SettingSiteName, &v); err == nil && v != "" {
		name = v
	}
	return name
}

// Send 发送一封 HTML 邮件;host 未配置返回 ErrNotConfigured。
func (s *Service) Send(to, subject, htmlBody string) error {
	cfg, err := s.config()
	if err != nil {
		return err
	}
	return s.SendWithConfig(cfg, to, subject, htmlBody)
}

// SendWithConfig 用调用方给出的配置发信（测试邮件可走未保存的表单值）。
// host 空返回 ErrNotConfigured；from 空则回退到 username（飞书公共邮箱常只填用户名）。
func (s *Service) SendWithConfig(cfg Config, to, subject, htmlBody string) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return ErrNotConfigured
	}
	from := effectiveFrom(cfg)
	cfg.From = from
	return s.sender(cfg, to, buildMessage(from, to, subject, htmlBody))
}

func effectiveFrom(cfg Config) string {
	if f := strings.TrimSpace(cfg.From); f != "" {
		return f
	}
	return strings.TrimSpace(cfg.Username)
}

// SendResetPassword 渲染重置密码模板并发送(auth.Mailer 实现)。lang 透传模板。
func (s *Service) SendResetPassword(to, link, lang string) error {
	sub, html := RenderResetPassword(s.siteName(), link, lang, s.templates().Reset)
	return s.Send(to, sub, html)
}

// SendVerifyEmail 渲染邮箱验证模板并发送(auth.Mailer 实现)。lang 透传模板。
func (s *Service) SendVerifyEmail(to, link, lang string) error {
	sub, html := RenderVerifyEmail(s.siteName(), link, lang, s.templates().Verify)
	return s.Send(to, sub, html)
}

// SendChangeEmail 换绑邮箱确认(auth.Mailer 实现)。
func (s *Service) SendChangeEmail(to, link, lang string) error {
	sub, html := RenderChangeEmail(s.siteName(), link, lang, s.templates().ChangeEmail)
	return s.Send(to, sub, html)
}

// SendWelcome 注册欢迎信。baseURL 用于拼设置页链接；SMTP 未配返回 ErrNotConfigured。
func (s *Service) SendWelcome(to, baseURL, lang string) error {
	sub, html := RenderWelcome(s.siteName(), baseURL, lang, s.templates().Welcome)
	return s.Send(to, sub, html)
}

// SendImageRejected 审核拒绝通知。
func (s *Service) SendImageRejected(to, imageKey, imageName, lang string) error {
	sub, html := RenderImageRejectedAt(s.siteName(), imageKey, imageName, lang, s.BaseURL, s.templates().Reject)
	return s.Send(to, sub, html)
}

// Preview 渲染一封信，不发送。over 非空则用其文案（表单草稿）；否则用已存 mail_templates。
func (s *Service) Preview(kind Kind, lang string, over *Templates) (subject, html string, err error) {
	tpl := s.templates()
	if over != nil {
		tpl = over.Normalize()
	}
	base := strings.TrimRight(s.BaseURL, "/")
	if base == "" {
		base = "https://example.com"
	}
	vars := SampleVars(s.siteName(), base)
	switch kind {
	case KindReset:
		vars.Link = base + "/reset-password?token=preview"
	case KindVerify:
		vars.Link = base + "/verify-email?token=preview"
	case KindChangeEmail:
		vars.Link = base + "/confirm-email?token=preview"
	case KindReject:
		vars.Link = base
	default:
		vars.Link = base + "/settings"
	}
	return Render(kind, lang, vars, tpl.Copy(kind), s.accent())
}

// SendKind 用 Preview 的渲染结果经已存 SMTP 发出。
func (s *Service) SendKind(to string, kind Kind, lang string, over *Templates) error {
	sub, html, err := s.Preview(kind, lang, over)
	if err != nil {
		return err
	}
	return s.Send(to, sub, html)
}

// SendKindWithConfig 同上，SMTP 用调用方覆盖（测发信可走未保存配置）。
func (s *Service) SendKindWithConfig(cfg Config, to string, kind Kind, lang string, over *Templates) error {
	sub, html, err := s.Preview(kind, lang, over)
	if err != nil {
		return err
	}
	return s.SendWithConfig(cfg, to, sub, html)
}

// buildMessage 组 RFC5322 信封:中文主题 RFC2047 Q 编码;HTML utf-8 正文 8bit 直发
// (现代 SMTP 普遍 8BITMIME,不做 quoted-printable——取舍见 spec §5)。
func buildMessage(from, to, subject, htmlBody string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}

// smtpSend 真实发送:ssl=TLS 直连;starttls=明文拨号后升级;none=明文(仅内网)。
func smtpSend(cfg Config, to string, msg []byte) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	var conn net.Conn
	var err error
	if cfg.Encryption == "ssl" {
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: dialTimeout}, "tcp", addr, &tls.Config{ServerName: cfg.Host})
	} else {
		conn, err = net.DialTimeout("tcp", addr, dialTimeout)
	}
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(dialTimeout))
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if cfg.Encryption == "starttls" {
		if err := c.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
			return err
		}
	}
	if err := smtpAuthenticate(c, cfg); err != nil {
		return err
	}
	if err := c.Mail(cfg.From); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
