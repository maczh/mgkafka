# MGin Kafka注册插件

## 安装
```bash
go get -u github.com/maczh/mgkafka
```

## 使用
在MGin微服务模块的main.go中,在mgin.Init()之后，加入一行

```go
	//加载Kafka消息队列
    mgin.MGin.Use("kafka",mgkafka.Kafka.Init,mgkafka.Kafka.Close,mgkafka.Kafka.Check)
```

## yml配置
### 在MGin微服务模块本地配置文件中
```yaml
go:
  config:
    used: kafka
    prefix:
      kafka: kafka
```

### 配置中心的kafka-test.yml配置
```yaml
go:
  data:
    kafka:
      servers: "59.56.77.23:9092"   #集群多个服务器之间用逗号分隔
      ack: all    #ack模式 no,local,all
      auto_commit: true   #是否自动提交
      partitioner: hash   #分区选择模式 hash,random,round-robin
      version: 2.8.1    #kafka版本 
```

## 发送消息
```go
    mgkafka.Kafka.Send("my_topic", "测试消息")
```

## 侦听主题消息并处理

- 定义消息处理函数
```go
func handleMsg(msg string) error {
	logs.Debug("收到Kafka消息:{}",msg)
	return nil
}

```

- 在main.go中添加侦听代码
```go
	//侦听kafka消息，说明，一个topic对应一个groupId
	err := mgkafka.Kafka.MessageListener("my_group_id","my_topic",handleMsg)
	if err != nil {
		logs.Error("侦听kafka消息失败")
	}
```