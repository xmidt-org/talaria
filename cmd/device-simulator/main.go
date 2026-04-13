// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultThemisURL  = "http://localhost:6500/issue"
	defaultTalariaURL = "ws://localhost:6200/api/v2/device"
	defaultDeviceID   = "mac:112233445566"
	defaultSerial     = "1800deadbeef"
)

// ThemisRequest represents the request to Themis for a JWT token
type ThemisRequest struct {
	DeviceID     string            `json:"device_id"`
	SerialNumber string            `json:"serial"`
	Claims       map[string]string `json:"claims,omitempty"`
}

// ThemisResponse represents the response from Themis with the JWT token
type ThemisResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// Device represents a simulated device
type Device struct {
	DeviceID     string
	SerialNumber string
	ThemisURL    string
	TalariaURL   string
	Token        string
	Conn         *websocket.Conn
}

// GetJWTToken requests a JWT token from Themis
func (d *Device) GetJWTToken() error {
	log.Printf("Requesting JWT token from Themis at %s", d.ThemisURL)

	// Make GET request to Themis to obtain JWT token
	resp, err := http.Get(d.ThemisURL)
	if err != nil {
		return fmt.Errorf("failed to request token from Themis: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Themis response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("themis returned status %d: %s", resp.StatusCode, string(body))
	}

	//var themisResp ThemisResponse
	fmt.Println(string(body))
	// if err := json.Unmarshal(body, &themisResp); err != nil {
	// 	return fmt.Errorf("failed to decode Themis response: %w", err)
	// }

	//d.Token = themisResp.Token
	d.Token = string(body)
	//log.Printf("✓ Successfully obtained JWT token (expires at %s)", time.Unix(themisResp.ExpiresAt, 0))
	log.Printf("Token: %s", d.Token[:50]+"...")

	return nil
}

// ConnectToTalaria establishes a WebSocket connection to Talaria
func (d *Device) ConnectToTalaria() error {
	log.Printf("Connecting to Talaria at %s", d.TalariaURL)

	// Parse the URL
	u, err := url.Parse(d.TalariaURL)
	if err != nil {
		return fmt.Errorf("invalid Talaria URL: %w", err)
	}

	// Create WebPA convey header (device metadata)
	conveyData := map[string]string{
		"boot-time":             time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"boot-time-retry-wait":  "1",
		"fw-name":               "v0.0.1",
		"hw-last-reboot-reason": "sleepy",
		"hw-manufacturer":       "testManufacturer",
		"hw-model":              "testModel",
		"hw-serial-number":      d.SerialNumber,
		"interfaces-available":  "",
		"webpa-interface-used":  "erouter0",
		"webpa-protocol":        "protocol",
	}
	conveyJSON, _ := json.Marshal(conveyData)
	conveyHeader := base64.StdEncoding.EncodeToString(conveyJSON)

	// Set up request headers
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+d.Token)
	headers.Set("X-Webpa-Device-Name", d.DeviceID)
	headers.Set("X-Webpa-Convey", conveyHeader)

	// Establish WebSocket connection
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(u.String(), headers)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("failed to connect to Talaria (status %d): %s, error: %w", resp.StatusCode, string(body), err)
		}
		return fmt.Errorf("failed to connect to Talaria: %w", err)
	}

	d.Conn = conn
	log.Printf("✓ Successfully connected to Talaria!")
	log.Printf("Connection established with device ID: %s", d.DeviceID)

	return nil
}

// ListenForMessages reads messages from Talaria
func (d *Device) ListenForMessages() {
	defer d.Conn.Close()

	for {
		messageType, message, err := d.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("Error reading message: %v", err)
			} else {
				log.Printf("Connection closed: %v", err)
			}
			return
		}

		switch messageType {
		case websocket.TextMessage:
			log.Printf("Received text message: %s", string(message))
		case websocket.BinaryMessage:
			log.Printf("Received binary message (%d bytes)", len(message))
		case websocket.PingMessage:
			log.Printf("Received ping")
		case websocket.PongMessage:
			log.Printf("Received pong")
		}
	}
}

// SendPing sends periodic pings to keep the connection alive
func (d *Device) SendPing(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := d.Conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
			log.Printf("Error sending ping: %v", err)
			return
		}
		log.Printf("Sent ping")
	}
}

func main() {
	// Command-line flags
	themisURL := flag.String("themis", defaultThemisURL, "Themis URL for JWT token")
	talariaURL := flag.String("talaria", defaultTalariaURL, "Talaria WebSocket URL")
	deviceID := flag.String("device-id", defaultDeviceID, "Device ID (MAC address)")
	serial := flag.String("serial", defaultSerial, "Device serial number")
	pingInterval := flag.Duration("ping-interval", 30*time.Second, "Interval for sending pings")
	flag.Parse()

	log.Printf("Device Simulator Starting...")
	log.Printf("Device ID: %s", *deviceID)
	log.Printf("Serial: %s", *serial)
	log.Printf("Themis URL: %s", *themisURL)
	log.Printf("Talaria URL: %s", *talariaURL)

	device := &Device{
		DeviceID:     *deviceID,
		SerialNumber: *serial,
		ThemisURL:    *themisURL,
		TalariaURL:   *talariaURL,
	}

	// Step 1: Get JWT token from Themis
	if err := device.GetJWTToken(); err != nil {
		log.Fatalf("Failed to get JWT token: %v", err)
	}

	// Step 2: Connect to Talaria
	if err := device.ConnectToTalaria(); err != nil {
		log.Fatalf("Failed to connect to Talaria: %v", err)
	}

	// Start ping sender
	go device.SendPing(*pingInterval)

	// Start message listener
	go device.ListenForMessages()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("Device simulator running. Press Ctrl+C to exit.")
	<-sigChan

	log.Printf("Shutting down...")
	if device.Conn != nil {
		device.Conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		device.Conn.Close()
	}
	log.Printf("Device simulator stopped.")
}
