// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
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
	w.Header().Set("Access-Control-Allow-Headers", "Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
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
	if strings.HasPrefix(r.URL.Path, "/image") {
		split := strings.Split(r.URL.Path, "/")
		if len(split) == 3 {
			fileName := split[2]
			if r.Method == http.MethodGet {
				if data, ok := dbRead("image", fileName); ok {
					w.Write(data)
					return
				}
				http.Error(w, "not found", http.StatusNotFound)
			} else if r.Method == http.MethodPost {
				split = strings.Split(fileName, ".")
				fileExt := ""
				if len(split) > 1 {
					fileExt = split[len(split)-1]
				}
				if sessionToken := r.Header.Get("Authorization"); sessionToken != "" {
					if token := getToken(sessionToken); token != "" {
						imageId := uuid.NewString() + "." + fileExt
						buf, err := io.ReadAll(r.Body)
						if err == nil && len(buf) > 0 {
							dbWrite("image", imageId, buf)
							fmt.Fprintf(w, string(imageId))
							myslog.Info("image", "imageId", imageId)
							return
						} else {
							http.Error(w, "failed to read body", http.StatusBadRequest)
							return
						}
					}
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Error(w, "invalid path", http.StatusBadRequest)
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
	myslog.Info("Listening on :8080")
	err := server.ListenAndServe()
	if err != nil {
		myslog.Error("ListenAndServe Error", "err", err)
	}
}
