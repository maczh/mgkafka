package mgkafka

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/sadlil/gologger"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

type kafka struct {
	configData []byte
	conf       *koanf.Koanf
	client     *kgo.Client
	admin      *kadm.Client
	topics     []string
	servers    []string
}

var Kafka = &kafka{}

var logger = gologger.GetLogger()

func (k *kafka) getConfig() []kgo.Opt {
	return []kgo.Opt{
		kgo.SeedBrokers(k.servers...),
		kgo.AllowAutoTopicCreation(),
	}
}

func (k *kafka) Init(kafkaConfigData []byte) {
	if kafkaConfigData != nil {
		k.configData = kafkaConfigData
	}
	if k.configData == nil {
		logger.Error("Kafka配置数据为空")
		return
	}
	if k.conf == nil {
		k.conf = koanf.New(".")
		err := k.conf.Load(rawbytes.Provider(k.configData), yaml.Parser())
		if err != nil {
			logger.Error("Kafka配置文件解析错误:" + err.Error())
			k.conf = nil
			return
		}
	}
	k.servers = strings.Split(k.conf.String("go.data.kafka.servers"), ",")
	for i := range k.servers {
		k.servers[i] = strings.TrimSpace(k.servers[i])
	}
	if len(k.servers) == 0 {
		logger.Error("Kafka服务器配置为空")
		return
	}
	client, err := kgo.NewClient(k.getConfig()...)
	if err != nil {
		logger.Error("Kafka建立连接失败: " + err.Error())
		return
	}
	k.client = client
	k.admin = kadm.NewClient(client)
	k.topics = make([]string, 0)
	logger.Info("Kafka建立连接成功")
}

func (k *kafka) Close() {
	if k.client != nil {
		k.client.Close()
	}
}

func (k *kafka) Check() error {
	if k.client == nil {
		logger.Error("Kafka client has closed")
		k.Init(k.configData)
		if k.client == nil {
			return fmt.Errorf("Kafka client closed")
		}
	}
	return nil
}

func (k *kafka) GetProducer() (*kgo.Client, error) {
	if k.client == nil {
		return nil, errors.New("kafka client is nil")
	}
	return k.client, nil
}

func (k *kafka) GetConsumer() (*kgo.Client, error) {
	if k.client == nil {
		return nil, errors.New("kafka client is nil")
	}
	return k.client, nil
}

func (k *kafka) GetAdminClient() (*kadm.Client, error) {
	if k.admin == nil {
		return nil, errors.New("kafka admin client is nil")
	}
	return k.admin, nil
}

func (k *kafka) GetConsumerGroup(id string) (*kgo.Client, error) {
	opts := append([]kgo.Opt{}, k.getConfig()...)
	opts = append(opts, kgo.ConsumeRegex())
	opts = append(opts, kgo.ConsumerGroup(id))
	consumerGroup, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return consumerGroup, nil
}

func (k *kafka) CreateTopic(topic string) error {
	admin, err := k.GetAdminClient()
	if err != nil {
		logger.Error("Kafka连接失败:" + err.Error())
		return err
	}
	_, err = admin.CreateTopics(context.Background(), 1, 1, nil, topic)
	if err != nil {
		logger.Error("Kafka创建topic: " + topic + "失败: " + err.Error())
	}
	return err
}

func (k *kafka) Send(topic, data string) error {
	if !stringArrayContains(k.topics, topic) {
		err := k.CreateTopic(topic)
		if err != nil {
			logger.Error("Kafka创建topic失败:" + err.Error())
			return err
		}
		k.topics = append(k.topics, topic)
	}
	producer, err := k.GetProducer()
	if err != nil {
		logger.Error("Kafka连接失败:" + err.Error())
		return err
	}
	done := make(chan error, 1)
	producer.Produce(context.Background(), &kgo.Record{Topic: topic, Value: []byte(data)}, func(_ *kgo.Record, err error) {
		done <- err
	})
	if err = <-done; err != nil {
		return err
	}
	return nil
}

func (k *kafka) SendMsgs(topic string, data []string) error {
	if !stringArrayContains(k.topics, topic) {
		err := k.CreateTopic(topic)
		if err != nil {
			logger.Error("Kafka创建topic失败:" + err.Error())
			return err
		}
		k.topics = append(k.topics, topic)
	}
	producer, err := k.GetProducer()
	if err != nil {
		logger.Error("Kafka连接失败:" + err.Error())
		return err
	}
	if data == nil || len(data) == 0 {
		return errors.New("No data to send")
	}
	for _, d := range data {
		done := make(chan error, 1)
		producer.Produce(context.Background(), &kgo.Record{Topic: topic, Value: []byte(d)}, func(_ *kgo.Record, err error) {
			done <- err
		})
		if err = <-done; err != nil {
			return err
		}
	}
	return nil
}

func (k *kafka) MessageListener(groupId, topic string, listener func(topic, msg string) error) error {
	if !stringArrayContains(k.topics, topic) {
		err := k.CreateTopic(topic)
		if err != nil {
			logger.Error("Kafka创建topic失败:" + err.Error())
			return err
		}
		k.topics = append(k.topics, topic)
	}
	consumerGroup, err := k.GetConsumerGroup(groupId)
	if err != nil {
		logger.Error("Kafka获取consumerGroup失败:" + err.Error())
		return err
	}

	go func() {
		ctx := context.Background()
		for {
			fetches := consumerGroup.PollFetches(ctx)
			fetches.EachError(func(_ string,_ int32, err error) {
				logger.Error("Kafka消费错误: " + err.Error())
			})
			fetches.EachRecord(func(record *kgo.Record) {
				if err := listener(record.Topic, string(record.Value)); err != nil {
					logger.Error("Kafka消息消费处理错误: " + err.Error())
				}
			})
			if err := consumerGroup.CommitRecords(ctx, fetches.Records()...); err != nil {
				logger.Error("Kafka提交offset失败: " + err.Error())
			}
		}
	}()
	return nil
}

func stringArrayContains(src []string, dst string) bool {
	if src == nil || len(src) == 0 {
		return false
	}
	for _, str := range src {
		if str == dst {
			return true
		}
	}
	return false
}
