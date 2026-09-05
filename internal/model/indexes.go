package model

import (
	"fmt"

	"gorm.io/gorm"
)

// ensureIndexes 补建全部 schema 声明而库中缺失的索引。
// 因果:glebarez/sqlite 的 Migrator.CreateConstraint 靠「重建整张表」加 FK,重建只搬
// 列/默认值/表内约束,不搬二级索引——全新 SQLite 库经 applyForeignKeys 后,被重建表
// 的索引(含 username/email/key/slug/token_hash 等唯一索引)全部丢失,唯一性在首启进
// 程内全靠应用层检查(见 spec 2026-07-26-imgli-fresh-sqlite-indexes-design.md)。
// 此处按 AllModels() 的 schema 声明统一补齐;Postgres 与存量 SQLite 路径上索引本就
// 齐全,HasIndex 全真,零 DDL——幂等,对全部方言无害。
func ensureIndexes(db *gorm.DB) error {
	m := db.Migrator()
	for _, mod := range AllModels() {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(mod); err != nil {
			return fmt.Errorf("解析模型 %T: %w", mod, err)
		}
		for _, idx := range stmt.Schema.ParseIndexes() {
			if m.HasIndex(mod, idx.Name) {
				continue
			}
			if err := m.CreateIndex(mod, idx.Name); err != nil {
				return fmt.Errorf("补建索引 %s.%s: %w", stmt.Table, idx.Name, err)
			}
		}
	}
	return ensureMigrateJobActiveIndex(db)
}

// ensureMigrateJobActiveIndex 同一 from 同时只允许一条 pending|running 搬迁任务。
func ensureMigrateJobActiveIndex(db *gorm.DB) error {
	if !db.Migrator().HasTable(&StorageMigrateJob{}) {
		return nil
	}
	return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_storage_migrate_jobs_active_from ON storage_migrate_jobs (from_policy_id) WHERE status IN ('pending','running')`).Error
}
