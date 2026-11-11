// Package db opens the MariaDB connection, creates the schema and seeds the
// initial administrator account.
package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schema string

// Config holds the database connection settings.
type Config struct {
	Host, Port, User, Password, Name string
}

// Open connects to the database (retrying while it starts up), then makes
// sure the schema and the default administrator exist.
func Open(c Config) (*sql.DB, error) {
	mc := mysql.NewConfig()
	mc.Net = "tcp"
	mc.Addr = c.Host + ":" + c.Port
	mc.User = c.User
	mc.Passwd = c.Password
	mc.DBName = c.Name
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Params = map[string]string{"charset": "utf8mb4", "time_zone": "'+00:00'"}

	conn, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		return nil, err
	}
	conn.SetConnMaxLifetime(3 * time.Minute)
	conn.SetMaxOpenConns(10)

	const attempts = 30
	for i := 1; ; i++ {
		if err = conn.Ping(); err == nil {
			break
		}
		if i == attempts {
			return nil, fmt.Errorf("database unreachable: %w", err)
		}
		log.Printf("waiting for database (%d/%d): %v", i, attempts, err)
		time.Sleep(2 * time.Second)
	}

	for _, stmt := range strings.Split(schema, ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := conn.Exec(stmt); err != nil {
			return nil, fmt.Errorf("schema: %w", err)
		}
	}
	if err := seedAdmin(conn); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}
	return conn, nil
}

// seedAdmin creates the single default administrator on an empty database.
func seedAdmin(conn *sql.DB) error {
	var n int
	if err := conn.QueryRow("SELECT COUNT(*) FROM instructors").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("changeme"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = conn.Exec("INSERT INTO instructors (name, password_hash, is_admin) VALUES ('Alex', ?, TRUE)", string(hash))
	if err == nil {
		log.Print("empty database: created administrator \"Alex\" with the default password")
	}
	return err
}
