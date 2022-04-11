package redis_discover

import "time"

const (
	redisKeyPrefix         = "rd:"
	redisMessageChannelKey = "msg"
	roleServer             = "server"
	roleNode               = "node"
	pingInterval           = 5 * time.Second
	nodeHealthTime         = 10 * time.Second
)

type NodeInfo struct {
	Id        string `json:"id,omitempty"`
	Role      string `json:"role,omitempty"`
	HeartTime int64  `json:"HeartTime,omitempty"`
}

type pingPongInfo struct {
	NodeId   string `json:"nodeId,omitempty"`
	PingTime int64  `json:"pingTime,omitempty"`
	PongTime int64  `json:"pongTime,omitempty"`
}

type MessageInfo struct {
	Id           int64  `json:"id,omitempty"`
	TargetNodeId string `json:"targetNodeId,omitempty"`
	SourceNodeId string `json:"sourceNodeId,omitempty"`
	Event        string `json:"event,omitempty"`
	Payload      []byte `json:"payload,omitempty"`
}
