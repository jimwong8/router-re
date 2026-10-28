package main

import (
	"log"
	"os"

	"github.com/jimwong/router-re/mgmtd/internal/server"
)

func main() {
	addr := os.Getenv("MGM_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	// 账号表必须有落盘路径。不设 MGM_USER_FILE 时默认用 /var/lib/router-re/users.json，
	// 而不是退回纯内存——否则运维少配一个变量，重启就把所有非 admin 账号静默清空，
	// 而界面上看不出来（只有下次登录失败才会发现）。
	userFile := os.Getenv("MGM_USER_FILE")
	if userFile == "" {
		userFile = "/var/lib/router-re/users.json"
	}
	cfg := server.Config{
		Addr:          addr,
		SecretKey:     []byte(os.Getenv("MGM_SESSION_SECRET")),
		AdminUser:     os.Getenv("MGM_ADMIN_USER"),
		AdminPassword: os.Getenv("MGM_ADMIN_PASSWORD"),
		UserFile:      userFile,
	}
	if len(cfg.SecretKey) == 0 {
		log.Fatal("MGM_SESSION_SECRET must be set (32+ bytes)")
	}
	log.Printf("账号表: %s（重启后账号从这里恢复）", userFile)
	s := server.New(cfg)
	log.Printf("mgmtd listening on %s", cfg.Addr)
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
