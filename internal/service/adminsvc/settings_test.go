package adminsvc

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/yixian-huang/imgli/internal/mail"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/moderation"
)

func rawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestGetSettingsDefaults(t *testing.T) {
	svc := New(model.TestDB(t))
	m, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if m["site_name"] != "img.li" {
		t.Errorf("site_name = %v, want img.li", m["site_name"])
	}
	if m["registration_mode"] != "open" {
		t.Errorf("registration_mode = %v, want open", m["registration_mode"])
	}
	if m["plaza_enabled"] != false {
		t.Errorf("plaza_enabled = %v, want false", m["plaza_enabled"])
	}
	mod, ok := m["moderation"].(map[string]any)
	if !ok {
		t.Fatalf("moderation 字段类型不对: %T", m["moderation"])
	}
	if mod["enabled"] != false || mod["provider"] != "webhook" || mod["threshold"] != 0.8 || mod["action"] != "pending" {
		t.Errorf("moderation 默认值不符: %+v", mod)
	}
	if mod["api_key"] != "" {
		t.Errorf("api_key 默认应为空串, got %v", mod["api_key"])
	}
}

func TestGetSettingsMasksAPIKey(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)

	// 直接写入一个带 api_key 的 moderation 配置（越过 PutSettings，模拟已配置状态）。
	cfg := moderation.Config{
		Enabled: true, Provider: "webhook", Endpoint: "https://mod.example.com/score",
		APIKey: "sk-verysecret12345", Threshold: 0.9, Action: "rejected",
	}
	b, _ := json.Marshal(cfg)
	if err := db.Save(&model.Setting{Key: model.SettingModeration, Value: string(b)}).Error; err != nil {
		t.Fatal(err)
	}

	m, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	mod := m["moderation"].(map[string]any)
	got := mod["api_key"].(string)
	if got != "****2345" {
		t.Errorf("api_key 打码 = %q, want ****2345", got)
	}
	// 打码后的值不应包含明文密钥的任何可辨识片段（防泄露的最基本断言）。
	if got == cfg.APIKey {
		t.Errorf("api_key 未打码")
	}
}

func TestGetSettingsMasksShortAPIKey(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)
	cfg := moderation.Config{Provider: "webhook", Threshold: 0.5, Action: "pending", APIKey: "abc"}
	b, _ := json.Marshal(cfg)
	db.Save(&model.Setting{Key: model.SettingModeration, Value: string(b)})

	m, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	mod := m["moderation"].(map[string]any)
	if mod["api_key"] != "****" {
		t.Errorf("短 api_key（长度<=4）应全打码为 ****, got %v", mod["api_key"])
	}
}

func TestPutSettingsSiteName(t *testing.T) {
	svc := New(model.TestDB(t))

	if err := svc.PutSettings(map[string]json.RawMessage{"site_name": rawJSON(t, "我的图床")}); err != nil {
		t.Fatal(err)
	}
	m, _ := svc.GetSettings()
	if m["site_name"] != "我的图床" {
		t.Errorf("site_name = %v, want 我的图床", m["site_name"])
	}

	if err := svc.PutSettings(map[string]json.RawMessage{"site_name": rawJSON(t, "")}); err == nil {
		t.Errorf("空 site_name 应报错")
	}
	longName := ""
	for i := 0; i < 65; i++ {
		longName += "x"
	}
	if err := svc.PutSettings(map[string]json.RawMessage{"site_name": rawJSON(t, longName)}); err == nil {
		t.Errorf("超过 64 字符的 site_name 应报错")
	}
}

func TestPutSettingsThemeAppearance(t *testing.T) {
	svc := New(model.TestDB(t))

	// defaults
	m, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if m["theme_accent"] != "" {
		t.Errorf("theme_accent default = %v, want empty", m["theme_accent"])
	}
	if m["theme_bg_color"] != "" {
		t.Errorf("theme_bg_color default = %v, want empty", m["theme_bg_color"])
	}
	if m["theme_bg_image_url"] != "" {
		t.Errorf("theme_bg_image_url default = %v, want empty", m["theme_bg_image_url"])
	}
	if m["theme_bg_dim"] != DefaultThemeBgDim {
		t.Errorf("theme_bg_dim default = %v, want %v", m["theme_bg_dim"], DefaultThemeBgDim)
	}
	if m["theme_glass"] != DefaultThemeGlass {
		t.Errorf("theme_glass default = %v, want %v", m["theme_glass"], DefaultThemeGlass)
	}

	if err := svc.PutSettings(map[string]json.RawMessage{
		"theme_accent":       rawJSON(t, "#3B82F6"),
		"theme_bg_color":     rawJSON(t, "#F0F4F8"),
		"theme_bg_image_url": rawJSON(t, "https://cdn.example.com/bg.jpg"),
		"theme_bg_dim":       rawJSON(t, 0.5),
		"theme_glass":        rawJSON(t, 0.6),
	}); err != nil {
		t.Fatal(err)
	}
	m, _ = svc.GetSettings()
	if m["theme_accent"] != "#3b82f6" {
		t.Errorf("theme_accent = %v, want #3b82f6", m["theme_accent"])
	}
	if m["theme_bg_color"] != "#f0f4f8" {
		t.Errorf("theme_bg_color = %v, want #f0f4f8", m["theme_bg_color"])
	}
	if m["theme_bg_image_url"] != "https://cdn.example.com/bg.jpg" {
		t.Errorf("theme_bg_image_url = %v", m["theme_bg_image_url"])
	}
	if m["theme_bg_dim"] != 0.5 {
		t.Errorf("theme_bg_dim = %v, want 0.5", m["theme_bg_dim"])
	}
	if m["theme_glass"] != 0.6 {
		t.Errorf("theme_glass = %v, want 0.6", m["theme_glass"])
	}

	// clear accent + expand short hex
	if err := svc.PutSettings(map[string]json.RawMessage{"theme_accent": rawJSON(t, "#abc")}); err != nil {
		t.Fatal(err)
	}
	m, _ = svc.GetSettings()
	if m["theme_accent"] != "#aabbcc" {
		t.Errorf("short hex expand = %v, want #aabbcc", m["theme_accent"])
	}

	if err := svc.PutSettings(map[string]json.RawMessage{"theme_accent": rawJSON(t, "not-a-color")}); err == nil {
		t.Error("invalid accent should fail")
	}
	if err := svc.PutSettings(map[string]json.RawMessage{"theme_bg_dim": rawJSON(t, 1.5)}); err == nil {
		t.Error("dim > 1 should fail")
	}
	if err := svc.PutSettings(map[string]json.RawMessage{"theme_glass": rawJSON(t, -0.1)}); err == nil {
		t.Error("glass < 0 should fail")
	}
	if err := svc.PutSettings(map[string]json.RawMessage{"theme_bg_image_url": rawJSON(t, "javascript:alert(1)")}); err == nil {
		t.Error("bad bg url should fail")
	}
	// empty clears
	if err := svc.PutSettings(map[string]json.RawMessage{
		"theme_accent":       rawJSON(t, ""),
		"theme_bg_color":     rawJSON(t, ""),
		"theme_bg_image_url": rawJSON(t, ""),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPutSettingsRegistrationMode(t *testing.T) {
	svc := New(model.TestDB(t))

	for _, mode := range []string{"open", "invite", "closed"} {
		if err := svc.PutSettings(map[string]json.RawMessage{"registration_mode": rawJSON(t, mode)}); err != nil {
			t.Errorf("registration_mode=%q 不应报错: %v", mode, err)
		}
		m, _ := svc.GetSettings()
		if m["registration_mode"] != mode {
			t.Errorf("写入后 = %v, want %v", m["registration_mode"], mode)
		}
	}

	if err := svc.PutSettings(map[string]json.RawMessage{"registration_mode": rawJSON(t, "bogus")}); err == nil {
		t.Errorf("非法 registration_mode 应报错")
	}
}

func TestPutSettingsRegistrationModeInvite(t *testing.T) {
	svc := New(model.TestDB(t))
	if err := svc.PutSettings(map[string]json.RawMessage{"registration_mode": rawJSON(t, "invite")}); err != nil {
		t.Fatalf("invite 应为合法注册模式, err = %v", err)
	}
	m, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if m["registration_mode"] != "invite" {
		t.Errorf("registration_mode = %v, want invite", m["registration_mode"])
	}
}

func TestPutSettingsUnknownKeyRejected(t *testing.T) {
	svc := New(model.TestDB(t))
	err := svc.PutSettings(map[string]json.RawMessage{"bogus_key": rawJSON(t, true)})
	if !errors.Is(err, ErrUnknownSetting) {
		t.Errorf("err = %v, want ErrUnknownSetting", err)
	}
}

func TestPutSettingsGuestUpload(t *testing.T) {
	svc := New(model.TestDB(t))

	m, _ := svc.GetSettings()
	if m["guest_upload_enabled"] != false {
		t.Errorf("guest_upload_enabled 默认应为 false, got %v", m["guest_upload_enabled"])
	}

	if err := svc.PutSettings(map[string]json.RawMessage{"guest_upload_enabled": rawJSON(t, true)}); err != nil {
		t.Fatalf("合法 bool 值不应报错: %v", err)
	}
	m, _ = svc.GetSettings()
	if m["guest_upload_enabled"] != true {
		t.Errorf("写入后 guest_upload_enabled = %v, want true", m["guest_upload_enabled"])
	}

	if err := svc.PutSettings(map[string]json.RawMessage{"guest_upload_enabled": rawJSON(t, "yes")}); !errors.Is(err, ErrGuestUploadInvalid) {
		t.Errorf("非 bool 值 err = %v, want ErrGuestUploadInvalid", err)
	}
	// 校验失败不应落库：值应仍为写入前的 true。
	m, _ = svc.GetSettings()
	if m["guest_upload_enabled"] != true {
		t.Errorf("非法值校验失败后不应改变已存值, got %v", m["guest_upload_enabled"])
	}
}

func TestPutSettingsPlazaEnabled(t *testing.T) {
	svc := New(model.TestDB(t))

	m, _ := svc.GetSettings()
	if m["plaza_enabled"] != false {
		t.Errorf("plaza_enabled 默认应为 false, got %v", m["plaza_enabled"])
	}

	if err := svc.PutSettings(map[string]json.RawMessage{"plaza_enabled": rawJSON(t, true)}); err != nil {
		t.Fatalf("合法 bool 值不应报错: %v", err)
	}
	m, _ = svc.GetSettings()
	if m["plaza_enabled"] != true {
		t.Errorf("写入后 plaza_enabled = %v, want true", m["plaza_enabled"])
	}

	if err := svc.PutSettings(map[string]json.RawMessage{"plaza_enabled": rawJSON(t, "x")}); !errors.Is(err, ErrPlazaEnabledInvalid) {
		t.Errorf("非 bool 值 err = %v, want ErrPlazaEnabledInvalid", err)
	}
	m, _ = svc.GetSettings()
	if m["plaza_enabled"] != true {
		t.Errorf("非法值校验失败后不应改变已存值, got %v", m["plaza_enabled"])
	}
}

func TestPutSettingsModerationValidationMatrix(t *testing.T) {
	svc := New(model.TestDB(t))

	valid := moderation.Config{
		Enabled: false, Provider: "webhook", Endpoint: "", APIKey: "", Threshold: 0.8, Action: "pending",
	}
	if err := svc.PutSettings(map[string]json.RawMessage{"moderation": rawJSON(t, valid)}); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}

	cases := map[string]moderation.Config{
		"threshold 越界(负)":    {Provider: "webhook", Threshold: -0.1, Action: "pending"},
		"threshold 越界(>1)":   {Provider: "webhook", Threshold: 1.5, Action: "pending"},
		"action 非法":          {Provider: "webhook", Threshold: 0.5, Action: "approved"},
		"provider 非法":        {Provider: "sightengine", Threshold: 0.5, Action: "pending"},
		"enabled 无 endpoint": {Enabled: true, Provider: "webhook", Threshold: 0.5, Action: "pending", Endpoint: ""},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if err := svc.PutSettings(map[string]json.RawMessage{"moderation": rawJSON(t, cfg)}); err == nil {
				t.Errorf("%s 应报错", name)
			}
		})
	}
}

func TestPutSettingsModerationAPIKeyRetainSemantics(t *testing.T) {
	svc := New(model.TestDB(t))

	initial := moderation.Config{
		Enabled: true, Provider: "webhook", Endpoint: "https://mod.example.com/score",
		APIKey: "sk-realsecretvalue", Threshold: 0.7, Action: "pending",
	}
	if err := svc.PutSettings(map[string]json.RawMessage{"moderation": rawJSON(t, initial)}); err != nil {
		t.Fatal(err)
	}

	m, _ := svc.GetSettings()
	masked := m["moderation"].(map[string]any)["api_key"].(string)
	if masked == "" || masked == initial.APIKey {
		t.Fatalf("前置条件失败：期望拿到打码后的 api_key, got %q", masked)
	}

	// 用打码后的值回传（模拟前端只读拿到打码值又原样提交），其余字段改动：threshold 变化。
	patched := initial
	patched.APIKey = masked
	patched.Threshold = 0.95
	if err := svc.PutSettings(map[string]json.RawMessage{"moderation": rawJSON(t, patched)}); err != nil {
		t.Fatal(err)
	}

	// 直接读库校验明文 api_key 未被打码值覆盖，且其余字段确已生效。
	var row model.Setting
	if err := svc.db.First(&row, "key = ?", model.SettingModeration).Error; err != nil {
		t.Fatal(err)
	}
	var stored moderation.Config
	if err := json.Unmarshal([]byte(row.Value), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.APIKey != initial.APIKey {
		t.Errorf("api_key 保留语义失败：stored=%q, want %q", stored.APIKey, initial.APIKey)
	}
	if stored.Threshold != 0.95 {
		t.Errorf("threshold 未生效: %v", stored.Threshold)
	}
}

func TestPutSettingsPartialFailureDoesNotPersistAnyKey(t *testing.T) {
	svc := New(model.TestDB(t))

	before, _ := svc.GetSettings()

	err := svc.PutSettings(map[string]json.RawMessage{
		"site_name":         rawJSON(t, "新站名"),
		"registration_mode": rawJSON(t, "bogus"), // 非法，整个请求应回滚不落库
	})
	if err == nil {
		t.Fatal("混合请求中一键非法应整体报错")
	}

	after, _ := svc.GetSettings()
	if after["site_name"] != before["site_name"] {
		t.Errorf("校验失败时 site_name 不应被写入: before=%v after=%v", before["site_name"], after["site_name"])
	}
}

func TestPutSettingsSMTP(t *testing.T) {
	svc := New(model.TestDB(t))
	// 合法写入
	if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, map[string]any{
		"host": "smtp.example", "port": 465, "username": "u", "password": "secret-pw", "from": "no-reply@img.li", "encryption": "ssl",
	})}); err != nil {
		t.Fatalf("合法 smtp err = %v", err)
	}
	m, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	smtp := m["smtp"].(map[string]any)
	if smtp["host"] != "smtp.example" || smtp["encryption"] != "ssl" {
		t.Errorf("回读不符: %v", smtp)
	}
	if pw := smtp["password"].(string); !strings.HasPrefix(pw, "****") || strings.Contains(pw, "secret") {
		t.Errorf("password 应打码, got %q", pw)
	}
	// 打码回传保留明文
	if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, map[string]any{
		"host": "smtp.example", "port": 465, "username": "u", "password": smtp["password"], "from": "no-reply@img.li", "encryption": "ssl",
	})}); err != nil {
		t.Fatal(err)
	}
	var row model.Setting
	if err := svc.db.First(&row, "key = ?", model.SettingSMTP).Error; err != nil {
		t.Fatal(err)
	}
	var stored mail.Config
	if err := json.Unmarshal([]byte(row.Value), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Password != "secret-pw" {
		t.Errorf("打码回传应保留明文, got %q", stored.Password)
	}
	// 校验矩阵：每类错误单独文案，避免再把「改用户名」理解成端口/加密填错。
	for name, tc := range map[string]struct {
		body map[string]any
		want error
	}{
		"port0":    {map[string]any{"host": "h", "port": 0, "encryption": "none"}, ErrSMTPPortInvalid},
		"port高":    {map[string]any{"host": "h", "port": 65536, "encryption": "none"}, ErrSMTPPortInvalid},
		"enc非法":    {map[string]any{"host": "h", "port": 25, "encryption": "tls13"}, ErrSMTPEncryptionInvalid},
		"from非邮":   {map[string]any{"host": "h", "port": 25, "encryption": "none", "from": "not-an-email"}, ErrSMTPFromInvalid},
		"from_a@":  {map[string]any{"host": "h", "port": 25, "encryption": "none", "from": "a@"}, ErrSMTPFromInvalid},
		"from_a@b": {map[string]any{"host": "h", "port": 25, "encryption": "none", "from": "a@b"}, ErrSMTPFromInvalid},
		"none+认证":  {map[string]any{"host": "h", "port": 25, "encryption": "none", "username": "u"}, ErrSMTPNoneWithAuth},
	} {
		if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, tc.body)}); !errors.Is(err, tc.want) {
			t.Errorf("%s err = %v, want %v", name, err, tc.want)
		}
	}
}

// TestPutSettingsSMTPMaskedPasswordHostChange 掩码密码 + 改 host → 拒绝(防凭据窃取)。
func TestPutSettingsSMTPMaskedPasswordHostChange(t *testing.T) {
	svc := New(model.TestDB(t))
	if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, map[string]any{
		"host": "smtp.example", "port": 465, "username": "u", "password": "secret-pw",
		"from": "no-reply@img.li", "encryption": "ssl",
	})}); err != nil {
		t.Fatal(err)
	}
	m, _ := svc.GetSettings()
	masked := m["smtp"].(map[string]any)["password"].(string)
	if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, map[string]any{
		"host": "evil.example", "port": 465, "username": "u", "password": masked,
		"from": "no-reply@img.li", "encryption": "ssl",
	})}); !errors.Is(err, ErrSMTPPasswordReenter) {
		t.Fatalf("掩码+改 host err = %v, want ErrSMTPPasswordReenter", err)
	}
}

// TestPutSettingsSMTPMaskedPasswordUsernameChange 掩码密码 + 改 username（飞书/Lark
// 公共邮箱用户先存了密码再补邮箱用户名的典型路径）→ 明确要求重输密码，而不是
// 笼统的 port/encryption/from 无效。
func TestPutSettingsSMTPMaskedPasswordUsernameChange(t *testing.T) {
	svc := New(model.TestDB(t))
	if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, map[string]any{
		"host": "smtp.larksuite.com", "port": 465, "username": "", "password": "imap-smtp-pw",
		"from": "noreply@qqqu.de", "encryption": "ssl",
	})}); err != nil {
		t.Fatal(err)
	}
	m, _ := svc.GetSettings()
	masked := m["smtp"].(map[string]any)["password"].(string)
	if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, map[string]any{
		"host": "smtp.larksuite.com", "port": 465, "username": "noreply@qqqu.de", "password": masked,
		"from": "noreply@qqqu.de", "encryption": "ssl",
	})}); !errors.Is(err, ErrSMTPPasswordReenter) {
		t.Fatalf("掩码+改 username err = %v, want ErrSMTPPasswordReenter", err)
	}
	if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, map[string]any{
		"host": "smtp.larksuite.com", "port": 465, "username": "noreply@qqqu.de", "password": "imap-smtp-pw",
		"from": "noreply@qqqu.de", "encryption": "ssl",
	})}); err != nil {
		t.Fatalf("改 username 并重输密码应成功, err = %v", err)
	}
}

// TestPutSettingsSMTPTrimsIdentity 复制粘贴带空白的 host/username/from 应被 trim 后落库。
func TestPutSettingsSMTPTrimsIdentity(t *testing.T) {
	svc := New(model.TestDB(t))
	if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, map[string]any{
		"host": " smtp.larksuite.com ", "port": 465, "username": "  noreply@qqqu.de\n",
		"password": "pw", "from": " noreply@qqqu.de ", "encryption": "ssl",
	})}); err != nil {
		t.Fatalf("trim 后应合法, err = %v", err)
	}
	var row model.Setting
	if err := svc.db.First(&row, "key = ?", model.SettingSMTP).Error; err != nil {
		t.Fatal(err)
	}
	var stored mail.Config
	if err := json.Unmarshal([]byte(row.Value), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Host != "smtp.larksuite.com" || stored.Username != "noreply@qqqu.de" || stored.From != "noreply@qqqu.de" {
		t.Errorf("未 trim: %+v", stored)
	}
}

// TestPutSettingsSMTPMaskedPasswordSameIdentity 掩码密码 + 同 host/username → 保留成功。
func TestPutSettingsSMTPMaskedPasswordSameIdentity(t *testing.T) {
	svc := New(model.TestDB(t))
	if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, map[string]any{
		"host": "smtp.example", "port": 465, "username": "u", "password": "secret-pw",
		"from": "no-reply@img.li", "encryption": "ssl",
	})}); err != nil {
		t.Fatal(err)
	}
	m, _ := svc.GetSettings()
	masked := m["smtp"].(map[string]any)["password"].(string)
	if err := svc.PutSettings(map[string]json.RawMessage{"smtp": rawJSON(t, map[string]any{
		"host": "smtp.example", "port": 587, "username": "u", "password": masked,
		"from": "no-reply@img.li", "encryption": "starttls",
	})}); err != nil {
		t.Fatalf("掩码+同 host/username 应成功, err = %v", err)
	}
	var row model.Setting
	if err := svc.db.First(&row, "key = ?", model.SettingSMTP).Error; err != nil {
		t.Fatal(err)
	}
	var stored mail.Config
	if err := json.Unmarshal([]byte(row.Value), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Password != "secret-pw" {
		t.Errorf("应保留明文密码, got %q", stored.Password)
	}
	if stored.Port != 587 || stored.Encryption != "starttls" {
		t.Errorf("其它字段应更新: port=%d enc=%s", stored.Port, stored.Encryption)
	}
}

func TestPutSettingsMailTemplates(t *testing.T) {
	svc := New(model.TestDB(t))
	if err := svc.PutSettings(map[string]json.RawMessage{"mail_templates": rawJSON(t, map[string]any{
		"welcome": map[string]any{
			"subject": map[string]string{"zh": "欢迎来到 {{site_name}}", "en": "Hi {{site_name}}"},
			"body":    map[string]string{"zh": "自己的欢迎词", "en": "our welcome"},
		},
	})}); err != nil {
		t.Fatalf("合法文案 err = %v", err)
	}
	m, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	tpl := m["mail_templates"].(mail.Templates)
	if tpl.Welcome.Body.ZH != "自己的欢迎词" {
		t.Errorf("回读 welcome body = %+v", tpl.Welcome)
	}
	defs := m["mail_template_defaults"].(mail.Templates)
	if defs.Reset.Subject.ZH == "" {
		t.Error("应回传内置默认供填入")
	}
	if err := svc.PutSettings(map[string]json.RawMessage{"mail_templates": rawJSON(t, map[string]any{
		"welcome": map[string]any{"subject": map[string]string{"zh": "x {{foo}}"}},
	})}); !errors.Is(err, mail.ErrCopyUnknownPlaceholder) {
		t.Fatalf("未知占位 err = %v", err)
	}
}
