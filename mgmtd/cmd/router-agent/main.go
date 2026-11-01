// router-agent 是装在路由终端上的常驻进程：主动外连管理中心，接受白名单命令。
//
// 首次注册用一次性令牌；之后身份存在 --state 指定的文件里（0600），不再需要令牌。
// 本机 mgmtd 的账号密码从环境变量读，凭据不会发给云端。
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/cloud"
)

func main() {
	var (
		cloudURL = flag.String("cloud-url", envOr("ROUTER_CLOUD_URL", "https://1.1ai.best"), "管理中心地址")
		token    = flag.String("cloud-enroll-token", os.Getenv("ROUTER_CLOUD_ENROLL_TOKEN"), "一次性注册令牌（只在首次注册时需要）")
		state    = flag.String("state", "/etc/router-re/cloud-agent.json", "设备身份文件")
		local    = flag.String("local", "http://127.0.0.1:8080", "本机 mgmtd 地址")
		user     = flag.String("local-user", envOr("MGM_ADMIN_USER", "admin"), "本机 mgmtd 账号")
		name     = flag.String("name", "", "设备显示名（默认取 hostname）")
		localPass = flag.String("local-pass", envOr("MGM_ADMIN_PASSWORD", "admin123"), "本机 mgmtd 密码")
	model    = flag.String("model", "router-re", "设备型号")
		version  = flag.String("version", "", "固件版本（可选）")
	)
	flag.Parse()

	if *localPass == "" {
		log.Fatal("必须设置 MGM_ADMIN_PASSWORD（本机 mgmtd 的密码，仅本地使用）")
	}
	if *name == "" {
		h, err := os.Hostname()
		if err != nil || h == "" {
			h = "router"
		}
		*name = h
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-ch
		log.Printf("router-agent: 收到退出信号")
		cancel()
	}()

	ag, err := cloud.NewAgent(ctx, cloud.AgentConfig{
		CloudURL:    *cloudURL,
		EnrollToken: *token,
		StatePath:   *state,
		LocalBase:   *local,
		LocalUser:   *user,
		LocalPass:   *localPass,
		Name:        *name,
		Model:       *model,
		Version:     *version,
	})
	if err != nil {
		log.Fatalf("router-agent: %v", err)
	}
	log.Printf("router-agent: 启动，云端 %s，本机 %s", *cloudURL, *local)
	for {
		ag.Run(ctx)
		if ctx.Err() != nil {
			return
		}
		// Run 正常只在 ctx 结束时返回；这里兜底避免忙循环。
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
