// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type LoginRequestBody struct {
	Token string `json:"token"`
}

type LoginResponseBody struct {
	SessionToken string          `json:"sessionToken"`
	UserId       string          `json:"userId"`
	User         json.RawMessage `json:"user"`
}

var addr = flag.String("addr", ":8080", "http service address")

var jsonHandler = slog.NewJSONHandler(os.Stdout, nil)
var myslog = slog.New(jsonHandler)

func serveHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.URL.Path == "/login" && r.Method == http.MethodPost {
		body := LoginRequestBody{}
		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Token == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}

		if sessionToken, ok := login(body.Token); ok {
			if userId, ok := dbRead("token_to_user_id", body.Token); ok {
				if userText, ok := dbRead("user", string(userId)); ok {
					user := User{}
					if json.Unmarshal(userText, &user) == nil {
						res := LoginResponseBody{
							SessionToken: sessionToken,
							UserId:       string(userId),
						}
						user.Presence = OnlinePresence
						res.User, _ = json.Marshal(user)
						resText, _ := json.Marshal(res)
						fmt.Fprintf(w, string(resText))
						myslog.Info("login", "userId", res.UserId)
						return
					}
				}
			}

		}

		http.Error(w, "login error", http.StatusInternalServerError)
		return
	}
	if r.URL.Path == "/register" && r.Method == http.MethodGet {
		token := register()
		fmt.Fprintf(w, `{"token":"`+token+`"}`)
		myslog.Info("register", "token", token)
		return
	}
	http.Error(w, "Not found", http.StatusNotFound)
	return
}

func main() {
	dbInit()
	flag.Parse()
	hub := newHub()
	go hub.run()
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	server := &http.Server{
		Addr:              *addr,
		ReadHeaderTimeout: 3 * time.Second,
	}
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
