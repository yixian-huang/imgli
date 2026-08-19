package mail

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExpandPlaceholders(t *testing.T) {
	v := Vars{SiteName: "白栗", Link: "https://x/r", ImageName: "a.png (k1)", ImageKey: "k1"}
	got := expand("欢迎 {{site_name}} 点 {{link}} 看 {{image_name}}/{{image_key}}", v)
	want := "欢迎 白栗 点 https://x/r 看 a.png (k1)/k1"
	if got != want {
		t.Fatalf("expand = %q, want %q", got, want)
	}
	if expand("{{ unknown }}", v) != "{{ unknown }}" {
		t.Error("未知占位应原样保留在 expand（保存阶段拒绝）")
	}
}

func TestValidateCopy(t *testing.T) {
	if err := ValidateCopy(KindWelcome, KindCopy{}); err != nil {
		t.Fatalf("空覆盖应合法: %v", err)
	}
	if err := ValidateCopy(KindReset, KindCopy{
		Body: LocaleText{ZH: "请点 {{link}}", EN: "click {{link}}"},
	}); err != nil {
		t.Fatalf("已知占位应合法: %v", err)
	}
	if err := ValidateCopy(KindWelcome, KindCopy{
		Subject: LocaleText{ZH: "hi {{foo}}"},
	}); !errors.Is(err, ErrCopyUnknownPlaceholder) {
		t.Fatalf("未知占位 err = %v, want ErrCopyUnknownPlaceholder", err)
	}
	long := strings.Repeat("啊", maxSubjectRunes+1)
	if err := ValidateCopy(KindWelcome, KindCopy{Subject: LocaleText{ZH: long}}); !errors.Is(err, ErrCopyTooLong) {
		t.Fatalf("超长主题 err = %v, want ErrCopyTooLong", err)
	}
	if utf8.RuneCountInString(strings.Repeat("b", maxBodyRunes)) != maxBodyRunes {
		t.Fatal("maxBodyRunes 自检")
	}
	if err := ValidateCopy(KindWelcome, KindCopy{Body: LocaleText{EN: strings.Repeat("b", maxBodyRunes+1)}}); !errors.Is(err, ErrCopyTooLong) {
		t.Fatalf("超长正文 err = %v", err)
	}
}

func TestRenderUsesOverrideAndKeepsShellLink(t *testing.T) {
	over := KindCopy{
		Subject: LocaleText{ZH: "{{site_name}} 重置"},
		Body:    LocaleText{ZH: "请到下方改密。"},
	}
	sub, html := RenderResetPassword("白栗", "https://img.li/reset-password?token=abc", "zh", over)
	if sub != "白栗 重置" {
		t.Fatalf("subject = %q", sub)
	}
	if !strings.Contains(html, "请到下方改密。") {
		t.Fatalf("自定义正文未进信: %s", html)
	}
	if !strings.Contains(html, "https://img.li/reset-password?token=abc") {
		t.Fatal("壳层必须仍带链接")
	}
	if !strings.Contains(html, "重置密码") {
		t.Fatal("按钮文案仍用内置")
	}
}

func TestRenderEmptyOverrideMatchesBuiltin(t *testing.T) {
	sub, html := RenderResetPassword("白栗", "https://img.li/reset-password?token=abc", "zh", KindCopy{})
	if !strings.Contains(sub, "重置你的 白栗 密码") || !strings.Contains(html, "https://img.li/reset-password?token=abc") {
		t.Fatalf("空覆盖应等同内置: %q %s", sub, html)
	}
}

func TestRenderEscapesHTMLAndBreaksNewlines(t *testing.T) {
	over := KindCopy{Body: LocaleText{ZH: "第一行\n<script>x</script>"}}
	_, html := RenderWelcome("S", "https://img.li", "zh", over)
	if strings.Contains(html, "<script>") {
		t.Fatal("正文 HTML 应转义")
	}
	if !strings.Contains(html, "&lt;script&gt;") || !strings.Contains(html, "<br>") {
		t.Fatalf("应转义并换行: %s", html)
	}
}

func TestParseKind(t *testing.T) {
	if _, ok := ParseKind("welcome"); !ok {
		t.Fatal("welcome")
	}
	if _, ok := ParseKind("nope"); ok {
		t.Fatal("nope")
	}
}

func TestBuiltinDefaultsHaveBothLocales(t *testing.T) {
	d := BuiltinDefaults()
	for name, c := range map[string]KindCopy{
		"welcome": d.Welcome, "verify": d.Verify, "reset": d.Reset,
		"change_email": d.ChangeEmail, "reject": d.Reject,
	} {
		if c.Subject.ZH == "" || c.Subject.EN == "" || c.Body.ZH == "" || c.Body.EN == "" {
			t.Errorf("%s 缺默认文案: %+v", name, c)
		}
		if err := ValidateCopy(Kind(name), c); err != nil {
			t.Errorf("内置 %s 自检失败: %v", name, err)
		}
	}
}
