package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	Room      string    `json:"room"`
	Timestamp time.Time `json:"timestamp"`
}

type Client struct {
	Username string
	Room     string
	Conn     *websocket.Conn
	Send     chan Message
}

type Hub struct {
	Clients    map[*Client]bool
	Broadcast  chan Message
	Register   chan *Client
	Unregister chan *Client
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan Message),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Clients[client] = true
			fmt.Printf("✅ [%s] %s님이 입장했습니다 (총 %d명)\n",
				client.Room, client.Username, len(h.Clients))

		case client := <-h.Unregister:
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
				fmt.Printf("❌ [%s] %s님이 퇴장했습니다 (총 %d명)\n",
					client.Room, client.Username, len(h.Clients))
			}

		case message := <-h.Broadcast:
			fmt.Printf("💬 [%s] %s: %s\n",
				message.Room, message.Username, message.Content)
			for client := range h.Clients {
				// 같은 방에 있는 클라이언트에게만 전송
				if client.Room == message.Room {
					select {
					case client.Send <- message:
					default:
						delete(h.Clients, client)
						close(client.Send)
					}
				}
			}
		}
	}
}

func (c *Client) ReadPump(hub *Hub) {
	defer func() {
		hub.Unregister <- c
		c.Conn.Close()
	}()

	for {
		var msg Message
		err := c.Conn.ReadJSON(&msg)
		if err != nil {
			break
		}

		msg.Username = c.Username
		msg.Room = c.Room
		msg.Timestamp = time.Now()
		hub.Broadcast <- msg
	}
}

func (c *Client) WritePump() {
	defer c.Conn.Close()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.Conn.WriteJSON(message)
		}
	}
}

func handleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		username = fmt.Sprintf("User_%d", time.Now().Unix()%1000)
	}

	room := r.URL.Query().Get("room")
	if room == "" {
		room = "general"
	}

	client := &Client{
		Username: username,
		Room:     room,
		Conn:     conn,
		Send:     make(chan Message, 256),
	}

	fmt.Printf("🔌 연결 시도: %s -> %s방\n", username, room)

	hub.Register <- client

	go client.WritePump()
	go client.ReadPump(hub)
}

func main() {
	hub := NewHub()
	go hub.Run()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	})

	fmt.Println("🚀 멀티룸 WebSocket 채팅 서버 시작!")
	fmt.Println("📡 연결 URL: ws://localhost:8080/ws?username=이름&room=방이름")
	fmt.Println("📋 예시: ws://localhost:8080/ws?username=홍길동&room=개발")
	fmt.Println(strings.Repeat("=", 30))
	http.ListenAndServe(":8080", nil)
}
