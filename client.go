package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle incoming messages from drivers (location updates)
		if c.userType == "driver" {
			c.handleDriverMessage(message)
		}
	}
}

//	func (c *Client) handleDriverMessage(message []byte) {
//		panic("unimplemented")
//	}
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleDriverMessage(message []byte) {
	log.Printf("📨 Received message from driver: %s", string(message))

	var wsMsg WebSocketMessage
	if err := json.Unmarshal(message, &wsMsg); err != nil {
		log.Printf("Error unmarshaling message: %v", err)
		return
	}
	log.Printf("📦 Message type: %s", wsMsg.Type)

	switch wsMsg.Type {
	case TypeLocationUpdate:
		log.Printf("📍 Processing location update...")
		var driverLoc DriverLocation
		if err := json.Unmarshal(wsMsg.Data, &driverLoc); err != nil {
			log.Printf("Error unmarshaling location: %v", err)
			return
		}
		log.Printf("🚗 Driver: %s, Delivery: %s, Lat: %.4f, Lon: %.4f",
			driverLoc.DriverID, driverLoc.DeliveryID,
			driverLoc.Location.Lat, driverLoc.Location.Lon)
		// Process the location update
		app.processLocation(&driverLoc)
		app.tracker.processLocationUpdate(&driverLoc)
		log.Printf("✅ Location processed successfully")
	}
}
