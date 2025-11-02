package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

type DeliveryTracker struct {
	db            *sql.DB
	hub           *Hub
	deliveries    map[string]*Delivery
	lastLocations map[string]*DriverLocation
	mu            sync.RWMutex
}

func NewDeliveryTracker(db *sql.DB, hub *Hub) *DeliveryTracker {
	return &DeliveryTracker{
		db:            db,
		hub:           hub,
		deliveries:    make(map[string]*Delivery),
		lastLocations: make(map[string]*DriverLocation),
	}
}

func (dt *DeliveryTracker) calculateETA(driverLoc *DriverLocation, delivery *Delivery) *ETAUpdate {
	distance := calculateDistance(
		driverLoc.Location.Lat,
		driverLoc.Location.Lon,
		delivery.DropoffLat,
		delivery.DropoffLon,
	)

	speed := driverLoc.Location.Speed
	if speed < 5 {
		speed = defaultSpeed
	}

	durationHours := distance / speed
	durationMinutes := int(durationHours * 60)

	estimatedArrival := time.Now().Add(time.Duration(durationMinutes) * time.Minute)

	return &ETAUpdate{
		DeliveryID:       delivery.ID,
		EstimatedArrival: estimatedArrival,
		DistanceKm:       distance,
		DurationMinutes:  durationMinutes,
	}
}
func (dt *DeliveryTracker) checkProximityAlerts(delivery *Delivery, driverLoc *DriverLocation, distanceKm float64) {
	distanceM := distanceKm * 1000

	var alertLevel string
	var message string
	var shouldUpdateStatus bool
	var newStatus DeliveryStatus

	if distanceM <= proximityArrived {
		alertLevel = "arrived"
		message = "Driver has arrived at your location"
		shouldUpdateStatus = true
		newStatus = StatusDelivered
	} else if distanceM <= proximityArriving {
		alertLevel = "arriving"
		message = fmt.Sprintf("Driver is arriving (%.0fm away)", distanceM)
		if delivery.Status != StatusArriving {
			shouldUpdateStatus = true
			newStatus = StatusArriving
		}
	} else if distanceM <= proximityNear {
		alertLevel = "nearby"
		message = fmt.Sprintf("Driver is nearby (%.0fm away)", distanceM)
		if delivery.Status != StatusNearby && delivery.Status != StatusArriving {
			shouldUpdateStatus = true
			newStatus = StatusNearby
		}
	}

	if alertLevel != "" {
		alert := ProximityAlert{
			DeliveryID: delivery.ID,
			DriverID:   driverLoc.DriverID,
			DistanceM:  distanceM,
			AlertLevel: alertLevel,
			Message:    message,
		}

		dt.broadcastProximityAlert(delivery.ID, &alert)

		if shouldUpdateStatus {
			dt.updateDeliveryStatus(delivery.ID, newStatus)
		}
	}
}
func (dt *DeliveryTracker) updateDeliveryStatus(deliveryID string, newStatus DeliveryStatus) {
	dt.mu.Lock()
	delivery := dt.deliveries[deliveryID]
	if delivery == nil {
		dt.mu.Unlock()
		return
	}

	oldStatus := delivery.Status
	delivery.Status = newStatus
	delivery.UpdatedAt = time.Now()
	dt.mu.Unlock()

	_, err := dt.db.Exec(
		"UPDATE deliveries SET status = ?, updated_at = ? WHERE id = ?",
		newStatus, delivery.UpdatedAt, deliveryID,
	)
	if err != nil {
		log.Printf("Error updating delivery status: %v", err)
		return
	}

	statusChange := StatusChangeEvent{
		DeliveryID: deliveryID,
		OldStatus:  oldStatus,
		NewStatus:  newStatus,
		Message:    fmt.Sprintf("Delivery status changed to %s", newStatus),
	}

	dt.broadcastStatusChange(deliveryID, &statusChange)
}
func (h *Hub) sendToDeliveryWatchers(deliveryID string, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.deliveries[deliveryID]; ok {
		for _, client := range clients {
			select {
			case client.send <- message:
			default:
				log.Printf("Failed to send to client %s", client.userID)
			}
		}
	}
}

func (dt *DeliveryTracker) broadcastLocationUpdate(deliveryID string, driverLoc *DriverLocation) {
	data, _ := json.Marshal(driverLoc)
	message := WebSocketMessage{
		Type:      TypeLocationUpdate,
		Timestamp: time.Now(),
		Data:      data,
	}

	msgBytes, _ := json.Marshal(message)
	dt.hub.sendToDeliveryWatchers(deliveryID, msgBytes)
}
func (dt *DeliveryTracker) saveLocation(driverLoc *DriverLocation) error {
	_, err := dt.db.Exec(`
			INSERT INTO driver_locations
			(driver_id, delivery_id, lat, lon, altitude, speed, bearing, accuracy, timestamp)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
		driverLoc.DriverID,
		driverLoc.DeliveryID,
		driverLoc.Location.Lat,
		driverLoc.Location.Lon,
		driverLoc.Location.Altitude,
		driverLoc.Location.Speed,
		driverLoc.Location.Bearing,
		driverLoc.Location.Accuracy,
		driverLoc.Location.Timestamp,
	)
	return err
}
func (dt *DeliveryTracker) getActiveDeliveryForDriver(driverID string) *Delivery {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	for _, delivery := range dt.deliveries {
		if delivery.DriverID == driverID &&
			(delivery.Status == StatusAssigned ||
				delivery.Status == StatusPickedUp ||
				delivery.Status == StatusInTransit ||
				delivery.Status == StatusNearby ||
				delivery.Status == StatusArriving) {
			return delivery
		}
	}
	return nil
}
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371

	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}
func (dt *DeliveryTracker) broadcastETAUpdate(deliveryID string, eta *ETAUpdate) {
	data, _ := json.Marshal(eta)
	message := WebSocketMessage{
		Type:      TypeETAUpdate,
		Timestamp: time.Now(),
		Data:      data,
	}

	msgBytes, _ := json.Marshal(message)
	dt.hub.sendToDeliveryWatchers(deliveryID, msgBytes)
}
func (dt *DeliveryTracker) broadcastProximityAlert(deliveryID string, alert *ProximityAlert) {
	data, _ := json.Marshal(alert)
	message := WebSocketMessage{
		Type:      TypeProximityAlert,
		Timestamp: time.Now(),
		Data:      data,
	}

	msgBytes, _ := json.Marshal(message)
	dt.hub.sendToDeliveryWatchers(deliveryID, msgBytes)
}

func (dt *DeliveryTracker) broadcastStatusChange(deliveryID string, statusChange *StatusChangeEvent) {
	data, _ := json.Marshal(statusChange)
	message := WebSocketMessage{
		Type:      TypeStatusChange,
		Timestamp: time.Now(),
		Data:      data,
	}

	msgBytes, _ := json.Marshal(message)
	dt.hub.sendToDeliveryWatchers(deliveryID, msgBytes)
}
func (dt *DeliveryTracker) processLocationUpdate(driverLoc *DriverLocation) {
	dt.mu.Lock()
	dt.lastLocations[driverLoc.DriverID] = driverLoc
	dt.mu.Unlock()

	dt.saveLocation(driverLoc)

	delivery := dt.getActiveDeliveryForDriver(driverLoc.DriverID)
	if delivery == nil {
		return
	}

	eta := dt.calculateETA(driverLoc, delivery)

	distance := calculateDistance(
		driverLoc.Location.Lat,
		driverLoc.Location.Lon,
		delivery.DropoffLat,
		delivery.DropoffLon,
	)

	dt.checkProximityAlerts(delivery, driverLoc, distance)

	dt.broadcastLocationUpdate(delivery.ID, driverLoc)

	dt.broadcastETAUpdate(delivery.ID, eta)
}
