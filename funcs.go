package redis_discover

import (
	"fmt"
	"math/rand"
	"os"
	"time"
)

func genNodeId(serviceName string) string {
	id, err := os.Hostname()
	if err != nil {
		id = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	id = fmt.Sprintf("%s:%s:%d", serviceName, id, rand.Intn(1000))
	return id
}
