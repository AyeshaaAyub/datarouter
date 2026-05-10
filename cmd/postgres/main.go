package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/AyeshaaAyub/datarouter/proto"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PostgresHandler struct {
	proto.UnimplementedStorageHandlerServer
	db *sql.DB
}

func (h *PostgresHandler) Store(ctx context.Context, req *proto.StoreRequest) (*proto.StoreResponse, error) {
	if req.GetCollection() == "" {
		req.Collection = "default"
	}

	doc, err := decodeProtoData(req.Data)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid payload: %v", err)
	}

	payload, err := json.Marshal(doc)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal payload: %v", err)
	}

	var id int
	err = h.db.QueryRowContext(ctx,
		`INSERT INTO data_router (collection, payload) VALUES ($1, $2) RETURNING id`,
		req.GetCollection(), payload).Scan(&id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to insert payload: %v", err)
	}

	return &proto.StoreResponse{Id: fmt.Sprintf("%d", id), DbType: "postgres"}, nil
}

func (h *PostgresHandler) Retrieve(ctx context.Context, req *proto.RetrieveRequest) (*proto.RetrieveResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if req.GetCollection() == "" {
		req.Collection = "default"
	}

	var payload []byte
	err := h.db.QueryRowContext(ctx,
		`SELECT payload FROM data_router WHERE id = $1 AND collection = $2`,
		req.GetId(), req.GetCollection()).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "record not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve payload: %v", err)
	}

	doc := make(map[string]interface{})
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to decode payload: %v", err)
	}

	return &proto.RetrieveResponse{Data: convertToProtoData(doc), DbType: "postgres"}, nil
}

func decodeProtoData(data *proto.Data) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(data.GetFields()))
	for key, raw := range data.GetFields() {
		var value interface{}
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

func convertToProtoData(data map[string]interface{}) *proto.Data {
	fields := make(map[string][]byte, len(data))
	for key, value := range data {
		if bytes, err := json.Marshal(value); err == nil {
			fields[key] = bytes
		}
	}
	return &proto.Data{Fields: fields}
}

func main() {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "user=postgres password=password dbname=postgres sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open postgres: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping postgres: %v", err)
	}

	if _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS data_router (
            id SERIAL PRIMARY KEY,
            collection TEXT NOT NULL,
            payload JSONB NOT NULL,
            created_at TIMESTAMPTZ NOT NULL DEFAULT now()
        )`); err != nil {
		log.Fatalf("failed to create table: %v", err)
	}

	handler := &PostgresHandler{db: db}
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	proto.RegisterStorageHandlerServer(s, handler)
	log.Printf("postgres handler listening on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("grpc server failed: %v", err)
	}
}
