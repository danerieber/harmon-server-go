package main

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 4096
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var openedClientsByPlace = map[int64]*list.List{}
var notifiedClientsByPlace = map[int64]*list.List{}

type SocketMessage struct {
	Action    int
	AccountId int64           `json:",omitempty"`
	PlaceId   int64           `json:",omitempty"`
	Data      json.RawMessage `json:",omitempty"`
	Error     string          `json:",omitempty"`
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	s *Server

	accountId int64
	placeIds  []int64
}

var actions = []func(c *Client, message *SocketMessage){
	Ping,
	NewChatMessage,
}

func (c *Client) SendMessage(message *SocketMessage) {
	messageBytes, _ := json.Marshal(message)
	c.send <- messageBytes
}

func (c *Client) SendError(message *SocketMessage, error string) {
	message.Error = error
	c.SendMessage(message)
}

func (c *Client) BroadcastMessage(message *SocketMessage) {
	messageBytes, _ := json.Marshal(message)
	c.hub.broadcast <- messageBytes
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		for _, placeId := range c.placeIds {
			if clients, ok := openedClientsByPlace[placeId]; ok {
				for e := clients.Front(); e != nil; e = e.Next() {
					if e.Value == c {
						clients.Remove(e)
						break
					}
				}
			}
			if clients, ok := notifiedClientsByPlace[placeId]; ok {
				for e := clients.Front(); e != nil; e = e.Next() {
					if e.Value == c {
						clients.Remove(e)
						break
					}
				}
			}
		}

		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		messageBytes = bytes.TrimSpace(bytes.Replace(messageBytes, newline, space, -1))

		message := SocketMessage{}
		err = json.Unmarshal(messageBytes, &message)
		if err != nil {
			continue
		}

		message.AccountId = c.accountId

		if message.Action >= 0 && message.Action < len(actions) {
			if message.PlaceId > 0 {
				var id int64
				err := c.s.dbpool.QueryRow(context.Background(), "select id from place where id = $1", message.PlaceId).Scan(&id)
				if err != nil {
					c.SendError(&message, "Place not found")
					continue
				}
			}
			actions[message.Action](c, &message)
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
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
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(newline)
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

// serveWs handles websocket requests from the peer.
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request, s *Server, accountId int64, mode string, placeIds []int64) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256), s: s, accountId: accountId, placeIds: placeIds}

	for _, placeId := range placeIds {
		switch mode {
		case "open":
			if _, ok := openedClientsByPlace[placeId]; !ok {
				openedClientsByPlace[placeId] = list.New()
			}
			openedClientsByPlace[placeId].PushBack(client)
			break
		case "notify":
			if _, ok := notifiedClientsByPlace[placeId]; !ok {
				notifiedClientsByPlace[placeId] = list.New()
			}
			notifiedClientsByPlace[placeId].PushBack(client)
			break
		}
	}

	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
