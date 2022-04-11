package main

import (
	"github.com/go-redis/redis/v8"
	redis_discover "github.com/u2go/redis-discover"
	"github.com/u2go/u2utils"
	"go.uber.org/zap"
	"time"
)

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})

	logger, err := zap.NewDevelopmentConfig().Build()
	u2utils.PanicOnError(err)

	go func() {
		rdServer, err := redis_discover.New(rdb, logger, "demo")
		u2utils.PanicOnError(err)
		u2utils.PanicOnError(rdServer.StartServer())
	}()

	time.Sleep(3 * time.Second) // wait for server ready

	for i := 0; i < 10; i++ {
		go func() {
			rdClient, err := redis_discover.New(rdb, logger, "demo")
			u2utils.PanicOnError(err)
			err = rdClient.Register(func(rd *redis_discover.RedisDiscover, info *redis_discover.NodeInfo) bool {
				logger.Info("Registered")
				return true
			})
			u2utils.PanicOnError(err)
		}()
	}

	time.Sleep(100000000 * time.Second)
}
