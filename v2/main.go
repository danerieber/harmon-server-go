package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var addr = flag.String("addr", ":8080", "http service address")

type Server struct {
	dbpool *pgxpool.Pool
}

func main() {
	s := Server{}

	dbpool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create connection pool: %v\n", err)
		os.Exit(1)
	}
	s.dbpool = dbpool
	defer dbpool.Close()

	hub := newHub()
	go hub.run()
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		sessionToken := r.Header.Get("x-harmon-session-token")
		if sessionToken == "" {
			http.Error(w, "Missing x-harmon-session-token", 401)
			return
		}

		accountId, ok := validateSessionToken(sessionToken)
		if !ok {
			http.Error(w, "Invalid session token", 401)
			return
		}

		mode := r.Header.Get("x-harmon-mode")
		if mode == "" {
			http.Error(w, "Missing x-harmon-mode", 400)
			return
		}
		if mode != "open" && mode != "notify" {
			http.Error(w, "Invalid mode", 400)
			return
		}

		placeIdsHeader := r.Header.Get("x-harmon-place-ids")
		if placeIdsHeader == "" {
			http.Error(w, "Missing x-harmon-place-ids", 400)
			return
		}
		placeIdStrings := strings.Split(placeIdsHeader, ",")
		placeIds := make([]int64, len(placeIdStrings))

		for i, placeIdString := range placeIdStrings {
			placeId, err := strconv.ParseInt(placeIdString, 10, 64)
			if err != nil {
				http.Error(w, "Failed to parse Place ID as int: "+placeIdString, 400)
				return
			}
			placeIds[i] = placeId
			var id int64
			err = s.dbpool.QueryRow(context.Background(), "select id from place_account where place_id = $1 and account_id = $2", placeId, accountId).Scan(&id)
			if err != nil {
				fmt.Println(err)
				http.Error(w, "Unknown place ID: "+placeIdString, 404)
				return
			}
		}

		serveWs(hub, w, r, &s, accountId, mode, placeIds)
	})

	http.HandleFunc("/login", s.Login)
	http.HandleFunc("/register", s.Register)
	http.HandleFunc("/place", s.Place)
	http.HandleFunc("/placeinvite", s.PlaceInvite)

	httpServer := &http.Server{
		Addr:              *addr,
		ReadHeaderTimeout: 3 * time.Second,
	}
	httpServer.ListenAndServe()
}
