package main

import (
	"main/database"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/github"

	log "github.com/sirupsen/logrus"
)

func main() {
	// setup database
	err := database.SetupDatabaseMigrations()
	if err != nil {
		log.Fatal(err)
	}
	err = database.Connect()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Hello world!")
}
