package main

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func (dt *DeliveryTracker) loadActiveDeliveries() error {
	rows, err := dt.db.Query(`
		SELECT id, driver_id, customer_id, pickup_lat, pickup_lon, 
		       dropoff_lat, dropoff_lon, status, estimated_time, created_at, updated_at
		FROM deliveries
		WHERE status NOT IN ('delivered', 'cancelled')
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	dt.mu.Lock()
	defer dt.mu.Unlock()

	for rows.Next() {
		var d Delivery
		err := rows.Scan(
			&d.ID, &d.DriverID, &d.CustomerID,
			&d.PickupLat, &d.PickupLon,
			&d.DropoffLat, &d.DropoffLon,
			&d.Status, &d.EstimatedTime,
			&d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return err
		}
		dt.deliveries[d.ID] = &d
	}

	return rows.Err()
}

type App struct {
	db      *sql.DB
	hub     *Hub
	tracker *DeliveryTracker
}

func (a *App) processLocation(location *DriverLocation) {
	panic("unimplemented")
}

var app *App

func initDatabase() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./delivery_tracking.db")
	if err != nil {
		return nil, err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS deliveries (
		id TEXT PRIMARY KEY,
		driver_id TEXT NOT NULL,
		customer_id TEXT NOT NULL,
		pickup_lat REAL NOT NULL,
		pickup_lon REAL NOT NULL,
		dropoff_lat REAL NOT NULL,
		dropoff_lon REAL NOT NULL,
		status TEXT NOT NULL,
		estimated_time DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	
	CREATE TABLE IF NOT EXISTS driver_locations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		driver_id TEXT NOT NULL,
		delivery_id TEXT,
		lat REAL NOT NULL,
		lon REAL NOT NULL,
		altitude REAL,
		speed REAL,
		bearing REAL,
		accuracy REAL,
		timestamp DATETIME NOT NULL
	);
	
	CREATE INDEX IF NOT EXISTS idx_driver_locations_driver ON driver_locations(driver_id);
	CREATE INDEX IF NOT EXISTS idx_driver_locations_delivery ON driver_locations(delivery_id);
	CREATE INDEX IF NOT EXISTS idx_driver_locations_timestamp ON driver_locations(timestamp);
	CREATE INDEX IF NOT EXISTS idx_deliveries_driver ON deliveries(driver_id);
	CREATE INDEX IF NOT EXISTS idx_deliveries_customer ON deliveries(customer_id);
	CREATE INDEX IF NOT EXISTS idx_deliveries_status ON deliveries(status);
	`

	_, err = db.Exec(schema)
	return db, err
}
