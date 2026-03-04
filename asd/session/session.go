package session

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pb "telegram_project/asd/api/grpc/proto"
	"telegram_project/asd/config"
)

// Session представляет изолированное Telegram соединение
type Session struct {
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	client  *telegram.Client
	api     *tg.Client
	updates *updates.Manager
	appID   int
	appHash string
	logger  *zap.Logger

	ready       bool
	readyMu     sync.RWMutex
	subMu       sync.RWMutex
	subscribers map[chan<- *pb.MessageUpdate]struct{}

	done chan struct{}
}

// NewSession создает новый экземпляр сессии
func NewSession(ctx context.Context, id string, cfg *config.Config, cancel context.CancelFunc) (*Session, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("create logger: %w", err)
	}

	client := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		Logger: logger,
	})

	updatesManager := updates.New(updates.Config{
		Logger: logger,
	})

	s := &Session{
		id:          id,
		ctx:         ctx,
		cancel:      cancel,
		client:      client,
		appID:       cfg.AppID,
		appHash:     cfg.AppHash,
		logger:      logger,
		updates:     updatesManager,
		subscribers: make(map[chan<- *pb.MessageUpdate]struct{}),
		done:        make(chan struct{}),
	}
	return s, nil
}

// userAuthenticator реализует интерфейс auth.UserAuthenticator для QR кода
type userAuthenticator struct{}

func (a userAuthenticator) Phone(_ context.Context) (string, error) {
	return "", nil
}

func (a userAuthenticator) Password(_ context.Context) (string, error) {
	return "", nil
}

func (a userAuthenticator) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

func (a userAuthenticator) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, nil
}

func (a userAuthenticator) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	return "", nil
}

// Run запускает клиента и возвращает QR код
func (s *Session) Run(parentCtx context.Context) (string, error) {
	type qrResult struct {
		url string
		err error
	}
	qrCh := make(chan qrResult, 1)

	go func() {
		defer close(s.done)

		err := s.client.Run(s.ctx, func(clientCtx context.Context) error {
			// Сохраняем API клиент
			s.api = s.client.API()

			// Получаем QR код с помощью правильного запроса
			token, err := s.api.AuthExportLoginToken(clientCtx, &tg.AuthExportLoginTokenRequest{
				APIID:     s.appID,
				APIHash:   s.appHash,
				ExceptIDs: []int64{}, // пустой список
			})
			if err != nil {
				return fmt.Errorf("export login token: %w", err)
			}

			// Преобразуем токен в URL для QR
			var qrURL string
			var loginToken []byte

			switch t := token.(type) {
			case *tg.AuthLoginToken:
				loginToken = t.Token
				// Формируем URL для QR кода
				qrURL = fmt.Sprintf("tg://login?token=%s", hex.EncodeToString(t.Token))
			case *tg.AuthLoginTokenMigrateTo:
				loginToken = t.Token
				qrURL = fmt.Sprintf("tg://login?token=%s&dc=%d", hex.EncodeToString(t.Token), t.DCID)
			default:
				return fmt.Errorf("unexpected token type: %T", token)
			}

			qrCh <- qrResult{url: qrURL, err: nil}

			// Запускаем цикл ожидания авторизации
			// В реальном приложении здесь нужно ожидать сканирования QR кода
			// и периодически проверять статус авторизации
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			authComplete := false
			for !authComplete {
				select {
				case <-clientCtx.Done():
					return clientCtx.Err()
				case <-ticker.C:
					// Проверяем статус авторизации
					_, err := s.api.AuthAcceptLoginToken(clientCtx, loginToken)
					if err == nil {
						authComplete = true
						break
					}

					// Если ошибка не критичная, продолжаем ждать
					if strings.Contains(err.Error(), "AUTH_TOKEN_ACCEPTED") {
						authComplete = true
						break
					}
				}
			}

			s.readyMu.Lock()
			s.ready = true
			s.readyMu.Unlock()
			s.logger.Info("session authorized", zap.String("id", s.id))

			// Создаем обработчик обновлений
			dispatcher := tg.NewUpdateDispatcher()
			dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewMessage) error {
				return s.onNewMessage(ctx, entities, update)
			})

			// Запускаем updates manager
			go func() {
				// Получаем информацию о текущем пользователе
				user, err := s.client.Self(clientCtx)
				if err != nil {
					s.logger.Error("failed to get current user", zap.Error(err))
					return
				}

				// Запускаем обработку обновлений
				if err := s.updates.Run(clientCtx, s.api, user.ID, updates.AuthOptions{
					IsBot: false,
				}); err != nil {
					s.logger.Error("updates manager stopped", zap.Error(err))
				}
			}()

			<-clientCtx.Done()
			return nil
		})

		if err != nil {
			select {
			case qrCh <- qrResult{err: err}:
			default:
			}
			s.logger.Error("session client error", zap.String("id", s.id), zap.Error(err))
		}
	}()

	select {
	case res := <-qrCh:
		return res.url, res.err
	case <-parentCtx.Done():
		return "", parentCtx.Err()
	}
}

// onNewMessage обрабатывает входящие сообщения
func (s *Session) onNewMessage(ctx context.Context, entities tg.Entities, update *tg.UpdateNewMessage) error {
	msg, ok := update.Message.(*tg.Message)
	if !ok || msg.Out {
		return nil
	}

	from := s.getSenderName(entities, msg)
	if from == "" {
		from = "unknown"
	}

	// Преобразуем int в int64 для timestamp
	msgID := int64(msg.ID)
	msgText := msg.Message
	timestamp := int64(msg.Date)

	pbMsg := &pb.MessageUpdate{
		MessageId: &msgID,
		From:      &from,
		Text:      &msgText,
		Timestamp: &timestamp,
	}

	s.broadcast(pbMsg)
	return nil
}

// getSenderName получает имя отправителя
func (s *Session) getSenderName(entities tg.Entities, msg *tg.Message) string {
	if msg.FromID == nil {
		return ""
	}

	switch peer := msg.FromID.(type) {
	case *tg.PeerUser:
		if user, ok := entities.Users[peer.UserID]; ok {
			if user.Username != "" {
				return "@" + user.Username
			}
			if user.FirstName != "" {
				return user.FirstName
			}
			return fmt.Sprintf("user %d", user.ID)
		}
	case *tg.PeerChat:
		if chat, ok := entities.Chats[peer.ChatID]; ok {
			return chat.Title
		}
	case *tg.PeerChannel:
		if channel, ok := entities.Channels[peer.ChannelID]; ok {
			return channel.Title
		}
	}
	return ""
}

// broadcast рассылает сообщение подписчикам
func (s *Session) broadcast(msg *pb.MessageUpdate) {
	s.subMu.RLock()
	subs := make([]chan<- *pb.MessageUpdate, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.subMu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- msg:
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Subscribe подписывает gRPC поток на сообщения
func (s *Session) Subscribe(stream grpc.ServerStream) error {
	ch := make(chan *pb.MessageUpdate, 10)
	s.subMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subMu.Unlock()

	defer func() {
		s.subMu.Lock()
		delete(s.subscribers, ch)
		close(ch)
		s.subMu.Unlock()
	}()

	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case msg := <-ch:
			if err := stream.SendMsg(msg); err != nil {
				return err
			}
		}
	}
}

// SendMessage отправляет сообщение
func (s *Session) SendMessage(ctx context.Context, peer, text string) (int64, error) {
	// Ожидаем готовности сессии
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		s.readyMu.RLock()
		ready := s.ready
		s.readyMu.RUnlock()
		if ready {
			break
		}
		select {
		case <-readyCtx.Done():
			return 0, errors.New("session not ready within timeout")
		case <-ticker.C:
		}
	}

	// Убираем @ из username
	username := strings.TrimPrefix(peer, "@")

	// Разрешаем username
	resolved, err := s.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: username,
	})
	if err != nil {
		return 0, fmt.Errorf("resolve username: %w", err)
	}

	// Определяем тип получателя
	var peerInput tg.InputPeerClass

	if len(resolved.Users) > 0 {
		// Это пользователь
		if user, ok := resolved.Users[0].(*tg.User); ok {
			peerInput = &tg.InputPeerUser{
				UserID:     user.ID,
				AccessHash: user.AccessHash,
			}
		} else {
			return 0, errors.New("failed to cast user")
		}
	} else if len(resolved.Chats) > 0 {
		// Это чат или канал
		if chat, ok := resolved.Chats[0].(*tg.Chat); ok {
			peerInput = &tg.InputPeerChat{
				ChatID: chat.ID,
			}
		} else if channel, ok := resolved.Chats[0].(*tg.Channel); ok {
			peerInput = &tg.InputPeerChannel{
				ChannelID:  channel.ID,
				AccessHash: channel.AccessHash,
			}
		} else {
			return 0, errors.New("failed to cast chat/channel")
		}
	} else {
		return 0, errors.New("peer not found")
	}

	// Отправляем сообщение
	randomID := rand.Int63()
	sentMsg, err := s.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peerInput,
		Message:  text,
		RandomID: randomID,
	})
	if err != nil {
		return 0, fmt.Errorf("send message: %w", err)
	}

	// Извлекаем ID сообщения из ответа
	if updates, ok := sentMsg.(*tg.Updates); ok {
		for _, update := range updates.Updates {
			if u, ok := update.(*tg.UpdateNewMessage); ok {
				if m, ok := u.Message.(*tg.Message); ok {
					return int64(m.ID), nil
				}
			}
		}
	}

	return 0, errors.New("could not get message ID")
}

// Stop останавливает сессию
func (s *Session) Stop() {
	s.readyMu.RLock()
	ready := s.ready
	s.readyMu.RUnlock()

	if ready && s.api != nil {
		// Выполняем logout в фоне
		go func() {
			logoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := s.api.AuthLogOut(logoutCtx); err != nil {
				s.logger.Error("logout failed", zap.String("id", s.id), zap.Error(err))
			}
		}()
	}

	s.cancel()
	<-s.done
	s.logger.Info("session stopped", zap.String("id", s.id))
}
