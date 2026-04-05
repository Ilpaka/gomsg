package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"goflow/backend/internal/service"
	httptransport "goflow/backend/internal/transport/http"
	wstransport "goflow/backend/internal/transport/ws"
)

type App struct {
	container *Container
	server    *http.Server
}

func New(c *Container) (*App, error) {
	if c == nil || c.Config == nil || c.Logger == nil {
		return nil, errors.New("app: invalid container")
	}

	mux := http.NewServeMux()
	var authSvc *service.AuthService
	if c.Users != nil && c.Sessions != nil {
		authSvc = service.NewAuthService(c.Users, c.Sessions, c.Config)
	}
	var userSvc *service.UserService
	if c.Users != nil {
		userSvc = service.NewUserService(c.Users)
	}
	var chatSvc *service.ChatService
	if c.Users != nil && c.Chats != nil {
		chatSvc = service.NewChatService(c.Chats, c.Users)
	}
	var msgSvc *service.MessageService
	if c.Messages != nil && c.Chats != nil {
		msgSvc = service.NewMessageService(c.Messages, c.Chats)
	}
	var wsHTTP http.Handler
	if msgSvc != nil && c.Chats != nil {
		hub := wstransport.NewHub()
		bc := wstransport.NewBroadcaster(hub, c.Chats)
		wsSvc := service.NewWSService(msgSvc, c.Chats, bc)
		wsHTTP = wstransport.NewHandler(hub, wsSvc, []byte(strings.TrimSpace(c.Config.JWT.Secret)), c.Logger)
	}
	httptransport.Register(mux, &httptransport.Deps{
		Config:   c.Config,
		Logger:   c.Logger,
		Auth:     authSvc,
		Users:    userSvc,
		Chats:    chatSvc,
		Messages: msgSvc,
		WS:       wsHTTP,
	})

	addr := fmt.Sprintf(":%s", c.Config.App.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &App{
		container: c,
		server:    srv,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	log := a.container.Logger
	log.Info("http server starting", "addr", a.server.Addr)

	errCh := make(chan error, 1)
	go func() {
		err := a.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		if err := <-errCh; err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		return err
	}
}
