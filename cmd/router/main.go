package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/AyeshaaAyub/datarouter/pkg/classifier"
	"github.com/AyeshaaAyub/datarouter/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Router struct {
	handlers       map[classifier.DBType]proto.StorageHandlerClient
	defaultHandler proto.StorageHandlerClient
}

func NewRouter() *Router {
	r := &Router{handlers: make(map[classifier.DBType]proto.StorageHandlerClient)}

	addHandler := func(dbType classifier.DBType, addr string) {
		client, err := connectGRPC(addr)
		if err != nil {
			log.Printf("warning: could not connect to %s handler at %s: %v", dbType, addr, err)
			return
		}
		r.handlers[dbType] = client
		if dbType == classifier.DBPostgres {
			r.defaultHandler = client
		}
	}

	addHandler(classifier.DBPostgres, getEnv("POSTGRES_GPRC_ADDR", "localhost:50051"))
	addHandler(classifier.DBMongo, getEnv("MONGO_GPRC_ADDR", "localhost:50052"))
	addHandler(classifier.DBRedis, getEnv("REDIS_GPRC_ADDR", "localhost:50053"))
	addHandler(classifier.DBInflux, getEnv("INFLUX_GPRC_ADDR", "localhost:50054"))

	return r
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func connectGRPC(addr string) (proto.StorageHandlerClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return proto.NewStorageHandlerClient(conn), nil
}

func (r *Router) storeHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}
	if len(data) == 0 {
		http.Error(w, "empty payload", http.StatusBadRequest)
		return
	}

	dbType := classifier.Classify(data)
	handler, ok := r.handlers[dbType]
	if !ok {
		if r.defaultHandler != nil {
			log.Printf("no handler registered for %s, falling back to Postgres", dbType)
			handler = r.defaultHandler
		} else {
			http.Error(w, "no available storage handler", http.StatusInternalServerError)
			return
		}
	}

	collection := req.URL.Query().Get("collection")
	if collection == "" {
		collection = "default"
	}

	reqProto := &proto.StoreRequest{
		Data:       convertToProtoData(data),
		Collection: collection,
	}

	resp, err := handler.Store(context.Background(), reqProto)
	if err != nil {
		log.Printf("storage handler error: %v", err)
		http.Error(w, fmt.Sprintf("storage error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":      resp.GetId(),
		"db_type": resp.GetDbType(),
	})
}

func (r *Router) retrieveHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dbType, id, collection, err := parseRetrieveRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	handler, ok := r.handlers[classifier.DBType(dbType)]
	if !ok {
		http.Error(w, fmt.Sprintf("no handler registered for db_type=%s", dbType), http.StatusBadRequest)
		return
	}

	resp, err := handler.Retrieve(context.Background(), &proto.RetrieveRequest{Id: id, Collection: collection})
	if err != nil {
		log.Printf("retrieve handler error: %v", err)
		if status.Code(err) == codes.NotFound {
			http.Error(w, "record not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("retrieve error: %v", err), http.StatusInternalServerError)
		return
	}

	result, err := convertFromProtoData(resp.Data)
	if err != nil {
		log.Printf("failed to convert retrieved payload: %v", err)
		http.Error(w, "failed to decode retrieved payload", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      id,
		"db_type": resp.GetDbType(),
		"data":    result,
	})
}

func parseRetrieveRequest(req *http.Request) (dbType, id, collection string, err error) {
	collection = req.URL.Query().Get("collection")
	if collection == "" {
		collection = "default"
	}

	dbType = req.URL.Query().Get("db_type")
	id = req.URL.Query().Get("id")
	if id != "" && dbType != "" {
		return dbType, id, collection, nil
	}

	path := strings.TrimPrefix(req.URL.Path, "/retrieve")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 2 {
		dbType = parts[0]
		id = parts[1]
		return dbType, id, collection, nil
	}
	if len(parts) == 1 && parts[0] != "" {
		id = parts[0]
		if dbType == "" {
			err = fmt.Errorf("missing db_type query parameter")
			return
		}
		return dbType, id, collection, nil
	}
	err = fmt.Errorf("invalid retrieve path; use /retrieve/{db_type}/{id} or /retrieve?id=...&db_type=...")
	return
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

func convertFromProtoData(data *proto.Data) (map[string]interface{}, error) {
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

func main() {
	router := NewRouter()
	http.HandleFunc("/store", router.storeHandler)
	http.HandleFunc("/retrieve/", router.retrieveHandler)
	http.HandleFunc("/retrieve", router.retrieveHandler)
	addr := getEnv("ROUTER_HTTP_ADDR", ":8080")
	log.Printf("router listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("failed to start HTTP server: %v", err)
	}
}
