package common

import (
	"os"
	"strconv"
)

var (
	Port            = 3000
	GinMode         = "debug"
	SessionSecret   string
	CryptoSecret    string
	DbType          = "sqlite"
	DbPath          = "./data/newmcp.db"
	SqlDSN          string
	RedisConnString string
	BaseURL         = "http://localhost:3000"
	LogLevel        = "info"
	UsingSQLite     bool
	UsingMySQL      bool
	UsingPostgreSQL bool
)

func InitEnv() {
	if v := os.Getenv("PORT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			Port = i
		}
	}
	if v := os.Getenv("GIN_MODE"); v != "" {
		GinMode = v
	}
	SessionSecret = os.Getenv("SESSION_SECRET")
	CryptoSecret = os.Getenv("CRYPTO_SECRET")
	if v := os.Getenv("DB_TYPE"); v != "" {
		DbType = v
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		DbPath = v
	}
	SqlDSN = os.Getenv("SQL_DSN")
	RedisConnString = os.Getenv("REDIS_CONN_STRING")
	if v := os.Getenv("BASE_URL"); v != "" {
		BaseURL = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		LogLevel = v
	}

	UsingSQLite = DbType == "sqlite"
	UsingMySQL = DbType == "mysql"
	UsingPostgreSQL = DbType == "postgres"
}
