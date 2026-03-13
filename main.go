package main

import (
	"log"
	"os"

	"github.com/afjuiekafdjsf/nexus-dm/db"
	"github.com/afjuiekafdjsf/nexus-dm/handler"
	"github.com/afjuiekafdjsf/nexus-dm/middleware"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	godotenv.Load()
	db.Init()

	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_HOST") + ":6379",
	})
	handler.RDB = rdb

	r := gin.Default()
	r.Use(cors())

	auth := r.Group("/")
	auth.Use(middleware.Auth())
	auth.POST("/dm/rooms", handler.GetOrCreateRoom)
	auth.GET("/dm/rooms", handler.GetRooms)
	auth.GET("/dm/rooms/:id/messages", handler.GetMessages)
	auth.GET("/dm/ws", handler.WebSocketHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("dm-service on :%s", port)
	r.Run(":" + port)
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
