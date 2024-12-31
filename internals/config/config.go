package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig
	Redis     RedisConfig
	RateLimit RateLimitConfig
}

type ServerConfig struct {
	Address      string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type RedisConfig struct {
	Address  string
	Password string
	DB       int
}

type RateLimitConfig struct {
	KeyPrefix string
	Rate      float64
	Capacity  float64
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("../..")
	viper.AddConfigPath("./config")

	//기본값 설정
	viper.SetDefault("server.address", "8080")
	viper.SetDefault("server.readTimeout", "5s")
	viper.SetDefault("server.writeTimeout", "10s")

	viper.SetDefault("redis.address", "localhost:6379")
	viper.SetDefault("redis.db", 0)

	viper.SetDefault("rateLimit.keyPrefix", "rateLimit")
	viper.SetDefault("rateLimit.rate", 10.0)
	viper.SetDefault("rateLimit.capacity", 100.0)

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil

}

//설정파일 읽어오기
//레디스 또는 저장소 셋업
//대기열 관리 셋업
//로그
