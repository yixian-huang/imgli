package mail

import (
	"errors"
	"strings"
	"testing"

	"github.com/yixian-huang/imgli/internal/model"
)

func TestSendNotConfigured(t *testing.T) {
	db := model.TestDB(t) // 播种的 smtp host 为空
	svc := New(db)
	if err := svc.Send("a@b.c", "s", "<p>x</p>"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("host 空应 ErrNotConfigured, got %v", err)
	}
}

func TestSendUsesInjectedSender(t *testing.T) {
	db := model.TestDB(t)
	if err := db.Model(&model.Setting{}).Where("key = ?", model.SettingSMTP).
		Update("value", `{"host":"smtp.example","port":587,"username":"u","password":"p","from":"no-reply@img.li","encryption":"starttls"}`).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db)
	var gotTo string
	var gotMsg []byte
	svc.sender = func(cfg Config, to string, msg []byte) error {
		if cfg.Host != "smtp.example" || cfg.Encryption != "starttls" {
			t.Errorf("cfg 未按 settings 解析: %+v", cfg)
		}
		gotTo, gotMsg = to, msg
		return nil
	}
	if err := svc.Send("user@img.li", "主题测试", "<p>你好</p>"); err != nil {
		t.Fatal(err)
	}
	if gotTo != "user@img.li" {
		t.Errorf("to = %q", gotTo)
	}
	m := string(gotMsg)
	for _, want := range []string{"From: no-reply@img.li", "To: user@img.li", "MIME-Version: 1.0", "Content-Type: text/html; charset=utf-8", "<p>你好</p>"} {
		if !strings.Contains(m, want) {
			t.Errorf("信封缺 %q\n%s", want, m)
		}
	}
	if !strings.Contains(m, "Subject: =?utf-8?") {
		t.Errorf("中文主题应 RFC2047 编码, got:\n%s", m)
	}
}

func TestSendFallsBackFromToUsername(t *testing.T) {
	db := model.TestDB(t)
	if err := db.Model(&model.Setting{}).Where("key = ?", model.SettingSMTP).
		Update("value", `{"host":"smtp.larksuite.com","port":465,"username":"noreply@qqqu.de","password":"p","from":"","encryption":"ssl"}`).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db)
	var gotCfg Config
	var gotMsg []byte
	svc.sender = func(cfg Config, to string, msg []byte) error {
		gotCfg, gotMsg = cfg, msg
		return nil
	}
	if err := svc.Send("user@img.li", "s", "<p>x</p>"); err != nil {
		t.Fatal(err)
	}
	if gotCfg.From != "noreply@qqqu.de" {
		t.Errorf("MAIL FROM 应回退到 username, got %q", gotCfg.From)
	}
	if !strings.Contains(string(gotMsg), "From: noreply@qqqu.de") {
		t.Errorf("信封 From 应回退到 username:\n%s", gotMsg)
	}
}

func TestSendWithConfigUsesOverrideNotSettings(t *testing.T) {
	db := model.TestDB(t) // settings host 为空
	svc := New(db)
	var got Config
	svc.sender = func(cfg Config, to string, msg []byte) error {
		got = cfg
		return nil
	}
	override := Config{
		Host: "smtp.larksuite.com", Port: 465, Username: "noreply@qqqu.de",
		Password: "imap-pw", From: "noreply@qqqu.de", Encryption: "ssl",
	}
	if err := svc.SendWithConfig(override, "user@img.li", "s", "<p>x</p>"); err != nil {
		t.Fatal(err)
	}
	if got.Host != "smtp.larksuite.com" || got.Username != "noreply@qqqu.de" {
		t.Errorf("应用覆盖配置: %+v", got)
	}
	if err := svc.SendWithConfig(Config{}, "user@img.li", "s", "<p>x</p>"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("空 host 应 ErrNotConfigured, got %v", err)
	}
}

func TestSendResetPasswordUsesStoredCopy(t *testing.T) {
	db := model.TestDB(t)
	if err := db.Model(&model.Setting{}).Where("key = ?", model.SettingSMTP).
		Update("value", `{"host":"smtp.example","port":587,"username":"u","password":"p","from":"no-reply@img.li","encryption":"starttls"}`).Error; err != nil {
		t.Fatal(err)
	}
	raw := `{"reset":{"subject":{"zh":"改密 {{site_name}}","en":""},"body":{"zh":"点下面。","en":""}}}`
	if err := db.Model(&model.Setting{}).Where("key = ?", model.SettingMailTemplates).
		Update("value", raw).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db)
	var got []byte
	svc.sender = func(cfg Config, to string, msg []byte) error {
		got = msg
		return nil
	}
	if err := svc.SendResetPassword("a@b.c", "https://img.li/reset-password?token=z", "zh"); err != nil {
		t.Fatal(err)
	}
	m := string(got)
	if !strings.Contains(m, "Subject: ") || !strings.Contains(m, "点下面。") || !strings.Contains(m, "https://img.li/reset-password?token=z") {
		t.Fatalf("应用已存文案并保留链接: %s", m)
	}
}

func TestTemplatesZH(t *testing.T) {
	sub, html := RenderResetPassword("白栗", "https://img.li/reset-password?token=abc", "zh", KindCopy{})
	if !strings.Contains(sub, "重置你的 白栗 密码") || !strings.Contains(html, "https://img.li/reset-password?token=abc") || !strings.Contains(html, "重置密码") {
		t.Errorf("reset zh 模板不完整: %q %q", sub, html)
	}
	// 空 lang 默认中文
	sub0, html0 := RenderResetPassword("白栗", "https://img.li/reset-password?token=abc", "", KindCopy{})
	if !strings.Contains(sub0, "重置你的 白栗 密码") || !strings.Contains(html0, "重置密码") {
		t.Errorf("reset 空 lang 应中文: %q %q", sub0, html0)
	}
	sub2, html2 := RenderVerifyEmail("白栗", "https://img.li/verify-email?token=x", "zh", KindCopy{})
	if !strings.Contains(sub2, "验证你的 白栗 邮箱") || !strings.Contains(html2, "verify-email?token=x") || !strings.Contains(html2, "验证邮箱") {
		t.Errorf("verify zh 模板不完整: %q %q", sub2, html2)
	}
}

func TestTemplatesEN(t *testing.T) {
	sub, html := RenderResetPassword("Imgli", "https://img.li/reset-password?token=abc", "en", KindCopy{})
	if sub != "Reset your Imgli password" {
		t.Errorf("reset en subject = %q", sub)
	}
	for _, want := range []string{"reset your password", "Reset password", "https://img.li/reset-password?token=abc", "1 hour"} {
		if !strings.Contains(strings.ToLower(html), strings.ToLower(want)) && !strings.Contains(html, want) {
			t.Errorf("reset en body 缺 %q\n%s", want, html)
		}
	}
	sub2, html2 := RenderVerifyEmail("Imgli", "https://img.li/verify-email?token=x", "en", KindCopy{})
	if sub2 != "Verify your Imgli email" {
		t.Errorf("verify en subject = %q", sub2)
	}
	for _, want := range []string{"Verify email", "verify-email?token=x", "24 hours"} {
		if !strings.Contains(html2, want) {
			t.Errorf("verify en body 缺 %q\n%s", want, html2)
		}
	}
}
