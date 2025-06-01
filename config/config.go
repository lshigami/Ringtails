package config

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type Config struct {
	Server       Server
	Database     Database
	GeminiApiKey string
}

type Server struct {
	Port string
}
type Database struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

func NewConfig() (*Config, error) {
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Warn().Err(err).Msg("Error reading config file")
	}

	var config Config

	config.Server.Port = viper.GetString("SERVER_PORT")
	config.Database.Host = viper.GetString("DATABASE_HOST")
	config.Database.Port = viper.GetString("DATABASE_PORT")
	config.Database.User = viper.GetString("DATABASE_USER")
	config.Database.Password = viper.GetString("DATABASE_PASSWORD")
	config.Database.Name = viper.GetString("DATABASE_NAME")

	config.GeminiApiKey = viper.GetString("GEMINI_API_KEY")

	log.Info().Interface("config", config).Msg("Config loaded")
	return &config, nil

}
