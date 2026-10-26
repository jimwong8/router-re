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
	cfg := server.Config{
		Addr:      addr,
		SecretKey: []byte(os.Getenv("MGM_SESSION_SECRET")),
	}
	if len(cfg.SecretKey) == 0 {
		log.Fatal("MGM_SESSION_SECRET must be set (32+ bytes)")
	}
	s := server.New(cfg)
	log.Printf("mgmtd listening on %s", cfg.Addr)
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
