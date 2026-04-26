package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"goflow/backend/internal/kafka"
	"goflow/backend/internal/service"
	httptransport "goflow/backend/internal/transport/http"
	httphandler "goflow/backend/internal/transport/http/handler"
	"goflow/backend/internal/transport/http/middleware"
	wstransport "goflow/backend/internal/transport/ws"
	"goflow/backend/internal/worker"
)

type App struct {
	container *Container
	server      *http.Server
	hub         *wstransport.Hub
	bgCancel    context.CancelFunc
	bgWG        sync.WaitGroup
	kafkaProd   *kafka.Producer
}

func New(c *Container) (*App, error) {
	if c == nil || c.Config == nil || c.Logger == nil {
		return nil, errors.New("app: invalid container")
	}

	out := &App{container: c}

	rl := c.Config.RateLimit
	registerLim := middleware.NewIPRateLimiter(c.Logger, "register", rl.RegisterPerMinute, 0)
	loginLim := middleware.NewIPRateLimiter(c.Logger, "login", rl.LoginPerMinute, 0)
	msgSendLim := middleware.NewIPRateLimiter(c.Logger, "message_send", rl.MessageSendPerMinute, 0)
	wsConnectLim := middleware.NewIPRateLimiter(c.Logger, "ws_connect", rl.WSConnectPerMinute, 0)

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
	if c.Messages != nil && c.Chats != nil && c.MessagesWriter != nil {
		msgSvc = service.NewMessageService(c.Messages, c.Chats, c.MessagesWriter, c.Metrics)
	}

	var hub *wstransport.Hub
	var wsHTTP http.Handler
	var wsTicketHTTP http.Handler
	var relayRedis func(context.Context) error

	bgCtx, bgCancel := context.WithCancel(context.Background())
	out.bgCancel = bgCancel

	var kafkaProd *kafka.Producer

	if msgSvc != nil && c.Chats != nil {
		hub = wstransport.NewHub(c.Metrics)
		bc := wstransport.NewBroadcaster(hub, c.Chats, c.PubSub)

		var presenceSvc *service.PresenceService
		if c.Presence != nil {
			presenceSvc = service.NewPresenceService(c.Presence)
		}

		var presenceHook wstransport.PresenceNotifier
		if presenceSvc != nil {
			presenceHook = presenceSvc
		}

		wsSvc := service.NewWSService(msgSvc, c.Chats, c.Typing, presenceHook, bc)
		if c.WSTicket != nil {
			wsHTTP = wstransport.NewHandler(hub, wsSvc, c.WSTicket, c.Config.WS.AllowedOrigins, c.Logger, presenceHook, c.Metrics)
			ticketTTL := time.Duration(c.Config.WS.TicketTTLSeconds) * time.Second
			ticketSvc := service.NewWSTicketService(c.WSTicket, ticketTTL)
			wTick := httphandler.NewWSTicket(ticketSvc, c.Logger)
			secret := []byte(strings.TrimSpace(c.Config.JWT.Secret))
			wsTicketHTTP = httptransport.WithRateLimit(wsConnectLim, middleware.RequireBearerAuth(secret, c.Logger)(http.HandlerFunc(wTick.Issue)))
		}

		if c.PubSub != nil {
			relayRedis = func(ctx context.Context) error {
				return bc.StartRedisRelay(ctx)
			}
		}

		if c.Outbox != nil {
			relay := &worker.OutboxRelay{
				Outbox:   c.Outbox,
				Fallback: bc,
				Log:      c.Logger,
				UseKafka: c.Config.Kafka.Enabled,
				Metrics:  c.Metrics,
			}
			if c.Config.Kafka.Enabled {
				brokers := splitBrokers(c.Config.Kafka.Brokers)
				kafkaProd = kafka.NewProducer(brokers, strings.TrimSpace(c.Config.Kafka.Topic))
				relay.Kafka = kafkaProd
			}
			out.bgWG.Add(1)
			go func() {
				defer out.bgWG.Done()
				relay.Run(bgCtx)
			}()

			if c.Config.Kafka.Enabled && kafkaProd != nil {
				wsGroup := kafkaWSFanoutConsumerGroup(c.Config.Kafka.ConsumerGroup)
				c.Logger.Info("kafka ws fanout consumer", "consumer_group", wsGroup, "topic", strings.TrimSpace(c.Config.Kafka.Topic))
				cons := &worker.KafkaWSConsumer{
					Brokers: splitBrokers(c.Config.Kafka.Brokers),
					Topic:   strings.TrimSpace(c.Config.Kafka.Topic),
					GroupID: wsGroup,
					BC:      bc,
					Log:     c.Logger,
					Metrics: c.Metrics,
				}
				out.bgWG.Add(1)
				go func() {
					defer out.bgWG.Done()
					_ = cons.Run(bgCtx)
				}()
			}
		}
	}

	if relayRedis != nil {
		out.bgWG.Add(1)
		go func() {
			defer out.bgWG.Done()
			_ = relayRedis(bgCtx)
		}()
	}

	httptransport.Register(mux, &httptransport.Deps{
		Config:            c.Config,
		Logger:            c.Logger,
		Metrics:           c.Metrics,
		Auth:              authSvc,
		Users:             userSvc,
		Chats:             chatSvc,
		Messages:          msgSvc,
		WS:                wsHTTP,
		WSTicket:          wsTicketHTTP,
		LimitRegister:     registerLim,
		LimitLogin:        loginLim,
		LimitMessageSend:  msgSendLim,
		LimitWSConnect:    wsConnectLim,
	})

	addr := fmt.Sprintf(":%s", c.Config.App.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           httptransport.Chain(mux, c.Logger, c.Metrics),
		ReadHeaderTimeout: 10 * time.Second,
	}

	out.server = srv
	out.hub = hub
	out.kafkaProd = kafkaProd
	return out, nil
}

func splitBrokers(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
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
		a.bgCancel()

		done := make(chan struct{})
		go func() {
			a.bgWG.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(12 * time.Second):
			log.Warn("background workers shutdown wait timed out")
		}

		if a.kafkaProd != nil {
			_ = a.kafkaProd.Close()
		}
		if a.hub != nil {
			a.hub.CloseAll()
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		if err := <-errCh; err != nil {
			return err
		}
		log.Info("http server stopped")
		return nil
	case err := <-errCh:
		return err
	}
}
