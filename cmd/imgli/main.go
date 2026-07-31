// imgli 图床 CLI 入口：serve | upload | doctor | migrate | storage-migrate | version。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/server"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	appver "github.com/yixian-huang/imgli/internal/version"
)

// 版本见 internal/version；ldflags: -X github.com/yixian-huang/imgli/internal/version.Version=

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "imgli:", err)
			os.Exit(1)
		}
	case "import-dir":
		if err := runImportDir(os.Args[2:]); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "imgli:", err)
			os.Exit(1)
		}
	case "upload":
		if err := runUpload(os.Args[2:]); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "imgli:", err)
			os.Exit(1)
		}
	case "doctor":
		if err := runDoctor(os.Args[2:]); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "imgli:", err)
			os.Exit(1)
		}
	case "migrate":
		fs := flag.NewFlagSet("migrate", flag.ExitOnError)
		cfgPath := fs.String("config", "", "配置文件路径（可选）")
		fs.Parse(os.Args[2:])
		cfg, err := config.Load(*cfgPath)
		if err == nil {
			var db *gorm.DB
			if db, err = model.Open(cfg); err == nil {
				if err = model.Migrate(db); err == nil {
					err = model.Seed(db)
				}
				if sqlDB, derr := db.DB(); derr == nil {
					sqlDB.Close()
				}
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "imgli:", err)
			os.Exit(1)
		}
		fmt.Println("迁移完成")
	case "storage-migrate":
		if err := runStorageMigrate(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "imgli:", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(appver.Version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法: imgli <serve|upload|import-dir|doctor|migrate|storage-migrate|version> [flags]")
	fmt.Fprintln(os.Stderr, "  upload  上传文件或 stdin 到图床（IMGLI_BASE_URL / IMGLI_TOKEN）")
	fmt.Fprintln(os.Stderr, "  doctor  检查 data 目录、数据库、base_url、存储等常见误配")
}

func runStorageMigrate(args []string) error {
	fs := flag.NewFlagSet("storage-migrate", flag.ExitOnError)
	cfgPath := fs.String("config", "", "配置文件路径（可选）")
	fromID := fs.Uint64("from", 0, "源 storage_policy id")
	toID := fs.Uint64("to", 0, "目标 storage_policy id")
	dryRun := fs.Bool("dry-run", false, "只统计不写盘不改库")
	delSrc := fs.Bool("delete-source", false, "成功后删除源对象")
	limit := fs.Int("limit", 0, "最多处理条数，0=不限")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fromID == 0 || *toID == 0 {
		return fmt.Errorf("需要 -from 与 -to（policy id）")
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	db, err := model.Open(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if sqlDB, e := db.DB(); e == nil {
			sqlDB.Close()
		}
	}()
	res, err := storagesvc.New(cfg, db).MigrateFiles(context.Background(), db, storagesvc.MigrateOpts{
		FromPolicyID: *fromID,
		ToPolicyID:   *toID,
		DryRun:       *dryRun,
		DeleteSource: *delSrc,
		Limit:        *limit,
	})
	if err != nil {
		return err
	}
	fmt.Printf("storage-migrate done dry_run=%v scanned=%d copied=%d skipped=%d failed=%d\n",
		*dryRun, res.Scanned, res.Copied, res.Skipped, res.Failed)
	for _, e := range res.Errors {
		fmt.Fprintln(os.Stderr, " ", e)
	}
	if res.Failed > 0 {
		return fmt.Errorf("%d 条失败", res.Failed)
	}
	return nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", "", "配置文件路径（可选）")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	db, err := model.Open(cfg)
	if err != nil {
		return err
	}
	if err := model.Migrate(db); err != nil {
		return err
	}
	if err := model.Seed(db); err != nil {
		return err
	}
	if sqlDB, derr := db.DB(); derr == nil {
		defer sqlDB.Close()
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("imgli %s 监听 %s\n", appver.Version, cfg.Listen)
	return server.New(server.Options{Cfg: cfg, DB: db}).Run(ctx)
}
