package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	grpcapi "telegram_project/asd/api/grpc"
	pb "telegram_project/asd/api/grpc/proto"
	"telegram_project/asd/config"
	"telegram_project/asd/session"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	sessionMgr := session.NewManager(cfg)

	grpcServer := grpc.NewServer()
	pb.RegisterTelegramServiceServer(grpcServer, grpcapi.NewServer(sessionMgr))

	lis, err := net.Listen("tcp", cfg.PortStr())

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Printf("gRPC server listening on :%s", cfg.PortStr())

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("grpc serve error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	grpcServer.GracefulStop()
	log.Println("server stopped")
}
