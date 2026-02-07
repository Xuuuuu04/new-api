package main

import (
	"fmt"
	"log"
	"os"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func mustEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("missing env: %s", key)
	}
	return val
}

func migrateBatch[T any](name string, src *gorm.DB, dst *gorm.DB) error {
	var items []T
	if err := src.Find(&items).Error; err != nil {
		return fmt.Errorf("load %s: %w", name, err)
	}
	if len(items) == 0 {
		log.Printf("%s: 0 rows", name)
		return nil
	}
	if err := dst.Session(&gorm.Session{CreateBatchSize: 200}).Create(&items).Error; err != nil {
		return fmt.Errorf("insert %s: %w", name, err)
	}
	log.Printf("%s: %d rows", name, len(items))
	return nil
}

func main() {
	sqlitePath := envOrDefault("SQLITE_PATH", "one-api.db")
	mysqlDSN := mustEnv("MYSQL_DSN")

	src, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}

	dst, err := gorm.Open(mysql.Open(mysqlDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}

	if err := dst.AutoMigrate(
		&model.Channel{},
		&model.Token{},
		&model.User{},
		&model.Option{},
		&model.Ability{},
		&model.Model{},
		&model.Vendor{},
		&model.PrefillGroup{},
		&model.Setup{},
	); err != nil {
		log.Fatalf("automigrate: %v", err)
	}

	if err := dst.Exec("SET FOREIGN_KEY_CHECKS=0").Error; err != nil {
		log.Printf("disable FK checks: %v", err)
	}

	if err := migrateBatch[model.User]("users", src, dst); err != nil {
		log.Fatal(err)
	}
	if err := migrateBatch[model.Token]("tokens", src, dst); err != nil {
		log.Fatal(err)
	}
	if err := migrateBatch[model.Channel]("channels", src, dst); err != nil {
		log.Fatal(err)
	}
	if err := migrateBatch[model.Option]("options", src, dst); err != nil {
		log.Fatal(err)
	}
	if err := migrateBatch[model.Ability]("abilities", src, dst); err != nil {
		log.Fatal(err)
	}
	if err := migrateBatch[model.Model]("models", src, dst); err != nil {
		log.Fatal(err)
	}
	if err := migrateBatch[model.Vendor]("vendors", src, dst); err != nil {
		log.Fatal(err)
	}
	if err := migrateBatch[model.PrefillGroup]("prefill_groups", src, dst); err != nil {
		log.Fatal(err)
	}
	if err := migrateBatch[model.Setup]("setup", src, dst); err != nil {
		log.Fatal(err)
	}

	if err := dst.Exec("SET FOREIGN_KEY_CHECKS=1").Error; err != nil {
		log.Printf("enable FK checks: %v", err)
	}

	log.Println("migration complete")
}
