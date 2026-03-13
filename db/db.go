package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func Init() {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"), getEnv("DB_PORT", "5432"),
		getEnv("DB_USER", "postgres"), getEnv("DB_PASSWORD", ""),
		getEnv("DB_NAME", "nexusdb"))
	var err error
	DB, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err = DB.Ping(); err == nil {
			break
		}
		log.Printf("waiting for DB... (%d/10)", i+1)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatal("DB ping:", err)
	}
	DB.SetMaxOpenConns(20)
	DB.SetMaxIdleConns(10)
	DB.SetConnMaxLifetime(5 * time.Minute)
	migrate()
	log.Println("dm-service DB connected")
}

func migrate() {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS dm_rooms (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user1_id VARCHAR(255) NOT NULL,
			user2_id VARCHAR(255) NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(user1_id, user2_id)
		)`,
		`CREATE TABLE IF NOT EXISTS dm_messages (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			room_id UUID REFERENCES dm_rooms(id) ON DELETE CASCADE,
			sender_id VARCHAR(255) NOT NULL,
			sender_name VARCHAR(100) NOT NULL,
			content TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_messages_room ON dm_messages(room_id, created_at DESC)`,
	}
	for _, s := range stmts {
		if _, err := DB.Exec(s); err != nil {
			log.Printf("migration warning: %v", err)
		}
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
