package main

import (
	"crypto/rand"
	"encoding/json"

	"github.com/google/uuid"
)

const alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var registeredTokens = map[string]bool{}
var activeSessionTokens = map[string]string{}

func randomString(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}

func randomUsername() string {
	username := "user" + randomString(20)
	if _, ok := dbRead("token", username); ok {
		return randomUsername()
	}
	return username
}

func randomUserId() string {
	return uuid.NewString()
}

func randomSessionToken() string {
	return randomString(48)
}

func register() string {
	token := randomString(96)
	registeredTokens[token] = true
	return token
}

func newActiveSessionToken(token string) string {
	sessionToken := randomSessionToken()
	if _, ok := activeSessionTokens[sessionToken]; ok {
		return newActiveSessionToken(token)
	}
	activeSessionTokens[sessionToken] = token
	return sessionToken
}

func login(token string) (sessionToken string, ok bool) {
	if _, ok := registeredTokens[token]; ok {
		delete(registeredTokens, token)
		userId := randomUserId()
		username := randomUsername()
		user := User{
			Username: username,
			Presence: 1,
			Status:   "New to Harmon!",
		}
		userText, _ := json.Marshal(user)
		dbWrite("token_to_user_id", token, []byte(userId))
		dbWrite("username_to_user_id", username, []byte(userId))
		dbWrite("user", userId, userText)
		return newActiveSessionToken(token), true
	}

	if _, ok := dbRead("token_to_user_id", token); ok {
		return newActiveSessionToken(token), true
	}

	return "", false
}

func getToken(sessionToken string) string {
	if token, ok := activeSessionTokens[sessionToken]; ok {
		return token
	}
	return ""
}
