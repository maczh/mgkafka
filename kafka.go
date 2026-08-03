package mgkafka

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/sadlil/gologger"
)

type kafka struct {
	configData []byte
	conf       *koanf.Koanf
	client     sarama.Client
	producer   sarama.AsyncProducer
	admin      sarama.ClusterAdmin
	topics     []string
	servers    []string
	config     *sarama.Config

	mu              sync.RWMutex
	consumerSession map[string]*consumerSession
}

type consumerSession struct {
	groupID       string
	topic         string
	consumerGroup sarama.ConsumerGroup
	cancel        context.CancelFunc
	done          chan struct{}
}

var Kafka = &kafka{consumerSession: make(map[string]*consumerSession)}

var logger = gologger.GetLogger()

func (k *kafka) getConfig() *sarama.Config {
	config := sarama.NewConfig()
	if k.conf == nil {
		return config
	}

	ack := strings.ToLower(k.conf.String("go.data.kafka.ack"))
	autoCommit := k.conf.Bool("go.data.kafka.auto_commit")
	partitioner := strings.ToLower(k.conf.String("go.data.kafka.partitioner"))
	ver := k.conf.String("go.data.kafka.version")
	acks := map[string]sarama.RequiredAcks{
		"no":    sarama.NoResponse,
		"local": sarama.WaitForLocal,
		"all":   sarama.WaitForAll,
	}
	if ver != "" {
		version, err := sarama.ParseKafkaVersion(ver)
		if err != nil {
			logger.Error("Kafka版本配置无效，使用默认版本: " + err.Error())
		} else {
			config.Version = version
		}
	}
	if ackValue, ok := acks[ack]; ok {
		config.Producer.RequiredAcks = ackValue
	} else {
		config.Producer.RequiredAcks = sarama.WaitForLocal
	}
	config.Consumer.Offsets.AutoCommit.Enable = autoCommit
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Return.Errors = true
	config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	config.Consumer.Group.Session.Timeout = 20 * time.Second
	config.Consumer.Group.Heartbeat.Interval = 3 * time.Second
	config.Consumer.Group.Rebalance.Timeout = 60 * time.Second
	switch partitioner {
	case "hash":
		config.Producer.Partitioner = sarama.NewHashPartitioner
	case "random":
		config.Producer.Partitioner = sarama.NewRandomPartitioner
	case "round-robin":
		config.Producer.Partitioner = sarama.NewRoundRobinPartitioner
	default:
		config.Producer.Partitioner = sarama.NewHashPartitioner
	}
	return config
}

func (k *kafka) Init(kafkaConfigData []byte) {
	if err := k.init(kafkaConfigData); err != nil {
		logger.Error(err.Error())
	}
}

func (k *kafka) init(kafkaConfigData []byte) error {
	if len(kafkaConfigData) > 0 {
		k.configData = kafkaConfigData
	}
	if len(k.configData) == 0 {
		return errors.New("Kafka配置数据为空")
	}

	if k.conf == nil {
		k.conf = koanf.New(".")
		err := k.conf.Load(rawbytes.Provider(k.configData), yaml.Parser())
		if err != nil {
			k.conf = nil
			return fmt.Errorf("Kafka配置文件解析错误: %w", err)
		}
	}

	servers := strings.TrimSpace(k.conf.String("go.data.kafka.servers"))
	if servers == "" {
		return errors.New("Kafka服务器配置为空")
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	k.servers = strings.Split(servers, ",")
	for i := range k.servers {
		k.servers[i] = strings.TrimSpace(k.servers[i])
	}
	k.config = k.getConfig()

	k.closeResourcesLocked()

	client, err := sarama.NewClient(k.servers, k.config)
	if err != nil {
		k.client = nil
		return fmt.Errorf("Kafka建立连接失败: %w", err)
	}
	k.client = client
	k.topics = []string{}
	topics, err := client.Topics()
	if err != nil {
		logger.Error("Kafka获取topic清单失败: " + err.Error())
	} else {
		k.topics = topics
	}
	logger.Info("Kafka建立连接成功")
	return nil
}

func (k *kafka) Close() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.closeResourcesLocked()
}

func (k *kafka) closeResourcesLocked() {
	for _, session := range k.consumerSession {
		if session.cancel != nil {
			session.cancel()
		}
		if session.done != nil {
			<-session.done
		}
	}
	if k.producer != nil {
		_ = k.producer.Close()
		k.producer = nil
	}
	if k.admin != nil {
		_ = k.admin.Close()
		k.admin = nil
	}
	if k.client != nil {
		_ = k.client.Close()
		k.client = nil
	}
	k.consumerSession = make(map[string]*consumerSession)
	k.topics = nil
}

func (k *kafka) Check() error {
	k.mu.RLock()
	client := k.client
	k.mu.RUnlock()
	if client == nil || client.Closed() {
		if err := k.init(k.configData); err != nil {
			return err
		}
	}
	return nil
}

func (k *kafka) GetProducer() (sarama.AsyncProducer, error) {
	if err := k.Check(); err != nil {
		return nil, err
	}

	k.mu.RLock()
	producer := k.producer
	k.mu.RUnlock()
	if producer != nil {
		return producer, nil
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	if k.producer != nil {
		return k.producer, nil
	}
	producer, err := sarama.NewAsyncProducerFromClient(k.client)
	if err != nil {
		return nil, err
	}
	k.producer = producer
	go func() {
		for err := range producer.Errors() {
			logger.Error("Kafka生产消息错误: " + err.Error())
		}
	}()
	return producer, nil
}

func (k *kafka) GetConsumer() (sarama.Consumer, error) {
	if err := k.Check(); err != nil {
		return nil, err
	}
	consumer, err := sarama.NewConsumer(k.servers, k.config)
	return consumer, err
}

func (k *kafka) GetAdminClient() (sarama.ClusterAdmin, error) {
	if err := k.Check(); err != nil {
		return nil, err
	}

	k.mu.RLock()
	admin := k.admin
	k.mu.RUnlock()
	if admin != nil {
		return admin, nil
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	if k.admin != nil {
		return k.admin, nil
	}
	admin, err := sarama.NewClusterAdminFromClient(k.client)
	if err != nil {
		return nil, err
	}
	k.admin = admin
	return admin, nil
}

func (k *kafka) GetConsumerGroup(id string) (sarama.ConsumerGroup, error) {
	if err := k.Check(); err != nil {
		return nil, err
	}
	consumerGroup, err := sarama.NewConsumerGroup(k.servers, id, k.config)
	return consumerGroup, err
}

func (k *kafka) CreateTopic(topic string) error {
	admin, err := k.GetAdminClient()
	if err != nil {
		logger.Error("Kafka连接失败:" + err.Error())
		return err
	}
	err = admin.CreateTopic(topic, &sarama.TopicDetail{NumPartitions: 1, ReplicationFactor: 1}, false)
	if err != nil {
		logger.Error("Kafka创建topic: " + topic + "失败: " + err.Error())
	}
	return err
}

func (k *kafka) ensureTopic(topic string) error {
	k.mu.RLock()
	topics := append([]string(nil), k.topics...)
	k.mu.RUnlock()
	if stringArrayContains(topics, topic) {
		return nil
	}
	if err := k.CreateTopic(topic); err != nil {
		return err
	}
	k.mu.Lock()
	k.topics = append(k.topics, topic)
	k.mu.Unlock()
	return nil
}

func (k *kafka) Send(topic, data string) error {
	if err := k.ensureTopic(topic); err != nil {
		logger.Error("Kafka创建topic失败:" + err.Error())
		return err
	}
	producer, err := k.GetProducer()
	if err != nil {
		logger.Error("Kafka连接失败:" + err.Error())
		return err
	}
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.StringEncoder(data),
	}
	producer.Input() <- msg
	return nil
}

func (k *kafka) SendMsgs(topic string, data []string) error {
	if err := k.ensureTopic(topic); err != nil {
		logger.Error("Kafka创建topic失败:" + err.Error())
		return err
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
		msg := &sarama.ProducerMessage{
			Topic: topic,
			Value: sarama.StringEncoder(d),
		}
		producer.Input() <- msg
	}
	return nil
}

func (k *kafka) Subscribe(groupID, topic string, listener func(topic, msg string) error) error {
	if listener == nil {
		return errors.New("Kafka listener cannot be nil")
	}
	if err := k.ensureTopic(topic); err != nil {
		return err
	}

	k.mu.RLock()
	session := k.consumerSession[groupID+":"+topic]
	k.mu.RUnlock()
	if session != nil {
		return nil
	}

	handler := MsgHandler{Handle: listener}
	consumerGroup, err := k.GetConsumerGroup(groupID)
	if err != nil {
		logger.Error("Kafka获取consumerGroup失败:" + err.Error())
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	session = &consumerSession{
		groupID:       groupID,
		topic:         topic,
		consumerGroup: consumerGroup,
		cancel:        cancel,
		done:          make(chan struct{}),
	}

	k.mu.Lock()
	k.consumerSession[groupID+":"+topic] = session
	k.mu.Unlock()

	go func() {
		defer close(session.done)
		for {
			err := consumerGroup.Consume(ctx, []string{topic}, handler)
			if err != nil {
				logger.Error("Kafka消费者错误: " + err.Error())
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		}
	}()
	return nil
}

func (k *kafka) MessageListener(groupID, topic string, listener func(topic, msg string) error) error {
	return k.Subscribe(groupID, topic, listener)
}

type MsgHandler struct {
	Handle func(topic, msg string) error
}

func (MsgHandler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (MsgHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }
func (h MsgHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		err := h.Handle(msg.Topic, string(msg.Value))
		if err != nil {
			logger.Error("Kafka消息消费处理错误: " + err.Error())
		}
		sess.MarkMessage(msg, "")
	}
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
