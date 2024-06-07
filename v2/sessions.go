package main

import "crypto/rand"

const alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func randomString(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}

var activeSessionTokens = map[string]int64{}

func newActiveSessionToken(userId int64) string {
	sessionToken := randomString(48)
	if _, ok := activeSessionTokens[sessionToken]; ok {
		return newActiveSessionToken(userId)
	}
	activeSessionTokens[sessionToken] = userId
	return sessionToken
}

func validateSessionToken(sessionToken string) (userId int64, ok bool) {
	userId, ok = activeSessionTokens[sessionToken]
	return userId, ok
}
