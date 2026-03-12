package main

import (
	"context"
	"database/sql"
	"net"

	"yourproject/proto/storage"

	_ "github.com/lib/pq"
	"google.golang.org/grpc"
)

type PostgresHandler struct {
	storage.UnimplementedStorageHandlerServer
	db *sql.DB
}

func (h *PostgresHandler) Store(ctx context.Context, req *storage.StoreRequest) (*storage.StoreResponse, error) {
	// Unmarshal req.Data, insert into table, return ID
	return &storage.StoreResponse{ID: "pg-123", DBType: "postgres"}, nil
}

func main() {
	db, _ := sql.Open("postgres", "conn-string")
	handler := &PostgresHandler{db: db}

	lis, _ := net.Listen("tcp", ":50051")
	s := grpc.NewServer()
	storage.RegisterStorageHandlerServer(s, handler)
	s.Serve(lis)
}
