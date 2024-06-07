package main

import (
	"context"
	"encoding/json"
	"strings"
)

func UnmarshalMessageData(c *Client, message *SocketMessage, v any) (ok bool) {
	err := json.Unmarshal(message.Data, v)
	if err != nil {
		c.SendError(message, "Failed to parse message data")
		return false
	}
	return true
}

func Ping(c *Client, message *SocketMessage) {
	message.Data, _ = json.Marshal("Pong")
	c.SendMessage(message)
}

func NewChatMessage(c *Client, message *SocketMessage) {
	if message.PlaceId == 0 {
		c.SendError(message, "No Place ID")
		return
	}
	chatMessage := struct {
		Id               int64
		Content          string
		ReplyToMessageId *int64 `json:",omitempty"`
		Attachments      []struct {
			Type        string
			LocationUri string
		} `json:",omitempty"`
	}{}
	if !UnmarshalMessageData(c, message, &chatMessage) {
		return
	}
	chatMessage.Content = strings.TrimSpace(chatMessage.Content)
	if chatMessage.Content == "" {
		c.SendError(message, "Message is empty")
		return
	}
	for _, attachment := range chatMessage.Attachments {
		if attachment.LocationUri == "" {
			c.SendError(message, "Attachment LocationUri is empty")
			return
		}
		if attachment.Type != "file" && attachment.Type != "image" && attachment.Type != "video" && attachment.Type != "audio" {
			c.SendError(message, "Invalid Attachment Type: "+attachment.Type)
			return
		}
	}
	if chatMessage.ReplyToMessageId != nil {
		var id int64
		err := c.s.dbpool.QueryRow(context.Background(), "select id from chat_message where id = $1 and place_id = $2", chatMessage.ReplyToMessageId, message.PlaceId).Scan(&id)
		if err != nil {
			c.SendError(message, "ReplyTo Message ID does not exist")
			return
		}
	}
	var attachments []byte = nil
	if len(chatMessage.Attachments) > 0 {
		attachments, _ = json.Marshal(chatMessage.Attachments)
	}
	err := c.s.dbpool.QueryRow(context.Background(), "insert into chat_message (place_id, account_id, content, reply_to_message_id, attachments) values ($1, $2, $3, $4, $5) returning id", message.PlaceId, message.AccountId, chatMessage.Content, chatMessage.ReplyToMessageId, attachments).Scan(&chatMessage.Id)
	if err != nil {
		c.SendError(message, err.Error())
		return
	}
	if chatMessage.Id == 0 {
		c.SendError(message, "Chat Message ID is 0")
		return
	}
	message.Data, _ = json.Marshal(chatMessage)
	c.SendMessage(message)
	if clients, ok := openedClientsByPlace[message.PlaceId]; ok {
		for e := clients.Front(); e != nil; e = e.Next() {
			client := e.Value.(*Client)
			if client != c {
				client.SendMessage(message)
			}
		}
	}
	message.AccountId = 0
	message.Data = nil
	if clients, ok := notifiedClientsByPlace[message.PlaceId]; ok {
		for e := clients.Front(); e != nil; e = e.Next() {
			client := e.Value.(*Client)
			if client != c {
				client.SendMessage(message)
			}
		}
	}
}
