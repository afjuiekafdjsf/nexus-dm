package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Accept X-User-ID from gateway (already validated upstream)
		if uid := c.GetHeader("X-User-ID"); uid != "" {
			c.Set("user_id", uid)
			c.Set("username", c.GetHeader("X-Username"))
			c.Next()
			return
		}

		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			secret = "nexus-secret"
		}

		tokenStr := ""
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tokenStr = auth[7:]
		} else if t := c.Query("token"); t != "" {
			tokenStr = t
		}

		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}

		c.Set("user_id", claims["user_id"].(string))
		c.Set("username", claims["username"].(string))
		c.Next()
	}
}
