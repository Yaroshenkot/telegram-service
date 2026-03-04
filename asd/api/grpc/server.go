package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "telegram_project/asd/api/grpc/proto"
	"telegram_project/asd/session"
)

type server struct {
	pb.UnimplementedTelegramServiceServer
	sessionMgr *session.Manager
}

func NewServer(mgr *session.Manager) *server {
	if mgr == nil {
		panic("session manager cannot be nil")
	}
	return &server{sessionMgr: mgr}
}

func (s *server) CreateSession(ctx context.Context, req *pb.CreateSessionRequest) (*pb.CreateSessionResponse, error) {
	sessionID, qrCode, err := s.sessionMgr.Create(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create session failed: %v", err)
	}

	// Возвращаем указатели на строки
	return &pb.CreateSessionResponse{
		SessionId: &sessionID,
		QrCode:    &qrCode,
	}, nil
}

func (s *server) DeleteSession(ctx context.Context, req *pb.DeleteSessionRequest) (*pb.DeleteSessionResponse, error) {
	// Разыменовываем указатель SessionId
	if err := s.sessionMgr.Delete(*req.SessionId); err != nil {
		return nil, status.Errorf(codes.Internal, "delete session failed: %v", err)
	}
	return &pb.DeleteSessionResponse{}, nil
}

func (s *server) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	// Разыменовываем все указатели
	msgID, err := s.sessionMgr.SendMessage(
		*req.SessionId,
		*req.Peer,
		*req.Text,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "send message failed: %v", err)
	}

	// Возвращаем указатель на int64
	return &pb.SendMessageResponse{
		MessageId: &msgID,
	}, nil
}

func (s *server) SubscribeMessages(req *pb.SubscribeMessagesRequest, stream pb.TelegramService_SubscribeMessagesServer) error {
	// Разыменовываем указатель SessionId
	if err := s.sessionMgr.SubscribeMessages(*req.SessionId, stream); err != nil {
		return status.Errorf(codes.Internal, "subscribe failed: %v", err)
	}
	return nil
}
