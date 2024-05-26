package main

import "encoding/json"

const (
	NewChatMessageAction   uint8 = 1
	ChangeUsernameAction   uint8 = 2
	RequestUserInfoAction  uint8 = 3
	GetChatMessagesAction  uint8 = 4
	UpdateMyUserInfoAction uint8 = 5
	GetAllUsersAction      uint8 = 6
	JoinCallAction         uint8 = 7
	GetMySettingsAction    uint8 = 8
	UpdateMySettingsAction uint8 = 9
)

const (
	OfflinePresence uint8 = 1
	OnlinePresence  uint8 = 2
	AwayPresence    uint8 = 3
	InCallPresence  uint8 = 3
)

type IncomingMessage struct {
	SessionToken string `json:"sessionToken"`
}

type Message struct {
	UserId string          `json:"userId"`
	Action uint8           `json:"action"`
	Data   json.RawMessage `json:"data"`
}

type User struct {
	Username        string `json:"username"`
	Presence        uint8  `json:"presence"`
	Status          string `json:"status"`
	Icon            string `json:"icon"`
	BannerUrl       string `json:"bannerUrl"`
	UsernameColor   string `json:"usernameColor"`
	ChangedUsername bool   `json:"changedUsername"`
	IsDeveloper     bool   `json:"isDeveloper"`
}

type NewChatMessage struct {
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

type ChangeUsername struct {
	Username string `json:"username"`
}

type RequestUserInfo struct {
	UserId string          `json:"userId"`
	User   json.RawMessage `json:"user"`
}

type GetChatMessages struct {
	ChatId   string          `json:"chatId"`
	Start    *int64          `json:"start"`
	Total    *int            `json:"total"`
	Messages json.RawMessage `json:"messages"`
}

type GetAllUsers struct {
	Users map[string]json.RawMessage `json:"users"`
}

type JoinCall struct {
	PeerId string `json:"peerId"`
}

type MyAudioSettings struct {
	EchoCancellation bool `json:"echoCancellation"`
	NoiseSuppression bool `json:"noiseSuppression"`
	AutoGainControl  bool `json:"autoGainControl"`
}

type MySettings struct {
	AudioSettings MyAudioSettings `json:"audioSettings"`
}

type Peer struct {
	UserId string `json:"userId"`
	PeerId string `json:"peerId"`
}

type AllPeers struct {
	Peers []Peer `json:"peer"`
}
