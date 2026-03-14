package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/afjuiekafdjsf/nexus-dm/db"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Message struct {
	ID         string    `json:"id"`
	RoomID     string    `json:"room_id"`
	SenderID   string    `json:"sender_id"`
	SenderName string    `json:"sender_name"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

type Room struct {
	ID        string    `json:"id"`
	User1ID   string    `json:"user1_id"`
	User2ID   string    `json:"user2_id"`
	CreatedAt time.Time `json:"created_at"`
}

func publishToRoom(roomID string, data []byte) {
	if RDB != nil {
		RDB.Publish(context.Background(), "dm:"+roomID, string(data))
	}
}

func GetOrCreateRoom(c *gin.Context) {
	userID := c.GetString("user_id")
	var req struct {
		OtherUserID string `json:"other_user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Canonical ordering to avoid duplicate rooms
	u1, u2 := userID, req.OtherUserID
	if u1 > u2 {
		u1, u2 = u2, u1
	}

	var room Room
	err := db.DB.QueryRow(
		`INSERT INTO dm_rooms (user1_id, user2_id) VALUES ($1,$2)
		 ON CONFLICT (user1_id, user2_id) DO UPDATE SET user1_id=EXCLUDED.user1_id
		 RETURNING id, user1_id, user2_id, created_at`,
		u1, u2,
	).Scan(&room.ID, &room.User1ID, &room.User2ID, &room.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, room)
}

func GetRooms(c *gin.Context) {
	userID := c.GetString("user_id")
	rows, err := db.DB.Query(
		`SELECT id, user1_id, user2_id, created_at FROM dm_rooms
		 WHERE user1_id=$1 OR user2_id=$1 ORDER BY created_at DESC LIMIT 50`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	rooms := []Room{}
	for rows.Next() {
		var r Room
		rows.Scan(&r.ID, &r.User1ID, &r.User2ID, &r.CreatedAt)
		rooms = append(rooms, r)
	}
	c.JSON(http.StatusOK, rooms)
}

func GetMessages(c *gin.Context) {
	roomID := c.Param("id")
	rows, err := db.DB.Query(
		`SELECT id, room_id, sender_id, sender_name, content, created_at
		 FROM dm_messages WHERE room_id=$1 ORDER BY created_at ASC LIMIT 100`, roomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	messages := []Message{}
	for rows.Next() {
		var m Message
		rows.Scan(&m.ID, &m.RoomID, &m.SenderID, &m.SenderName, &m.Content, &m.CreatedAt)
		messages = append(messages, m)
	}
	c.JSON(http.StatusOK, messages)
}

func WebSocketHandler(c *gin.Context) {
	userID := c.GetString("user_id")
	username := c.GetString("username")
	roomID := c.Query("room_id")

	if roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room_id required"})
		return
	}

	// Verify user is member of room
	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM dm_rooms WHERE id=$1 AND (user1_id=$2 OR user2_id=$2)`,
		roomID, userID).Scan(&count)
	if count == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "not member of room"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("ws upgrade error:", err)
		return
	}
	defer conn.Close()

	// Subscribe to Redis pub/sub for this room (sole delivery path — avoids double-send)
	if RDB != nil {
		redisSub := RDB.Subscribe(context.Background(), "dm:"+roomID)
		defer redisSub.Close()
		go func() {
			for msg := range redisSub.Channel() {
				conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
			}
		}()
	}

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var req struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(msgBytes, &req); err != nil || req.Content == "" {
			continue
		}

		var m Message
		err = db.DB.QueryRow(
			`INSERT INTO dm_messages (room_id, sender_id, sender_name, content)
			 VALUES ($1,$2,$3,$4)
			 RETURNING id, room_id, sender_id, sender_name, content, created_at`,
			roomID, userID, username, req.Content,
		).Scan(&m.ID, &m.RoomID, &m.SenderID, &m.SenderName, &m.Content, &m.CreatedAt)
		if err != nil {
			log.Println("dm insert error:", err)
			continue
		}

		data, _ := json.Marshal(m)
		publishToRoom(roomID, data)
	}
}
