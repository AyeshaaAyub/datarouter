package main

import (
	"context"
	"net/http"

	"github.com/AyeshaaAyub/datarouter/pkg/classifier"
	"github.com/AyeshaaAyub/datarouter/proto" // Generated

	"google.golang.org/grpc"
)

type Router struct {
	handlers map[classifier.DBType]proto.StorageHandlerClient
}

func NewRouter() *Router {
	// Connect to handlers via gRPC (addresses from env/config)
	return &Router{
		handlers: map[classifier.DBType]proto.StorageHandlerClient{
			classifier.DBPostgres: connectGRPC("localhost:50051"),
			// Add others
		},
	}
}

func connectGRPC(addr string) proto.StorageHandlerClient {
	conn, _ := grpc.Dial(addr, grpc.WithInsecure())
	return proto.NewStorageHandlerClient(conn)
}

// HTTP Handler for ingestion
func (r *Router) storeHandler(w http.ResponseWriter, req *http.Request) {
	var data map[string]interface{}
	// Parse JSON body into data

	dbType := classifier.Classify(data)
	handler, ok := r.handlers[dbType]
	if !ok {
		http.Error(w, "No handler", http.StatusInternalServerError)
		return
	}

	// resp, err := handler.Store(context.Background(), &proto.StoreRequest{
	_, err := handler.Store(context.Background(), &proto.StoreRequest{
		Data:       marshalData(data), // Convert to proto
		Collection: "default",         // From query param
	})
	if err != nil {
		// Handle error
	}
	// Respond with resp.ID and dbType
}

func main() {
	router := NewRouter()
	http.HandleFunc("/store", router.storeHandler)
	http.ListenAndServe(":8080", nil)
}
