// 独立的外部测试包（package model_test）：db.go 的 settingSMTPDefaultJSON 字面量
// 需要与 mail.DefaultConfig() 逐字段核对一致，但 mail 包 import 了 model 包。
// 若这个断言放在 package model 内部测试文件里，会与 mail 反向 import model 形成循环依赖。
// 放到外部测试包 model_test 则不构成环。
package model_test

import (
	"encoding/json"
	"testing"

	"github.com/yixian-huang/imgli/internal/mail"
	"github.com/yixian-huang/imgli/internal/model"
)

// TestSettingSMTPSeedMatchesDefaultConfig 断言 db.go 手写的 smtp 播种
// JSON 字面量与 mail.DefaultConfig() 逐字段一致，防止两处漂移。
func TestSettingSMTPSeedMatchesDefaultConfig(t *testing.T) {
	db := model.TestDB(t)
	var row model.Setting
	if err := db.First(&row, "key = ?", model.SettingSMTP).Error; err != nil {
		t.Fatal(err)
	}
	var seeded mail.Config
	if err := json.Unmarshal([]byte(row.Value), &seeded); err != nil {
		t.Fatalf("播种的 smtp JSON 解析失败: %v", err)
	}
	if seeded != mail.DefaultConfig() {
		t.Errorf("播种值 = %+v, DefaultConfig() = %+v", seeded, mail.DefaultConfig())
	}
}

func TestSettingMailTemplatesSeedMatchesDefault(t *testing.T) {
	db := model.TestDB(t)
	var row model.Setting
	if err := db.First(&row, "key = ?", model.SettingMailTemplates).Error; err != nil {
		t.Fatal(err)
	}
	var seeded mail.Templates
	if err := json.Unmarshal([]byte(row.Value), &seeded); err != nil {
		t.Fatalf("播种的 mail_templates JSON 解析失败: %v", err)
	}
	if seeded != mail.DefaultTemplates() {
		t.Errorf("播种值 = %+v, DefaultTemplates() = %+v", seeded, mail.DefaultTemplates())
	}
}
