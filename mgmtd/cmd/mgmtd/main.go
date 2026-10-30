package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

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
	// 配置存储打不开就别启动。以前这里会静默退回内存存储：服务照常跑、
	// 所有配置变更返回 200，重启后全部消失，运行期间一点迹象都没有。
	if err := s.InitError(); err != nil {
		log.Fatalf("mgmtd: %v\n提示：修好配置文件，或显式设 MGM_ALLOW_EPHEMERAL_STORE=1 接受「重启丢配置」", err)
	}
	// 退出前把监控历史落盘：重启后 360 分钟视图还能看到重启前的曲线，
	// 否则每次重启图表都从零开始，用户会以为监控坏了
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		s.SaveStatsHistory()
		os.Exit(0)
	}()
	log.Printf("mgmtd listening on %s（配置存储: %s）", cfg.Addr, os.Getenv("MGM_CONFIG_FILE"))
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
