// Command server runs entire backend
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/loqui-chat/loqui-backend/internal/api"
	"github.com/loqui-chat/loqui-backend/internal/auth"
	"github.com/loqui-chat/loqui-backend/internal/channel"
	"github.com/loqui-chat/loqui-backend/internal/config"
	"github.com/loqui-chat/loqui-backend/internal/db"
	"github.com/loqui-chat/loqui-backend/internal/gateway"
	"github.com/loqui-chat/loqui-backend/internal/logging"
	"github.com/loqui-chat/loqui-backend/internal/message"
	"github.com/loqui-chat/loqui-backend/internal/snowflake"
	"github.com/loqui-chat/loqui-backend/internal/user"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		println("config error:", err.Error())
		os.Exit(1)
	}

	log := logging.New(cfg.LogLevel, cfg.LogFormat)

	gen, err := snowflake.New(cfg.NodeID)
	if err != nil {
		log.Error("snowflake init failed", "err", err)
		os.Exit(1)
	}

	priv, ephemeral, err := auth.LoadKeyOrEphermal(cfg.JWTPrivateKeyPath)
	if err != nil {
		log.Error("load jwt key failed", "err", err)
		os.Exit(1)
	}
	if ephemeral {
		log.Warn("not JWT_PRIVATE_KEY_FILE set, using an ephemeral key. Token will not survice a restart")
	}
	issuer := auth.NewIssuer(priv, cfg.AccessTTL, cfg.RefreshTTL)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	users := user.NewStore(pool, gen)
	channels := channel.NewStore(pool, gen)
	messages := message.NewStore(pool, gen)
	gw := gateway.New(issuer, channels, log)
	server := api.NewServer(log, pool, users, channels, messages, gw, issuer)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("sever listening", "addr", cfg.HTTPAddr, "env", cfg.Env, "node", cfg.NodeID)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutdown requested")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "err", err)
	}
	log.Info("stopped")
}
