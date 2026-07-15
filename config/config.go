package config

import (
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port string `mapstructure:"port"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type MongoDBConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
}

type AIConfig struct {
	Provider string         `mapstructure:"provider"`
	DeepSeek DeepSeekConfig `mapstructure:"deepseek"`
}

type DeepSeekConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Endpoint  string `mapstructure:"endpoint"`
	Model     string `mapstructure:"model"`
	APIKey    string `mapstructure:"api_key"`
	APISecret string `mapstructure:"api_secret"`
}

type AppConfigType struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	MongoDB  MongoDBConfig  `mapstructure:"mongodb"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	AI       AIConfig       `mapstructure:"ai"`
}

type JWTConfig struct {
	Secret string `mapstructure:"secret"`
	Expire int    `mapstructure:"expire"`
}

var AppConfig AppConfigType

func LoadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	viper.SetDefault("server.port", "8080")
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", "3306")
	viper.SetDefault("database.username", "root")
	viper.SetDefault("database.password", "")
	viper.SetDefault("database.dbname", "ai_meeting")
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("mongodb.host", "localhost")
	viper.SetDefault("mongodb.port", 27017)
	viper.SetDefault("mongodb.dbname", "ai_meeting")
	viper.SetDefault("jwt.secret", "your-secret-key-here")
	viper.SetDefault("jwt.expire", 7200)
	viper.SetDefault("ai.provider", "deepseek")
	viper.SetDefault("ai.deepseek.enabled", true)
	viper.SetDefault("ai.deepseek.endpoint", "https://api.deepseek.com/chat/completions")
	viper.SetDefault("ai.deepseek.model", "deepseek-chat")
	viper.SetDefault("ai.deepseek.api_key", "")
	viper.SetDefault("ai.deepseek.api_secret", "")

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		logrus.Warnf("Config file not found, using defaults: %v", err)
	}

	if err := viper.Unmarshal(&AppConfig); err != nil {
		return err
	}

	logrus.Info("Config loaded successfully")
	return nil
}

func GetConfig() *AppConfigType {
	return &AppConfig
}
