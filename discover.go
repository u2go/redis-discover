package redis_discover

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	"github.com/u2go/u2utils"
	"go.uber.org/zap"
	"time"
)

type RedisDiscover struct {
	redis          *redis.Client
	logger         *zap.Logger
	serviceName    string
	nodeId         string
	role           string
	nodes          map[string]*NodeInfo
	messageHandler map[string][]func(info *MessageInfo) error
}

func New(redisClient *redis.Client, logger *zap.Logger, serviceName string) (*RedisDiscover, error) {
	err := redisClient.Ping(u2utils.Ctx(1)).Err()
	if err != nil {
		return nil, err
	}

	return &RedisDiscover{
		redis:       redisClient,
		logger:      logger,
		serviceName: serviceName,
		nodeId:      genNodeId(serviceName),
	}, nil
}

// StartServer 启动主服务。
// 接受新节点，并维护节点状态
func (rd *RedisDiscover) StartServer() error {
	rd.role = roleServer

	// msg listen
	rd.subMessage()

	// handler node register
	rd.OnMessage("register", func(message *MessageInfo) error {
		info := &NodeInfo{}
		err := json.Unmarshal(message.Payload, info)
		if err != nil {
			return err
		}

		return rd.SendMessage(info.Id, "registered", info)
	})
	rd.OnMessage("register_confirm", func(message *MessageInfo) error {
		info := &NodeInfo{}
		err := json.Unmarshal(message.Payload, info)
		if err != nil {
			return err
		}

		rd.Add(info)

		return nil
	})

	// add ping handler
	rd.OnMessage("ping", func(message *MessageInfo) error {
		info := &pingPongInfo{}
		err := json.Unmarshal(message.Payload, info)
		if err != nil {
			return err
		}

		// not pong node not in list
		if _, ok := rd.nodes[info.NodeId]; !ok {
			return errors.Errorf("node not in nodes list when ping (nodeId: %s)", info.NodeId)
		}

		return rd.pong(message.TargetNodeId, info)
	})

	// remove no ping node
	go rd.removeNoPingNode()

	return nil
}

func (rd *RedisDiscover) CloseServer() error {
	// todo
	return nil
}

// Register 节点注册。
// 将当前节点注册到服务中
func (rd *RedisDiscover) Register(fn func(rd *RedisDiscover, info *NodeInfo) bool) error {
	rd.role = roleNode

	// sub message
	rd.subMessage()

	// register node
	info := &NodeInfo{
		Id:   rd.nodeId,
		Role: rd.role,
	}
	err := rd.SendMessageToServer("register", info)
	if err != nil {
		return err
	}

	// on registered
	rd.OnMessage("registered", func(message *MessageInfo) error {
		info := &NodeInfo{}
		err := json.Unmarshal(message.Payload, info)
		if err != nil {
			return err
		}

		if fn == nil || fn(rd, info) {
			// send ping
			go rd.pingLoop()

			return rd.SendMessageToServer("register_confirm", info)
		}

		return nil
	})

	return nil
}

// SendMessage 向指定节点发送消息
func (rd *RedisDiscover) SendMessage(nodeId string, event string, payload any) error {
	return rd.send(nodeId, event, payload)
}

// SendMessageToServer 向服务节点发送消息
func (rd *RedisDiscover) SendMessageToServer(event string, payload any) error {
	return rd.send("__server", event, payload)
}

// BroadcastMessage 向所有节点广播消息
func (rd *RedisDiscover) BroadcastMessage(event string, payload any) error {
	return rd.send("", event, payload)
}

// OnMessage 收到消息的处理
func (rd *RedisDiscover) OnMessage(event string, handler func(message *MessageInfo) error) {
	if _, ok := rd.messageHandler[event]; !ok {
		rd.messageHandler[event] = []func(*MessageInfo) error{}
	}
	rd.messageHandler[event] = append(rd.messageHandler[event], handler)
}

// Add 添加节点
func (rd *RedisDiscover) Add(info *NodeInfo) {
	info.RegisterTime = time.Now().UnixNano()
	rd.nodes[info.Id] = info
}

// Remove 添加节点
func (rd *RedisDiscover) Remove(nodeId string) {
	delete(rd.nodes, nodeId)
}

func (rd *RedisDiscover) removeNoPingNode() {
	for {
		time.Sleep(1 * time.Second)

		for id, v := range rd.nodes {
			if v.RegisterTime <= time.Now().UnixNano()-int64(10*time.Second) {
				rd.Remove(id)
			}
		}
	}
}

func (rd *RedisDiscover) pingLoop() {
	for {
		time.Sleep(pingInterval)

		err := rd.ping()
		rd.logger.Error("ping error", zap.Error(err))

		// todo ping error process
	}
}

// 节点心跳
func (rd *RedisDiscover) ping() error {
	return rd.SendMessageToServer("ping", &pingPongInfo{
		NodeId:   rd.nodeId,
		PingTime: time.Now().UnixNano(),
	})
}

// 节点心跳
func (rd *RedisDiscover) pong(nodeId string, info *pingPongInfo) error {
	info.PongTime = time.Now().UnixNano()
	return rd.send(nodeId, "pong", info)
}

// 添加节点
func (rd *RedisDiscover) send(targetNodeId string, event string, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	msgBytes, err := json.Marshal(&MessageInfo{
		Id:           time.Now().UnixNano(),
		TargetNodeId: targetNodeId,
		SourceNodeId: rd.nodeId,
		Event:        event,
		Payload:      payloadBytes,
	})
	if err != nil {
		return err
	}

	return rd.redis.Publish(u2utils.Ctx(1),
		rd.redisKey(redisMessageChannelKey), msgBytes).Err()
}

// 订阅事件
func (rd *RedisDiscover) subMessage() {
	sub := rd.redis.Subscribe(u2utils.Ctx(1),
		rd.redisKey(redisMessageChannelKey))

	msgProcess := func() error {
		msg, err := sub.ReceiveMessage(context.Background())
		if err != nil {
			return err
		}
		var info MessageInfo
		err = json.Unmarshal([]byte(msg.Payload), &info)
		if err != nil {
			return err
		}
		if info.TargetNodeId == "" || // all
			info.TargetNodeId == rd.nodeId || // node
			(rd.role == roleServer && info.TargetNodeId == "__server") { // server

			if handlers, ok := rd.messageHandler[info.Event]; ok {
				for _, handler := range handlers {
					err := handler(&info)
					if err != nil {
						rd.logger.Error("message handler error", zap.Error(err))
					}
				}
			}
		}
		return nil
	}

	go func() {
		for {
			err := msgProcess()
			if err != nil {
				rd.logger.Error("message process error", zap.Error(err))
			}
		}
	}()
}

// 生成 redis key
func (rd *RedisDiscover) redisKey(key string) string {
	return fmt.Sprintf("%s%s:%s", redisKeyPrefix, rd.serviceName, key)
}
