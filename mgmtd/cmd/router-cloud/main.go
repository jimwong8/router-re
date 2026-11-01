// router-cloud 是 router-re 的远程管理中心服务端（部署在公网，例如 https://1.1ai.best）。
//
// 只做三件事：设备注册、反向隧道、白名单命令下发 + 审计。
// 不托管设备配置、不存设备口令——设备侧始终是"被叫方"。
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/cloud"
	"golang.org/x/crypto/acme/autocert"
)

func main() {
	var (
		addr      = flag.String("addr", "127.0.0.1:8444", "监听地址（生产放反代后面）")
		storePath = flag.String("store", "/var/lib/router-cloud/state.json", "状态文件")
		certFile  = flag.String("tls-cert", "", "TLS 证书（留空则按明文 HTTP 监听，仅供反代后面的内网端口）")
		keyFile   = flag.String("tls-key", "", "TLS 私钥")
		adminUser = flag.String("admin-user", envOr("ROUTER_CLOUD_ADMIN_USER", "admin"), "控制台账号")
		timeout   = flag.Duration("exec-timeout", 15*time.Second, "单条远端命令超时")

		releaseDir = flag.String("release-dir", envOr("ROUTER_CLOUD_RELEASE_DIR", "/var/lib/router-cloud/releases"), "在线升级发布包目录")
		publicBase = flag.String("public-base", envOr("ROUTER_CLOUD_PUBLIC_BASE", ""), "设备可访问的云端根地址，留空则按请求头推断")
		acmeHost   = flag.String("acme-host", os.Getenv("ROUTER_CLOUD_ACME_HOST"), "用 Let's Encrypt 自动签发证书的域名（TLS-ALPN-01，逗号分隔）")
		acmeEmail  = flag.String("acme-email", os.Getenv("ROUTER_CLOUD_ACME_EMAIL"), "ACME 联系邮箱")
		acmeCache  = flag.String("acme-cache", "/var/lib/router-cloud/certs", "证书缓存目录")
	)
	flag.Parse()

	pass := os.Getenv("ROUTER_CLOUD_ADMIN_PASSWORD")
	if pass == "" {
		log.Fatal("必须设置 ROUTER_CLOUD_ADMIN_PASSWORD")
	}
	secret := []byte(os.Getenv("ROUTER_CLOUD_SESSION_SECRET"))
	if len(secret) == 0 {
		// 没有会话密钥就随机生成：重启后所有会话失效，但不会静默用弱密钥。
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			log.Fatalf("生成会话密钥失败: %v", err)
		}
		log.Printf("警告: 未设置 ROUTER_CLOUD_SESSION_SECRET，本次启动随机生成（重启后会话全部失效）")
	}

	svc, err := cloud.NewService(cloud.Config{
		StorePath:     *storePath,
		AdminUser:     *adminUser,
		AdminPassword: pass,
		JWTSecret:     secret,
		ExecTimeout:   *timeout,
		ReleaseDir:    *releaseDir,
		PublicBase:    *publicBase,
	})
	if err != nil {
		log.Fatalf("router-cloud: %v", err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           svc.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// 隧道是长连接，不能设 WriteTimeout；读超时也不设（流式响应）。
	}

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		log.Printf("router-cloud: 收到退出信号，落盘后退出")
		_ = svc.Store().Save()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Printf("router-cloud listening on %s (store=%s, acme=%q)", *addr, *storePath, *acmeHost)
	var serveErr error
	switch {
	case *acmeHost != "":
		// 证书走 TLS-ALPN-01：续期也在这条 443 连接上完成，不需要停服务、不需要 80 端口
		// （本机 80 被运营商挡了，HTTP-01 用不了）。
		if err := os.MkdirAll(*acmeCache, 0o700); err != nil {
			log.Fatalf("创建证书缓存目录失败: %v", err)
		}
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(strings.Split(*acmeHost, ",")...),
			Cache:      autocert.DirCache(*acmeCache),
			Email:      *acmeEmail,
		}
		srv.TLSConfig = m.TLSConfig()
		serveErr = srv.ListenAndServeTLS("", "")
	case *certFile != "" && *keyFile != "":
		serveErr = srv.ListenAndServeTLS(*certFile, *keyFile)
	default:
		serveErr = srv.ListenAndServe()
	}
	if serveErr != nil && serveErr != http.ErrServerClosed {
		log.Fatalf("router-cloud: %v", serveErr)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
