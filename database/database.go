package database

import (
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
)

var Conn *sqlx.DB

func SetupDatabaseMigrations() error {
	m, err := migrate.New(
		"github://jdfergason/pv-api/database/migrations",
		os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
		return err
	}
	m.Steps(2)
	log.Info("Database migrated")
	return nil
}

func Connect() error {
	var err error
	Conn, err = sqlx.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	if err = Conn.Ping(); err != nil {
		return err
	}
	return nil
}
