package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		drivers:    make(map[string]*Client),
		customers:  make(map[string][]*Client),
		deliveries: make(map[string][]*Client),
	}
}
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)
		case client := <-h.unregister:
			h.unregisterClient(client)
		case message := <-h.broadcast:
			h.broadcastToAll(message)
		}
	}
}

func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[client] = true

	switch client.userType {
	case "driver":
		h.drivers[client.userID] = client
		log.Printf("Driver %s connected", client.userID)
	case "customer":
		h.customers[client.userID] = append(h.customers[client.userID], client)
		if client.deliveryID != "" {
			h.deliveries[client.deliveryID] = append(h.deliveries[client.deliveryID], client)
		}
		log.Printf("Customer %s connected, tracking delivery %s", client.userID, client.deliveryID)
	}
}
func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)

		switch client.userType {
		case "driver":
			delete(h.drivers, client.userID)
		case "customer":
			h.removeCustomerClient(client)
		}

		log.Printf("Client disconnected: %s (%s)", client.userID, client.userType)
	}
}

func (h *Hub) removeCustomerClient(client *Client) {

	if clients, ok := h.customers[client.userID]; ok {
		for i, c := range clients {
			if c == client {
				h.customers[client.userID] = append(clients[:i], clients[i+1:]...)
				break
			}
		}
	}
	if client.deliveryID != "" {
		if clients, ok := h.deliveries[client.deliveryID]; ok {
			for i, c := range clients {
				if c == client {
					h.deliveries[client.deliveryID] = append(clients[:i], clients[i+1:]...)
					break
				}
			}
		}
	}
}
func (h *Hub) broadcastToAll(message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {

		return true
	},
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	userID := r.URL.Query().Get("user_id")
	userType := r.URL.Query().Get("user_type")
	deliveryID := r.URL.Query().Get("delivery_id")

	if userID == "" || userType == "" {
		conn.Close()
		return
	}

	client := &Client{
		hub:        hub,
		conn:       conn,
		send:       make(chan []byte, 256),
		userID:     userID,
		userType:   userType,
		deliveryID: deliveryID,
	}

	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}
func main() {

	db, err := initDatabase()
	if err != nil {
		log.Fatal("Database initialization failed:", err)
	}
	defer db.Close()

	hub := newHub()
	tracker := NewDeliveryTracker(db, hub)

	app = &App{
		db:      db,
		hub:     hub,
		tracker: tracker,
	}

	if err := tracker.loadActiveDeliveries(); err != nil {
		log.Fatal("Failed to load active deliveries:", err)
	}

	go hub.run()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	http.Handle("/", http.FileServer(http.Dir("./frontend")))

	http.HandleFunc("/api/deliveries", handleDeliveries)
	http.HandleFunc("/api/deliveries/status", handleStatusUpdate)

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
func handleDeliveries(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleStatusUpdate(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
