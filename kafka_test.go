package mgkafka

import (
	"fmt"
	"testing"
	"time"
)

func TestSubscribe(t *testing.T) {
	cfg := `go:
  data:
    kafka:
      servers: "127.0.0.1:9092"   #集群多个服务器之间用逗号分隔
      ack: all  #ack模式 no,local,all
      auto_commit: true  #是否自动提交
      partitioner: hash   #分区选择模式 hash,random,round-robin
      version: 3.7.1  #kafka版本
`
	Kafka.Init([]byte(cfg))
	defer Kafka.Close()
	topic := "biz.order.ae48ee12-f901-4cbb-ba0d-4c7497a26c23"
	topic1 := "biz.table.close.ae48ee12-f901-4cbb-ba0d-4c7497a26c23"
	Kafka.Subscribe("test-group", topic, consumerListener)
	Kafka.Subscribe("test-group", topic1, consumerListener)
	time.Sleep(20 * time.Second)
}

func consumerListener(topic, msg string) error {
	fmt.Printf("接收到%s的消息：%s\n", topic, msg)
	return nil
}
