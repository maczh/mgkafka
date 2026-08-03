package mgkafka

import (
	"fmt"
	"testing"
	"time"
)

<<<<<<< HEAD
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
=======
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
>>>>>>> df1f1f28eeb224e41c9b9373a482eadcc4ee0337
	return nil
}
