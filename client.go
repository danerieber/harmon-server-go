// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
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

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	// Extra data for each connection.
	userId string
	peerId string
}

var nClients = sync.Map{}
var presences = sync.Map{}
var peerMap = sync.Map{}

var developers, _ = dbReadAll("developer")

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		// Detect when a user has 0 connections left and set their status to offline
		if n, ok := nClients.Load(c.userId); ok {
			n := n.(int)
			if n == 1 {
				nClients.Delete(c.userId)
				presences.Store(c.userId, OfflinePresence)
				if userText, ok := dbRead("user", c.userId); ok {
					user := User{}
					if json.Unmarshal(userText, &user) == nil {
						user.Presence = OfflinePresence
						message := Message{
							UserId: c.userId,
							Action: UpdateMyUserInfoAction,
						}
						message.Data, _ = json.Marshal(user)
						messageText, _ := json.Marshal(message)
						c.hub.broadcast <- messageText
					}
				}
			} else {
				nClients.Store(c.userId, n-1)
			}
			myslog.Info("disconnect", "userId", c.userId, "openConnections", n-1)
		}
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, messageText, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		messageText = bytes.TrimSpace(bytes.Replace(messageText, newline, space, -1))

		// Ensure message has session token
		incomingMessage := IncomingMessage{}
		err = json.Unmarshal(messageText, &incomingMessage)
		if err != nil || incomingMessage.SessionToken == "" {
			continue
		}

		// Validate session token
		token := getToken(incomingMessage.SessionToken)
		if token == "" {
			continue
		}

		// Get user info
		userId, ok := dbRead("token_to_user_id", token)
		if !ok {
			continue
		}

		// Save this client's userId and increment nClients
		if c.userId == "" {
			c.userId = string(userId)
			if n, ok := nClients.Load(c.userId); ok {
				n := n.(int)
				nClients.Store(c.userId, n+1)
			} else {
				nClients.Store(c.userId, 1)
			}
		}

		// Get this user's infor from the db
		userText, ok := dbRead("user", string(userId))
		if !ok {
			continue
		}
		user := User{}
		if json.Unmarshal(userText, &user) != nil {
			continue
		}

		// Construct new message from incoming text
		message := Message{}
		err = json.Unmarshal(messageText, &message)
		if err != nil || message.Action <= 0 {
			continue
		}
		message.UserId = string(userId)

		// Save this user's presence as online
		if _, ok := presences.Load(message.UserId); !ok {
			presences.Store(message.UserId, OnlinePresence)
		}

		// Whether or not we should broadcast the message to all clients or only respond to the sender
		broadcast := true

		// Handle message actions
		if message.Action == NewChatMessageAction {
			// Parse and validate new chat message
			r := NewChatMessage{}
			err = json.Unmarshal(message.Data, &r)
			r.Data.Content = strings.TrimSpace(r.Data.Content)
			if err != nil || r.Data.Content == "" || r.ChatId == "" {
				continue
			}
			if r.ChatId != "global" && !dbExists("chat_messages", r.ChatId) {
				continue
			}

			r.Data.Timestamp = time.Now().UnixMilli()
			message.Data, _ = json.Marshal(r.Data)

			// Save new chat message to db
			dbMessage, _ := json.Marshal(message)
			dbMessage = append(dbMessage, "\n"...)
			dbAppend("chat_messages", "global", []byte(dbMessage))
		} else if message.Action == ChangeUsernameAction {
			// Parse and validate username
			r := ChangeUsername{}
			if json.Unmarshal(message.Data, &r) != nil {
				continue
			}
			// Between 3-24 characters
			if len(r.Username) < 3 || len(r.Username) > 24 {
				continue
			}
			// Alphanumeric, a few symbols, no spaces at start/end
			match, _ := regexp.MatchString("^[A-Za-z0-9!?.,:;()$%*<]+[A-Za-z0-9!?.,:;()$%*< ]+[A-Za-z0-9!?.,:;()$%*<]+$", r.Username)
			if !match {
				continue
			}
			// Disallow multiple consecutive spaces
			match, _ = regexp.MatchString(" {2,}", r.Username)
			if match {
				continue
			}

			// Ensure username is not already taken
			if _, ok := dbRead("username_to_user_id", r.Username); ok {
				continue
			}

			// Save new username and delete old one
			dbWrite("username_to_user_id", r.Username, []byte(message.UserId))
			dbDelete("username_to_user_id", user.Username)
			if !user.ChangedUsername {
				user.ChangedUsername = (user.Username != r.Username)
			}
			user.Username = r.Username
			userText, _ = json.Marshal(user)
			dbWrite("user", message.UserId, userText)
		} else if message.Action == RequestUserInfoAction {
			broadcast = false

			// Parse and validate request
			r := RequestUserInfo{}
			if json.Unmarshal(message.Data, &r) != nil {
				continue
			}
			if r.UserId == "" {
				continue
			}

			userText, ok := dbRead("user", r.UserId)
			if !ok {
				continue
			}

			user := User{}
			err := json.Unmarshal(userText, &user)
			if err != nil {
				continue
			}

			if presence, ok := presences.Load(r.UserId); ok {
				user.Presence = presence.(uint8)
			} else {
				user.Presence = OfflinePresence
			}
			user.IsDeveloper = false
			if developers != nil && developers[r.UserId] != nil {
				user.IsDeveloper = true
			}

			r.User, _ = json.Marshal(user)

			message.Data, _ = json.Marshal(r)
		} else if message.Action == GetChatMessagesAction {
			broadcast = false

			// Parse and validate request
			r := GetChatMessages{}
			if json.Unmarshal(message.Data, &r) != nil {
				continue
			}
			if r.ChatId == "" {
				continue
			}
			if r.Total == nil {
				continue
			}

			// If a start value is not provided, assume we are seeking from the end of the file
			var offset int64
			var whence int
			if r.Start == nil {
				offset = -int64(*r.Total)
				whence = io.SeekEnd
			} else {
				offset = *r.Start
				whence = io.SeekCurrent
			}
			entries, newOffset, newTotal, ok := dbReadEntries("chat_messages", r.ChatId, offset, whence, *r.Total)

			if ok {
				r.Start = &newOffset
				r.Total = &newTotal
				r.Messages = entries
			} else {
				r.Messages = []byte("[]")
			}
			message.Data, _ = json.Marshal(r)
		} else if message.Action == UpdateMyUserInfoAction {
			// Parse and validate request
			r := User{}
			if json.Unmarshal(message.Data, &r) != nil {
				continue
			}

			if r.Presence > 0 {
				presences.Store(message.UserId, r.Presence)
				user.Presence = r.Presence
			} else if presence, ok := presences.Load(message.UserId); ok {
				user.Presence = presence.(uint8)
			}
			if r.Status != "" {
				user.Status = r.Status
			}
			if r.Status != "" {
				user.Icon = r.Icon
			}
			if r.BannerUrl != "" {
				user.BannerUrl = r.BannerUrl
			}
			if r.UsernameColor != "" {
				user.UsernameColor = r.UsernameColor
			}
			user.IsDeveloper = false
			if developers != nil && developers[message.UserId] != nil {
				user.IsDeveloper = true
			}

			if updatedUserText, err := json.Marshal(user); err == nil {
				dbWrite("user", message.UserId, updatedUserText)
				message.Data = updatedUserText
			} else {
				continue
			}
		} else if message.Action == GetAllUsersAction {
			broadcast = false

			r := GetAllUsers{}

			values, ok := dbReadAll("user")

			if ok && len(values) > 0 {
				r.Users = map[string]json.RawMessage{}
				for userId, userText := range values {
					user := User{}
					if json.Unmarshal(userText, &user) == nil {
						if presence, ok := presences.Load(userId); ok {
							user.Presence = presence.(uint8)
						} else {
							user.Presence = OfflinePresence
						}
						user.IsDeveloper = false
						if developers != nil && developers[userId] != nil {
							user.IsDeveloper = true
						}
						r.Users[userId], _ = json.Marshal(user)
					}
				}
			} else {
				r.Users = map[string]json.RawMessage{}
			}

			message.Data, _ = json.Marshal(r)
		} else if message.Action == JoinCallAction {
			// Parse and validate request
			r := JoinCall{}
			if json.Unmarshal(message.Data, &r) != nil {
				continue
			}

			c.peerId = r.PeerId

			message.Data, _ = json.Marshal(r)

			// peer := Peer{
			// 	UserId: message.UserId,
			// 	PeerId: r.PeerId,
			// }

			// peers, ok := peerMap.Load(message.UserId)
			// if ok {
			// 	peers = append(peers.([]Peer), peer)
			// } else {
			// 	peers = []Peer{peer}
			// 	peerMap.Store(message.UserId, peers)
			// }

			// var allPeers []Peer

			// peerMap.Range(func(key, value any) bool {
			// 	allPeers = append(allPeers, value.([]Peer)...)
			// 	return true
			// })
		} else if message.Action == GetMySettingsAction {
			broadcast = false

			message.Data, ok = dbRead("settings", message.UserId)
			if !ok {
				continue
			}
		} else if message.Action == UpdateMySettingsAction {
			broadcast = false

			// Parse and validate request
			r := MySettings{}
			if json.Unmarshal(message.Data, &r) != nil {
				continue
			}

			settingsText, _ := json.Marshal(r)
			dbWrite("settings", message.UserId, settingsText)
		} else if message.Action == EditChatMessageAction {
			r := EditChatMessage{}
			err = json.Unmarshal(message.Data, &r)
			r.Data.Content = strings.TrimSpace(r.Data.Content)
			if err != nil || r.Data.Content == "" || r.ChatId == "" || r.Data.EditForTimestamp == 0 || r.Total == 0 {
				continue
			}
			if _, ok := dbRead("chat_messages", r.ChatId); !ok {
				continue
			}

			entries, _, _, ok := dbReadEntries("chat_messages", r.ChatId, r.Start, io.SeekCurrent, r.Total)
			if !ok {
				continue
			}
			dbMessages := []Message{}
			if json.Unmarshal(entries, &dbMessages) != nil {
				continue
			}

			chatMessage := ChatMessage{}
			found := false
			for _, dbMessage := range dbMessages {
				err = json.Unmarshal(dbMessage.Data, &chatMessage)
				if err != nil {
					break
				}
				if chatMessage.Timestamp == r.Data.EditForTimestamp && dbMessage.UserId == message.UserId {
					found = true
					break
				}
			}
			if !found {
				continue
			}

			r.Data.Timestamp = time.Now().UnixMilli()

			message.Data, _ = json.Marshal(r.Data)

			// Save new chat message to db
			dbMessage, _ := json.Marshal(message)
			dbMessage = append(dbMessage, "\n"...)
			dbAppend("chat_messages", "global", []byte(dbMessage))
		}

		messageText, _ = json.Marshal(message)

		if broadcast {
			c.hub.broadcast <- messageText
			myslog.Info("a", "userId", message.UserId, "action", message.Action, "data", message.Data)
		} else {
			c.send <- messageText
			myslog.Debug("a", "userId", message.UserId, "action", message.Action, "data", message.Data)
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
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
