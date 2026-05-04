package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
)

type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func GenerateToken(user *model.User) (string, error) {
	secret := common.SessionSecret
	if secret == "" {
		secret = "default-secret-change-me"
	}
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func parseToken(tokenStr string) (*Claims, error) {
	secret := common.SessionSecret
	if secret == "" {
		secret = "default-secret-change-me"
	}
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrTokenInvalidClaims
}

func UserAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""
		auth := c.GetHeader("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		}
		if tokenStr == "" {
			common.Error(c, http.StatusUnauthorized, "未提供认证信息")
			c.Abort()
			return
		}
		claims, err := parseToken(tokenStr)
		if err != nil {
			common.Error(c, http.StatusUnauthorized, "无效的认证令牌")
			c.Abort()
			return
		}
		user, err := model.GetUserByID(claims.UserID)
		if err != nil || user.Status != common.StatusEnabled {
			common.Error(c, http.StatusUnauthorized, "用户不存在或已禁用")
			c.Abort()
			return
		}
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("role", user.Role)
		c.Next()
	}
}

func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""
		auth := c.GetHeader("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		}
		if tokenStr == "" {
			common.Error(c, http.StatusUnauthorized, "未提供认证信息")
			c.Abort()
			return
		}
		claims, err := parseToken(tokenStr)
		if err != nil {
			common.Error(c, http.StatusUnauthorized, "无效的认证令牌")
			c.Abort()
			return
		}
		if claims.Role != common.RoleAdminUser {
			common.Error(c, http.StatusForbidden, "需要管理员权限")
			c.Abort()
			return
		}
		user, err := model.GetUserByID(claims.UserID)
		if err != nil || user.Status != common.StatusEnabled {
			common.Error(c, http.StatusUnauthorized, "用户不存在或已禁用")
			c.Abort()
			return
		}
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("role", user.Role)
		c.Next()
	}
}
