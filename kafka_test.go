package mgkafka

import (
	"fmt"
	"testing"
	"time"
)

var kafkaConfig = []byte(`go:
  data:
    kafka:
      servers: "127.0.0.1:9092"
      ack: all
      auto_commit: true
      partitioner: hash
      version: 3.7.1`)

func TestKafka(t *testing.T) {
	Kafka.Init(kafkaConfig)
	defer Kafka.Close()
	Kafka.MessageListener("testGroup", "biz.test", consumer)
	time.Sleep(5 * time.Second)
	for i := 0; i < 10; i++ {
		Kafka.Send("biz.test", fmt.Sprintf("测试：test msg %d", i))
		time.Sleep(time.Second)
	}
	time.Sleep(3 * time.Second)
}

func consumer(topic, msg string) error {
	logger.Info(fmt.Sprintf("接收到Kafka主题%s的消息:%s", topic, msg))
	return nil
}
