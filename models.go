package main

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10

	proximityNear     = 500
	proximityArriving = 100
	proximityArrived  = 50
	defaultSpeed      = 40.0
)

type MessageType string

const (
	TypeLocationUpdate    MessageType = "location_update"
	TypeStatusChange      MessageType = "status_change"
	TypeETAUpdate         MessageType = "eta_update"
	TypeProximityAlert    MessageType = "proximity_alert"
	TypeDeliveryAssigned  MessageType = "delivery_assigned"
	TypeDeliveryCompleted MessageType = "delivery_completed"
)

type DeliveryStatus string

const (
	StatusPending   DeliveryStatus = "pending"
	StatusAssigned  DeliveryStatus = "assigned"
	StatusPickedUp  DeliveryStatus = "picked_up"
	StatusInTransit DeliveryStatus = "in_transit"
	StatusNearby    DeliveryStatus = "nearby"
	StatusArriving  DeliveryStatus = "arriving"
	StatusDelivered DeliveryStatus = "delivered"
	StatusCancelled DeliveryStatus = "cancelled"
)

type Delivery struct {
	ID            string         `json:"id"`
	DriverID      string         `json:"driver_id"`
	CustomerID    string         `json:"customer_id"`
	PickupLat     float64        `json:"pickup_lat"`
	PickupLon     float64        `json:"pickup_lon"`
	DropoffLat    float64        `json:"dropoff_lat"`
	DropoffLon    float64        `json:"dropoff_lon"`
	Status        DeliveryStatus `json:"status"`
	EstimatedTime time.Time      `json:"estimated_time"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
type Location struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Altitude  float64   `json:"altitude"`
	Speed     float64   `json:"speed"`
	Bearing   float64   `json:"bearing"`
	Accuracy  float64   `json:"accuracy"`
	Timestamp time.Time `json:"timestamp"`
}
type DriverLocation struct {
	DriverID   string   `json:"driver_id"`
	DeliveryID string   `json:"delivery_id"`
	Location   Location `json:"location"`
}
type WebSocketMessage struct {
	Type      MessageType     `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}
type ETAUpdate struct {
	DeliveryID       string    `json:"delivery_id"`
	EstimatedArrival time.Time `json:"estimated_arrival"`
	DistanceKm       float64   `json:"distance_km"`
	DurationMinutes  int       `json:"duration_minutes"`
}
type ProximityAlert struct {
	DeliveryID string  `json:"delivery_id"`
	DriverID   string  `json:"driver_id"`
	DistanceM  float64 `json:"distance_m"`
	AlertLevel string  `json:"alert_level"` // "nearby", "arriving", "arrived"
	Message    string  `json:"message"`
}
type StatusChangeEvent struct {
	DeliveryID string         `json:"delivery_id"`
	OldStatus  DeliveryStatus `json:"old_status"`
	NewStatus  DeliveryStatus `json:"new_status"`
	Message    string         `json:"message"`
}
type Client struct {
	hub        *Hub
	conn       *websocket.Conn
	send       chan []byte
	userID     string
	userType   string
	deliveryID string
}
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client

	drivers    map[string]*Client
	customers  map[string][]*Client
	deliveries map[string][]*Client

	mu sync.RWMutex
}
