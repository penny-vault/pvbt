package main

import (
	"fmt"
	"main/cmd"

	"github.com/spf13/viper"
)

func configureViper() {
	// read config file
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath("/etc/penny-vault/")
	viper.AddConfigPath("$HOME/.config/penny-vault")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("fatal error config file: %w", err))
	}
}

func main() {
	configureViper()
	cmd.Execute()
}
