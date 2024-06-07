package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(token string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(token), 12)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func Authenticate(w http.ResponseWriter, r *http.Request) (accountId int64, ok bool) {
	sessionToken := r.Header.Get("x-harmon-session-token")
	if sessionToken == "" {
		http.Error(w, "Missing x-harmon-session-token", 401)
		return 0, false
	}

	accountId, ok = validateSessionToken(sessionToken)
	if !ok {
		http.Error(w, "Invalid session token", 401)
		return 0, false
	}

	return accountId, true
}

func (s Server) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	creds := struct {
		Username string
		Password string
	}{}
	err := json.NewDecoder(r.Body).Decode(&creds)

	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	if creds.Username == "" || creds.Password == "" {
		http.Error(w, "Missing username and/or password", http.StatusBadRequest)
		return
	}

	var userId int64
	var hash string
	err = s.dbpool.QueryRow(context.Background(), "select id, password_hash from account where username = $1", creds.Username).Scan(&userId, &hash)

	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !CheckPasswordHash(creds.Password, hash) {
		http.Error(w, "Password incorrect", http.StatusUnauthorized)
		return
	}

	responseBody := struct {
		UserId       int64
		SessionToken string
	}{
		UserId:       userId,
		SessionToken: newActiveSessionToken(userId),
	}
	resData, _ := json.Marshal(responseBody)
	fmt.Fprintf(w, string(resData))
}

func (s Server) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	creds := struct {
		Username string
		Password string
	}{}
	err := json.NewDecoder(r.Body).Decode(&creds)

	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	if creds.Username == "" || creds.Password == "" {
		http.Error(w, "Missing username and/or password", http.StatusBadRequest)
		return
	}

	var exists bool
	err = s.dbpool.QueryRow(context.Background(), "select exists(select 1 from account where username = $1)", creds.Username).Scan(&exists)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "Username taken", http.StatusForbidden)
		return
	}

	hash, err := HashPassword(creds.Password)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = s.dbpool.Exec(context.Background(), "insert into account (username, password_hash) values ($1, $2)", creds.Username, hash)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s Server) Place(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accountId, ok := Authenticate(w, r)
	if !ok {
		return
	}

	place := struct {
		Id                   int64
		IsPublic             bool
		Name                 string
		AllowMembersToInvite bool
	}{}
	err := json.NewDecoder(r.Body).Decode(&place)

	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	place.Name = strings.TrimSpace(place.Name)

	if place.Name == "" {
		http.Error(w, "Place name is empty", http.StatusBadRequest)
		return
	}

	err = s.dbpool.QueryRow(context.Background(), "insert into place (owner_account_id, is_public, name, allow_members_to_invite) values ($1, $2, $3, $4) returning id", accountId, place.IsPublic, place.Name, place.AllowMembersToInvite).Scan(&place.Id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if place.Id == 0 {
		http.Error(w, "Place ID is 0", http.StatusInternalServerError)
		return
	}

	_, err = s.dbpool.Exec(context.Background(), "insert into place_account (place_id, account_id) values ($1, $2)", place.Id, accountId)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resData, _ := json.Marshal(place)
	fmt.Fprintf(w, string(resData))
}

func (s Server) PlaceInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accountId, ok := Authenticate(w, r)
	if !ok {
		return
	}

	invite := struct {
		Id        uuid.UUID
		PlaceId   int64
		ExpiresAt *time.Time
	}{}
	err := json.NewDecoder(r.Body).Decode(&invite)

	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	if invite.PlaceId == 0 {
		http.Error(w, "Place ID is missing", http.StatusBadRequest)
		return
	}
	if invite.ExpiresAt != nil && invite.ExpiresAt.Before(time.Now().Add(15*time.Minute)) {
		http.Error(w, "Invite must expire in at least 15 minutes", http.StatusBadRequest)
		return
	} else {
		expiresAt := time.Now().Add(7 * 24 * time.Hour)
		invite.ExpiresAt = &expiresAt
	}

	var place struct {
		Id                   int64
		OwnerAccountId       int64
		AllowMembersToInvite bool
	}
	err = s.dbpool.QueryRow(context.Background(), "select id, owner_account_id, allow_members_to_invite from place where id = $1", invite.PlaceId).Scan(&place.Id, &place.OwnerAccountId, &place.AllowMembersToInvite)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if place.AllowMembersToInvite {
		var exists bool
		err = s.dbpool.QueryRow(context.Background(), "select exists(select 1 from place_account where place_id = $1 and account_id = $2)", place.Id, accountId).Scan(&exists)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else if accountId != place.OwnerAccountId {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	err = s.dbpool.QueryRow(context.Background(), "insert into place_invite (place_id, created_by_account_id, expires_at) values ($1, $2, $3) returning id", invite.PlaceId, accountId, invite.ExpiresAt).Scan(&invite.Id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resData, _ := json.Marshal(invite)
	fmt.Fprintf(w, string(resData))
}
